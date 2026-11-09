package main_test

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// RV-05 変更前と変更後の差分（GET /api/review の entries[].diff。docs/formats.md）。
// 期待値は手で作った例から用意する。文字の単位の差分は、切れ目を決め打ちせず、仕様から言える範囲
// （つなげると変更前・変更後の文になる、変わった語が delete・insert に、変わっていない語が equal に入る）で確かめる。

type diffPart struct {
	Op   string `json:"op"`
	Text string `json:"text"`
}

type diffLine struct {
	Op        string     `json:"op"`
	Before    *int       `json:"before"`
	After     *int       `json:"after"`
	Text      string     `json:"text"`
	Parts     []diffPart `json:"parts"`
	NoNewline bool       `json:"noNewline"`
}

type entryDiff struct {
	Lines  []diffLine            `json:"lines"`
	Fields map[string][]diffPart `json:"fields"`
}

// 手で作ったファイルの例。ops は、行の単位の差分の op の並び（手で決めたもの。行の LCS が 1 通りに決まる例にする）。
var diffFiles = []struct {
	path, before, after string
	ops                 []string
}{
	{"e2e/a.txtar", "a\nb\nc\n", "a\nB\nc\nd\n", []string{"equal", "delete", "insert", "equal", "insert"}},
	{"e2e/m.txtar", "p\nq1\nq2\nr\n", "p\nQ1\nQ2\nQ3\nr\n", []string{"equal", "delete", "delete", "insert", "insert", "insert", "equal"}},
	{"e2e/w.txtar", "exec vanda add 買い物を登録した\nstdout ok\n", "exec vanda add 買い物を足した\nstdout ok\n", []string{"delete", "insert", "equal"}},
	{"e2e/only-del.txtar", "a\nb\nc\n", "a\nc\n", []string{"equal", "delete", "equal"}},
	// 最後の行の改行の有無だけが違うときは、delete と insert の 2 行
	{"e2e/nl.txtar", "x\ny", "x\ny\n", []string{"equal", "delete", "insert"}},
}

const diffConfig = `version = %q

[[layer]]
id = "model"
name = "モデル"
description = "判定と状態の変化"

[[annotation]]
paths = ["e2e/*.txtar"]
adapter = "txtar"
types = ["scenario-test"]
`

const diffPromisesBase = `[[term]]
id = "t"
name = "用語"
definition = "前の定義"

[[feature]]
id = "f"
name = "機能"
definition = "同じ定義"
core = true
formal = false
formal_waiver = "まだ形式手法を使わない"

[[claim]]
id = "c"
subjects = ["t"]
statement = "タスクの ID は重ならない"
rationale = "消す相手を決めるため"
layers = ["model"]
guesses = ["前の推測"]

[[claim.evidence]]
stage = "implementation"
type = "unit-test"
layers = ["model"]
detail = "前の中身"
`

const diffPromisesStory = `[[term]]
id = "t"
name = "用語"
definition = "後の定義"

[[feature]]
id = "f"
name = "機能"
definition = "同じ定義"
core = true
formal = false
formal_waiver = "当面は形式手法を使わない"

[[claim]]
id = "c"
subjects = ["t"]
statement = "タスクの ID は絶対に重ならない"
rationale = "消す相手を決めるため"
condition = "タスクがある"
layers = ["model"]
guesses = ["後の推測"]

[[claim.evidence]]
stage = "implementation"
type = "unit-test"
layers = ["model"]
detail = "後の中身"
`

func setupDiff(t *testing.T) reviewResponse {
	t.Helper()
	bin := buildVanda(t)
	version, err := runningVersion(bin)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	g := newRVGit(t, root)
	g.run("init", "-q", "-b", "main")
	writeFile(t, root, ".vanda/config.toml", fmt.Sprintf(diffConfig, version))
	writeFile(t, root, ".vanda/promises.toml", diffPromisesBase)
	for _, f := range diffFiles {
		writeFile(t, root, f.path, f.before)
	}
	writeFile(t, root, "e2e/r.txtar", "名前だけ変える\n")
	writeFile(t, root, "e2e/del.txtar", "消す\n")
	g.commitAll("はじめ", "#1")
	g.run("switch", "-q", "-c", "story-24")
	writeFile(t, root, ".vanda/promises.toml", diffPromisesStory)
	for _, f := range diffFiles {
		writeFile(t, root, f.path, f.after)
	}
	g.run("mv", "e2e/r.txtar", "e2e/r2.txtar")
	g.run("rm", "-q", "e2e/del.txtar")
	writeFile(t, root, "e2e/new.txtar", "足す\n")
	g.commitAll("変える", "#24")

	s := startServe(t, bin, root, g.env)
	var got reviewResponse
	if code, msg := s.get("/api/review?view=branch&base=main", &got); code != 200 {
		t.Fatalf("一覧を読めない（%d）: %s", code, msg)
	}
	return got
}

