package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStartupArg(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "book.zip")
	cbzPath := filepath.Join(dir, "comic.cbz")
	txtPath := filepath.Join(dir, "notes.txt")
	upperPath := filepath.Join(dir, "BOOK.ZIP")
	subDir := filepath.Join(dir, "sub")
	for _, p := range []string{zipPath, cbzPath, txtPath, upperPath} {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	cases := []struct {
		name string
		args []string
		want string
	}{
		{"zip", []string{zipPath}, zipPath},
		{"cbz", []string{cbzPath}, cbzPath},
		{"uppercase extension", []string{upperPath}, upperPath},
		{"folder", []string{subDir}, subDir},
		{"non-archive file", []string{txtPath}, ""},
		{"flag ignored", []string{"-debug", zipPath}, zipPath},
		{"empty", nil, ""},
		{"missing path", []string{filepath.Join(dir, "gone.zip")}, ""},
		{"first valid wins", []string{txtPath, zipPath, cbzPath}, zipPath},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := startupArg(c.args)
			if c.want == "" {
				if got != "" {
					t.Errorf("startupArg(%v) = %q, want empty", c.args, got)
				}
				return
			}
			if got != c.want {
				t.Errorf("startupArg(%v) = %q, want %q", c.args, got, c.want)
			}
		})
	}
}

func TestStartupArgReturnsAbsolutePath(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "rel.zip")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	// A relative path must be resolved so the frontend can match it against the
	// absolute paths the scanner reports.
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(wd)

	got := startupArg([]string{"rel.zip"})
	if !filepath.IsAbs(got) {
		t.Errorf("startupArg returned a relative path: %q", got)
	}
	if filepath.Base(got) != "rel.zip" {
		t.Errorf("startupArg = %q", got)
	}
}
