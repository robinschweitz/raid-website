package httpapi_test

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// The addons page and its downloads are public, like the guide (ADR 085).
func TestAddonDownloadsArePublicZips(t *testing.T) {
	srv, _ := setupServer(t)
	res, err := http.Get(srv.URL + "/api/v1/addons")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("list status %d", res.StatusCode)
	}
	var out struct {
		Data []struct {
			Name        string `json:"name"`
			Version     string `json:"version"`
			FileName    string `json:"file_name"`
			SizeBytes   int    `json:"size_bytes"`
			DownloadURL string `json:"download_url"`
		} `json:"data"`
	}
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if len(out.Data) != 2 || out.Data[0].Name != "RaidLedger" || out.Data[1].Name != "RaidWishlist" {
		t.Fatalf("addons %+v", out.Data)
	}

	for _, addon := range out.Data {
		download, err := http.Get(srv.URL + addon.DownloadURL)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(download.Body)
		download.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if download.StatusCode != http.StatusOK || download.Header.Get("Content-Type") != "application/zip" || len(body) != addon.SizeBytes {
			t.Fatalf("%s download: %d %q %d bytes", addon.Name, download.StatusCode, download.Header.Get("Content-Type"), len(body))
		}
		if disposition := download.Header.Get("Content-Disposition"); disposition != `attachment; filename="`+addon.FileName+`"` {
			t.Fatalf("%s disposition %q", addon.Name, disposition)
		}
		reader, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
		if err != nil {
			t.Fatal(err)
		}
		for _, file := range reader.File {
			if !strings.HasPrefix(file.Name, addon.Name+"/") {
				t.Fatalf("%s zip entry %q is outside the addon folder", addon.Name, file.Name)
			}
		}
	}

	missing, err := http.Get(srv.URL + "/api/v1/addons/RaidSync/download")
	if err != nil {
		t.Fatal(err)
	}
	missing.Body.Close()
	if missing.StatusCode != http.StatusNotFound {
		t.Fatalf("unknown addon status %d", missing.StatusCode)
	}
}