func entryByKey(got reviewResponse, key string) *reviewEntry {
	for i := range got.Entries {
		if got.Entries[i].key() == key {
			return &got.Entries[i]
		}
	}
	return nil
}

func decodeDiff(t *testing.T, e *reviewEntry) *entryDiff {
	t.Helper()
	if isNull(e.Diff) {
		return nil
	}
	var d entryDiff
	if err := json.Unmarshal(e.Diff, &d); err != nil {
		t.Fatalf("%s の diff を読めない: %v（%s）", e.key(), err, e.Diff)
	}
	return &d
}

func joinParts(ps []diffPart, keep ...string) string {
	var b strings.Builder
	for _, p := range ps {
		for _, k := range keep {
			if p.Op == k {
				b.WriteString(p.Text)
			}
		}
	}
	return b.String()
}

// rebuild は、行の差分から、片方の側（equal と op の行）のファイルの中身を組み立てる。
func rebuild(lines []diffLine, op string) string {
	var b strings.Builder
	for _, l := range lines {
		if l.Op == "equal" || l.Op == op {
			b.WriteString(l.Text)
			if !l.NoNewline {
				b.WriteString("\n")
			}
		}
	}
	return b.String()
}

// rewritten は、行ごとに、書き換えた所（消した行が続くすぐ後に足した行が続く所）の行かを返す。
func rewritten(ops []string) []bool {
	r := make([]bool, len(ops))
	for i := 0; i < len(ops); {
		if ops[i] != "delete" {
			i++
			continue
		}
		j := i
		for j < len(ops) && ops[j] == "delete" {
			j++
		}
		k := j
		for k < len(ops) && ops[k] == "insert" {
			k++
		}
		if k > j {
			for x := i; x < k; x++ {
				r[x] = true
			}
		}
		i = k
	}
	return r
}

