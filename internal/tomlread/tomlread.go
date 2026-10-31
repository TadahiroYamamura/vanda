// Package tomlread は、vanda の TOML のファイルを読み、項目ごとに型を確かめ、行の付いたエラーを集める。
//
// 構造体へ直接読み込まず、表を map で読んでから項目を取り出す。構造体への読み込みは、型の違いを行の分からないエラーにし、
// 決まっていない項目も黙って捨てるため。
package tomlread

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/BurntSushi/toml"

	"github.com/TadahiroYamamura/vanda/internal/diag"
	"github.com/TadahiroYamamura/vanda/internal/tomlpos"
)

// Doc は、読んだ 1 つのファイル。File はリポジトリの直下からの場所。
type Doc struct {
	File  string
	Src   []byte
	Root  map[string]any
	Lines map[string]int
	Errs  *[]diag.Error
}

// Load は、root の下の file を読む。読めなければ、エラーを errs に足して nil を返す。
func Load(root, file string, errs *[]diag.Error) *Doc {
	src, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(file)))
	if err != nil {
		what := "ファイルを読めない（" + err.Error() + "）"
		fix := "ファイルの場所と、読む権限を確かめる"
		if errors.Is(err, os.ErrNotExist) {
			what, fix = "ファイルがない", "ファイルを作るか、設定（.vanda/config.toml）の場所を直す"
		}
		*errs = append(*errs, diag.Error{File: file, What: what, Fix: fix})
		return nil
	}
	return Parse(file, src, errs)
}

// Parse は、読み込んだ中身 src を TOML として読む。
func Parse(file string, src []byte, errs *[]diag.Error) *Doc {
	var root map[string]any
	if _, err := toml.Decode(string(src), &root); err != nil {
		var pe toml.ParseError
		if errors.As(err, &pe) {
			*errs = append(*errs, diag.Error{File: file, Line: pe.Position.Line, What: "TOML として読めない（" + pe.Message + "）", Fix: "TOML の書き方に合わせて直す"})
		} else {
			*errs = append(*errs, diag.Error{File: file, What: "TOML として読めない（" + err.Error() + "）", Fix: "TOML の書き方に合わせて直す"})
		}
		return nil
	}
	return &Doc{File: file, Src: src, Root: root, Lines: tomlpos.Lines(src), Errs: errs}
}

// Table は、ファイルの中の 1 つの表。Path は tomlpos の場所の書き方、Label はエラーで見せる名前。
type Table struct {
	Doc   *Doc
	Path  string
	Label string
	M     map[string]any
	used  map[string]bool
}

func (d *Doc) Top() *Table {
	return &Table{Doc: d, M: d.Root, used: map[string]bool{}}
}

func (t *Table) sub(path, label string, m map[string]any) *Table {
	return &Table{Doc: t.Doc, Path: path, Label: label, M: m, used: map[string]bool{}}
}

func (t *Table) keyPath(key string) string {
	if t.Path == "" {
		return key
	}
	return t.Path + "." + key
}

func (t *Table) keyLabel(key string) string {
	if t.Label == "" {
		return key
	}
	return t.Label + " の " + key
}

// ErrAt は、項目 key の行にエラーを足す。key が空なら表の見出しの行にする。行が見つからなければ、行を書かず、Label で場所を示す。
func (t *Table) ErrAt(key, what, fix string) {
	e := diag.Error{File: t.Doc.File, What: what, Fix: fix}
	if key == "" {
		e.Line = t.Doc.Lines[t.Path]
		e.Item = t.Label
	} else {
		e.Line = t.Doc.Lines[t.keyPath(key)]
		e.Item = t.keyLabel(key)
	}
	*t.Doc.Errs = append(*t.Doc.Errs, e)
}

// Line は、項目 key の行を返す。key が空なら表の見出しの行。見つからなければ 0。
func (t *Table) Line(key string) int {
	if key == "" {
		return t.Doc.Lines[t.Path]
	}
	return t.Doc.Lines[t.keyPath(key)]
}

func (t *Table) missing(key, example string) {
	t.ErrAt("", fmt.Sprintf("%s がない", t.keyLabel(key)), fmt.Sprintf("%s を書く", example))
}

