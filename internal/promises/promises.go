// Package promises は、約束リスト（既定は .vanda/promises.toml）を読む。
//
// ここでは、読めること、決まった表と項目だけがあること、必須の項目があること、項目の型を確かめる。
// 値が決まった語の中にあるか、ID の形と重複、指す先があるかは、チェックで確かめる。
package promises

import (
	"github.com/TadahiroYamamura/vanda/internal/diag"
	"github.com/TadahiroYamamura/vanda/internal/glob"
	"github.com/TadahiroYamamura/vanda/internal/tomlread"
)

type List struct {
	Files      []string
	Properties []*Property
	Truths     []*Truth
	Terms      []*Term
	Features   []*Feature
	Specs      []*Spec
}

// Item は、性質・前提・用語・機能・仕様に共通するもの。Raw は読み込んだ表そのもの（フィンガープリントを作る元）。
type Item struct {
	ID    string
	Table *tomlread.Table
	Raw   map[string]any
}

type Property struct {
	Item
	Statement string
	Rationale string
	Refines   []string
	Assumes   []string
}

type Truth struct {
	Item
	Statement string
}

type Term struct {
	Item
	Name       string
	Definition string
	PartOf     string // 書いていなければ空
	Assumes    []string
}

type Feature struct {
	Item
	Name       string
	Definition string
	Refines    []string
	Core       bool
	// Formal は、書いていなければ、core = true のとき真とみなした値
	Formal       bool
	FormalWaiver string // 書いていなければ空
}

type Spec struct {
	Item
	Refines   []string
	Assumes   []string
	Statement string
	Rationale string
	Condition string
	Layers    []string
	Flags     []string
	Guesses   []string
	Evidence  []*Evidence
	// ModelCheckWaiver は、TLA+ で確かめない理由。書いていなければ空
	ModelCheckWaiver string
}

// HasFlagsOrGuesses は、フラグか推測が残っているかを返す。残っていれば、承認済でも、チェック agreement は通さない。
// そのチェックと、レビューの画面の一覧が食い違わないように、どちらもこれで決める
func (c *Spec) HasFlagsOrGuesses() bool {
	return len(c.Flags) > 0 || len(c.Guesses) > 0
}

type Evidence struct {
	Table  *tomlread.Table
	Stage  string
	Type   string
	Layers []string
	Detail string
}

// Load は、files（リポジトリの直下からの場所の一覧）のうち patterns にマッチするものを、約束リストとして読む。allowMissing なら、ワイルドカードのないパターンのファイルがなくても誤りにしない（設定に書かなかった既定のファイルのため）。
func Load(root string, files, patterns []string, allowMissing bool) (*List, diag.Errors) {
	return LoadWith(tomlread.OSReader(root), files, patterns, allowMissing)
}

// LoadWith は、Load と同じく読む。ファイルの中身は read で読む。
func LoadWith(read tomlread.Reader, files, patterns []string, allowMissing bool) (*List, diag.Errors) {
	var errs []diag.Error
	matched, missing := glob.Select(files, patterns)
	if allowMissing {
		missing = nil
	}
	for _, m := range missing {
		errs = append(errs, diag.Error{File: m, What: "約束リストのファイルがありません", Fix: "ファイルを作るか、.vanda/config.toml の promises を直してください"})
	}
	l := &List{Files: matched}
	for _, f := range matched {
		doc := tomlread.LoadWith(read, f, &errs)
		if doc == nil {
			continue
		}
		l.read(doc.Top())
	}
	if len(errs) > 0 {
		diag.Sort(errs)
		return l, errs
	}
	return l, nil
}

func (l *List) read(top *tomlread.Table) {
	properties, _ := top.Tables("property", false)
	for _, t := range properties {
		x := &Property{Item: item(t, "property")}
		x.Statement, _ = t.Str("statement", true)
		x.Rationale, _ = t.Str("rationale", false)
		x.Refines, _ = t.Strs("refines", false)
		x.Assumes, _ = t.Strs("assumes", false)
		t.Unknown("id, statement, rationale, refines, assumes")
		l.Properties = append(l.Properties, x)
	}
	truths, _ := top.Tables("truth", false)
	for _, t := range truths {
		x := &Truth{Item: item(t, "truth")}
		x.Statement, _ = t.Str("statement", true)
		t.Unknown("id, statement")
		l.Truths = append(l.Truths, x)
	}
	terms, _ := top.Tables("term", false)
	for _, t := range terms {
		x := &Term{Item: item(t, "term")}
		x.Name, _ = t.Str("name", true)
		x.Definition, _ = t.Str("definition", true)
		x.PartOf, _ = t.Str("part_of", false)
		x.Assumes, _ = t.Strs("assumes", false)
		t.Unknown("id, name, definition, part_of, assumes")
		l.Terms = append(l.Terms, x)
	}
	features, _ := top.Tables("feature", false)
	for _, t := range features {
		x := &Feature{Item: item(t, "feature")}
		x.Name, _ = t.Str("name", true)
		x.Definition, _ = t.Str("definition", true)
		x.Refines = nonEmpty(t, "refines", "性質の ID")
		x.Core, _ = t.Bool("core", false)
		formal, ok := t.Bool("formal", false)
		if ok {
			x.Formal = formal
		} else {
			x.Formal = x.Core
		}
		x.FormalWaiver, _ = t.Str("formal_waiver", false)
		t.Unknown("id, name, definition, refines, core, formal, formal_waiver")
		l.Features = append(l.Features, x)
	}
	specs, _ := top.Tables("spec", false)
	for _, t := range specs {
		x := &Spec{Item: item(t, "spec")}
		x.Refines = nonEmpty(t, "refines", "機能の ID")
		x.Assumes, _ = t.Strs("assumes", false)
		x.Statement, _ = t.Str("statement", true)
		x.Rationale, _ = t.Str("rationale", false)
		x.Condition, _ = t.Str("condition", false)
		x.Layers = nonEmpty(t, "layers", "レイヤーの ID")
		x.Flags, _ = t.Strs("flags", false)
		x.Guesses, _ = t.Strs("guesses", false)
		x.ModelCheckWaiver, _ = t.Str("model_check_waiver", false)
		evs, ok := t.Tables("evidence", true)
		if ok && len(evs) == 0 {
			t.ErrAt("evidence", "evidence が 1 つもありません", "[[spec.evidence]] を 1 つ以上書いてください")
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
		t.Unknown("id, refines, assumes, statement, rationale, condition, layers, flags, guesses, evidence, model_check_waiver")
		l.Specs = append(l.Specs, x)
	}
	top.Unknown("property, truth, term, feature, spec")
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
		t.ErrAt(key, key+" が空です", key+" に"+what+" を 1 つ以上書いてください")
	}
	return xs
}

// FromSources は、読み込み済みの中身 srcs（場所から中身へ）を、names の順に約束リストとして読む。
// マージベースの中身を読むときに使う。
func FromSources(names []string, srcs map[string][]byte) (*List, diag.Errors) {
	var errs []diag.Error
	l := &List{Files: names}
	for _, f := range names {
		doc := tomlread.Parse(f, srcs[f], &errs)
		if doc == nil {
			continue
		}
		l.read(doc.Top())
	}
	if len(errs) > 0 {
		diag.Sort(errs)
		return l, errs
	}
	return l, nil
}