// RV-05
// @vanda:evidence review-file-diff-lines
func Testファイルの差分は変更前と変更後の行をファイルの順に1度ずつ並べ書き換えた行だけを文字の単位で比べる(t *testing.T) {
	t.Parallel()
	got := setupDiff(t)
	for _, f := range diffFiles {
		t.Run(f.path, func(t *testing.T) {
			t.Parallel()
			e := entryByKey(got, f.path)
			if e == nil {
				t.Fatalf("一覧にない")
			}
			d := decodeDiff(t, e)
			if d == nil {
				t.Fatalf("変更したファイルに diff がない")
			}
			var ops []string
			for _, l := range d.Lines {
				ops = append(ops, l.Op)
			}
			if !reflect.DeepEqual(ops, f.ops) {
				t.Errorf("行の op の並び: %v（思ったもの %v）", ops, f.ops)
			}
			if b := rebuild(d.Lines, "delete"); b != f.before {
				t.Errorf("equal と delete の行をつなげても変更前にならない: %q", b)
			}
			if a := rebuild(d.Lines, "insert"); a != f.after {
				t.Errorf("equal と insert の行をつなげても変更後にならない: %q", a)
			}
			nb, na := 0, 0
			rw := rewritten(ops)
			for i, l := range d.Lines {
				if l.Op != "insert" {
					nb++
					if l.Before == nil || *l.Before != nb {
						t.Errorf("%d 行目（%s %q）の before が %v（%d のはず）", i+1, l.Op, l.Text, l.Before, nb)
					}
				} else if l.Before != nil {
					t.Errorf("%d 行目（insert）の before が null でない", i+1)
				}
				if l.Op != "delete" {
					na++
					if l.After == nil || *l.After != na {
						t.Errorf("%d 行目（%s %q）の after が %v（%d のはず）", i+1, l.Op, l.Text, l.After, na)
					}
				} else if l.After != nil {
					t.Errorf("%d 行目（delete）の after が null でない", i+1)
				}
				if rw[i] != (l.Parts != nil) {
					t.Errorf("%d 行目（%s %q）: parts は書き換えた行だけに付く（書き換え %v、parts %v）", i+1, l.Op, l.Text, rw[i], l.Parts)
				}
				if l.Parts != nil {
					if joinParts(l.Parts, "equal", "delete", "insert") != l.Text {
						t.Errorf("%d 行目の parts をつなげても text にならない: %+v", i+1, l.Parts)
					}
					for _, p := range l.Parts {
						if p.Op != "equal" && p.Op != l.Op {
							t.Errorf("%d 行目（%s）の parts に %s がある", i+1, l.Op, p.Op)
						}
					}
				}
			}
		})
	}

	// 書き換えた行の中で、変わった語は delete・insert に、変わっていない語は equal に入る
	w := decodeDiff(t, entryByKey(got, "e2e/w.txtar"))
	if w != nil && len(w.Lines) == 3 {
		del, ins := w.Lines[0].Parts, w.Lines[1].Parts
		if !strings.Contains(joinParts(del, "delete"), "登録") || !strings.Contains(joinParts(ins, "insert"), "足") ||
			!strings.Contains(joinParts(del, "equal"), "exec vanda add 買い物を") || !strings.Contains(joinParts(ins, "equal"), "exec vanda add 買い物を") {
			t.Errorf("書き換えた行の文字の単位の差分: 消した行 %+v、足した行 %+v", del, ins)
		}
	}
	// 改行のない最後の行には noNewline が付き、ほかの行には付かない
	if nl := decodeDiff(t, entryByKey(got, "e2e/nl.txtar")); nl != nil && len(nl.Lines) == 3 {
		if nl.Lines[0].NoNewline || !nl.Lines[1].NoNewline || nl.Lines[2].NoNewline {
			t.Errorf("noNewline: %+v", nl.Lines)
		}
	}
}

// @vanda:evidence review-renamed-file
// @vanda:evidence review-before-after
func Test差分は変更前と変更後の両方があるものだけに付き名前だけを変えたファイルは古い場所の削除と新しい場所の追加になり差分を付けない(t *testing.T) {
	t.Parallel()
	got := setupDiff(t)
	for _, key := range []string{"e2e/new.txtar", "e2e/del.txtar"} {
		e := entryByKey(got, key)
		if e == nil || !isNull(e.Diff) {
			t.Errorf("%s（追加か削除）の diff が null でない: %+v", key, e)
		}
	}
	for key, change := range map[string]string{"e2e/r.txtar": "deleted", "e2e/r2.txtar": "added"} {
		e := entryByKey(got, key)
		if e == nil || e.Change != change {
			t.Errorf("名前を変えたファイルの %s が %s として一覧にない: %+v", key, change, e)
			continue
		}
		if !isNull(e.Diff) {
			t.Errorf("名前を変えたファイルの %s（%s）の diff が null でない: %s", key, change, e.Diff)
		}
	}
}

