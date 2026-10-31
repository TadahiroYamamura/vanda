// Package promises は、約束リスト（既定は .vanda/promises.toml）を読む。
//
// ここでは、読めること、決まった表と項目だけがあること、必須の項目があること、項目の型を確かめる。
// 値が決まった語の中にあるか、ID の形と重なり、指す先があるかは、審査で確かめる。
package promises

import (
	"github.com/TadahiroYamamura/vanda/internal/diag"
	"github.com/TadahiroYamamura/vanda/internal/glob"
	"github.com/TadahiroYamamura/vanda/internal/tomlread"
)

type List struct {
	Files    []string
	Terms    []*Term
	Features []*Feature
	Claims   []*Claim
}

// Item は、用語・機能・約束に共通するもの。Raw は読み込んだ表そのもの（指紋を作る元）。
type Item struct {
	ID    string
	Table *tomlread.Table
	Raw   map[string]any
}

type Term struct {
	Item
	Name       string
	Definition string
	PartOf     string // 書いていなければ空
}

type Feature struct {
	Item
	Name       string
	Definition string
	Core       bool
	// Formal は、書いていなければ、core = true のとき真とみなした値
	Formal       bool
	FormalWaiver string // 書いていなければ空
}

type Claim struct {
	Item
	Subjects  []string
	Statement string
	Rationale string
	Condition string
	Layers    []string
	Flags     []string
	Guesses   []string
	Evidence  []*Evidence
}

type Evidence struct {
	Table  *tomlread.Table
	Stage  string
	Type   string
	Layers []string
	Detail string
}

// Load は、files（リポジトリの直下からの場所の一覧）のうち patterns に当たるものを、約束リストとして読む。
func Load(root string, files, patterns []string) (*List, diag.Errors) {
	var errs []diag.Error
	matched, missing := glob.Select(files, patterns)
	for _, m := range missing {
		errs = append(errs, diag.Error{File: m, What: "約束リストのファイルがない", Fix: "ファイルを作るか、.vanda/config.toml の promises を直す"})
	}
	l := &List{Files: matched}
	for _, f := range matched {
		doc := tomlread.Load(root, f, &errs)
		if doc == nil {
			continue
		}
		l.read(doc.Top())
	}
	if len(errs) > 0 {
		return l, errs
	}
	return l, nil
}

func (l *List) read(top *tomlread.Table) {
	terms, _ := top.Tables("term", false)
	for _, t := range terms {
		x := &Term{Item: item(t, "term")}
		x.Name, _ = t.Str("name", true)
		x.Definition, _ = t.Str("definition", true)
		x.PartOf, _ = t.Str("part_of", false)
		t.Unknown("id, name, definition, part_of")
		l.Terms = append(l.Terms, x)
	}
	features, _ := top.Tables("feature", false)
	for _, t := range features {
		x := &Feature{Item: item(t, "feature")}
		x.Name, _ = t.Str("name", true)
		x.Definition, _ = t.Str("definition", true)
		x.Core, _ = t.Bool("core", false)
		formal, ok := t.Bool("formal", false)
		if ok {
			x.Formal = formal
		} else {
			x.Formal = x.Core
		}
		x.FormalWaiver, _ = t.Str("formal_waiver", false)
		t.Unknown("id, name, definition, core, formal, formal_waiver")
		l.Features = append(l.Features, x)
	}
	claims, _ := top.Tables("claim", false)
	for _, t := range claims {
		x := &Claim{Item: item(t, "claim")}
		x.Subjects = nonEmpty(t, "subjects", "用語か機能の ID")
		x.Statement, _ = t.Str("statement", true)
		x.Rationale, _ = t.Str("rationale", false)
		x.Condition, _ = t.Str("condition", false)
		x.Layers = nonEmpty(t, "layers", "レイヤーの ID")
		x.Flags, _ = t.Strs("flags", false)
		x.Guesses, _ = t.Strs("guesses", false)
		evs, ok := t.Tables("evidence", true)
		if ok && len(evs) == 0 {
			t.ErrAt("evidence", "evidence が 1 つもない", "[[claim.evidence]] を 1 つ以上書く")
		}
		for _, et := range evs {
			e := &Evidence{Table: et}
			e.Stage, _ = et.Str("stage", true)
			e.Type, _ = et.Str("type", true)
			e.Layers = nonEmpty(et, "layers", "レイヤーの ID")
			e.Detail, _ = et.Str("detail", false)
			et.Unknown("stage, type, layers, detail")
			x.Evidence = append(x.Evidence, e)
		}
		t.Unknown("id, subjects, statement, rationale, condition, layers, flags, guesses, evidence")
		l.Claims = append(l.Claims, x)
	}
	top.Unknown("term, feature, claim")
}

func item(t *tomlread.Table, kind string) Item {
	id, _ := t.Str("id", true)
	if id != "" {
		t.Label = kind + ` "` + id + `"`
	}
	return Item{ID: id, Table: t, Raw: t.M}
}

func nonEmpty(t *tomlread.Table, key, what string) []string {
	xs, ok := t.Strs(key, true)
	if ok && len(xs) == 0 {
		t.ErrAt(key, key+" が空", key+" に"+what+" を 1 つ以上書く")
	}
	return xs
}
