package web

import (
	"io/fs"
	"net/http"
)

// IndexHandler serves the embedded index.html for any non-static
// non-API path. It always responds with text/html.
func IndexHandler() http.Handler {
	html, err := fs.ReadFile(staticFS, "static/index.html")
	if err != nil {
		panic("nottario: missing embedded static/index.html: " + err.Error())
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(html)
	})
}
