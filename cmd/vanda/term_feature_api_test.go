package main_test

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// 用語・機能・性質・前提の詳細の API（GET /api/terms/<ID>・GET /api/features/<ID>・GET /api/properties/<ID>・GET /api/truths/<ID>）と、
// 仕様の詳細の refines・assumes の先を、vanda serve を外から動かして確かめる。
// 形は docs/formats.md の「GUI の API」。期待値は、下の約束リストから手で書いた。

// main の約束リスト。t-old と f-old は、story-24 で消す
const tfAPIBase = `[[term]]
id = "t-old"
name = "消す用語"
definition = "消す用語の定義"

[[feature]]
id = "f-old"
name = "消す機能"
definition = "消す機能の定義"
refines = ["p"]

[[term]]
id = "t-root"
name = "タスク"
definition = "やること"

[[property]]
id = "p"
statement = "性質の文"

[[property]]
id = "p-top"
statement = "前の一番上の性質"

[[truth]]
id = "tr-b"
statement = "前の前提"
`

// story-24 の約束リスト（今の約束リスト）。
// 子の用語（t-child-b・t-child-a）と仕様（c-z・c-a・c-m）は、ID の順と書いた順が違うように並べる。
// 仕様の refines は、機能ごとに、refines する仕様がない・1 つ・2 つになるように書く。
// 機能は core・formal の書き方の組み合わせ（両方省く、core だけ書く、formal だけ書く、両方書く）を全部置く
const tfAPIStory = `[[term]]
id = "t-root"
name = "タスク"
definition = "やること"

[[term]]
id = "t-mid"
name = "タスクの ID"
definition = "{@id:t-root}を見分ける名前。{@@id:x}は文字"
part_of = "t-root"
assumes = ["tr-shown"]

[[term]]
id = "t-child-b"
name = "ID の接頭辞"
definition = "{@id:t-mid}の頭"
part_of = "t-mid"

[[term]]
id = "t-other"
name = "ほかの用語"
definition = "子でない用語"
part_of = "t-root"

[[term]]
id = "t-child-a"
name = "ID の番号"
definition = "{@id:t-mid}の後ろ"
part_of = "t-mid"

[[feature]]
id = "f-plain"
name = "ふつうの機能"
definition = "{@id:t-root}を見る"
refines = ["p"]

[[feature]]
id = "f-core"
name = "コアの機能"
definition = "{@id:f-plain}より大事"
refines = ["p-leaf", "p"]
core = true

[[feature]]
id = "f-core-noformal"
name = "形式手法を使わないコアの機能"
definition = "大事だが形で確かめない"
refines = ["p"]
core = true
formal = false
formal_waiver = "{@id:t-root}は小さい"

[[feature]]
id = "f-formal"
name = "形式手法を使う機能の例"
definition = "形で確かめる"
refines = ["p"]
formal = true

[[feature]]
id = "f-both-false"
name = "どちらでもない機能"
definition = "書いて偽にした"
refines = ["p"]
core = false
formal = false

[[spec]]
id = "c-z"
refines = ["f-core"]
assumes = ["tr-shown", "tr-b"]
statement = "{@id:t-mid}は重ならない"
layers = ["model"]
model_check_waiver = "小さい"

[[spec.evidence]]
stage = "implementation"
type = "unit-test"
layers = ["model"]

[[spec]]
id = "c-a"
refines = ["f-both-false"]
statement = "{@id:t-root}を足せる"
layers = ["model"]

[[spec.evidence]]
stage = "implementation"
type = "unit-test"
layers = ["model"]

[[spec]]
id = "c-m"
refines = ["f-plain", "f-core"]
statement = "{@id:f-core}は{@id:c-a}の後に{@id:f-formal}を使う"
layers = ["model"]
model_check_waiver = "小さい"

[[spec.evidence]]
stage = "implementation"
type = "unit-test"
layers = ["model"]

[[property]]
id = "p"
statement = "性質の文"

[[property]]
id = "p-shown"
statement = "{@id:t-root}を守る"

[[truth]]
id = "tr-shown"
statement = "ディスクは読める"

[[term]]
id = "t-refs"
name = "性質と前提を指す用語"
definition = "{@id:p-shown}と{@id:tr-shown}を前にする"

[[property]]
id = "p-top"
statement = "一番上の性質"

# refines と assumes の先は、ID の順と書いた順が違うように並べる
[[property]]
id = "p-leaf"
statement = "{@id:t-root}は{@id:tr-shown}のもとで失われない"
rationale = "{@id:p-top}のため"
refines = ["p-top", "p-shown"]
assumes = ["tr-shown", "tr-b"]

[[truth]]
id = "tr-b"
statement = "{@id:t-root}は消えない"
`

