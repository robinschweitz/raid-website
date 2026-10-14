package sheetsync

import (
    "bytes"
    "context"
    "crypto"
    "crypto/rand"
    "crypto/rsa"
    "crypto/sha256"
    "crypto/x509"
    "encoding/base64"
    "encoding/hex"
    "encoding/json"
    "encoding/pem"
    "errors"
    "fmt"
    "io"
    "net/http"
    "net/url"
    "strings"
    "time"
)

// MirrorWriter writes website-owned snapshot tabs, never the legacy source tabs.
// Existing edits to snapshot tabs are rejected unless they match the previous export.
type MirrorWriter interface {
    Publish(context.Context, string, map[string][][]string, map[string]string) (map[string]string, error)
}

type serviceAccount struct {
    ClientEmail string `json:"client_email"`
    PrivateKey string `json:"private_key"`
}

type GoogleWriter struct {
    email string
    key *rsa.PrivateKey
    client *http.Client
}

func NewGoogleWriter(credentialsJSON string) (*GoogleWriter, error) {
    var credentials serviceAccount
    if err := json.Unmarshal([]byte(credentialsJSON), &credentials); err != nil { return nil, fmt.Errorf("decode service account: %w", err) }
    block, _ := pem.Decode([]byte(credentials.PrivateKey))
    if block == nil || strings.TrimSpace(credentials.ClientEmail) == "" { return nil, errors.New("service account email/private key required") }
    parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
    if err != nil {
        // Some Google service accounts use a PKCS#1 private key.
        parsed, err = x509.ParsePKCS1PrivateKey(block.Bytes)
        if err != nil { return nil, fmt.Errorf("parse service account key: %w", err) }
    }
    key, ok := parsed.(*rsa.PrivateKey)
    if !ok { return nil, errors.New("service account RSA private key required") }
    return &GoogleWriter{email: credentials.ClientEmail, key: key, client: &http.Client{Timeout: 30 * time.Second}}, nil
}

func (g *GoogleWriter) token(ctx context.Context) (string, error) {
    now := time.Now().Unix()
    header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
    claims, err := json.Marshal(map[string]any{
        "iss": g.email, "scope": "https://www.googleapis.com/auth/spreadsheets",
        "aud": "https://oauth2.googleapis.com/token", "iat": now, "exp": now + 3000,
    })
    if err != nil { return "", err }
    signingInput := header + "." + base64.RawURLEncoding.EncodeToString(claims)
    digest := sha256.Sum256([]byte(signingInput))
    signature, err := rsa.SignPKCS1v15(rand.Reader, g.key, crypto.SHA256, digest[:])
    if err != nil { return "", err }
    assertion := signingInput + "." + base64.RawURLEncoding.EncodeToString(signature)
    values := url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"}, "assertion": {assertion}}
    req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://oauth2.googleapis.com/token", strings.NewReader(values.Encode()))
    if err != nil { return "", err }
    req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
    resp, err := g.client.Do(req)
    if err != nil { return "", err }
    defer resp.Body.Close()
    body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
    if err != nil { return "", err }
    if resp.StatusCode != http.StatusOK { return "", fmt.Errorf("google oauth token HTTP %d: %s", resp.StatusCode, string(body)) }
    var decoded struct { AccessToken string `json:"access_token"` }
    if err := json.Unmarshal(body, &decoded); err != nil { return "", err }
    if decoded.AccessToken == "" { return "", errors.New("google oauth returned no token") }
    return decoded.AccessToken, nil
}

func (g *GoogleWriter) request(ctx context.Context, token, method, path string, body any, output any) error {
    var payload io.Reader
    if body != nil {
        data, err := json.Marshal(body)
        if err != nil { return err }
        payload = bytes.NewReader(data)
    }
    req, err := http.NewRequestWithContext(ctx, method, "https://sheets.googleapis.com/v4/spreadsheets/"+path, payload)
    if err != nil { return err }
    req.Header.Set("Authorization", "Bearer "+token)
    if body != nil { req.Header.Set("Content-Type", "application/json") }
    resp, err := g.client.Do(req)
    if err != nil { return err }
    defer resp.Body.Close()
    data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
    if err != nil { return err }
    if resp.StatusCode < 200 || resp.StatusCode >= 300 {
        return fmt.Errorf("google sheets %s HTTP %d: %s", method, resp.StatusCode, string(data))
    }
    if output != nil { return json.Unmarshal(data, output) }
    return nil
}

