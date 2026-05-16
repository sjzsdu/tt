package webui

import (
	"embed"
	"io/fs"
	"net/http"
)

// formulaDist embeds the built React app for tt formula dashboard.
//
//go:embed formula/dist
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
