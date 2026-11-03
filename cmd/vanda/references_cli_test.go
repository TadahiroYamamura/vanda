package main_test

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
)

// 参照（{@id:<ID>}）を約束リストから読むこと（約束 reference-form・reference-literal・reference-broken・reference-fingerprint-raw）を、
// 一時のリポジトリで vanda check を動かして確かめる。期待値は約束の文と計画（docs/plans/stage4-references.md）から手で書いた。

// refFields は、名前を除く、自然言語で書く項目の全部（計画の「書き方」）。
var refFields = []string{
	"term.definition",
	"feature.definition",
	"feature.formal_waiver",
	"claim.statement",
	"claim.rationale",
	"claim.condition",
	"claim.guesses",
	"claim.model_check_waiver",
	"evidence.detail",
}

// refPromises は、refFields のどの項目も 1 度ずつ書いた約束リストを作る。項目の値は vals（なければ参照のない文）。
// names は、用語と機能の名前（空なら参照のない名前）。
// 項目ごとに、値の前に「［<項目>］」を付け、誤りの行を探すのに使う。
func refPromises(vals map[string]string, termName, featureName string) string {
	v := func(field string) string {
		val, ok := vals[field]
		if !ok {
			val = "参照のない文"
		}
		return fmt.Sprintf("%q", "［"+field+"］"+val)
	}
	if termName == "" {
		termName = "タスク"
	}
	if featureName == "" {
		featureName = "タスクを足す"
	}
	return fmt.Sprintf(`[[term]]
id = "t"
name = %q
definition = %s

[[feature]]
id = "f-formal"
name = "形で確かめる機能"
definition = "形で確かめる"
core = true

[[feature]]
id = "f-waived"
name = %q
definition = %s
core = true
formal = false
formal_waiver = %s

[[claim]]
id = "c"
subjects = ["t", "f-formal"]
statement = %s
rationale = %s
condition = %s
layers = ["model"]
guesses = ["参照のない推測", %s]
model_check_waiver = %s

[[claim.evidence]]
stage = "implementation"
type = "unit-test"
layers = ["model"]
detail = %s
`, termName, v("term.definition"), featureName, v("feature.definition"), v("feature.formal_waiver"),
		v("claim.statement"), v("claim.rationale"), v("claim.condition"), v("claim.guesses"), v("claim.model_check_waiver"), v("evidence.detail"))
}

// fieldLine は、約束リストの中で、項目 field を書いた行の番号（1 から）を返す。
func fieldLine(t *testing.T, promises, field string) int {
	t.Helper()
	for i, l := range strings.Split(promises, "\n") {
		if strings.Contains(l, "［"+field+"］") {
			return i + 1
		}
	}
	t.Fatalf("約束リストに %s の行がない", field)
	return 0
}

const refShapeConfig = `version = %q

[[layer]]
id = "model"
name = "モデル"
description = "判定と状態の変化"
`

// refCheck は、約束リスト promises のリポジトリを作ってコミットし、promise-list-shape の審査を動かす。
func refCheck(t *testing.T, bin, version, promises string) (int, string) {
	t.Helper()
	root := t.TempDir()
	g := newRVGit(t, root)
	g.run("init", "-q", "-b", "main")
	writeFile(t, root, ".vanda/config.toml", fmt.Sprintf(refShapeConfig, version))
	writeFile(t, root, ".vanda/checks/shape.toml", "[[check]]\nname = \"promise-list-shape\"\nscope = \"all\"\n")
	writeFile(t, root, ".vanda/promises.toml", promises)
	g.commitAll("審査の前にコミットする", "#1")
	return runCheck(t, bin, root, ".vanda/checks/shape.toml")
}

func refSetup(t *testing.T) (string, string) {
	t.Helper()
	bin := buildVanda(t)
	version, err := runningVersion(bin)
	if err != nil {
		t.Fatal(err)
	}
	return bin, version
}

// allFields は、どの項目にも同じ値 val を書く。
func allFields(val string) map[string]string {
	m := map[string]string{}
	for _, f := range refFields {
		m[f] = val
	}
	return m
}

// wantErrorsAt は、stderr に、項目ごとにその行を指す誤りがあることを確かめる。ids が空でなければ、その項目の誤りに ID も書いてあることを確かめる。
func wantErrorsAt(t *testing.T, promises, out string, ids map[string]string) {
	t.Helper()
	for _, f := range refFields {
		line := fieldLine(t, promises, f)
		pat := fmt.Sprintf(`(?m)promises\.toml:%d: `, line)
		if id := ids[f]; id != "" {
			pat += ".*" + regexp.QuoteMeta(id)
		}
		if !regexp.MustCompile(pat).MatchString(out) {
			t.Errorf("%s（%d 行目）を指す誤りがない（%s）\n%s", f, line, pat, out)
		}
	}
	if strings.Contains(out, "panic: ") {
		t.Errorf("panic で止まった\n%s", out)
	}
}

