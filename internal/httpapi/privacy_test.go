package httpapi_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"raid-website/internal/httpapi"
)

// The privacy contact is public and carries exactly name and address (FR-30.1, SC-004).
func TestReqPrivacy010_AC01(t *testing.T) {
	srv, _ := setupServerWithOptions(t, httpapi.Options{PrivacyContactName: "Max Mustermann", PrivacyContactAddress: "owner@example.org"})
	res, err := http.Get(srv.URL + "/api/v1/privacy-contact")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}
	var out map[string]map[string]string
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || len(out["data"]) != 2 || out["data"]["name"] != "Max Mustermann" || out["data"]["address"] != "owner@example.org" {
		t.Fatalf("response %+v", out)
	}
}
