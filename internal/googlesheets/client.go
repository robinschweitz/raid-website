// Package googlesheets provides the small authenticated subset of the Google
// Sheets API needed by the legacy-sheet synchronization.
package googlesheets

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

const sheetsScope = "https://www.googleapis.com/auth/spreadsheets"

type credentials struct {
	ClientEmail  string `json:"client_email"`
	PrivateKey   string `json:"private_key"`
	TokenURI     string `json:"token_uri"`
	ProjectID    string `json:"project_id"`
	PrivateKeyID string `json:"private_key_id"`
}

// Client authenticates as a Google service account. It is safe for concurrent use.
type Client struct {
	email    string
	key      *rsa.PrivateKey
	tokenURI string
	http     *http.Client

	mu        sync.Mutex
	token     string
	expiresAt time.Time
}

var ErrSheetExists = errors.New("Google Sheet tab already exists")

// NewFromFile loads a service-account JSON credential. The file must remain
// outside source control.
func NewFromFile(path string) (*Client, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read service account credential: %w", err)
	}
	var creds credentials
	if err := json.Unmarshal(data, &creds); err != nil {
		return nil, fmt.Errorf("parse service account credential: %w", err)
	}
	if strings.TrimSpace(creds.ClientEmail) == "" || strings.TrimSpace(creds.PrivateKey) == "" || strings.TrimSpace(creds.TokenURI) == "" {
		return nil, errors.New("service account credential is incomplete")
	}
	block, _ := pem.Decode([]byte(creds.PrivateKey))
	if block == nil {
		return nil, errors.New("service account private key is not PEM")
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse service account private key: %w", err)
	}
	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("service account private key is not RSA")
	}
	return &Client{email: creds.ClientEmail, key: rsaKey, tokenURI: creds.TokenURI, http: &http.Client{Timeout: 30 * time.Second}}, nil
}

// Values returns display values for an A1 range, preserving the sheet's text.
func (c *Client) Values(ctx context.Context, spreadsheetID, a1Range string) ([][]string, error) {
	token, err := c.accessToken(ctx)
	if err != nil {
		return nil, err
	}
	u := "https://sheets.googleapis.com/v4/spreadsheets/" + url.PathEscape(spreadsheetID) + "/values/" + url.PathEscape(a1Range)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("read Google Sheet: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, googleAPIError("read Google Sheet", resp)
	}
	var result struct {
		Values [][]string `json:"values"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode Google Sheet values: %w", err)
	}
	return result.Values, nil
}

// UpdateValues replaces the values in an A1 range without interpreting the
// supplied strings as formulas or dates.
func (c *Client) UpdateValues(ctx context.Context, spreadsheetID, a1Range string, values [][]string) error {
	return c.writeValues(ctx, http.MethodPut, spreadsheetID, a1Range, values, "?valueInputOption=RAW", "update Google Sheet")
}

// AppendRows appends rows after the table in an A1 range without interpreting
// the supplied strings as formulas or dates.
func (c *Client) AppendRows(ctx context.Context, spreadsheetID, a1Range string, rows [][]string) error {
	return c.writeValues(ctx, http.MethodPost, spreadsheetID, a1Range, rows, ":append?valueInputOption=RAW&insertDataOption=INSERT_ROWS", "append Google Sheet rows")
}

// AddSheet creates a worksheet tab. ErrSheetExists lets callers safely ensure
// an append-only audit tab without treating an existing tab as a delivery failure.
func (c *Client) AddSheet(ctx context.Context, spreadsheetID, title string) error {
	token, err := c.accessToken(ctx)
	if err != nil {
		return err
	}
	body, err := json.Marshal(struct {
		Requests []struct {
			AddSheet struct {
				Properties struct {
					Title string `json:"title"`
				} `json:"properties"`
			} `json:"addSheet"`
		} `json:"requests"`
	}{Requests: []struct {
		AddSheet struct {
			Properties struct {
				Title string `json:"title"`
			} `json:"properties"`
		} `json:"addSheet"`
	}{{AddSheet: struct {
		Properties struct {
			Title string `json:"title"`
		} `json:"properties"`
	}{Properties: struct {
		Title string `json:"title"`
	}{Title: title}}}}})
	if err != nil {
		return fmt.Errorf("encode Google Sheet add tab: %w", err)
	}
	u := "https://sheets.googleapis.com/v4/spreadsheets/" + url.PathEscape(spreadsheetID) + ":batchUpdate"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("create Google Sheet tab: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		apiErr := googleAPIError("create Google Sheet tab", resp)
		if strings.Contains(strings.ToLower(apiErr.Error()), "already exists") {
			return ErrSheetExists
		}
		return apiErr
	}
	return nil
}

func (c *Client) writeValues(ctx context.Context, method, spreadsheetID, a1Range string, values [][]string, suffix, operation string) error {
	token, err := c.accessToken(ctx)
	if err != nil {
		return err
	}
	body, err := json.Marshal(struct {
		Values [][]string `json:"values"`
	}{Values: values})
	if err != nil {
		return fmt.Errorf("encode Google Sheet values: %w", err)
	}
	u := "https://sheets.googleapis.com/v4/spreadsheets/" + url.PathEscape(spreadsheetID) + "/values/" + url.PathEscape(a1Range) + suffix
	req, err := http.NewRequestWithContext(ctx, method, u, strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return googleAPIError(operation, resp)
	}
	return nil
}

func (c *Client) accessToken(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && time.Until(c.expiresAt) > time.Minute {
		return c.token, nil
	}
	now := time.Now().UTC()
	header, _ := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT"})
	claims, _ := json.Marshal(map[string]any{
		"iss": c.email, "scope": sheetsScope, "aud": c.tokenURI,
		"iat": now.Unix(), "exp": now.Add(time.Hour).Unix(),
	})
	unsigned := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(claims)
	hash := sha256.Sum256([]byte(unsigned))
	signature, err := rsa.SignPKCS1v15(rand.Reader, c.key, crypto.SHA256, hash[:])
	if err != nil {
		return "", fmt.Errorf("sign service account token: %w", err)
	}
	assertion := unsigned + "." + base64.RawURLEncoding.EncodeToString(signature)
	form := url.Values{"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"}, "assertion": {assertion}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.tokenURI, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("request Google access token: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", googleAPIError("request Google access token", resp)
	}
	var result struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode Google access token: %w", err)
	}
	if result.AccessToken == "" || result.ExpiresIn <= 0 {
		return "", errors.New("Google access token response is incomplete")
	}
	c.token = result.AccessToken
	c.expiresAt = now.Add(time.Duration(result.ExpiresIn) * time.Second)
	return c.token, nil
}

func googleAPIError(operation string, resp *http.Response) error {
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<10))
	var body struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(data, &body) == nil && strings.TrimSpace(body.Error.Message) != "" {
		return fmt.Errorf("%s: unexpected status %d: %s", operation, resp.StatusCode, body.Error.Message)
	}
	return fmt.Errorf("%s: unexpected status %d", operation, resp.StatusCode)
}
