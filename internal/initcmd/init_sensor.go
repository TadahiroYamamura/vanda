package initcmd

// このファイルは、.vanda/ を作った後の事後条件の確かめ（センサー）を持つ。設定の [[annotation]] の **/*_sensor.go で読む。

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Hooks は、テストから、事後条件を破る状態を作るための口。本番では nil のまま使わない。
var Hooks struct {
	// AfterWrite は、Write がファイルを書き終えた後、checkCreated の前に呼ぶ。root はリポジトリの根
	AfterWrite func(root string)
}

// checkCreated は、.vanda/ の中が、書いたファイル files（場所と中身）とちょうど同じかを確かめる。
// 作る前からあった .vanda/ に書き込んでいれば、ほかのファイルが混じるか中身が違うので、ここで見つかる。
//
// @vanda:sensor init-keeps-existing
func checkCreated(root string, files [][2]string) error {
	want := map[string]string{}
	for _, f := range files {
		want[f[0]] = f[1]
	}
	var extra, differ []string
	dir := filepath.Join(root, ".vanda")
	err := filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		body, ok := want[rel]
		if !ok {
			extra = append(extra, rel)
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		if string(b) != body {
			differ = append(differ, rel)
		}
		delete(want, rel)
		return nil
	})
	if err != nil {
		return fmt.Errorf("作った .vanda/ を確かめられません（%v）。.vanda/ の中身を確かめてください。", err)
	}
	var missing []string
	for f := range want {
		missing = append(missing, f)
	}
	sort.Strings(missing)
	var parts []string
	if len(extra) > 0 {
		parts = append(parts, "作っていないファイル "+strings.Join(extra, "・")+" があります")
	}
	if len(differ) > 0 {
		parts = append(parts, strings.Join(differ, "・")+" の中身が、書いたものと違います")
	}
	if len(missing) > 0 {
		parts = append(parts, strings.Join(missing, "・")+" がありません")
	}
	if len(parts) > 0 {
		return fmt.Errorf(".vanda/ を作った後の確かめで、食い違いが見つかりました（%s）。ほかのファイルを消さないよう、.vanda/ はそのまま残しました。.vanda/ の中身を確かめてください。", strings.Join(parts, "。"))
	}
	return nil
}
