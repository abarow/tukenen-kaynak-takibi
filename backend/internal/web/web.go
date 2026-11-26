// Package web derlenmiş SvelteKit arayüzünü ikili dosyaya gömer.
// `cd frontend && npm run build` çıktıyı dist/ klasörüne yazar.
package web

import (
	"embed"
	"io/fs"
)

//go:embed all:dist
var dist embed.FS

func FS() fs.FS {
	sub, err := fs.Sub(dist, "dist")
	if err != nil {
		panic(err)
	}
	return sub
}