// RV-05
// @vanda:evidence review-before-after
func Test約束と用語と機能の差分は両方に書いた文字の項目だけを文字の単位で比べる(t *testing.T) {
	t.Parallel()
	got := setupDiff(t)
	cases := []struct {
		key    string
		fields map[string][2]string // 項目 → [変更前, 変更後]
	}{
		// condition は変更後にだけあるので入れない。evidence の detail と guesses は差分を付けない
		{"c", map[string][2]string{"statement": {"タスクの ID は重ならない", "タスクの ID は絶対に重ならない"}, "rationale": {"消す相手を決めるため", "消す相手を決めるため"}}},
		{"t", map[string][2]string{"name": {"用語", "用語"}, "definition": {"前の定義", "後の定義"}}},
		{"f", map[string][2]string{"name": {"機能", "機能"}, "definition": {"同じ定義", "同じ定義"}, "formalWaiver": {"まだ形式手法を使わない", "当面は形式手法を使わない"}}},
	}
	for _, c := range cases {
		e := entryByKey(got, c.key)
		if e == nil {
			t.Errorf("%s が一覧にない", c.key)
			continue
		}
		d := decodeDiff(t, e)
		if d == nil {
			t.Errorf("%s に diff がない", c.key)
			continue
		}
		var keys, want []string
		for k := range d.Fields {
			keys = append(keys, k)
		}
		for k := range c.fields {
			want = append(want, k)
		}
		sort.Strings(keys)
		sort.Strings(want)
		if !reflect.DeepEqual(keys, want) {
			t.Errorf("%s の差分の項目: %v（思ったもの %v）", c.key, keys, want)
		}
		for k, ba := range c.fields {
			ps := d.Fields[k]
			if joinParts(ps, "equal", "delete") != ba[0] || joinParts(ps, "equal", "insert") != ba[1] {
				t.Errorf("%s の %s の差分をつなげても変更前・変更後にならない: %+v", c.key, k, ps)
			}
			if ba[0] == ba[1] && (len(ps) != 1 || ps[0].Op != "equal") {
				t.Errorf("%s の変わっていない %s は equal の 1 つのはず: %+v", c.key, k, ps)
			}
		}
	}
	c := decodeDiff(t, entryByKey(got, "c"))
	if c != nil {
		st := c.Fields["statement"]
		if !strings.Contains(joinParts(st, "insert"), "絶対に") || strings.Contains(joinParts(st, "delete", "insert"), "タスクの ID") ||
			!strings.Contains(joinParts(st, "equal"), "重ならない") {
			t.Errorf("statement の文字の単位の差分: %+v", st)
		}
	}
	f := decodeDiff(t, entryByKey(got, "f"))
	if f != nil {
		fw := f.Fields["formalWaiver"]
		if !strings.Contains(joinParts(fw, "equal"), "形式手法を使わない") || !strings.Contains(joinParts(fw, "delete"), "まだ") || !strings.Contains(joinParts(fw, "insert"), "当面") {
			t.Errorf("formalWaiver の文字の単位の差分: %+v", fw)
		}
	}
}

// RV-05: 約束のエビデンスの予定の、エビデンスの単位の差分（diff.evidence）。
// 手で作った例: ステージとタイプの組ごとに、変わらない・詳細が変わった・レイヤーが変わった・消した・足した、と、
// 同じ組（test・random-check）が変更前に 2 つ、変更後に 3 つあるもの（現れた順に対応し、3 つ目は足した）。
type evidenceMark struct {
	Op     string     `json:"op"`
	Layers []diffPart `json:"layers"`
	Detail []diffPart `json:"detail"`
}

type evidenceDiff struct {
	Evidence *struct {
		Before []evidenceMark `json:"before"`
		After  []evidenceMark `json:"after"`
	} `json:"evidence"`
}

const evidenceDiffTerm = "[[term]]\nid = \"t\"\nname = \"用語\"\ndefinition = \"定義\"\n\n[[claim]]\nid = \"ev\"\nsubjects = [\"t\"]\nstatement = \"エビデンスを変える\"\nlayers = [\"model\", \"db\"]\n"

func evidenceTOML(stage, typ string, layers []string, detail string) string {
	s := fmt.Sprintf("\n[[claim.evidence]]\nstage = %q\ntype = %q\nlayers = [\"%s\"]\n", stage, typ, strings.Join(layers, "\", \""))
	if detail != "" {
		s += fmt.Sprintf("detail = %q\n", detail)
	}
	return s
}

