package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestArchiveArg(t *testing.T) {
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "book.zip")
	cbzPath := filepath.Join(dir, "comic.cbz")
	txtPath := filepath.Join(dir, "notes.txt")
	upperPath := filepath.Join(dir, "BOOK.ZIP")
	for _, p := range []string{zipPath, cbzPath, txtPath, upperPath} {
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	cases := []struct {
		name string
		args []string
		want string
	}{
		{"zip", []string{zipPath}, zipPath},
		{"cbz", []string{cbzPath}, cbzPath},
		{"uppercase extension", []string{upperPath}, upperPath},
		{"non-archive", []string{txtPath}, ""},
		{"flag ignored", []string{"-debug", zipPath}, zipPath},
		{"empty", nil, ""},
		{"missing file", []string{filepath.Join(dir, "gone.zip")}, ""},
		{"first archive wins", []string{txtPath, zipPath, cbzPath}, zipPath},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := archiveArg(c.args)
			if c.want == "" {
				if got != "" {
					t.Errorf("archiveArg(%v) = %q, want empty", c.args, got)
				}
				return
			}
			if got != c.want {
				t.Errorf("archiveArg(%v) = %q, want %q", c.args, got, c.want)
			}
		})
	}
}

func TestArchiveArgReturnsAbsolutePath(t *testing.T) {
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

	got := archiveArg([]string{"rel.zip"})
	if !filepath.IsAbs(got) {
		t.Errorf("archiveArg returned a relative path: %q", got)
	}
	if filepath.Base(got) != "rel.zip" {
		t.Errorf("archiveArg = %q", got)
	}
}
