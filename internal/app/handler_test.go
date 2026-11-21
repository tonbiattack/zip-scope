package app

import (
	"bytes"
	"image"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func serve(t *testing.T, a *App, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	rec := httptest.NewRecorder()
	a.ImageHandler().ServeHTTP(rec, req)
	return rec
}

func TestHandlerServesThumbnail(t *testing.T) {
	a := testApp(t)
	dir := t.TempDir()
	p := filepath.Join(dir, "h.zip")
	makeZip(t, p, []entry{{"a.jpg", encodeJPEG(t, 300, 200, 60)}})
	sum, err := a.IndexArchive(p)
	if err != nil {
		t.Fatalf("index: %v", err)
	}

	rec := serve(t, a, "/img/t"+itoa64(sum.ID)+"/"+itoa(sum.CoverOrdinal)+"/192")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/jpeg" && ct != "image/png" {
		t.Errorf("content type = %q", ct)
	}
	if rec.Body.Len() == 0 {
		t.Error("empty body")
	}
	// The bytes must actually decode as an image.
	if _, _, err := image.Decode(bytes.NewReader(rec.Body.Bytes())); err != nil {
		t.Errorf("body is not a decodable image: %v", err)
	}
}

func TestHandlerServesFullImage(t *testing.T) {
	a := testApp(t)
	dir := t.TempDir()
	p := filepath.Join(dir, "f.zip")
	original := encodeJPEG(t, 120, 90, 33)
	makeZip(t, p, []entry{{"a.jpg", original}})
	sum, err := a.IndexArchive(p)
	if err != nil {
		t.Fatalf("index: %v", err)
	}
	rec := serve(t, a, "/img/f"+itoa64(sum.ID)+"/"+itoa(sum.CoverOrdinal))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if !bytes.Equal(rec.Body.Bytes(), original) {
		t.Error("full image bytes differ from the original")
	}
}

func TestHandlerServesCover(t *testing.T) {
	a := testApp(t)
	dir := t.TempDir()
	p := filepath.Join(dir, "c.zip")
	makeZip(t, p, []entry{{"a.jpg", encodeJPEG(t, 200, 200, 20)}})
	sum, err := a.IndexArchive(p)
	if err != nil {
		t.Fatalf("index: %v", err)
	}
	rec := serve(t, a, "/img/c"+itoa64(sum.ID)+"/128")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestHandlerRejectsArbitraryPaths(t *testing.T) {
	a := testApp(t)
	// Every one of these must be rejected before any filesystem access.
	bad := []string{
		"/img/../../windows/win.ini",
		"/img/C:/Windows/win.ini",
		"/img/file:///C:/secret.jpg",
		"/img/http://example.com/a.jpg",
		"/img/t1/2",
		"/img/t1/2/3/4",
		"/img/",
		"/img/t-1/2/3",
		"/img/t1/2/999999999",
		"/img/notanid",
	}
	for _, path := range bad {
		rec := serve(t, a, path)
		if rec.Code == http.StatusOK {
			t.Errorf("%s returned 200, want a rejection", path)
		}
		if rec.Code != http.StatusBadRequest && rec.Code != http.StatusNotFound {
			t.Errorf("%s status = %d, want 400 or 404", path, rec.Code)
		}
	}
}

func TestHandlerRejectsUnknownArchive(t *testing.T) {
	a := testApp(t)
	rec := serve(t, a, "/img/t999999/0/192")
	if rec.Code == http.StatusOK {
		t.Fatal("unknown archive id returned 200")
	}
}

func TestHandlerRejectsNonGET(t *testing.T) {
	a := testApp(t)
	req := httptest.NewRequest(http.MethodPost, "/img/t1/0/192", nil)
	rec := httptest.NewRecorder()
	a.ImageHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rec.Code)
	}
}

func TestHandlerReportsErrorKind(t *testing.T) {
	a := testApp(t)
	dir := t.TempDir()
	p := filepath.Join(dir, "bad.zip")
	makeZip(t, p, []entry{{"bad.jpg", []byte("not an image at all")}})
	sum, err := a.IndexArchive(p)
	if err != nil {
		t.Fatalf("index: %v", err)
	}
	rec := serve(t, a, "/img/t"+itoa64(sum.ID)+"/"+itoa(sum.CoverOrdinal)+"/192")
	if rec.Code == http.StatusOK {
		t.Fatal("broken entry returned 200")
	}
	if kind := rec.Header().Get("X-ZipScope-Error"); kind == "" {
		t.Error("no error kind header")
	}
}

func TestHandlerHeadRequest(t *testing.T) {
	a := testApp(t)
	dir := t.TempDir()
	p := filepath.Join(dir, "head.zip")
	makeZip(t, p, []entry{{"a.jpg", encodeJPEG(t, 100, 100, 10)}})
	sum, err := a.IndexArchive(p)
	if err != nil {
		t.Fatalf("index: %v", err)
	}
	req := httptest.NewRequest(http.MethodHead, "/img/t"+itoa64(sum.ID)+"/"+itoa(sum.CoverOrdinal)+"/192", nil)
	rec := httptest.NewRecorder()
	a.ImageHandler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Error("HEAD returned a body")
	}
	if rec.Header().Get("Content-Length") == "" {
		t.Error("HEAD did not report a content length")
	}
}