// 手で書いた、フィンガープリントの元の JSON（docs/formats.md の「フィンガープリント」）
const (
	tfPLeafJSON  = `{"assumes":["tr-shown","tr-b"],"id":"p-leaf","rationale":"{@id:p-top}のため","refines":["p-top","p-shown"],"statement":"{@id:t-root}は{@id:tr-shown}のもとで失われない"}`
	tfPTopOld    = `{"id":"p-top","statement":"前の一番上の性質"}`
	tfPTopJSON   = `{"id":"p-top","statement":"一番上の性質"}`
	tfTruthBOld  = `{"id":"tr-b","statement":"前の前提"}`
	tfTruthBJSON = `{"id":"tr-b","statement":"{@id:t-root}は消えない"}`
)

// tfLink は、詳細の refines・assumes の 1 つ（指す先の種類・ID・名前）。
type tfLink struct {
	Kind *string `json:"kind"`
	ID   string  `json:"id"`
	Name *string `json:"name"`
}

// link は、期待値を手で書くためのもの。name が空なら null（名前を持たない性質・前提・仕様）
func link(kind, id, name string) tfLink {
	l := tfLink{Kind: &kind, ID: id}
	if name != "" {
		l.Name = &name
	}
	return l
}

func wantLinks(t *testing.T, where string, got, want []tfLink) {
	t.Helper()
	if got == nil {
		t.Errorf("%s が null（なければ [] のはず）", where)
	}
	strs := func(ls []tfLink) []string {
		out := []string{}
		for _, l := range ls {
			name := "null"
			if l.Name != nil {
				name = *l.Name
			}
			kind := "null"
			if l.Kind != nil {
				kind = *l.Kind
			}
			out = append(out, kind+" "+l.ID+" "+name)
		}
		return out
	}
	if fmt.Sprint(strs(got)) != fmt.Sprint(strs(want)) {
		t.Errorf("%s: %v（思ったもの %v）", where, strs(got), strs(want))
	}
}

type tfPropertyTruth struct {
	ID          string             `json:"id"`
	Statement   frags              `json:"statement"`
	Rationale   frags              `json:"rationale"`
	Refines     []tfLink           `json:"refines"`
	Assumes     []tfLink           `json:"assumes"`
	State       string             `json:"state"`
	Fingerprint *string            `json:"fingerprint"`
	Refs        map[string]refInfo `json:"refs"`
}

type tfSpecLinks struct {
	Refines []tfLink `json:"refines"`
	Assumes []tfLink `json:"assumes"`
}

type tfSpec struct {
	ID        string `json:"id"`
	Statement frags  `json:"statement"`
}

type tfNamed struct {
	ID   string  `json:"id"`
	Name *string `json:"name"`
}

type tfTerm struct {
	ID         string             `json:"id"`
	Name       string             `json:"name"`
	Definition frags              `json:"definition"`
	PartOf     *tfNamed           `json:"partOf"`
	Children   []tfNamed          `json:"children"`
	Assumes    []tfLink           `json:"assumes"`
	Refs       map[string]refInfo `json:"refs"`
}

