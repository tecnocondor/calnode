package handler

import (
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
)

// Tecnocondor overlay (fork patch): the stylesheet, every font it references and the
// licences are served; anything else is a 404; and each public template loads the
// overlay after its own inline <style>, so it wins by source order.

func serveTC(t *testing.T, file string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/tc/"+file, nil)
	req.SetPathValue("file", file)
	rec := httptest.NewRecorder()
	(&Handler{}).TecnocondorAsset(rec, req)
	return rec
}

func TestTecnocondorAsset_cssAndFonts(t *testing.T) {
	rec := serveTC(t, "tecnocondor.css")
	if rec.Code != http.StatusOK {
		t.Fatalf("tecnocondor.css: status %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/css") {
		t.Errorf("tecnocondor.css Content-Type = %q", ct)
	}
	if rec.Header().Get("ETag") == "" {
		t.Error("tecnocondor.css must carry an ETag")
	}
	css := rec.Body.String()
	urls := regexp.MustCompile(`url\(/tc/([^)]+)\)`).FindAllStringSubmatch(css, -1)
	if len(urls) == 0 {
		t.Fatal("tecnocondor.css references no /tc/ fonts")
	}
	for _, m := range urls {
		f := serveTC(t, m[1])
		if f.Code != http.StatusOK {
			t.Errorf("%s referenced by the CSS: status %d", m[1], f.Code)
		}
		if ct := f.Header().Get("Content-Type"); ct != "font/woff2" {
			t.Errorf("%s Content-Type = %q", m[1], ct)
		}
	}
	for _, lic := range []string{"geist", "geist-mono", "funnel-display"} {
		if serveTC(t, "fonts/OFL-"+lic+".txt").Code != http.StatusOK {
			t.Errorf("missing OFL licence for %s", lic)
		}
	}
}

func TestTecnocondorAsset_notFound(t *testing.T) {
	for _, p := range []string{"nope.css", "../booking.css", "fonts", "tecnocondor/tecnocondor.css"} {
		if rec := serveTC(t, p); rec.Code != http.StatusNotFound {
			t.Errorf("%q: status %d, want 404", p, rec.Code)
		}
	}
}

func TestTecnocondorOverlay_linkedAfterInlineStyle(t *testing.T) {
	const link = `<link rel="stylesheet" href="/tc/tecnocondor.css">`
	for _, name := range []string{"book.html", "manage.html", "directory.html"} {
		b, err := os.ReadFile("templates/" + name)
		if err != nil {
			t.Fatal(err)
		}
		src := string(b)
		at := strings.Index(src, link)
		if at < 0 {
			t.Errorf("%s does not load the Tecnocondor overlay", name)
			continue
		}
		if end := strings.Index(src, "</style>"); end < 0 || at < end {
			t.Errorf("%s: the overlay must come after the inline <style>", name)
		}
	}
}