// TestRapidSelectionNeverServesStaleImage is the "連打で旧画像が出ない" check:
// hammering the handler with alternating entries must always return the bytes
// belonging to the requested entry, never a neighbour's.
func TestRapidSelectionNeverServesStaleImage(t *testing.T) {
	a := testApp(t)
	dir := t.TempDir()
	p := filepath.Join(dir, "rapid.zip")

	// Each entry is a distinct solid colour so a mix-up is detectable.
	shades := []uint8{10, 60, 110, 160, 210}
	var entries []entry
	for i, s := range shades {
		entries = append(entries, entry{
			name: "p" + itoa(i) + ".jpg",
			data: encodeJPEG(t, 240, 240, s),
		})
	}
	makeZip(t, p, entries)
	sum, err := a.IndexArchive(p)
	if err != nil {
		t.Fatalf("index: %v", err)
	}
	detail, err := a.GetArchiveEntries(sum.ID)
	if err != nil {
		t.Fatalf("entries: %v", err)
	}

	// Record the expected bytes for each ordinal.
	expected := make(map[int][]byte)
	for _, e := range detail.Entries {
		resp, err := a.FullImage(sum.ID, e.Ordinal)
		if err != nil {
			t.Fatalf("full %d: %v", e.Ordinal, err)
		}
		expected[e.Ordinal] = resp.Data
	}

	var wg sync.WaitGroup
	errs := make(chan string, 64)
	for round := 0; round < 8; round++ {
		for _, e := range detail.Entries {
			wg.Add(1)
			go func(ord int) {
				defer wg.Done()
				// Bump the selection the way the UI does on every click.
				a.BumpSelection()
				rec := serve(t, a, "/img/f"+itoa64(sum.ID)+"/"+itoa(ord))
				if rec.Code != http.StatusOK {
					errs <- "status " + itoa(rec.Code)
					return
				}
				if !bytes.Equal(rec.Body.Bytes(), expected[ord]) {
					errs <- "ordinal " + itoa(ord) + " returned another entry's bytes"
				}
			}(e.Ordinal)
		}
	}
	wg.Wait()
	close(errs)
	for msg := range errs {
		t.Error(msg)
	}
}

func TestHandlerCacheHeader(t *testing.T) {
	a := testApp(t)
	dir := t.TempDir()
	p := filepath.Join(dir, "ch.zip")
	makeZip(t, p, []entry{{"a.jpg", encodeJPEG(t, 200, 200, 44)}})
	sum, err := a.IndexArchive(p)
	if err != nil {
		t.Fatalf("index: %v", err)
	}
	url := "/img/t" + itoa64(sum.ID) + "/" + itoa(sum.CoverOrdinal) + "/192"

	first := serve(t, a, url)
	if got := first.Header().Get("X-ZipScope-Cache"); got != "miss" {
		t.Errorf("first request cache header = %q, want miss", got)
	}
	second := serve(t, a, url)
	if got := second.Header().Get("X-ZipScope-Cache"); got != "hit" {
		t.Errorf("second request cache header = %q, want hit", got)
	}
}

func TestHandlerDoesNotServeHTML(t *testing.T) {
	a := testApp(t)
	dir := t.TempDir()
	p := filepath.Join(dir, "html.zip")
	// An entry with an image extension but HTML content must never be served
	// with an HTML content type.
	makeZip(t, p, []entry{{"x.jpg", []byte("<html><body>hi</body></html>")}})
	sum, err := a.IndexArchive(p)
	if err != nil {
		t.Fatalf("index: %v", err)
	}
	rec := serve(t, a, "/img/f"+itoa64(sum.ID)+"/"+itoa(sum.CoverOrdinal))
	if rec.Code == http.StatusOK {
		t.Fatal("HTML payload was served as an image")
	}
	if ct := rec.Header().Get("Content-Type"); ct == "text/html" {
		t.Error("response was labelled as HTML")
	}
}

func TestHandlerMissingArchiveFile(t *testing.T) {
	a := testApp(t)
	dir := t.TempDir()
	p := filepath.Join(dir, "gone.zip")
	makeZip(t, p, []entry{{"a.jpg", encodeJPEG(t, 100, 100, 10)}})
	sum, err := a.IndexArchive(p)
	if err != nil {
		t.Fatalf("index: %v", err)
	}
	// Simulate an unplugged external drive.
	if err := os.Remove(p); err != nil {
		t.Fatalf("remove: %v", err)
	}
	rec := serve(t, a, "/img/t"+itoa64(sum.ID)+"/"+itoa(sum.CoverOrdinal)+"/192")
	if rec.Code == http.StatusOK {
		t.Fatal("missing archive returned 200")
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}

func itoa64(n int64) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
