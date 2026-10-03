package legacyimport

import "testing"

func TestDetectTokenItems(t *testing.T) {
	if !DetectTokenItems("Weiheabzeichen des Beschützers") {
		t.Fatal("should detect")
	}
	if !DetectTokenItems("WEIHEABZEICHEN DES BEZWINGERS") {
		t.Fatal("should detect case-insensitive")
	}
	if DetectTokenItems("Blutfall") {
		t.Fatal("should not detect")
	}
}
