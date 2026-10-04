package legacyimport

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type sourceLimitsTransport func(*http.Request) (*http.Response, error)

func (f sourceLimitsTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type sourceLimitsBody struct {
	io.Reader
	readErr error
	closed  bool
}

func (b *sourceLimitsBody) Read(p []byte) (int, error) {
	if b.readErr != nil {
		return 0, b.readErr
	}
	return b.Reader.Read(p)
}

func (b *sourceLimitsBody) Close() error {
	b.closed = true
	return nil
}

func TestGoogleFetcherSourceLimits(t *testing.T) {
	const limit = 8 << 20
	names := DefaultSheetNames()
	tabs := []string{names.Raid, names.Loot, names.Character, names.Points}
	for tabIndex, tab := range tabs {
		for _, size := range []int{limit, limit + 1} {
			name := tab + "/exact_limit"
			if size > limit {
				name = tab + "/over_limit"
			}
			t.Run(name, func(t *testing.T) {
				// Both the full body and its 8 MiB prefix are syntactically valid CSV.
				csv := "name\n" + strings.Repeat("x", size-len("name\n"))
				var bodies []*sourceLimitsBody
				var parts []string
				fetcher := GoogleFetcher{Client: &http.Client{Transport: sourceLimitsTransport(func(req *http.Request) (*http.Response, error) {
					index := len(bodies)
					if index >= len(tabs) {
						t.Fatal("unexpected extra tab request")
					}
					if got := req.URL.Query().Get("sheet"); got != tabs[index] {
						t.Fatalf("requested tab = %q, want %q", got, tabs[index])
					}
					part := "name\nexample\n"
					if index == tabIndex {
						part = csv
					}
					body := &sourceLimitsBody{Reader: strings.NewReader(part)}
					bodies = append(bodies, body)
					parts = append(parts, part)
					return &http.Response{StatusCode: http.StatusOK, Body: body}, nil
				})}}

				data, err := fetcher.Fetch(context.Background(), "test-sheet")
				for index, body := range bodies {
					if !body.closed {
						t.Errorf("body %d was not closed", index)
					}
				}
				if size > limit {
					if !errors.Is(err, ErrSourceTooLarge) {
						t.Fatalf("Fetch error = %v, want ErrSourceTooLarge", err)
					}
					if data.Hash != "" || data.Raid != nil || data.Loot != nil || data.Character != nil || data.Points != nil {
						t.Fatal("failed Fetch returned a hash or partial sheet data")
					}
					if len(bodies) != tabIndex+1 {
						t.Fatalf("requests = %d, want %d", len(bodies), tabIndex+1)
					}
					return
				}
				if err != nil {
					t.Fatalf("Fetch: %v", err)
				}
				if len(bodies) != len(tabs) {
					t.Fatalf("requests = %d, want %d", len(bodies), len(tabs))
				}
				hash := sha256.New()
				for index, part := range [][]byte{data.Raid, data.Loot, data.Character, data.Points} {
					if string(part) != parts[index] {
						t.Fatalf("tab %d content was truncated or changed", index)
					}
					_, _ = hash.Write([]byte(parts[index]))
					_, _ = hash.Write([]byte{0})
				}
				if want := hex.EncodeToString(hash.Sum(nil)); data.Hash != want {
					t.Fatalf("hash = %q, want %q", data.Hash, want)
				}
			})
		}
	}
}

func TestGoogleFetcherSourceReadError(t *testing.T) {
	wantErr := errors.New("source read failed")
	body := &sourceLimitsBody{readErr: wantErr}
	requests := 0
	fetcher := GoogleFetcher{Client: &http.Client{Transport: sourceLimitsTransport(func(*http.Request) (*http.Response, error) {
		requests++
		return &http.Response{StatusCode: http.StatusOK, Body: body}, nil
	})}}
	data, err := fetcher.Fetch(context.Background(), "test-sheet")
	if !errors.Is(err, wantErr) {
		t.Fatalf("Fetch error = %v, want %v", err, wantErr)
	}
	if !body.closed {
		t.Fatal("body was not closed")
	}
	if requests != 1 {
		t.Fatalf("requests = %d, want 1", requests)
	}
	if data.Hash != "" || data.Raid != nil || data.Loot != nil || data.Character != nil || data.Points != nil {
		t.Fatal("failed Fetch returned a hash or partial sheet data")
	}
}