// 前提: 参照のない約束リストは、形の審査を通る（通らなければ、ほかのテストの誤りが参照のせいと言えない）
func refBaseline(t *testing.T, bin, version string) {
	t.Helper()
	if code, out := refCheck(t, bin, version, refPromises(nil, "", "")); code != 0 {
		t.Fatalf("参照のない約束リストが形の審査を通らない（%d）\n%s", code, out)
	}
}

// @vanda:evidence reference-form
func Test自然言語で書く項目のどれでも参照を読み用語と機能と約束のどれのIDも指せる(t *testing.T) {
	t.Parallel()
	bin, version := refSetup(t)
	refBaseline(t, bin, version)

	// 用語・機能・約束のそれぞれを指す参照は、どの項目でも通る
	ok := refPromises(allFields("{@id:t} と {@id:f-waived} と {@id:c} を指す"), "", "")
	if code, out := refCheck(t, bin, version, ok); code != 0 {
		t.Errorf("今ある用語・機能・約束を指す参照で、形の審査が通らない（%d）\n%s", code, out)
	}

	// ない ID を指すと、どの項目でも誤りになる（参照として読んでいる）。項目ごとに別の ID にして、どの誤りかを見分ける
	vals, ids := map[string]string{}, map[string]string{}
	for i, f := range refFields {
		id := fmt.Sprintf("nope-%d", i+1)
		vals[f] = "前の文{@id:" + id + "}後の文"
		ids[f] = id
	}
	bad := refPromises(vals, "", "")
	code, out := refCheck(t, bin, version, bad)
	if code != 1 {
		t.Errorf("ない ID を指す参照で、終了コードが %d\n%s", code, out)
	}
	if !strings.Contains(out, "promise-list-shape") {
		t.Errorf("promise-list-shape の誤りとして出ていない\n%s", out)
	}
	wantErrorsAt(t, bad, out, ids)
}

// @vanda:evidence reference-form
func Test用語と機能の名前の中の参照の形の文字は参照として読まない(t *testing.T) {
	t.Parallel()
	bin, version := refSetup(t)
	refBaseline(t, bin, version)
	// 名前の中にない ID を指す形の文字があっても、参照として読まないので誤りにならない
	for _, c := range []struct{ what, term, feature string }{
		{"用語の名前", "タスク{@id:nope-term}", ""},
		{"機能の名前", "", "タスクを足す{@id:nope-feature}"},
		{"形でない文字の名前", "タスク{@id:nope", "{@term:t}を足す"},
	} {
		if code, out := refCheck(t, bin, version, refPromises(nil, c.term, c.feature)); code != 0 {
			t.Errorf("%sに参照の形の文字を書くと、形の審査が通らない（%d）\n%s", c.what, code, out)
		}
	}
}

// @vanda:evidence reference-literal
func Test二つ重ねたアットは文字として読み参照にも誤りにもならない(t *testing.T) {
	t.Parallel()
	bin, version := refSetup(t)
	refBaseline(t, bin, version)
	// {@@ は文字の {@。続く文字は参照の形でも、ない ID でも、閉じていなくても読まない
	for _, val := range []string{
		"{@@",
		"{@@@",
		"{@@id:nope}",
		"{@@id:nope",
		"{@@term:t}",
		"{@@}",
		"{@@@id:nope}",
		"{{@@id:nope}",
		"{@@id:nope} と {@id:t}",
		"{@@{@id:t}",
	} {
		if code, out := refCheck(t, bin, version, refPromises(allFields(val), "", "")); code != 0 {
			t.Errorf("どの項目にも %q を書くと、形の審査が通らない（%d）\n%s", val, code, out)
		}
	}
}

// @vanda:evidence reference-literal
func Test二つ重ねたアットの直後の参照は参照として読む(t *testing.T) {
	t.Parallel()
	bin, version := refSetup(t)
	// {@@ が文字の {@ になった後の {@ は、ふつうの参照の始まり。ない ID なら誤り
	bad := refPromises(allFields("{@@{@id:nope-after}"), "", "")
	code, out := refCheck(t, bin, version, bad)
	if code != 1 {
		t.Errorf("{@@ の直後のない ID を指す参照で、終了コードが %d\n%s", code, out)
	}
	wantErrorsAt(t, bad, out, allFields("nope-after"))
}

