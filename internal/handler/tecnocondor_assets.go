package handler

// Tecnocondor overlay (fork patch, not upstream).
//
// Serves the visual identity of tecnocondor.dev for the public pages (book, manage,
// directory) from internal/handler/tecnocondor/: one stylesheet loaded AFTER booking.css
// and each page's inline <style>, plus the self-hosted fonts it uses (OFL, licences next
// to the files). Kept in its own file and folder so upstream updates of booking.css and
// the templates rarely conflict: the only upstream lines touched are one <link> per
// template and one route in internal/server/server.go. See docs/TECNOCONDOR.md.

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io/fs"
	"net/http"
	"path"
	"strings"
	"time"
)

//go:embed tecnocondor
var tecnocondorFS embed.FS

// tecnocondorETags holds a content-hash ETag per embedded file, computed once.
var tecnocondorETags = func() map[string]string {
	tags := map[string]string{}
	_ = fs.WalkDir(tecnocondorFS, "tecnocondor", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, readErr := tecnocondorFS.ReadFile(p)
		if readErr != nil {
			return readErr
		}
		sum := sha256.Sum256(b)
		tags[strings.TrimPrefix(p, "tecnocondor/")] = `"` + hex.EncodeToString(sum[:])[:16] + `"`
		return nil
	})
	return tags
}()

// TecnocondorAsset serves GET /tc/{file...}: tecnocondor.css and fonts/*.woff2|*.txt.
// The page <link> is unversioned, so everything revalidates cheaply through the
// content-hash ETag (304 when unchanged) and a redeploy shows up within minutes.
func (h *Handler) TecnocondorAsset(w http.ResponseWriter, r *http.Request) {
	name := path.Clean(r.PathValue("file"))
	etag, ok := tecnocondorETags[name]
	if !ok {
		http.NotFound(w, r)
		return
	}
	b, err := tecnocondorFS.ReadFile("tecnocondor/" + name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	switch path.Ext(name) {
	case ".css":
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=300, must-revalidate")
	case ".woff2":
		w.Header().Set("Content-Type", "font/woff2")
		w.Header().Set("Cache-Control", "public, max-age=2592000")
	default:
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=86400")
	}
	w.Header().Set("ETag", etag)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, path.Base(name), time.Time{}, bytes.NewReader(b))
}
