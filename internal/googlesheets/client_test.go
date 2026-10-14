package googlesheets

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestUpdateValuesAuthenticatesAndSendsValues(t *testing.T) {
	client, server := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("method = %s, want PUT", r.Method)
		}
		if r.URL.EscapedPath() != "/v4/spreadsheets/sheet%2Fid/values/Sheet1%21A1:B2" {
			t.Errorf("path = %q", r.URL.EscapedPath())
		}
		if r.URL.RawQuery != "valueInputOption=RAW" {
			t.Errorf("query = %q", r.URL.RawQuery)
		}
		if r.Header.Get("Authorization") != "Bearer test-access-token" {
			t.Errorf("authorization = %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("content-type = %q", r.Header.Get("Content-Type"))
		}
		var request struct {
			Values [][]string `json:"values"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		want := [][]string{{"one", "two"}, {"three", "four"}}
		if !equalValues(request.Values, want) {
			t.Errorf("values = %#v, want %#v", request.Values, want)
		}
		w.WriteHeader(http.StatusOK)
	})
	defer server.Close()

	if err := client.UpdateValues(context.Background(), "sheet/id", "Sheet1!A1:B2", [][]string{{"one", "two"}, {"three", "four"}}); err != nil {
		t.Fatal(err)
	}
}

func TestAppendRowsAuthenticatesAndSendsRows(t *testing.T) {
	client, server := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.EscapedPath() != "/v4/spreadsheets/sheet-id/values/Raid%20Archive%21A2:Z:append" {
			t.Errorf("path = %q", r.URL.EscapedPath())
		}
		query := r.URL.Query()
		if query.Get("valueInputOption") != "RAW" || query.Get("insertDataOption") != "INSERT_ROWS" {
			t.Errorf("query = %q", r.URL.RawQuery)
		}
		if r.Header.Get("Authorization") != "Bearer test-access-token" {
			t.Errorf("authorization = %q", r.Header.Get("Authorization"))
		}
		var request struct {
			Values [][]string `json:"values"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		want := [][]string{{"new", "row"}}
		if !equalValues(request.Values, want) {
			t.Errorf("values = %#v, want %#v", request.Values, want)
		}
		w.WriteHeader(http.StatusOK)
	})
	defer server.Close()

	if err := client.AppendRows(context.Background(), "sheet-id", "Raid Archive!A2:Z", [][]string{{"new", "row"}}); err != nil {
		t.Fatal(err)
	}
}

func TestAppendRowsReturnsGoogleAPIError(t *testing.T) {
	client, server := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"message":"permission denied"}}`))
	})
	defer server.Close()

	err := client.AppendRows(context.Background(), "sheet-id", "Sheet1!A1", [][]string{{"value"}})
	if err == nil || err.Error() != "append Google Sheet rows: unexpected status 403: permission denied" {
		t.Fatalf("error = %v", err)
	}
}

func TestAddSheetSendsBatchUpdate(t *testing.T) {
	client, server := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.URL.EscapedPath() != "/v4/spreadsheets/sheet-id:batchUpdate" {
			t.Errorf("path = %q", r.URL.EscapedPath())
		}
		var request struct {
			Requests []struct {
				AddSheet struct {
					Properties struct {
						Title string `json:"title"`
					} `json:"properties"`
				} `json:"addSheet"`
			} `json:"requests"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if len(request.Requests) != 1 || request.Requests[0].AddSheet.Properties.Title != "Website Sync Audit" {
			t.Fatalf("request = %#v", request)
		}
		w.WriteHeader(http.StatusOK)
	})
	defer server.Close()

	if err := client.AddSheet(context.Background(), "sheet-id", "Website Sync Audit"); err != nil {
		t.Fatal(err)
	}
}

func newTestClient(t *testing.T, sheetsHandler http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	var publicKey *rsa.PublicKey = &key.PublicKey
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			assertTokenRequest(t, r, publicKey, serverURL(r))
			_, _ = w.Write([]byte(`{"access_token":"test-access-token","expires_in":3600}`))
			return
		}
		sheetsHandler(w, r)
	}))
	serverURLValue, err := url.Parse(server.URL)
	if err != nil {
		server.Close()
		t.Fatal(err)
	}
	client := &Client{
		email:    "service-account@example.test",
		key:      key,
		tokenURI: server.URL + "/token",
		http: &http.Client{Transport: rewriteSheetsTransport{
			base: serverURLValue,
		}},
	}
	return client, server
}

func assertTokenRequest(t *testing.T, r *http.Request, publicKey *rsa.PublicKey, tokenURI string) {
	t.Helper()
	if r.Method != http.MethodPost {
		t.Errorf("token method = %s, want POST", r.Method)
	}
	if err := r.ParseForm(); err != nil {
		t.Fatal(err)
	}
	if r.Form.Get("grant_type") != "urn:ietf:params:oauth:grant-type:jwt-bearer" {
		t.Errorf("grant type = %q", r.Form.Get("grant_type"))
	}
	parts := strings.Split(r.Form.Get("assertion"), ".")
	if len(parts) != 3 {
		t.Fatalf("JWT has %d parts", len(parts))
	}
	unsigned := parts[0] + "." + parts[1]
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte(unsigned))
	if err := rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, hash[:], signature); err != nil {
		t.Fatalf("verify JWT signature: %v", err)
	}
	claimsJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatal(err)
	}
	var claims struct {
		Issuer   string `json:"iss"`
		Scope    string `json:"scope"`
		Audience string `json:"aud"`
		IssuedAt int64  `json:"iat"`
		Expires  int64  `json:"exp"`
	}
	if err := json.Unmarshal(claimsJSON, &claims); err != nil {
		t.Fatal(err)
	}
	if claims.Issuer != "service-account@example.test" || claims.Scope != sheetsScope || claims.Audience != tokenURI {
		t.Errorf("claims = %#v", claims)
	}
	if claims.Expires-claims.IssuedAt != int64(time.Hour/time.Second) {
		t.Errorf("JWT lifetime = %d seconds", claims.Expires-claims.IssuedAt)
	}
}

func serverURL(r *http.Request) string {
	return "http://" + r.Host + "/token"
}

type rewriteSheetsTransport struct {
	base *url.URL
}

func (t rewriteSheetsTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Host == "sheets.googleapis.com" {
		r.URL.Scheme = t.base.Scheme
		r.URL.Host = t.base.Host
	}
	return http.DefaultTransport.RoundTrip(r)
}

func equalValues(got, want [][]string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if len(got[i]) != len(want[i]) {
			return false
		}
		for j := range got[i] {
			if got[i][j] != want[i][j] {
				return false
			}
		}
	}
	return true
}