// @vanda:evidence review-evidence-diff-pairing
func Test約束のエビデンスの予定の差分はステージとタイプで対応づけ足した消した変わった変わらないを示し同じ組は現れた順に対応させる(t *testing.T) {
	t.Parallel()
	model, both := []string{"model"}, []string{"model", "db"}
	before := evidenceDiffTerm +
		evidenceTOML("implementation", "scenario-test", model, "同じ") +
		evidenceTOML("plan", "model-check", model, "前の詳細") +
		evidenceTOML("implementation", "unit-test", model, "") +
		evidenceTOML("release", "human-review", model, "") +
		evidenceTOML("implementation", "random-check", model, "一つ目") +
		evidenceTOML("implementation", "random-check", model, "二つ目")
	after := evidenceDiffTerm +
		evidenceTOML("implementation", "scenario-test", model, "同じ") +
		evidenceTOML("plan", "model-check", model, "後の詳細") +
		evidenceTOML("implementation", "unit-test", both, "") +
		evidenceTOML("operation", "sensor", model, "") +
		evidenceTOML("implementation", "random-check", model, "一つ目") +
		evidenceTOML("implementation", "random-check", model, "二つ目を変える") +
		evidenceTOML("implementation", "random-check", model, "三つ目")
	// [変更前の値, 変更後の値]。modified のときだけ確かめる
	type side struct{ layers, detail [2]string }
	wantBefore := []string{"equal", "modified", "modified", "removed", "equal", "modified"}
	wantAfter := []string{"equal", "modified", "modified", "added", "equal", "modified", "added"}
	modified := map[int]side{ // 変更前の位置 → 変更前と変更後の値
		1: {[2]string{"model", "model"}, [2]string{"前の詳細", "後の詳細"}},
		2: {[2]string{"model", "model・db"}, [2]string{"", ""}},
		5: {[2]string{"model", "model"}, [2]string{"二つ目", "二つ目を変える"}},
	}
	afterPos := map[int]int{1: 1, 2: 2, 5: 5} // 変更前の位置 → 対応する変更後の位置

	bin := buildVanda(t)
	version, err := runningVersion(bin)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	g := newRVGit(t, root)
	g.run("init", "-q", "-b", "main")
	writeFile(t, root, ".vanda/config.toml", fmt.Sprintf(diffConfig, version)+"\n[[layer]]\nid = \"db\"\nname = \"DB\"\ndescription = \"保存\"\n")
	writeFile(t, root, ".vanda/promises.toml", before)
	g.commitAll("はじめ", "#1")
	g.run("switch", "-q", "-c", "story-24")
	writeFile(t, root, ".vanda/promises.toml", after)
	g.commitAll("エビデンスを変える", "#24")
	s := startServe(t, bin, root, g.env)
	var got reviewResponse
	if code, msg := s.get("/api/review?view=branch&base=main", &got); code != 200 {
		t.Fatalf("一覧を読めない（%d）: %s", code, msg)
	}
	e := entryByKey(got, "ev")
	if e == nil || isNull(e.Diff) {
		t.Fatalf("約束 ev の diff がない: %+v", e)
	}
	var d evidenceDiff
	if err := json.Unmarshal(e.Diff, &d); err != nil {
		t.Fatal(err)
	}
	if d.Evidence == nil {
		t.Fatalf("diff.evidence がない: %s", e.Diff)
	}
	ops := func(ms []evidenceMark) []string {
		var out []string
		for _, m := range ms {
			out = append(out, m.Op)
		}
		return out
	}
	if !reflect.DeepEqual(ops(d.Evidence.Before), wantBefore) {
		t.Errorf("変更前のマーク: %v（思ったもの %v）", ops(d.Evidence.Before), wantBefore)
	}
	if !reflect.DeepEqual(ops(d.Evidence.After), wantAfter) {
		t.Errorf("変更後のマーク: %v（思ったもの %v）", ops(d.Evidence.After), wantAfter)
	}
	check := func(where string, m evidenceMark, sd side) {
		for _, f := range []struct {
			name  string
			parts []diffPart
			want  [2]string
		}{{"layers", m.Layers, sd.layers}, {"detail", m.Detail, sd.detail}} {
			if joinParts(f.parts, "equal", "delete") != f.want[0] || joinParts(f.parts, "equal", "insert") != f.want[1] {
				t.Errorf("%s の %s の差分をつなげても変更前 %q・変更後 %q にならない: %+v", where, f.name, f.want[0], f.want[1], f.parts)
			}
		}
	}
	for i, m := range d.Evidence.Before {
		if sd, ok := modified[i]; ok && m.Op == "modified" {
			check(fmt.Sprintf("変更前の %d 番目", i+1), m, sd)
			if j := afterPos[i]; j < len(d.Evidence.After) {
				check(fmt.Sprintf("変更後の %d 番目", j+1), d.Evidence.After[j], sd)
			}
		} else if m.Layers != nil || m.Detail != nil {
			t.Errorf("変更前の %d 番目（%s）の layers・detail が null でない: %+v", i+1, m.Op, m)
		}
	}
	for j, m := range d.Evidence.After {
		if m.Op != "modified" && (m.Layers != nil || m.Detail != nil) {
			t.Errorf("変更後の %d 番目（%s）の layers・detail が null でない: %+v", j+1, m.Op, m)
		}
	}
}
