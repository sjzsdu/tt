package webui

import (
	"embed"
	"io"
	"io/fs"
	"net/http"
	"strings"
)

//go:embed all:formula/dist
var formulaDist embed.FS

func FormulaIndex() []byte {
	b, err := formulaDist.ReadFile("formula/dist/index.html")
	if err != nil {
		return []byte("<!doctype html><title>tt formula dashboard</title><p>formula dashboard web UI is not built. Run <code>make web-build</code>.</p>")
	}
	return b
}

func FormulaAssetsHandler() http.Handler {
	sub, err := fs.Sub(formulaDist, "formula/dist/assets")
	if err != nil {
		return http.NotFoundHandler()
	}
	return http.StripPrefix("/assets/", http.FileServer(http.FS(sub)))
}

func FormulaFaviconHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" || path == "favicon.ico" {
			path = "favicon.svg"
		}
		f, err := formulaDist.Open("formula/dist/" + path)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()
		stat, err := f.Stat()
		if err != nil || stat.IsDir() {
			http.NotFound(w, r)
			return
		}
		http.ServeContent(w, r, stat.Name(), stat.ModTime(), f.(io.ReadSeeker))
	})
}