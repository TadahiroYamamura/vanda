package main_test

import (
	"fmt"
	"testing"
)

// 用語と機能の詳細の API（GET /api/terms/<ID>・GET /api/features/<ID>）を、vanda serve を外から動かして確かめる。
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

[[term]]
id = "t-root"
name = "タスク"
definition = "やること"
`

// story-24 の約束リスト（今の約束リスト）。
// 子の用語（t-child-b・t-child-a）と約束（c-z・c-a・c-m）は、ID の順と書いた順が違うように並べる。
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

[[feature]]
id = "f-core"
name = "コアの機能"
definition = "{@id:f-plain}より大事"
core = true

[[feature]]
id = "f-core-noformal"
name = "形式手法を使わないコアの機能"
definition = "大事だが形で確かめない"
core = true
formal = false
formal_waiver = "{@id:t-root}は小さい"

[[feature]]
id = "f-formal"
name = "形式手法を使う機能の例"
definition = "形で確かめる"
formal = true

[[feature]]
id = "f-both-false"
name = "どちらでもない機能"
definition = "書いて偽にした"
core = false
formal = false

[[claim]]
id = "c-z"
subjects = ["t-mid", "f-core"]
statement = "{@id:t-mid}は重ならない"
layers = ["model"]
model_check_waiver = "小さい"

[[claim.evidence]]
stage = "implementation"
type = "unit-test"
layers = ["model"]

[[claim]]
id = "c-a"
subjects = ["t-root"]
statement = "{@id:t-root}を足せる"
layers = ["model"]

[[claim.evidence]]
stage = "implementation"
type = "unit-test"
layers = ["model"]

[[claim]]
id = "c-m"
subjects = ["f-plain", "t-mid", "f-core"]
statement = "{@id:f-core}は{@id:c-a}の後に{@id:f-formal}を使う"
layers = ["model"]
model_check_waiver = "小さい"

[[claim.evidence]]
stage = "implementation"
type = "unit-test"
layers = ["model"]
`

type tfClaim struct {
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
	Claims     []tfClaim          `json:"claims"`
	Refs       map[string]refInfo `json:"refs"`
}

type tfFeature struct {
	ID         string             `json:"id"`
	Name       string             `json:"name"`
	Definition frags              `json:"definition"`
	Core       *bool              `json:"core"`
	Formal     *bool              `json:"formal"`
	Claims     []tfClaim          `json:"claims"`
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
	g.commitAll("はじめの約束を置く", "#1")
	g.run("switch", "-q", "-c", "story-24")
	writeFile(t, root, ".vanda/promises.toml", tfAPIStory)
	g.commitAll("用語と機能を足す", "#24")
	// 書いた約束リストが形の審査を通ること（通らなければ、API の誤りと言えない）
	if code, out := runCheck(t, bin, root, ".vanda/checks/shape.toml"); code != 0 {
		t.Fatalf("テストの約束リストが形の審査を通らない（%d）\n%s", code, out)
	}
	return startServe(t, bin, root, g.env)
}

