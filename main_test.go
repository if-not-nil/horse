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

func TestCopyPathDoesNotOverwriteExistingFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "source.txt")
	dst := filepath.Join(dir, "destination.txt")

	if err := os.WriteFile(src, []byte("new content"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(dst, []byte("keep this"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := copyPath(src, dst); err == nil {
		t.Fatal("expected copying to an existing destination to fail")
	}

	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}

	if string(got) != "keep this" {
		t.Fatalf("destination changed: got %q", got)
	}
}
