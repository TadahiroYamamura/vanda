// Package web は、作った画面のファイル（dist/app/）を Go に埋め込む。
package web

import (
	"embed"
	"io/fs"
)

// dist/ には .keep だけをコミットし、画面は make build が dist/app/ に作る。
// go:embed は当たるファイルがないとビルドできないため、画面を作る前でも Go のビルドとテストが通るように、こう分けた。
//
//go:embed all:dist
var dist embed.FS

// Files は、画面のファイルを返す。画面を作っていなければ、空の中身を返す（index.html がない）。
func Files() fs.FS {
	sub, err := fs.Sub(dist, "dist/app")
	if err != nil {
		return emptyFS{}
	}
	return sub
}

type emptyFS struct{}

func (emptyFS) Open(name string) (fs.File, error) {
	return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
}
