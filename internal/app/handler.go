package app

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/tonbiattack/zip-scope/internal/imgserve"
)

// ImageHandler serves /img/<id> requests from the WebView. It never accepts a
// filesystem path or an external URL: the identifier is parsed, validated and
// resolved against archives the app has already opened.
//
// Routing is done by hand rather than with http.ServeMux because ServeMux
// rewrites paths containing ".." or "//" with a 307 redirect, and a redirect is
// a worse answer than a flat rejection for a request that is already invalid.
func (a *App) ImageHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/img/") {
			http.NotFound(w, r)
			return
		}
		a.handleImage(w, r)
	})
}

func (a *App) handleImage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	raw := strings.TrimPrefix(r.URL.Path, "/img/")
	id, err := imgserve.Parse(raw)
	if err != nil {
		http.Error(w, "bad image id", http.StatusBadRequest)
		return
	}

	var resp *ImageResponse
	switch id.Kind {
	case imgserve.KindThumb:
		resp, err = a.Thumbnail(id.Archive, id.Ordinal, id.Width)
	case imgserve.KindFull:
		resp, err = a.FullImage(id.Archive, id.Ordinal)
	case imgserve.KindCover:
		resp, err = a.ArchiveCover(id.Archive, id.Width)
	default:
		http.Error(w, "bad image id", http.StatusBadRequest)
		return
	}
	if err != nil {
		status := http.StatusInternalServerError
		var ie *ImageError
		if errors.As(err, &ie) {
			switch ie.Kind {
			case ErrKindMissing:
				status = http.StatusNotFound
			case ErrKindEncrypted, ErrKindUnsupported, ErrKindTooLarge, ErrKindTooManyPix:
				status = http.StatusUnprocessableEntity
			case ErrKindCorrupt:
				status = http.StatusUnprocessableEntity
			}
		}
		// The body is a short machine-readable kind so the grid can render a
		// placeholder instead of a broken image icon.
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("X-ZipScope-Error", errorKind(err))
		w.WriteHeader(status)
		w.Write([]byte(errorKind(err)))
		return
	}

	h := w.Header()
	h.Set("Content-Type", resp.ContentType)
	h.Set("Content-Length", strconv.Itoa(len(resp.Data)))
	// Thumbnails are immutable for a given key, so the WebView may cache them
	// aggressively. Full images are keyed by archive id and ordinal, which the
	// app only reuses for the same fingerprint.
	if resp.Cached {
		h.Set("X-ZipScope-Cache", "hit")
	} else {
		h.Set("X-ZipScope-Cache", "miss")
	}
	h.Set("Cache-Control", "private, max-age=3600")
	h.Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
	if r.Method == http.MethodHead {
		return
	}
	w.Write(resp.Data)
}

func errorKind(err error) string {
	var ie *ImageError
	if errors.As(err, &ie) {
		return ie.Kind
	}
	return ErrKindUnknown
}