func normalizedRows(rows [][]string) [][]string {
    out := make([][]string, 0, len(rows))
    for _, row := range rows {
        end := len(row)
        for end > 0 && row[end-1] == "" { end-- }
        trimmed := append([]string(nil), row[:end]...)
        if trimmed == nil { trimmed = []string{} }
        out = append(out, trimmed)
    }
    for len(out) > 0 && len(out[len(out)-1]) == 0 { out = out[:len(out)-1] }
    return out
}

func rowsHash(rows [][]string) string {
    encoded, _ := json.Marshal(normalizedRows(rows))
    digest := sha256.Sum256(encoded)
    return hex.EncodeToString(digest[:])
}

func (g *GoogleWriter) Publish(ctx context.Context, sheetID string, tabs map[string][][]string, previous map[string]string) (map[string]string, error) {
    token, err := g.token(ctx)
    if err != nil { return nil, err }
    path := url.PathEscape(sheetID)
    var metadata struct {
        Sheets []struct {
            Properties struct { Title string `json:"title"` } `json:"properties"`
        } `json:"sheets"`
    }
    if err := g.request(ctx, token, http.MethodGet, path+"?fields=sheets(properties(title))", nil, &metadata); err != nil { return nil, err }
    existing := map[string]bool{}
    for _, tab := range metadata.Sheets { existing[tab.Properties.Title] = true }
    // Always validate all tabs before modifying anything, so a manual change
    // to an exported tab cannot cause a partial multi-tab export.
    type tabWrite struct { title string; rows [][]string; hash string; needsCreate bool }
    updates := make([]tabWrite, 0, len(tabs))
    result := make(map[string]string, len(previous))
    for key, value := range previous { result[key] = value }
    for title, rows := range tabs {
        current := [][]string{}
        if existing[title] {
            escapedRange := url.PathEscape("'"+strings.ReplaceAll(title, "'", "''")+"'!A1:Z")
            var valueRange struct { Values [][]string `json:"values"` }
            if err := g.request(ctx, token, http.MethodGet, path+"/values/"+escapedRange, nil, &valueRange); err != nil { return nil, err }
            current = valueRange.Values
        }
        desired := rowsHash(rows)
        old := previous[title]
        actual := rowsHash(current)
        if existing[title] && actual != desired && (old == "" || actual != old) {
            return nil, fmt.Errorf("snapshot tab %q changed outside website; refusing overwrite", title)
        }
        updates = append(updates, tabWrite{title, rows, desired, !existing[title]})
    }
    for _, update := range updates {
        if update.needsCreate {
            request := map[string]any{"requests": []any{map[string]any{"addSheet": map[string]any{"properties": map[string]any{"title": update.title}}}}}
            if err := g.request(ctx, token, http.MethodPost, path+":batchUpdate", request, nil); err != nil { return nil, err }
        }
        if !update.needsCreate && previous[update.title] == update.hash { 
            // The snapshot may be identical, but we already verified the remote tab.
            result[update.title] = update.hash
            continue
        }
        rangeName := url.PathEscape("'"+strings.ReplaceAll(update.title, "'", "''")+"'!A1:Z")
        if err := g.request(ctx, token, http.MethodPost, path+"/values/"+rangeName+":clear", map[string]any{}, nil); err != nil { return nil, err }
        payload := map[string]any{"range": "'"+update.title+"'!A1", "majorDimension": "ROWS", "values": update.rows}
        if err := g.request(ctx, token, http.MethodPut, path+"/values/"+url.PathEscape("'"+update.title+"'!A1")+"?valueInputOption=RAW", payload, nil); err != nil { return nil, err }
        result[update.title] = update.hash
    }
    return result, nil
}
