// managed_files_test.go verifies managed-path confinement and upload validation contracts.

package main

import (
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// These tests verify filename validation, no-overwrite writes, and os.Root confinement.
func TestValidateManagedFilenameRejectsPathComponents(t *testing.T) {
	invalid := []string{"", ".", "..", "../secret", "nested/file.pdf", `..\\secret`, `C:\\secret.pdf`}
	for _, name := range invalid {
		if err := validateManagedFilename(name); err == nil {
			t.Errorf("validateManagedFilename(%q) unexpectedly succeeded", name)
		}
	}
	if err := validateManagedFilename("550e8400-e29b-41d4-a716-446655440000.pdf"); err != nil {
		t.Fatalf("valid managed filename rejected: %v", err)
	}
}

func TestManagedFileOperationsCannotEscapeRoot(t *testing.T) {
	rootDir := t.TempDir()
	outsideDir := t.TempDir()
	outsideFile := filepath.Join(outsideDir, "secret.txt")
	if err := os.WriteFile(outsideFile, []byte("outside"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideFile, filepath.Join(rootDir, "escape.txt")); err != nil {
		t.Fatal(err)
	}
	if _, err := openManagedFile(rootDir, "escape.txt"); err == nil {
		t.Fatal("openManagedFile followed a symlink outside the managed root")
	}
	if err := writeManagedFile(rootDir, "../outside.txt", []byte("escape"), 0600); err == nil {
		t.Fatal("writeManagedFile accepted a path traversal")
	}
	if err := removeManagedFile(rootDir, "../secret.txt"); err == nil {
		t.Fatal("removeManagedFile accepted a path traversal")
	}
	data, err := os.ReadFile(outsideFile)
	if err != nil || string(data) != "outside" {
		t.Fatalf("outside file changed: data=%q err=%v", data, err)
	}
}

func TestWriteManagedFileDoesNotOverwrite(t *testing.T) {
	dir := t.TempDir()
	if err := writeManagedFile(dir, "asset.bin", []byte("first"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := writeManagedFile(dir, "asset.bin", []byte("second"), 0600); !errors.Is(err, os.ErrExist) {
		t.Fatalf("second write error = %v, want os.ErrExist", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "asset.bin"))
	if err != nil || string(data) != "first" {
		t.Fatalf("existing file was changed: data=%q err=%v", data, err)
	}
}

func TestValidateResourceFileChecksContentAndSanitizesFilename(t *testing.T) {
	validPDF := base64.StdEncoding.EncodeToString([]byte("%PDF-1.7\nbody"))
	input := resourceInput{
		Title: "test resource", Type: "其他", Filename: "../../document.pdf", Data: validPDF,
	}
	data, filename, mimeType, err := validateResourceFile(input)
	if err != nil {
		t.Fatalf("valid PDF rejected: %v", err)
	}
	if string(data) != "%PDF-1.7\nbody" || filename != "document.pdf" || mimeType != "application/pdf" {
		t.Fatalf("unexpected validation result: filename=%q mime=%q data=%q", filename, mimeType, data)
	}

	input.Filename = "document.pdf"
	input.Data = base64.StdEncoding.EncodeToString([]byte("not a PDF"))
	if _, _, _, err := validateResourceFile(input); err == nil {
		t.Fatal("PDF extension with non-PDF content was accepted")
	}
}

func TestValidImageSignatureRequiresMatchingFormat(t *testing.T) {
	png := []byte("\x89PNG\r\n\x1a\nrest")
	if !validImageSignature("image/png", png) {
		t.Fatal("valid PNG signature rejected")
	}
	if validImageSignature("image/jpeg", png) {
		t.Fatal("PNG data accepted as JPEG")
	}
	if validImageSignature("image/png", []byte("short")) {
		t.Fatal("short data accepted as PNG")
	}
}
