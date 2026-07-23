package webui

import (
	_ "embed"
	"net/http"
)

//go:embed team/index.html
var teamIndex []byte

func TeamIndex() []byte {
	return append([]byte(nil), teamIndex...)
}

func TeamIndexHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; script-src 'self' 'unsafe-inline'; connect-src 'self'")
		_, _ = w.Write(teamIndex)
	})
}