type tfFeature struct {
	ID         string             `json:"id"`
	Name       string             `json:"name"`
	Definition frags              `json:"definition"`
	Core       *bool              `json:"core"`
	Formal     *bool              `json:"formal"`
	Refines    []tfLink           `json:"refines"`
	Specs      []tfSpec           `json:"specs"`
	Refs       map[string]refInfo `json:"refs"`
}

func setupTFAPI(t *testing.T) *vandaServer {
	t.Helper()
	bin, version := refSetup(t)
	root := t.TempDir()
	g := newRVGit(t, root)
	g.run("init", "-q", "-b", "main")
	writeFile(t, root, ".vanda/config.toml", fmt.Sprintf(refAPIConfig, version))
	writeFile(t, root, ".vanda/checks/shape.toml", "[[check]]\nname = \"promise-list-shape\"\nscope = \"all\"\n")
	writeFile(t, root, ".vanda/promises.toml", tfAPIBase)
	// 承認の記録。性質 p-top と前提 tr-b は、前の中身を #1 で、今の中身を #24 で承認する。p-leaf は今の中身を承認し、p-shown と tr-shown は承認しない。
	// 終わりの 2 件は、同じ ID で種類の違う記録。性質・前提の履歴に、その種類の記録だけが出ることを見る
	writeFile(t, root, ".vanda/agreements.toml",
		agreementTOML("property", "p-top", sha256Hex([]byte(tfPTopOld)), "", "#1", "テスト担当", "tester@example.com")+
			agreementTOML("truth", "tr-b", sha256Hex([]byte(tfTruthBOld)), "", "#1", "テスト担当", "tester@example.com"))
	g.commitAll("はじめの仕様を置く", "#1")
	g.run("switch", "-q", "-c", "story-24")
	writeFile(t, root, ".vanda/promises.toml", tfAPIStory)
	g.commitAll("用語と機能を足す", "#24")
	f, err := os.OpenFile(filepath.Join(root, ".vanda", "agreements.toml"), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	_, err = f.WriteString(agreementTOML("property", "p-top", sha256Hex([]byte(tfPTopJSON)), "", "#24", "オーナー", "owner@example.com") +
		agreementTOML("truth", "tr-b", sha256Hex([]byte(tfTruthBJSON)), "", "#24", "オーナー", "owner@example.com") +
		agreementTOML("property", "p-leaf", sha256Hex([]byte(tfPLeafJSON)), "", "#24", "オーナー", "owner@example.com") +
		agreementTOML("truth", "p-top", sha256Hex([]byte(tfPTopJSON)), "", "#24", "オーナー", "owner@example.com") +
		agreementTOML("property", "tr-b", sha256Hex([]byte(tfTruthBJSON)), "", "#24", "オーナー", "owner@example.com"))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	g.commitAll("(chore) 承認する", "#24")
	// 書いた約束リストが形のチェックを通ること（通らなければ、API の誤りと言えない）
	if code, out := runCheck(t, bin, root, ".vanda/checks/shape.toml"); code != 0 {
		t.Fatalf("テストの約束リストが形のチェックを通らない（%d）\n%s", code, out)
	}
	return startServe(t, bin, root, g.env)
}

func wantSpecs(t *testing.T, where string, got []tfSpec, want []tfSpec) {
	t.Helper()
	if got == nil {
		t.Errorf("%s の specs が null（なければ [] のはず）", where)
	}
	if len(got) != len(want) {
		t.Errorf("%s の specs の数: %d（思ったもの %d）: %+v", where, len(got), len(want), got)
		return
	}
	for i := range want {
		if got[i].ID != want[i].ID {
			t.Errorf("%s の specs[%d] の ID: %s（思ったもの %s）", where, i, got[i].ID, want[i].ID)
			continue
		}
		wantFrags(t, fmt.Sprintf("%s の specs[%d]（%s）の statement", where, i, want[i].ID), got[i].Statement, want[i].Statement)
	}
}

func named(id, name string) tfNamed { return tfNamed{ID: id, Name: &name} }

func wantNamedList(t *testing.T, where string, got []tfNamed, want []tfNamed) {
	t.Helper()
	if got == nil {
		t.Errorf("%s が null（なければ [] のはず）", where)
	}
	if fmt.Sprint(namedStrs(got)) != fmt.Sprint(namedStrs(want)) {
		t.Errorf("%s: %v（思ったもの %v）", where, namedStrs(got), namedStrs(want))
	}
}

func namedStrs(ns []tfNamed) []string {
	out := []string{}
	for _, n := range ns {
		out = append(out, n.ID+"="+str(n.Name))
	}
	return out
}

// 今の約束リストの、参照先の中身（refs の 1 つずつ）
var tfRefs = map[string]refInfo{
	"t-root":  {Kind: "term", Name: strp("タスク"), Text: "やること", Current: true, Pieces: textOf("やること")},
	"t-mid":   {Kind: "term", Name: strp("タスクの ID"), Text: "タスクを見分ける名前。{@id:x}は文字", Current: true, Pieces: fragsOf("タスク>t-root", "を見分ける名前。{@id:x}は文字")},
	"f-plain": {Kind: "feature", Name: strp("ふつうの機能"), Text: "タスクを見る", Current: true, Pieces: fragsOf("タスク>t-root", "を見る"), Core: boolp(false), Formal: boolp(false)},
	// formal を省いたので core と同じ
	"f-core": {Kind: "feature", Name: strp("コアの機能"), Text: "ふつうの機能より大事", Current: true, Pieces: fragsOf("ふつうの機能>f-plain", "より大事"), Core: boolp(true), Formal: boolp(true)},
	// core を省いたので偽
	"f-formal": {Kind: "feature", Name: strp("形式手法を使う機能の例"), Text: "形で確かめる", Current: true, Pieces: textOf("形で確かめる"), Core: boolp(false), Formal: boolp(true)},
	"c-a":      {Kind: "spec", Name: nil, Text: "タスクを足せる", Current: true, Pieces: fragsOf("タスク>t-root", "を足せる")},
	"p-top":    {Kind: "property", Name: nil, Text: "一番上の性質", Current: true, Pieces: textOf("一番上の性質")},
	"tr-shown": {Kind: "truth", Name: nil, Text: "ディスクは読める", Current: true, Pieces: textOf("ディスクは読める")},
}

func tfRefsOf(ids ...string) map[string]refInfo {
	out := map[string]refInfo{}
	for _, id := range ids {
		out[id] = tfRefs[id]
	}
	return out
}

// @vanda:evidence term-page-content
// @vanda:evidence reference-shown
func Test用語の詳細は名前とIDとセグメントに分けた定義と親の用語と書いた順の子の用語とassumesの先を返す(t *testing.T) {
	t.Parallel()
	s := setupTFAPI(t)

	var mid tfTerm
	if code, msg := s.get("/api/terms/t-mid", &mid); code != 200 {
		t.Fatalf("用語 t-mid の詳細を読めない（%d）: %s", code, msg)
	}
	if mid.ID != "t-mid" || mid.Name != "タスクの ID" {
		t.Errorf("t-mid の id・name: %q・%q", mid.ID, mid.Name)
	}
	wantFrags(t, "t-mid の definition", mid.Definition, fragsOf("タスク>t-root", "を見分ける名前。{@id:x}は文字"))
	if mid.PartOf == nil {
		t.Errorf("t-mid の partOf が null（t-root のはず）")
	} else if mid.PartOf.ID != "t-root" || str(mid.PartOf.Name) != "タスク" {
		t.Errorf("t-mid の partOf: %s=%s（思ったもの t-root=タスク）", mid.PartOf.ID, str(mid.PartOf.Name))
	}
	// 子は、part_of に t-mid を書いた用語だけを、約束リストに書いた順に（t-other は t-root の子なので入らない）
	wantNamedList(t, "t-mid の children", mid.Children, []tfNamed{named("t-child-b", "ID の接頭辞"), named("t-child-a", "ID の番号")})
	wantRefs(t, "t-mid の詳細", mid.Refs, tfRefsOf("t-root"))
	// assumes は、用語の assumes の先。書いていなければ []
	wantLinks(t, "t-mid の assumes", mid.Assumes, []tfLink{link("truth", "tr-shown", "")})

	// part_of を書いていない用語は partOf が null。子のない用語は [] にする
	var root tfTerm
	if code, msg := s.get("/api/terms/t-root", &root); code != 200 {
		t.Fatalf("用語 t-root の詳細を読めない（%d）: %s", code, msg)
	}
	if root.PartOf != nil {
		t.Errorf("t-root の partOf が null でない: %+v", root.PartOf)
	}
	wantNamedList(t, "t-root の children", root.Children, []tfNamed{named("t-mid", "タスクの ID"), named("t-other", "ほかの用語")})
	wantLinks(t, "t-root の assumes", root.Assumes, nil)

	var leaf tfTerm
	if code, msg := s.get("/api/terms/t-child-a", &leaf); code != 200 {
		t.Fatalf("用語 t-child-a の詳細を読めない（%d）: %s", code, msg)
	}
	wantNamedList(t, "t-child-a の children", leaf.Children, nil)
	if leaf.Refs == nil {
		t.Errorf("t-child-a の refs が null（何も指していなくても {} のはず）")
	}
}

// 性質と前提は名前を持たないので、参照は ID に置き換えて見せる（仕様と同じ）。refs の中身は docs/formats.md の「セグメントの配列」の表
func Test性質と前提への参照はIDに置き換えて見せrefsに種類と文を返す(t *testing.T) {
	t.Parallel()
	s := setupTFAPI(t)

	var got tfTerm
	if code, msg := s.get("/api/terms/t-refs", &got); code != 200 {
		t.Fatalf("用語 t-refs の詳細を読めない（%d）: %s", code, msg)
	}
	wantFrags(t, "t-refs の definition", got.Definition, fragsOf("p-shown>p-shown", "と", "tr-shown>tr-shown", "を前にする"))
	wantRefs(t, "t-refs の詳細", got.Refs, map[string]refInfo{
		"p-shown":  {Kind: "property", Name: nil, Text: "タスクを守る", Current: true, Pieces: fragsOf("タスク>t-root", "を守る")},
		"tr-shown": {Kind: "truth", Name: nil, Text: "ディスクは読める", Current: true, Pieces: textOf("ディスクは読める")},
	})
}

// @vanda:evidence feature-page-content
// @vanda:evidence term-feature-page-claims
// @vanda:evidence reference-shown
func Test機能の詳細は名前とIDとセグメントに分けた定義とコア機能か形式手法を使う機能かとrefinesの先とそれをrefinesする仕様を書いた順に返す(t *testing.T) {
	t.Parallel()
	s := setupTFAPI(t)

	// core・formal は書いていなければ偽、formal は書いていなければ core と同じ値
	cases := []struct {
		id, name     string
		definition   frags
		core, formal bool
		refines      []tfLink
		specs        []tfSpec
		refs         []string
	}{
		{"f-plain", "ふつうの機能", fragsOf("タスク>t-root", "を見る"), false, false, []tfLink{link("property", "p", "")},
			[]tfSpec{{ID: "c-m", Statement: fragsOf("コアの機能>f-core", "は", "c-a>c-a", "の後に", "形式手法を使う機能の例>f-formal", "を使う")}},
			[]string{"t-root", "f-core", "c-a", "f-formal"}},
		{"f-core", "コアの機能", fragsOf("ふつうの機能>f-plain", "より大事"), true, true, []tfLink{link("property", "p-leaf", ""), link("property", "p", "")},
			[]tfSpec{
				{ID: "c-z", Statement: fragsOf("タスクの ID>t-mid", "は重ならない")},
				{ID: "c-m", Statement: fragsOf("コアの機能>f-core", "は", "c-a>c-a", "の後に", "形式手法を使う機能の例>f-formal", "を使う")},
			},
			[]string{"f-plain", "t-mid", "f-core", "c-a", "f-formal"}},
		{"f-core-noformal", "形式手法を使わないコアの機能", textOf("大事だが形で確かめない"), true, false, []tfLink{link("property", "p", "")}, nil, nil},
		{"f-formal", "形式手法を使う機能の例", textOf("形で確かめる"), false, true, []tfLink{link("property", "p", "")}, nil, nil},
		{"f-both-false", "どちらでもない機能", textOf("書いて偽にした"), false, false, []tfLink{link("property", "p", "")},
			[]tfSpec{{ID: "c-a", Statement: fragsOf("タスク>t-root", "を足せる")}},
			[]string{"t-root"}},
	}
	for _, c := range cases {
		var f tfFeature
		if code, msg := s.get("/api/features/"+c.id, &f); code != 200 {
			t.Errorf("機能 %s の詳細を読めない（%d）: %s", c.id, code, msg)
			continue
		}
		if f.ID != c.id || f.Name != c.name {
			t.Errorf("%s の id・name: %q・%q", c.id, f.ID, f.Name)
		}
		wantFrags(t, c.id+" の definition", f.Definition, c.definition)
		if f.Core == nil || *f.Core != c.core || f.Formal == nil || *f.Formal != c.formal {
			t.Errorf("%s の core・formal: %s・%s（思ったもの %v・%v）", c.id, boolStr(f.Core), boolStr(f.Formal), c.core, c.formal)
		}
		wantLinks(t, c.id+" の refines", f.Refines, c.refines)
		wantSpecs(t, c.id, f.Specs, c.specs)
		wantRefs(t, c.id+" の詳細", f.Refs, tfRefsOf(c.refs...))
	}
}

// @vanda:evidence term-feature-page-missing
func Test今の約束リストにないIDの用語と機能と性質と前提の詳細は404でそのIDが約束リストにないと返す(t *testing.T) {
	t.Parallel()
	s := setupTFAPI(t)

	cases := []struct{ path, want string }{
		// どこにもない ID
		{"/api/terms/t-none", "用語 t-none は約束リストにありません。"},
		{"/api/features/f-none", "機能 f-none は約束リストにありません。"},
		// 比べる元（main）にだけある ID。今の約束リストだけを見る
		{"/api/terms/t-old", "用語 t-old は約束リストにありません。"},
		{"/api/features/f-old", "機能 f-old は約束リストにありません。"},
		// 今の約束リストにあっても、種類の違うもの（用語の ID の機能、機能の ID の用語、仕様の ID）は、その種類の約束リストの項目にない
		{"/api/features/t-root", "機能 t-root は約束リストにありません。"},
		{"/api/terms/f-core", "用語 f-core は約束リストにありません。"},
		{"/api/terms/c-a", "用語 c-a は約束リストにありません。"},
		{"/api/features/c-a", "機能 c-a は約束リストにありません。"},
		// 性質と前提も同じ。履歴も、ない ID なら 404
		{"/api/properties/p-none", "性質 p-none は約束リストにありません。"},
		{"/api/truths/tr-none", "前提 tr-none は約束リストにありません。"},
		{"/api/properties/tr-b", "性質 tr-b は約束リストにありません。"},
		{"/api/truths/p-top", "前提 p-top は約束リストにありません。"},
		{"/api/properties/c-a", "性質 c-a は約束リストにありません。"},
		{"/api/truths/t-root", "前提 t-root は約束リストにありません。"},
		{"/api/features/p-top", "機能 p-top は約束リストにありません。"},
		{"/api/terms/tr-b", "用語 tr-b は約束リストにありません。"},
		{"/api/properties/p-none/history", "性質 p-none は約束リストにありません。"},
		{"/api/truths/tr-none/history", "前提 tr-none は約束リストにありません。"},
		{"/api/properties/tr-b/history", "性質 tr-b は約束リストにありません。"},
		{"/api/truths/p-top/history", "前提 p-top は約束リストにありません。"},
	}
	for _, c := range cases {
		code, msg := s.get(c.path, nil)
		if code != 404 || msg != c.want {
			t.Errorf("%s: %d %q（思ったもの 404 %q）", c.path, code, msg, c.want)
		}
	}
}

// 性質と前提の詳細（docs/formats.md の GET /api/properties/<ID>・GET /api/truths/<ID>）
func Test性質と前提の詳細は文と理由とrefinesとassumesの先と承認の状態とフィンガープリントを返す(t *testing.T) {
	t.Parallel()
	s := setupTFAPI(t)

	cases := []struct {
		path, id         string
		statement        frags
		rationale        frags // nil なら null
		refines, assumes []tfLink
		state, fpJSON    string
		refs             []string
	}{
		{"/api/properties/p-leaf", "p-leaf", fragsOf("タスク>t-root", "は", "tr-shown>tr-shown", "のもとで失われない"), fragsOf("p-top>p-top", "のため"),
			[]tfLink{link("property", "p-top", ""), link("property", "p-shown", "")}, []tfLink{link("truth", "tr-shown", ""), link("truth", "tr-b", "")},
			"agreed", tfPLeafJSON, []string{"t-root", "tr-shown", "p-top"}},
		// 理由と refines と assumes を書いていない性質。前の中身への承認と、今の中身への承認がある
		{"/api/properties/p-top", "p-top", textOf("一番上の性質"), nil, nil, nil, "agreed", tfPTopJSON, nil},
		// 承認の記録のない性質
		{"/api/properties/p-shown", "p-shown", fragsOf("タスク>t-root", "を守る"), nil, nil, nil, "proposed", `{"id":"p-shown","statement":"{@id:t-root}を守る"}`, []string{"t-root"}},
		{"/api/truths/tr-b", "tr-b", fragsOf("タスク>t-root", "は消えない"), nil, nil, nil, "agreed", tfTruthBJSON, []string{"t-root"}},
		{"/api/truths/tr-shown", "tr-shown", textOf("ディスクは読める"), nil, nil, nil, "proposed", `{"id":"tr-shown","statement":"ディスクは読める"}`, nil},
	}
	for _, c := range cases {
		var got tfPropertyTruth
		if code, msg := s.get(c.path, &got); code != 200 {
			t.Errorf("%s を読めない（%d）: %s", c.path, code, msg)
			continue
		}
		if got.ID != c.id {
			t.Errorf("%s の id: %q", c.path, got.ID)
		}
		wantFrags(t, c.path+" の statement", got.Statement, c.statement)
		if c.rationale == nil {
			if got.Rationale != nil {
				t.Errorf("%s の rationale が null でない: %v", c.path, got.Rationale)
			}
		} else {
			wantFrags(t, c.path+" の rationale", got.Rationale, c.rationale)
		}
		// 前提は refines・assumes を持たない（docs/formats.md の例に項目がない）ので、性質だけで見る
		if strings.HasPrefix(c.path, "/api/properties/") {
			wantLinks(t, c.path+" の refines", got.Refines, c.refines)
			wantLinks(t, c.path+" の assumes", got.Assumes, c.assumes)
		}
		if got.State != c.state {
			t.Errorf("%s の state: %q（思ったもの %q）", c.path, got.State, c.state)
		}
		if want := sha256Hex([]byte(c.fpJSON)); str(got.Fingerprint) != want {
			t.Errorf("%s の fingerprint: %q（思ったもの %q）", c.path, str(got.Fingerprint), want)
		}
		wantRefs(t, c.path, got.Refs, tfRefsOf(c.refs...))
	}
}

// 性質と前提の履歴（docs/formats.md の GET /api/properties/<ID>/history・GET /api/truths/<ID>/history）。形と決まりは仕様の履歴と同じ
func Test性質と前提の履歴は中身を変えたコミットとその種類でそのIDを指す承認の記録だけを新しい順に返す(t *testing.T) {
	t.Parallel()
	s := setupTFAPI(t)

	cases := []struct {
		path, oldJSON, curJSON string
	}{
		{"/api/properties/p-top/history", tfPTopOld, tfPTopJSON},
		{"/api/truths/tr-b/history", tfTruthBOld, tfTruthBJSON},
	}
	for _, c := range cases {
		var h specHistory
		if code, msg := s.get(c.path, &h); code != 200 {
			t.Errorf("%s を読めない（%d）: %s", c.path, code, msg)
			continue
		}
		// 変更履歴は新しい順。承認のコミットは中身を変えていないので出ない
		type origin struct {
			subject, change string
			stories         []string
		}
		var got []origin
		for _, o := range h.Origin {
			got = append(got, origin{o.Subject, o.Change, o.Stories})
		}
		want := []origin{{"用語と機能を足す", "modified", []string{"#24"}}, {"はじめの仕様を置く", "added", []string{"#1"}}}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s の origin: %+v（思ったもの %+v）", c.path, got, want)
		}
		// 承認の履歴は、同じ日時ならファイルで後に書いたものが先。種類の違う記録（同じ ID を指す、ほかの種類のもの）は出ない
		type agreement struct {
			story, fp, user string
			current         bool
		}
		var gotA []agreement
		for _, a := range h.Agreements {
			gotA = append(gotA, agreement{a.Story, a.Fingerprint, a.User.Name, a.Current})
			if a.Commit == nil {
				t.Errorf("%s の承認の記録（%s）の commit が null（コミットしてある）", c.path, a.Story)
			}
		}
		wantA := []agreement{{"#24", sha256Hex([]byte(c.curJSON)), "オーナー", true}, {"#1", sha256Hex([]byte(c.oldJSON)), "テスト担当", false}}
		if !reflect.DeepEqual(gotA, wantA) {
			t.Errorf("%s の agreements: %+v（思ったもの %+v）", c.path, gotA, wantA)
		}
	}
	// その時点の中身を返す API は、仕様にだけある
	for _, path := range []string{"/api/properties/p-top/at/" + strings.Repeat("0", 40), "/api/truths/tr-b/at/" + strings.Repeat("0", 40)} {
		if code, _ := s.get(path, nil); code != 404 {
			t.Errorf("%s が %d（性質と前提には、その時点の中身の API がないので 404 のはず）", path, code)
		}
	}
}

// 仕様の詳細の refines・assumes（docs/formats.md の GET /api/specs/<ID>）
func Test仕様の詳細はrefinesとassumesの先を書いた順に種類と名前を付けて返す(t *testing.T) {
	t.Parallel()
	s := setupTFAPI(t)

	cases := []struct {
		id               string
		refines, assumes []tfLink
	}{
		{"c-z", []tfLink{link("feature", "f-core", "コアの機能")}, []tfLink{link("truth", "tr-shown", ""), link("truth", "tr-b", "")}},
		{"c-m", []tfLink{link("feature", "f-plain", "ふつうの機能"), link("feature", "f-core", "コアの機能")}, nil},
		{"c-a", []tfLink{link("feature", "f-both-false", "どちらでもない機能")}, nil},
	}
	for _, c := range cases {
		var got tfSpecLinks
		if code, msg := s.get("/api/specs/"+c.id, &got); code != 200 {
			t.Errorf("仕様 %s の詳細を読めない（%d）: %s", c.id, code, msg)
			continue
		}
		wantLinks(t, c.id+" の refines", got.Refines, c.refines)
		wantLinks(t, c.id+" の assumes", got.Assumes, c.assumes)
	}
}
