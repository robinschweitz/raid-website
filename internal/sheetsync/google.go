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
	"sort"
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
	PrivateKey  string `json:"private_key"`
}

type GoogleWriter struct {
	email  string
	key    *rsa.PrivateKey
	client *http.Client
}

func NewGoogleWriter(credentialsJSON string) (*GoogleWriter, error) {
	var credentials serviceAccount
	if err := json.Unmarshal([]byte(credentialsJSON), &credentials); err != nil {
		return nil, fmt.Errorf("decode service account: %w", err)
	}
	block, _ := pem.Decode([]byte(credentials.PrivateKey))
	if block == nil || strings.TrimSpace(credentials.ClientEmail) == "" {
		return nil, errors.New("service account email/private key required")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		// Some Google service accounts use a PKCS#1 private key.
		parsed, err = x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse service account key: %w", err)
		}
	}
	key, ok := parsed.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("service account RSA private key required")
	}
	return &GoogleWriter{email: credentials.ClientEmail, key: key, client: &http.Client{Timeout: 30 * time.Second}}, nil
}

func (g *GoogleWriter) token(ctx context.Context) (string, error) {
	now := time.Now().Unix()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	claims, err := json.Marshal(map[string]any{
		"iss": g.email, "scope": "https://www.googleapis.com/auth/spreadsheets",
		"aud": "https://oauth2.googleapis.com/token", "iat": now, "exp": now + 3000,
	})
	if err != nil {
		return "", err
	}
	signingInput := header + "." + base64.RawURLEncoding.EncodeToString(claims)
	digest := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, g.key, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	assertion := signingInput + "." + base64.RawURLEncoding.EncodeToString(signature)
	values := url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"}, "assertion": {assertion}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://oauth2.googleapis.com/token", strings.NewReader(values.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := g.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("google oauth token HTTP %d: %s", resp.StatusCode, string(body))
	}
	var decoded struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &decoded); err != nil {
		return "", err
	}
	if decoded.AccessToken == "" {
		return "", errors.New("google oauth returned no token")
	}
	return decoded.AccessToken, nil
}

func (g *GoogleWriter) request(ctx context.Context, token, method, path string, body any, output any) error {
	var payload io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		payload = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, "https://sheets.googleapis.com/v4/spreadsheets/"+path, payload)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := g.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("google sheets %s HTTP %d: %s", method, resp.StatusCode, string(data))
	}
	if output != nil {
		return json.Unmarshal(data, output)
	}
	return nil
}

func normalizedRows(rows [][]string) [][]string {
	out := make([][]string, 0, len(rows))
	for _, row := range rows {
		end := len(row)
		for end > 0 && row[end-1] == "" {
			end--
		}
		trimmed := append([]string(nil), row[:end]...)
		if trimmed == nil {
			trimmed = []string{}
		}
		out = append(out, trimmed)
	}
	for len(out) > 0 && len(out[len(out)-1]) == 0 {
		out = out[:len(out)-1]
	}
	return out
}