// Has は、項目 key が書かれているかを返す。
func (t *Table) Has(key string) bool {
	_, ok := t.M[key]
	return ok
}

// Str は、文字列の項目を返す。ok は、項目があり文字列だったか。
func (t *Table) Str(key string, required bool) (string, bool) {
	t.used[key] = true
	v, ok := t.M[key]
	if !ok {
		if required {
			t.missing(key, key+` = "…"`)
		}
		return "", false
	}
	s, ok := v.(string)
	if !ok {
		t.ErrAt(key, key+" が文字列でない", key+` = "…" のように文字列で書く`)
		return "", false
	}
	return s, true
}

// Bool は、真偽の項目を返す。
func (t *Table) Bool(key string, required bool) (bool, bool) {
	t.used[key] = true
	v, ok := t.M[key]
	if !ok {
		if required {
			t.missing(key, key+" = true か "+key+" = false")
		}
		return false, false
	}
	b, ok := v.(bool)
	if !ok {
		t.ErrAt(key, key+" が真偽でない", key+" = true か "+key+" = false と書く")
		return false, false
	}
	return b, true
}

// Strs は、文字列の配列の項目を返す。
func (t *Table) Strs(key string, required bool) ([]string, bool) {
	t.used[key] = true
	v, ok := t.M[key]
	if !ok {
		if required {
			t.missing(key, key+` = ["…"]`)
		}
		return nil, false
	}
	arr, ok := v.([]any)
	if !ok {
		t.ErrAt(key, key+" が文字列の配列でない", key+` = ["…"] のように書く`)
		return nil, false
	}
	out := make([]string, 0, len(arr))
	for _, e := range arr {
		s, ok := e.(string)
		if !ok {
			t.ErrAt(key, key+" に文字列でない値がある", key+` = ["…"] のように、文字列だけを並べる`)
			return nil, false
		}
		out = append(out, s)
	}
	return out, true
}

// Tables は、表の配列（`[[key]]`）の項目を、要素ごとの Table にして返す。label は要素の名前（例: "claim"）。
func (t *Table) Tables(key string, required bool) ([]*Table, bool) {
	t.used[key] = true
	v, ok := t.M[key]
	if !ok {
		if required {
			t.missing(key, "[["+t.keyPath(key)+"]]")
		}
		return nil, false
	}
	var ms []map[string]any
	switch arr := v.(type) {
	case []map[string]any:
		ms = arr
	case []any:
		for _, e := range arr {
			m, ok := e.(map[string]any)
			if !ok {
				t.ErrAt(key, key+" が表の配列でない", "[["+t.keyPath(key)+"]] の表で書く")
				return nil, false
			}
			ms = append(ms, m)
		}
	default:
		t.ErrAt(key, key+" が表の配列でない", "[["+t.keyPath(key)+"]] の表で書く")
		return nil, false
	}
	out := make([]*Table, len(ms))
	for i, m := range ms {
		out[i] = t.sub(fmt.Sprintf("%s[%d]", t.keyPath(key), i), t.keyLabel(fmt.Sprintf("%s の %d 件目", key, i+1)), m)
	}
	return out, true
}

// Table は、表（`[key]`）の項目を返す。
func (t *Table) Table(key string, required bool) (*Table, bool) {
	t.used[key] = true
	v, ok := t.M[key]
	if !ok {
		if required {
			t.missing(key, "["+t.keyPath(key)+"]")
		}
		return nil, false
	}
	m, ok := v.(map[string]any)
	if !ok {
		t.ErrAt(key, key+" が表でない", "["+t.keyPath(key)+"] の表で書く")
		return nil, false
	}
	return t.sub(t.keyPath(key), t.keyLabel(key), m), true
}

// Unknown は、ここまでに読まなかった項目を、決まっていない項目としてエラーにする。
func (t *Table) Unknown(allowed string) {
	var keys []string
	for k := range t.M {
		if !t.used[k] {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		t.ErrAt(k, k+" は決まった項目でない", "消すか、名前を直す（書ける項目: "+allowed+"）")
	}
}