func wantClaims(t *testing.T, where string, got []tfClaim, want []tfClaim) {
	t.Helper()
	if got == nil {
		t.Errorf("%s の claims が null（なければ [] のはず）", where)
	}
	if len(got) != len(want) {
		t.Errorf("%s の claims の数: %d（思ったもの %d）: %+v", where, len(got), len(want), got)
		return
	}
	for i := range want {
		if got[i].ID != want[i].ID {
			t.Errorf("%s の claims[%d] の ID: %s（思ったもの %s）", where, i, got[i].ID, want[i].ID)
			continue
		}
		wantFrags(t, fmt.Sprintf("%s の claims[%d]（%s）の statement", where, i, want[i].ID), got[i].Statement, want[i].Statement)
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
	"c-a":      {Kind: "claim", Name: nil, Text: "タスクを足せる", Current: true, Pieces: fragsOf("タスク>t-root", "を足せる")},
}

func tfRefsOf(ids ...string) map[string]refInfo {
	out := map[string]refInfo{}
	for _, id := range ids {
		out[id] = tfRefs[id]
	}
	return out
}

// @vanda:evidence term-page-content
// @vanda:evidence term-feature-page-claims
// @vanda:evidence reference-shown
func Test用語の詳細は名前とIDと片に分けた定義と親の用語と書いた順の子の用語とそれを対象に持つ約束を書いた順に返す(t *testing.T) {
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
	// subjects に t-mid を持つ約束を、約束リストに書いた順に（c-a は持たないので入らない）
	wantClaims(t, "t-mid", mid.Claims, []tfClaim{
		{ID: "c-z", Statement: fragsOf("タスクの ID>t-mid", "は重ならない")},
		{ID: "c-m", Statement: fragsOf("コアの機能>f-core", "は", "c-a>c-a", "の後に", "形式手法を使う機能の例>f-formal", "を使う")},
	})
	wantRefs(t, "t-mid の詳細", mid.Refs, tfRefsOf("t-root", "t-mid", "f-core", "c-a", "f-formal"))

	// part_of を書いていない用語は partOf が null。子も約束もない用語は [] にする
	var root tfTerm
	if code, msg := s.get("/api/terms/t-root", &root); code != 200 {
		t.Fatalf("用語 t-root の詳細を読めない（%d）: %s", code, msg)
	}
	if root.PartOf != nil {
		t.Errorf("t-root の partOf が null でない: %+v", root.PartOf)
	}
	wantNamedList(t, "t-root の children", root.Children, []tfNamed{named("t-mid", "タスクの ID"), named("t-other", "ほかの用語")})
	wantClaims(t, "t-root", root.Claims, []tfClaim{{ID: "c-a", Statement: fragsOf("タスク>t-root", "を足せる")}})

	var leaf tfTerm
	if code, msg := s.get("/api/terms/t-child-a", &leaf); code != 200 {
		t.Fatalf("用語 t-child-a の詳細を読めない（%d）: %s", code, msg)
	}
	wantNamedList(t, "t-child-a の children", leaf.Children, nil)
	wantClaims(t, "t-child-a", leaf.Claims, nil)
	if leaf.Refs == nil {
		t.Errorf("t-child-a の refs が null（何も指していなくても {} のはず）")
	}
}

// @vanda:evidence feature-page-content
// @vanda:evidence term-feature-page-claims
// @vanda:evidence reference-shown
func Test機能の詳細は名前とIDと片に分けた定義とコア機能か形式手法を使う機能かとそれを対象に持つ約束を書いた順に返す(t *testing.T) {
	t.Parallel()
	s := setupTFAPI(t)

	// core・formal は書いていなければ偽、formal は書いていなければ core と同じ値
	cases := []struct {
		id, name     string
		definition   frags
		core, formal bool
		claims       []tfClaim
		refs         []string
	}{
		{"f-plain", "ふつうの機能", fragsOf("タスク>t-root", "を見る"), false, false,
			[]tfClaim{{ID: "c-m", Statement: fragsOf("コアの機能>f-core", "は", "c-a>c-a", "の後に", "形式手法を使う機能の例>f-formal", "を使う")}},
			[]string{"t-root", "f-core", "c-a", "f-formal"}},
		{"f-core", "コアの機能", fragsOf("ふつうの機能>f-plain", "より大事"), true, true,
			[]tfClaim{
				{ID: "c-z", Statement: fragsOf("タスクの ID>t-mid", "は重ならない")},
				{ID: "c-m", Statement: fragsOf("コアの機能>f-core", "は", "c-a>c-a", "の後に", "形式手法を使う機能の例>f-formal", "を使う")},
			},
			[]string{"f-plain", "t-mid", "f-core", "c-a", "f-formal"}},
		{"f-core-noformal", "形式手法を使わないコアの機能", textOf("大事だが形で確かめない"), true, false, nil, nil},
		{"f-formal", "形式手法を使う機能の例", textOf("形で確かめる"), false, true, nil, nil},
		{"f-both-false", "どちらでもない機能", textOf("書いて偽にした"), false, false, nil, nil},
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
		wantClaims(t, c.id, f.Claims, c.claims)
		wantRefs(t, c.id+" の詳細", f.Refs, tfRefsOf(c.refs...))
	}
}

// @vanda:evidence term-feature-page-missing
func Test今の約束リストにないIDの用語と機能の詳細は404でそのIDが約束リストにないと返す(t *testing.T) {
	t.Parallel()
	s := setupTFAPI(t)

	cases := []struct{ path, want string }{
		// どこにもない ID
		{"/api/terms/t-none", "用語 t-none は約束リストにありません。"},
		{"/api/features/f-none", "機能 f-none は約束リストにありません。"},
		// 比べる元（main）にだけある ID。今の約束リストだけを見る
		{"/api/terms/t-old", "用語 t-old は約束リストにありません。"},
		{"/api/features/f-old", "機能 f-old は約束リストにありません。"},
		// 今の約束リストにあっても、種類の違うもの（用語の ID の機能、機能の ID の用語、約束の ID）は、その種類の約束リストの項目にない
		{"/api/features/t-root", "機能 t-root は約束リストにありません。"},
		{"/api/terms/f-core", "用語 f-core は約束リストにありません。"},
		{"/api/terms/c-a", "用語 c-a は約束リストにありません。"},
		{"/api/features/c-a", "機能 c-a は約束リストにありません。"},
	}
	for _, c := range cases {
		code, msg := s.get(c.path, nil)
		if code != 404 || msg != c.want {
			t.Errorf("%s: %d %q（思ったもの 404 %q）", c.path, code, msg, c.want)
		}
	}
}