func rowsHash(rows [][]string) string {
	encoded, _ := json.Marshal(normalizedRows(rows))
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func (g *GoogleWriter) Publish(ctx context.Context, sheetID string, tabs map[string][][]string, previous map[string]string) (map[string]string, error) {
	return g.publish(ctx, sheetID, tabs, previous, false)
}

// Validate checks mirror ownership without creating or changing any tabs.
func (g *GoogleWriter) Validate(ctx context.Context, sheetID string, tabs map[string][][]string, previous map[string]string) error {
	_, err := g.publish(ctx, sheetID, tabs, previous, true)
	return err
}

func (g *GoogleWriter) publish(ctx context.Context, sheetID string, tabs map[string][][]string, previous map[string]string, previewOnly bool) (map[string]string, error) {
	if sheetID != "10aqVmmODH9Q8F6_RiGoVM2CXK3qtwtjz9hcsQN8_NO8" {
		return nil, errors.New("export must target the authorized working copy")
	}
	allowed := map[string]bool{"Website Raids": true, "Website Raidsetup": true, "Website Teilnahmen": true, "Website Loot": true, "Website Charaktere": true, "Website Punkte": true, "Website Raidserien": true}
	for title := range tabs {
		if !allowed[title] {
			return nil, fmt.Errorf("export tab %q is not website-owned", title)
		}
	}
	token, err := g.token(ctx)
	if err != nil {
		return nil, err
	}
	path := url.PathEscape(sheetID)
	type properties struct {
		Title string `json:"title"`
		ID    int64  `json:"sheetId"`
		Grid  struct {
			Rows    int `json:"rowCount"`
			Columns int `json:"columnCount"`
		} `json:"gridProperties"`
	}
	var metadata struct {
		Sheets []struct {
			Properties properties `json:"properties"`
		} `json:"sheets"`
	}
	if err = g.request(ctx, token, http.MethodGet, path+"?fields=sheets(properties(title,sheetId,gridProperties))", nil, &metadata); err != nil {
		return nil, err
	}
	existing := map[string]properties{}
	var nextID int64 = 1
	for _, sheet := range metadata.Sheets {
		existing[sheet.Properties.Title] = sheet.Properties
		if sheet.Properties.ID >= nextID {
			nextID = sheet.Properties.ID + 1
		}
	}
	titles := make([]string, 0, len(tabs))
	for title := range tabs {
		titles = append(titles, title)
	}
	sort.Strings(titles)
	result := map[string]string{}
	requests := []any{}
	for _, title := range titles {
		rows := tabs[title]
		prop, exists := existing[title]
		if !exists && previous[title] != "" {
			return nil, fmt.Errorf("snapshot tab %q removed outside website; refusing recreation", title)
		}
		current := [][]string{}
		if exists {
			var values struct {
				Values [][]string `json:"values"`
			}
			rangeName := url.PathEscape("'" + strings.ReplaceAll(title, "'", "''") + "'!A1:Z")
			if err = g.request(ctx, token, http.MethodGet, path+"/values/"+rangeName, nil, &values); err != nil {
				return nil, err
			}
			current = values.Values
		}
		actual, desired := rowsHash(current), rowsHash(rows)
		if exists && actual != desired && (previous[title] == "" || actual != previous[title]) {
			return nil, fmt.Errorf("snapshot tab %q changed outside website; refusing overwrite", title)
		}
		result[title] = desired
		// A successful remote batch with a lost response is also an idempotent retry.
		if exists && actual == desired {
			continue
		}
		endRows := len(rows)
		if len(current) > endRows {
			endRows = len(current)
		}
		if endRows == 0 {
			endRows = 1
		}
		if !exists {
			if nextID > 2147483647 {
				return nil, errors.New("no available sheet ID for atomic export")
			}
			prop.ID = nextID
			nextID++
			requests = append(requests, map[string]any{"addSheet": map[string]any{"properties": map[string]any{"title": title, "sheetId": prop.ID, "gridProperties": map[string]int{"rowCount": endRows, "columnCount": 26}}}})
		} else if prop.Grid.Rows < endRows || prop.Grid.Columns < 26 {
			gridRows := prop.Grid.Rows
			if gridRows < endRows {
				gridRows = endRows
			}
			gridColumns := prop.Grid.Columns
			if gridColumns < 26 {
				gridColumns = 26
			}
			requests = append(requests, map[string]any{"updateSheetProperties": map[string]any{"properties": map[string]any{"sheetId": prop.ID, "gridProperties": map[string]int{"rowCount": gridRows, "columnCount": gridColumns}}, "fields": "gridProperties.rowCount,gridProperties.columnCount"}})
		}
		rowData := make([]any, 0, len(rows))
		for _, row := range rows {
			if len(row) > 26 {
				return nil, fmt.Errorf("snapshot tab %q exceeds export width", title)
			}
			cells := make([]any, 0, len(row))
			for _, value := range row {
				cells = append(cells, map[string]any{"userEnteredValue": map[string]string{"stringValue": value}})
			}
			rowData = append(rowData, map[string]any{"values": cells})
		}
		// Missing cells inside this explicit range are cleared in the same atomic batch.
		requests = append(requests, map[string]any{"updateCells": map[string]any{"range": map[string]any{"sheetId": prop.ID, "startRowIndex": 0, "startColumnIndex": 0, "endRowIndex": endRows, "endColumnIndex": 26}, "fields": "userEnteredValue", "rows": rowData}})
	}
	if len(requests) > 0 && !previewOnly {
		if err = g.request(ctx, token, http.MethodPost, path+":batchUpdate", map[string]any{"requests": requests}, nil); err != nil {
			return nil, err
		}
	}
	return result, nil
}