// @vanda:evidence reference-broken
func Test壊れた参照は壊れ方のどれでも自然言語で書く項目のどれでもその行を指す約束リストの形の誤りになる(t *testing.T) {
	t.Parallel()
	bin, version := refSetup(t)
	refBaseline(t, bin, version)
	cases := []struct {
		what, val, id string
	}{
		// 形でない（} がない）
		{"閉じの } がない", "前の文{@id:t 後の文", ""},
		// 形でない（: がない）
		{": がない", "前の文{@idt}後の文", ""},
		{"空の参照", "前の文{@}後の文", ""},
		// 種類が id でない
		{"種類が term", "前の文{@term:t}後の文", ""},
		{"種類が大文字の ID", "前の文{@ID:t}後の文", ""},
		{"種類が空", "前の文{@:t}後の文", ""},
		// 指す ID の用語・機能・約束がない
		{"ない ID", "前の文{@id:missing-id}後の文", "missing-id"},
		{"空の ID", "前の文{@id:}後の文", ""},
	}
	for _, c := range cases {
		t.Run(c.what, func(t *testing.T) {
			t.Parallel()
			bad := refPromises(allFields(c.val), "", "")
			code, out := refCheck(t, bin, version, bad)
			if code != 1 {
				t.Errorf("%s の参照で、終了コードが %d\n%s", c.what, code, out)
			}
			if !regexp.MustCompile(`(?m)^審査 promise-list-shape（all）: 失敗`).MatchString(out) {
				t.Errorf("promise-list-shape の失敗として出ていない\n%s", out)
			}
			ids := map[string]string{}
			if c.id != "" {
				ids = allFields(c.id)
			}
			wantErrorsAt(t, bad, out, ids)
		})
	}
}

// 指紋の元の JSON は、約束リストの書いたままの文（参照を置き換える前）から手で書いた（docs/formats.md の「指紋」）。
const (
	fpClaimJSON = `{"evidence":[{"layers":["model"],"stage":"implementation","type":"unit-test"}],"id":"c","layers":["model"],"statement":"{@id:t}は{@id:f}で足し、{@@id:x}は文字","subjects":["t"]}`
	fpTermV1    = `{"definition":"やること","id":"t","name":"タスク"}`
	fpTermV2    = `{"definition":"やるべきこと","id":"t","name":"仕事"}`
	fpFeatureV1 = `{"definition":"タスクを足す","id":"f","name":"タスクの登録"}`
	fpFeatureV2 = `{"definition":"仕事を足す","id":"f","name":"仕事の登録"}`
)

func fpPromises(termName, termDef, featureName, featureDef string) string {
	return fmt.Sprintf(`[[term]]
id = "t"
name = %q
definition = %q

[[feature]]
id = "f"
name = %q
definition = %q

[[claim]]
id = "c"
subjects = ["t"]
statement = "{@id:t}は{@id:f}で足し、{@@id:x}は文字"
layers = ["model"]

[[claim.evidence]]
stage = "implementation"
type = "unit-test"
layers = ["model"]
`, termName, termDef, featureName, featureDef)
}

// @vanda:evidence reference-fingerprint-raw
func Test参照先の用語と機能の名前と定義を変えても参照元の約束の合意は外れない(t *testing.T) {
	t.Parallel()
	bin, version := refSetup(t)
	root := t.TempDir()
	g := newRVGit(t, root)
	g.run("init", "-q", "-b", "main")
	writeFile(t, root, ".vanda/config.toml", fmt.Sprintf(refShapeConfig, version))
	writeFile(t, root, ".vanda/checks/agreement.toml", "[[check]]\nname = \"promise-list-shape\"\nscope = \"all\"\n\n[[check]]\nname = \"agreement\"\nscope = \"all\"\n")
	who := func(kind, id, json string) string {
		return agreementTOML(kind, id, sha256Hex([]byte(json)), "", "#1", "テスト担当", "tester@example.com")
	}
	// 約束 c には、書いたままの文の指紋で 1 度だけ合意する
	writeFile(t, root, ".vanda/promises.toml", fpPromises("タスク", "やること", "タスクの登録", "タスクを足す"))
	writeFile(t, root, ".vanda/agreements.toml", who("claim", "c", fpClaimJSON)+who("term", "t", fpTermV1)+who("feature", "f", fpFeatureV1))
	g.commitAll("約束を置いて合意する", "#1")
	if code, out := runCheck(t, bin, root, ".vanda/checks/agreement.toml"); code != 0 {
		t.Fatalf("書いたままの文の指紋に合意した約束が、合意済にならない（%d）\n%s", code, out)
	}

	// 参照先の用語と機能の名前と定義を変え、その今の中身にだけ合意し直す。約束 c には合意し直さない
	writeFile(t, root, ".vanda/promises.toml", fpPromises("仕事", "やるべきこと", "仕事の登録", "仕事を足す"))
	writeFile(t, root, ".vanda/agreements.toml", who("claim", "c", fpClaimJSON)+who("term", "t", fpTermV1)+who("feature", "f", fpFeatureV1)+
		who("term", "t", fpTermV2)+who("feature", "f", fpFeatureV2))
	g.commitAll("用語と機能の名前と定義を変える", "#1")
	if code, out := runCheck(t, bin, root, ".vanda/checks/agreement.toml"); code != 0 {
		t.Errorf("参照先の名前と定義を変えると、参照元の約束の合意が外れた（%d）\n%s", code, out)
	}
}
