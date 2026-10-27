package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestManagedFileRoundTripAndNoOverwrite(t *testing.T) {
	dir := t.TempDir()
	if err := writeManagedFile(dir, "asset.pdf", []byte("first"), 0600); err != nil {
		t.Fatal(err)
	}
	got, err := readManagedFile(dir, "asset.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "first" {
		t.Fatalf("readManagedFile() = %q, want first", got)
	}
	if err := writeManagedFile(dir, "asset.pdf", []byte("second"), 0600); err == nil {
		t.Fatal("writeManagedFile overwrote an existing path")
	}
	got, err = readManagedFile(dir, "asset.pdf")
	if err != nil || string(got) != "first" {
		t.Fatalf("existing file changed after rejected overwrite: data=%q err=%v", got, err)
	}
	if err := removeManagedFile(dir, "asset.pdf"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "asset.pdf")); !os.IsNotExist(err) {
		t.Fatalf("file still exists after removeManagedFile: %v", err)
	}
}

func TestManagedFilesRejectPathComponents(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(filepath.Dir(dir), "outside-managed-test")
	if err := os.WriteFile(outside, []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(outside) })
	unsafeNames := []string{"", ".", "..", "../outside-managed-test", "sub/file", "sub" + string(rune(92)) + "file"}
	for _, name := range unsafeNames {
		if err := writeManagedFile(dir, name, []byte("bad"), 0600); err == nil {
			t.Errorf("writeManagedFile accepted unsafe name %q", name)
		}
		if _, err := readManagedFile(dir, name); err == nil {
			t.Errorf("readManagedFile accepted unsafe name %q", name)
		}
		if err := removeManagedFile(dir, name); err == nil {
			t.Errorf("removeManagedFile accepted unsafe name %q", name)
		}
	}
	got, err := os.ReadFile(outside)
	if err != nil || string(got) != "preserve" {
		t.Fatalf("outside file changed: data=%q err=%v", got, err)
	}
}

func TestManagedFilesCannotFollowSymlinkOutsideRoot(t *testing.T) {
	dir := t.TempDir()
	outsideDir := t.TempDir()
	outside := filepath.Join(outsideDir, "secret.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "linked.txt")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if _, err := readManagedFile(dir, "linked.txt"); err == nil {
		t.Fatal("readManagedFile followed a symlink outside the configured root")
	}
	if err := writeManagedFile(dir, "linked.txt", []byte("overwrite"), 0600); err == nil {
		t.Fatal("writeManagedFile overwrote a symlink path")
	}
	got, err := os.ReadFile(outside)
	if err != nil || string(got) != "secret" {
		t.Fatalf("outside symlink target changed: data=%q err=%v", got, err)
	}
}
