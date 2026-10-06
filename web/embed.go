package web

import (
	"embed"
	"io/fs"
	"net/http"
)

// staticFiles is compiled into the single backend binary.
//
//go:embed static
var staticFiles embed.FS

func Handler() http.Handler {
	content, err := fs.Sub(staticFiles, "static")
	if err != nil {
		panic(err)
	}
	return http.FileServer(http.FS(content))
}
