package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCreateNewFileDoesNotTruncateExistingFile(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "notes.txt")
	original := []byte("keep this content")

	if err := os.WriteFile(filePath, original, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := createNewFile(filePath); err == nil {
		t.Fatal("expected creating an existing file to fail")
	}

	got, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatal(err)
	}

	if string(got) != string(original) {
		t.Fatalf("file changed: got %q, want %q", got, original)
	}
}
