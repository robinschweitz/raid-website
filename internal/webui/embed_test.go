package webui

import (
	"errors"
	"testing"
	"testing/fstest"
)

func TestCheckIndexMissing(t *testing.T) {
	root := fstest.MapFS{"assets/app.js": {Data: []byte("x")}}
	if err := checkIndex(root); !errors.Is(err, ErrNotBuilt) {
		t.Fatalf("checkIndex without index.html = %v, want ErrNotBuilt", err)
	}
}

func TestCheckIndexDirectory(t *testing.T) {
	root := fstest.MapFS{"index.html/x": {Data: []byte("x")}}
	if err := checkIndex(root); !errors.Is(err, ErrNotBuilt) {
		t.Fatalf("checkIndex with an index.html directory = %v, want ErrNotBuilt", err)
	}
}

func TestCheckIndexPresent(t *testing.T) {
	root := fstest.MapFS{"index.html": {Data: []byte("<!doctype html>")}}
	if err := checkIndex(root); err != nil {
		t.Fatalf("checkIndex with index.html = %v, want nil", err)
	}
}
