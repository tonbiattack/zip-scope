//go:build windows

package shellreg

import "testing"

func TestNormalize(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{"zip", ".zip", false},
		{".zip", ".zip", false},
		{".ZIP", ".zip", false},
		{"  .Cbz  ", ".cbz", false},
		{"rar", "", true},
		{".7z", "", true},
		{"", "", true},
	}
	for _, c := range cases {
		got, err := Normalize(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("Normalize(%q) = %q, want an error", c.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("Normalize(%q) returned %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("Normalize(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestExtensions(t *testing.T) {
	got := Extensions()
	if len(got) != 2 || got[0] != ".zip" || got[1] != ".cbz" {
		t.Fatalf("Extensions() = %v, want [.zip .cbz]", got)
	}
}

// TestEnableDisableRoundTrip exercises the registry round trip for an extension
// that has no handler of its own. It cleans up after itself.
func TestEnableDisableRoundTrip(t *testing.T) {
	const ext = ".cbz"

	// Start from a known state: whatever this machine had before, put it back
	// and then record it, so the test does not depend on the developer's setup.
	if err := SetEnabled(ext, false); err != nil {
		t.Fatalf("reset: %v", err)
	}
	before, err := HandlerCLSID(ext)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = SetEnabled(ext, false) })

	if err := SetEnabled(ext, true); err != nil {
		t.Fatalf("enable: %v", err)
	}
	on, err := Enabled(ext)
	if err != nil {
		t.Fatal(err)
	}
	if !on {
		t.Fatal("expected the handler to be enabled")
	}

	// Enabling twice must be idempotent and must not overwrite the backup with
	// our own CLSID, which would make disable unable to restore anything.
	if err := SetEnabled(ext, true); err != nil {
		t.Fatalf("second enable: %v", err)
	}
	backup, err := readString(0x80000001, backupKey(ext), "")
	if err != nil {
		t.Fatal(err)
	}
	if equalCLSID(backup, previewCLSID) {
		t.Fatal("backup holds our own CLSID; disable could not restore the original")
	}

	if err := SetEnabled(ext, false); err != nil {
		t.Fatalf("disable: %v", err)
	}
	after, err := HandlerCLSID(ext)
	if err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("after disable the handler is %q, want the original %q", after, before)
	}
	on, err = Enabled(ext)
	if err != nil {
		t.Fatal(err)
	}
	if on {
		t.Fatal("expected the handler to be disabled")
	}
}

func TestEqualCLSID(t *testing.T) {
	if !equalCLSID("{6F1B2A12-8C3D-4E57-9A21-5B7C0D3E8F41}",
		"{6f1b2a12-8c3d-4e57-9a21-5b7c0d3e8f41}") {
		t.Fatal("CLSID comparison should ignore case")
	}
	if equalCLSID("{6F1B2A12-8C3D-4E57-9A21-5B7C0D3E8F41}", "") {
		t.Fatal("an empty CLSID should not match")
	}
}
