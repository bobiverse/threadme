package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestReadLinesReturnsAllLinesInOrder(t *testing.T) {
	tempDir := t.TempDir()
	path := filepath.Join(tempDir, "sample.txt")

	content := "first\nsecond\nthird\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	got, err := readLines(path)
	if err != nil {
		t.Fatalf("readLines returned unexpected error: %v", err)
	}

	want := []string{"first", "second", "third"}
	if len(got) != len(want) {
		t.Fatalf("line count mismatch: got %d, want %d", len(got), len(want))
	}

	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("line %d mismatch: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestReadLinesEmptyFileReturnsNoLines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.txt")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	got, err := readLines(path)
	if err != nil {
		t.Fatalf("readLines returned unexpected error: %v", err)
	}

	if len(got) != 0 {
		t.Fatalf("expected no lines, got %d: %v", len(got), got)
	}
}

func TestReadLinesMissingFileReturnsError(t *testing.T) {
	_, err := readLines(filepath.Join(t.TempDir(), "does-not-exist.txt"))
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}

	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected os.ErrNotExist, got %v", err)
	}
}
