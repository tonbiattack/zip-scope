package imgserve

import "testing"

func TestParseValid(t *testing.T) {
	cases := []struct {
		in   string
		want ID
	}{
		{"t12/34/256", ID{Kind: KindThumb, Archive: 12, Ordinal: 34, Width: 256}},
		{"f7/9", ID{Kind: KindFull, Archive: 7, Ordinal: 9}},
		{"c3/128", ID{Kind: KindCover, Archive: 3, Width: 128}},
		{"t0/0/96", ID{Kind: KindThumb, Archive: 0, Ordinal: 0, Width: 96}},
	}
	for _, c := range cases {
		got, err := Parse(c.in)
		if err != nil {
			t.Errorf("Parse(%q) error: %v", c.in, err)
			continue
		}
		if got != c.want {
			t.Errorf("Parse(%q) = %+v, want %+v", c.in, got, c.want)
		}
	}
}

func TestParseRejectsHostileInput(t *testing.T) {
	bad := []string{
		"",
		"t",
		"t1",
		"t1/2",
		"t1/2/3/4",
		"../../etc/passwd",
		"C:\\Windows\\System32\\config\\SAM",
		"file:///C:/secret.jpg",
		"http://example.com/a.jpg",
		"t1/2/99999999999999999999",
		"t-1/2/3",
		"t1/-2/3",
		"t1/2/0",
		"t1/2/99999",
		"f1/2/3",
		"c1",
		"c1/2/3",
		"t1/2/3 ",
		" t1/2/3",
		"t1/2/3\n",
		"t1%2F2%2F3",
		"t1/2/3;rm -rf",
		"x1/2/3",
		"t99999999999999/2/3",
	}
	for _, s := range bad {
		if _, err := Parse(s); err == nil {
			t.Errorf("Parse(%q) succeeded, want an error", s)
		}
	}
}

func TestParseRejectsOverlongInput(t *testing.T) {
	long := "t1/2/"
	for i := 0; i < 100; i++ {
		long += "3"
	}
	if _, err := Parse(long); err == nil {
		t.Error("overlong identifier accepted")
	}
}

func TestIDStringRoundTrip(t *testing.T) {
	ids := []ID{
		{Kind: KindThumb, Archive: 5, Ordinal: 6, Width: 128},
		{Kind: KindFull, Archive: 5, Ordinal: 6},
		{Kind: KindCover, Archive: 5, Width: 128},
	}
	for _, id := range ids {
		s := id.String()
		got, err := Parse(s)
		if err != nil {
			t.Errorf("Parse(%q): %v", s, err)
			continue
		}
		if got != id {
			t.Errorf("round trip %+v -> %q -> %+v", id, s, got)
		}
	}
}

func TestContentType(t *testing.T) {
	if got := ContentType("png"); got != "image/png" {
		t.Errorf("png = %q", got)
	}
	if got := ContentType("jpeg"); got != "image/jpeg" {
		t.Errorf("jpeg = %q", got)
	}
	// Anything else must not produce a servable type, so a cache file can never
	// be served as HTML or SVG.
	for _, s := range []string{"html", "svg", "js", "", "text/html"} {
		if got := ContentType(s); got != "" {
			t.Errorf("ContentType(%q) = %q, want empty", s, got)
		}
	}
}

func TestSniffContentType(t *testing.T) {
	jpeg := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0, 0, 0, 0, 0, 0, 0, 0}
	if got := SniffContentType(jpeg); got != "image/jpeg" {
		t.Errorf("jpeg = %q", got)
	}
	png := []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A, 0, 0, 0, 0}
	if got := SniffContentType(png); got != "image/png" {
		t.Errorf("png = %q", got)
	}
	gif := []byte{'G', 'I', 'F', '8', '9', 'a', 0, 0, 0, 0, 0, 0}
	if got := SniffContentType(gif); got != "image/gif" {
		t.Errorf("gif = %q", got)
	}
	webp := []byte{'R', 'I', 'F', 'F', 0, 0, 0, 0, 'W', 'E', 'B', 'P'}
	if got := SniffContentType(webp); got != "image/webp" {
		t.Errorf("webp = %q", got)
	}
	// HTML must never be recognised as an image.
	html := []byte("<html><body>hi</body></html>")
	if got := SniffContentType(html); got != "" {
		t.Errorf("html = %q, want empty", got)
	}
	if got := SniffContentType([]byte("short")); got != "" {
		t.Errorf("short = %q, want empty", got)
	}
}
