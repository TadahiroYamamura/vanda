package main_test

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// 参照の API での見せ方（約束 reference-shown・reference-diff・reference-fingerprint-raw の api）を、vanda serve を外から動かして確かめる。
// API の形は計画（docs/plans/stage4-references.md の「API の形」）。期待値は、下の約束リストから手で書いた。

const refAPIConfig = `version = %q

[[layer]]
id = "model"
name = "モデル"
description = "判定と状態の変化"

[gui]
base = "main"
`

// main の約束リスト。c-ref は、名前を変える用語・機能と、ストーリーで消す用語・機能と、約束を指す
const refAPIBase = `[[term]]
id = "t-keep"
name = "タスク"
definition = "やること"

[[term]]
id = "t-rename"
name = "古い用語"
definition = "前の定義"

[[term]]
id = "t-gone"
name = "消える用語"
definition = "消える定義"

[[feature]]
id = "f-rename"
name = "古い機能"
definition = "前の機能の定義"

[[feature]]
id = "f-gone"
name = "消える機能"
definition = "消える機能の定義"

[[claim]]
id = "c-other"
subjects = ["t-keep"]
statement = "ほかの約束は{@id:t-keep}を数える"
layers = ["model"]

[[claim.evidence]]
stage = "implementation"
type = "unit-test"
layers = ["model"]

[[claim]]
id = "c-ref"
subjects = ["t-keep"]
statement = "{@id:t-rename}と{@id:f-rename}は{@id:t-gone}と{@id:f-gone}と{@id:c-other}を使う"
rationale = "{@@id:x}は文字"
layers = ["model"]

[[claim.evidence]]
stage = "implementation"
type = "unit-test"
layers = ["model"]
detail = "{@id:f-rename}で試す"

[[claim]]
id = "c-same"
subjects = ["t-keep"]
statement = "{@id:t-keep}と新しい用語を数える"
layers = ["model"]

[[claim.evidence]]
stage = "implementation"
type = "unit-test"
layers = ["model"]
`

// story-24 の約束リスト。t-rename・f-rename の名前と定義を変え、t-gone・f-gone を消し、c-ref の文から消したものへの参照を外す
const refAPIStory = `[[term]]
id = "t-keep"
name = "タスク"
definition = "やること"

[[term]]
id = "t-rename"
name = "新しい用語"
definition = "後の定義"

[[feature]]
id = "f-rename"
name = "新しい機能"
definition = "{@id:t-keep}を足す"

[[claim]]
id = "c-other"
subjects = ["t-keep"]
statement = "ほかの約束は{@id:t-keep}を数える"
layers = ["model"]

[[claim.evidence]]
stage = "implementation"
type = "unit-test"
layers = ["model"]

[[claim]]
id = "c-ref"
subjects = ["t-keep"]
statement = "{@id:t-rename}と{@id:f-rename}は{@id:c-other}を必ず使う"
rationale = "{@@id:x}は文字"
condition = "{@id:t-keep}がある"
layers = ["model"]
guesses = ["{@id:t-rename}の推測"]
model_check_waiver = "{@id:f-rename}は形で確かめない"

[[claim.evidence]]
stage = "implementation"
type = "unit-test"
layers = ["model"]
detail = "{@id:f-rename}で確かめる"

[[claim]]
id = "c-same"
subjects = ["t-keep"]
statement = "タスクと{@id:t-rename}をいつも数える"
layers = ["model"]

[[claim.evidence]]
stage = "implementation"
type = "unit-test"
layers = ["model"]
`

type refDiffPart struct {
	Op   string  `json:"op"`
	Text string  `json:"text"`
	Ref  *string `json:"ref"`
}

type refClaimContent struct {
	Statement        frags   `json:"statement"`
	Rationale        frags   `json:"rationale"`
	Condition        frags   `json:"condition"`
	Guesses          []frags `json:"guesses"`
	ModelCheckWaiver frags   `json:"modelCheckWaiver"`
	Evidence         []struct {
		Detail frags `json:"detail"`
	} `json:"evidence"`
}

type refItemContent struct {
	Name         string `json:"name"`
	Definition   frags  `json:"definition"`
	FormalWaiver frags  `json:"formalWaiver"`
}

type refReview struct {
	Entries []reviewEntry      `json:"entries"`
	Refs    map[string]refInfo `json:"refs"`
}

type refDiff struct {
	Fields   map[string][]refDiffPart `json:"fields"`
	Evidence *struct {
		Before []struct {
			Op     string        `json:"op"`
			Detail []refDiffPart `json:"detail"`
		} `json:"before"`
		After []struct {
			Op     string        `json:"op"`
			Detail []refDiffPart `json:"detail"`
		} `json:"after"`
	} `json:"evidence"`
}

type refAPIRepo struct {
	root   string
	g      *rvGit
	s      *vandaServer
	review refReview
}

func setupRefAPI(t *testing.T) *refAPIRepo {
	t.Helper()
	bin, version := refSetup(t)
	root := t.TempDir()
	g := newRVGit(t, root)
	g.run("init", "-q", "-b", "main")
	writeFile(t, root, ".vanda/config.toml", fmt.Sprintf(refAPIConfig, version))
	writeFile(t, root, ".vanda/checks/shape.toml", "[[check]]\nname = \"promise-list-shape\"\nscope = \"all\"\n")
	writeFile(t, root, ".vanda/promises.toml", refAPIBase)
	g.commitAll("はじめの約束を置く", "#1")
	g.run("switch", "-q", "-c", "story-24")
	writeFile(t, root, ".vanda/promises.toml", refAPIStory)
	g.commitAll("参照先の名前を変え、消す", "#24")
	// 書いた約束リストが形のチェックを通ること（通らなければ、API の誤りと言えない）
	if code, out := runCheck(t, bin, root, ".vanda/checks/shape.toml"); code != 0 {
		t.Fatalf("テストの約束リストが形のチェックを通らない（%d）\n%s", code, out)
	}
	s := startServe(t, bin, root, g.env)
	r := &refAPIRepo{root: root, g: g, s: s}
	if code, msg := s.get("/api/review?view=branch&base=main", &r.review); code != 200 {
		t.Fatalf("一覧を読めない（%d）: %s", code, msg)
	}
	return r
}

func (r *refAPIRepo) entry(t *testing.T, key string) *reviewEntry {
	t.Helper()
	for i := range r.review.Entries {
		if r.review.Entries[i].key() == key {
			return &r.review.Entries[i]
		}
	}
	t.Fatalf("%s が一覧にない", key)
	return nil
}

func decodeInto(t *testing.T, where string, raw json.RawMessage, out any) {
	t.Helper()
	if err := json.Unmarshal(raw, out); err != nil {
		t.Fatalf("%s を読めない: %v（%s）", where, err, raw)
	}
}

func wantFrags(t *testing.T, where string, got, want frags) {
	t.Helper()
	if got.norm() != want.norm() {
		t.Errorf("%s のセグメント:\n%s\n--- 思ったもの\n%s", where, got.norm(), want.norm())
	}
}

func strp(s string) *string { return &s }

// wantRefs は、refs のキーがちょうど want のキーで、それぞれの中身が want と同じことを確かめる。
// 指す先の pieces の中の参照の指す先は refs に足さないので、余分なキーがあっても誤りにする
func wantRefs(t *testing.T, where string, got map[string]refInfo, want map[string]refInfo) {
	t.Helper()
	for id, w := range want {
		g, ok := got[id]
		if !ok {
			t.Errorf("%s の refs に %s がない: %+v", where, id, got)
			continue
		}
		wantFrags(t, where+" の refs の "+id+" の pieces", g.Pieces, w.Pieces)
		g.Pieces, w.Pieces = nil, nil
		if !reflect.DeepEqual(g, w) {
			t.Errorf("%s の refs の %s: %+v（思ったもの %+v、name %v、core %v、formal %v）", where, id, g, w, str(g.Name), boolStr(g.Core), boolStr(g.Formal))
		}
	}
	for id := range got {
		if _, ok := want[id]; !ok {
			t.Errorf("%s の refs に、レスポンスのセグメントが指していない %s がある", where, id)
		}
	}
}

func boolp(b bool) *bool { return &b }

func boolStr(b *bool) string {
	if b == nil {
		return "（なし）"
	}
	return fmt.Sprint(*b)
}

// 今の約束リスト（story-24）の、参照先の中身。用語と機能は名前と定義、約束は文（中の参照は見せる文字に置き換えた文字列と、セグメントの配列）。
// 機能は core・formal を持ち、どちらも書いていないので偽
var refNow = map[string]refInfo{
	"t-keep":   {Kind: "term", Name: strp("タスク"), Text: "やること", Current: true, Pieces: textOf("やること")},
	"t-rename": {Kind: "term", Name: strp("新しい用語"), Text: "後の定義", Current: true, Pieces: textOf("後の定義")},
	"f-rename": {Kind: "feature", Name: strp("新しい機能"), Text: "タスクを足す", Current: true, Pieces: fragsOf("タスク>t-keep", "を足す"), Core: boolp(false), Formal: boolp(false)},
	"c-other":  {Kind: "claim", Name: nil, Text: "ほかの約束はタスクを数える", Current: true, Pieces: fragsOf("ほかの約束は", "タスク>t-keep", "を数える")},
	// 今ない用語と機能は、比べる元（main）の名前と定義
	"t-gone": {Kind: "term", Name: strp("消える用語"), Text: "消える定義", Current: false, Pieces: textOf("消える定義")},
	"f-gone": {Kind: "feature", Name: strp("消える機能"), Text: "消える機能の定義", Current: false, Pieces: textOf("消える機能の定義"), Core: boolp(false), Formal: boolp(false)},
}

func refsOf(ids ...string) map[string]refInfo {
	out := map[string]refInfo{}
	for _, id := range ids {
		out[id] = refNow[id]
	}
	return out
}

// @vanda:evidence reference-shown
func Test一覧の変更前と変更後は自然言語で書く項目をセグメントに分け参照は用語と機能なら今の名前で約束ならIDで見せrefsに指す先を返す(t *testing.T) {
	t.Parallel()
	r := setupRefAPI(t)

	var after refClaimContent
	decodeInto(t, "c-ref の after", r.entry(t, "c-ref").After, &after)
	wantFrags(t, "c-ref の after の statement", after.Statement,
		fragsOf("新しい用語>t-rename", "と", "新しい機能>f-rename", "は", "c-other>c-other", "を必ず使う"))
	// {@@ は文字の {@ にした、参照でないセグメント
	wantFrags(t, "c-ref の after の rationale", after.Rationale, textOf("{@id:x}は文字"))
	wantFrags(t, "c-ref の after の condition", after.Condition, fragsOf("タスク>t-keep", "がある"))
	if len(after.Guesses) != 1 {
		t.Errorf("c-ref の after の guesses: %+v", after.Guesses)
	} else {
		wantFrags(t, "c-ref の after の guesses[0]", after.Guesses[0], fragsOf("新しい用語>t-rename", "の推測"))
	}
	wantFrags(t, "c-ref の after の modelCheckWaiver", after.ModelCheckWaiver, fragsOf("新しい機能>f-rename", "は形で確かめない"))
	if len(after.Evidence) != 1 {
		t.Errorf("c-ref の after の evidence: %+v", after.Evidence)
	} else {
		wantFrags(t, "c-ref の after の evidence[0].detail", after.Evidence[0].Detail, fragsOf("新しい機能>f-rename", "で確かめる"))
	}

	// 用語と機能の定義もセグメントに分ける。名前はセグメントに分けない（文字列のまま）
	var fr refItemContent
	decodeInto(t, "f-rename の after", r.entry(t, "f-rename").After, &fr)
	if fr.Name != "新しい機能" {
		t.Errorf("f-rename の after の name: %q", fr.Name)
	}
	wantFrags(t, "f-rename の after の definition", fr.Definition, fragsOf("タスク>t-keep", "を足す"))

	wantRefs(t, "一覧", r.review.Refs, refsOf("t-keep", "t-rename", "f-rename", "c-other", "t-gone", "f-gone"))
}

// @vanda:evidence reference-shown
func Test約束の詳細とその時点の中身は自然言語で書く項目をセグメントに分けrefsに指す先を返す(t *testing.T) {
	t.Parallel()
	r := setupRefAPI(t)

	var detail struct {
		refClaimContent
		Refs map[string]refInfo `json:"refs"`
	}
	if code, msg := r.s.get("/api/claims/c-ref", &detail); code != 200 {
		t.Fatalf("約束の詳細を読めない（%d）: %s", code, msg)
	}
	wantFrags(t, "詳細の statement", detail.Statement,
		fragsOf("新しい用語>t-rename", "と", "新しい機能>f-rename", "は", "c-other>c-other", "を必ず使う"))
	wantFrags(t, "詳細の rationale", detail.Rationale, textOf("{@id:x}は文字"))
	wantFrags(t, "詳細の condition", detail.Condition, fragsOf("タスク>t-keep", "がある"))
	if len(detail.Guesses) != 1 {
		t.Errorf("詳細の guesses: %+v", detail.Guesses)
	} else {
		wantFrags(t, "詳細の guesses[0]", detail.Guesses[0], fragsOf("新しい用語>t-rename", "の推測"))
	}
	if len(detail.Evidence) != 1 {
		t.Errorf("詳細の evidence: %+v", detail.Evidence)
	} else {
		wantFrags(t, "詳細の evidence[0].detail", detail.Evidence[0].Detail, fragsOf("新しい機能>f-rename", "で確かめる"))
	}
	wantRefs(t, "約束の詳細", detail.Refs, refsOf("t-keep", "t-rename", "f-rename", "c-other"))

	// main の時点の中身。今もある t-rename・f-rename は今の名前、今ない t-gone・f-gone はその時点の名前。
	// t-keep は f-rename の定義と c-other の文の中にしかないので、refs に入らない
	var at struct {
		Content refClaimContent    `json:"content"`
		Refs    map[string]refInfo `json:"refs"`
	}
	commit := r.g.run("rev-parse", "main")
	if code, msg := r.s.get("/api/claims/c-ref/at/"+commit, &at); code != 200 {
		t.Fatalf("main の時点の中身を読めない（%d）: %s", code, msg)
	}
	wantFrags(t, "main の時点の statement", at.Content.Statement,
		fragsOf("新しい用語>t-rename", "と", "新しい機能>f-rename", "は", "消える用語>t-gone", "と", "消える機能>f-gone", "と", "c-other>c-other", "を使う"))
	wantFrags(t, "main の時点の rationale", at.Content.Rationale, textOf("{@id:x}は文字"))
	if at.Content.Condition != nil || at.Content.ModelCheckWaiver != nil {
		t.Errorf("main の時点で書いていない condition・modelCheckWaiver が null でない: %+v %+v", at.Content.Condition, at.Content.ModelCheckWaiver)
	}
	wantRefs(t, "main の時点の中身", at.Refs, refsOf("t-rename", "f-rename", "c-other", "t-gone", "f-gone"))
}

// runeRefs は、セグメントの並びの見せる文字の 1 文字ずつに、指す ID（参照でなければ空）を並べる。
func runeRefs(fs frags) []string {
	var out []string
	for _, f := range fs {
		for range []rune(f.Text) {
			if f.Ref != nil {
				out = append(out, *f.Ref)
			} else {
				out = append(out, "")
			}
		}
	}
	return out
}

func joinRefParts(ps []refDiffPart, keep ...string) string {
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

// checkRefDiff は、文字の差分 parts が、変更前 before と変更後 after の見せる文字どうしの差分で、参照の境で分かれ、
// 参照にかかる分に ref が付いていることを確かめる（delete は変更前の側、equal・insert は変更後の側の参照の位置で決める）。
func checkRefDiff(t *testing.T, where string, parts []refDiffPart, before, after frags) {
	t.Helper()
	if joinRefParts(parts, "equal", "delete") != before.String() || joinRefParts(parts, "equal", "insert") != after.String() {
		t.Errorf("%s の差分をつなげても変更前 %q・変更後 %q にならない: %+v", where, before.String(), after.String(), parts)
		return
	}
	bRefs, aRefs := runeRefs(before), runeRefs(after)
	bi, ai := 0, 0
	for _, p := range parts {
		ref := ""
		if p.Ref != nil {
			ref = *p.Ref
		}
		for range []rune(p.Text) {
			switch p.Op {
			case "delete":
				if bRefs[bi] != ref {
					t.Errorf("%s の delete の %q: 変更前の側の参照は %q のはずが %q", where, p.Text, bRefs[bi], ref)
					return
				}
				bi++
			case "insert":
				if aRefs[ai] != ref {
					t.Errorf("%s の insert の %q: 変更後の側の参照は %q のはずが %q", where, p.Text, aRefs[ai], ref)
					return
				}
				ai++
			case "equal":
				if aRefs[ai] != ref {
					t.Errorf("%s の equal の %q: 変更後の側の参照は %q のはずが %q", where, p.Text, aRefs[ai], ref)
					return
				}
				bi++
				ai++
			default:
				t.Errorf("%s に知らない op %q", where, p.Op)
				return
			}
		}
	}
}

// @vanda:evidence reference-diff
func Test差分は両方の参照を今の名前に置き換えて比べ名前を変えただけの参照は差にならず今ない用語と機能は比べる元の名前で比べる(t *testing.T) {
	t.Parallel()
	r := setupRefAPI(t)
	e := r.entry(t, "c-ref")
	var before, after refClaimContent
	decodeInto(t, "c-ref の before", e.Before, &before)
	decodeInto(t, "c-ref の after", e.After, &after)

	// 変更前も、今もある用語・機能は今の名前で見せる。今ない用語・機能は比べる元の名前
	wantBefore := fragsOf("新しい用語>t-rename", "と", "新しい機能>f-rename", "は", "消える用語>t-gone", "と", "消える機能>f-gone", "と", "c-other>c-other", "を使う")
	wantAfter := fragsOf("新しい用語>t-rename", "と", "新しい機能>f-rename", "は", "c-other>c-other", "を必ず使う")
	wantFrags(t, "c-ref の before の statement", before.Statement, wantBefore)
	wantFrags(t, "c-ref の after の statement", after.Statement, wantAfter)

	if isNull(e.Diff) {
		t.Fatalf("c-ref の diff がない")
	}
	var d refDiff
	decodeInto(t, "c-ref の diff", e.Diff, &d)
	st := d.Fields["statement"]
	checkRefDiff(t, "statement", st, wantBefore, wantAfter)
	// 名前を変えただけの参照（t-rename・f-rename）は、変わっていない所に入る
	for _, name := range []string{"新しい用語", "新しい機能", "古い用語", "古い機能"} {
		if strings.Contains(joinRefParts(st, "delete", "insert"), name) {
			t.Errorf("名前を変えただけの参照 %q が、差として出た: %+v", name, st)
		}
	}
	for _, name := range []string{"新しい用語", "新しい機能"} {
		if !strings.Contains(joinRefParts(st, "equal"), name) {
			t.Errorf("名前を変えただけの参照 %q が、変わっていない所にない: %+v", name, st)
		}
	}
	// 消した用語・機能への参照は、比べる元の名前で消した所に入る
	for _, name := range []string{"消える用語", "消える機能"} {
		if !strings.Contains(joinRefParts(st, "delete"), name) {
			t.Errorf("今ない %q が、消した所にない: %+v", name, st)
		}
	}
	// 変わっていない rationale は、{@@ を {@ にした文の equal
	checkRefDiff(t, "rationale", d.Fields["rationale"], textOf("{@id:x}は文字"), textOf("{@id:x}は文字"))

	// エビデンスの詳細の差分も、見せる文字どうしで比べる
	if d.Evidence == nil || len(d.Evidence.Before) != 1 || len(d.Evidence.After) != 1 || d.Evidence.Before[0].Op != "modified" {
		t.Fatalf("c-ref の diff.evidence: %s", e.Diff)
	}
	checkRefDiff(t, "evidence の detail", d.Evidence.Before[0].Detail, fragsOf("新しい機能>f-rename", "で試す"), fragsOf("新しい機能>f-rename", "で確かめる"))
	checkRefDiff(t, "evidence の detail（変更後の側）", d.Evidence.After[0].Detail, fragsOf("新しい機能>f-rename", "で試す"), fragsOf("新しい機能>f-rename", "で確かめる"))

	// 変わっていない所（equal）の参照は、変更後の側で決める。c-same は、変更前は「タスク」が参照で「新しい用語」が文字、
	// 変更後はその逆なので、見せる文字は変わらないが、equal の参照は変更後の側の位置に付く
	se := r.entry(t, "c-same")
	var sb, sa refClaimContent
	decodeInto(t, "c-same の before", se.Before, &sb)
	decodeInto(t, "c-same の after", se.After, &sa)
	sameBefore := fragsOf("タスク>t-keep", "と新しい用語を数える")
	sameAfter := fragsOf("タスクと", "新しい用語>t-rename", "をいつも数える")
	wantFrags(t, "c-same の before の statement", sb.Statement, sameBefore)
	wantFrags(t, "c-same の after の statement", sa.Statement, sameAfter)
	var sd refDiff
	decodeInto(t, "c-same の diff", se.Diff, &sd)
	checkRefDiff(t, "c-same の statement", sd.Fields["statement"], sameBefore, sameAfter)

	// 用語と機能の定義の差分も、セグメントの見せる文字どうしで比べる
	fe := r.entry(t, "f-rename")
	var fd refDiff
	decodeInto(t, "f-rename の diff", fe.Diff, &fd)
	checkRefDiff(t, "f-rename の definition", fd.Fields["definition"], textOf("前の機能の定義"), fragsOf("タスク>t-keep", "を足す"))
	// 名前はセグメントに分けないので、差分は書いたとおりの名前どうし
	if joinRefParts(fd.Fields["name"], "equal", "delete") != "古い機能" || joinRefParts(fd.Fields["name"], "equal", "insert") != "新しい機能" {
		t.Errorf("f-rename の name の差分: %+v", fd.Fields["name"])
	}
}

// @vanda:evidence reference-diff
func Test参照先の名前を変えただけの約束は一覧に出ない(t *testing.T) {
	t.Parallel()
	r := setupRefAPI(t)
	// c-other は {@id:t-keep} を指したまま、書いた文は変えていない。t-rename の名前を変えた story-24 でも出ない
	var keys []string
	for _, e := range r.review.Entries {
		keys = append(keys, e.key())
	}
	sort.Strings(keys)
	want := []string{"c-ref", "c-same", "f-gone", "f-rename", "t-gone", "t-rename"}
	if !reflect.DeepEqual(keys, want) {
		t.Errorf("一覧の件: %v（思ったもの %v）", keys, want)
	}
}

// @vanda:evidence reference-fingerprint-raw
func Test参照先の名前と定義を変えても約束の詳細のフィンガープリントは書いたままの文から作り承認済のままになる(t *testing.T) {
	t.Parallel()
	bin, version := refSetup(t)
	root := t.TempDir()
	g := newRVGit(t, root)
	g.run("init", "-q", "-b", "main")
	writeFile(t, root, ".vanda/config.toml", fmt.Sprintf(refAPIConfig, version))
	who := func(kind, id, js, story string) string {
		return agreementTOML(kind, id, sha256Hex([]byte(js)), "", story, "テスト担当", "tester@example.com")
	}
	agreed := who("claim", "c", fpClaimJSON, "#1") + who("term", "t", fpTermV1, "#1") + who("feature", "f", fpFeatureV1, "#1")
	writeFile(t, root, ".vanda/promises.toml", fpPromises("タスク", "やること", "タスクの登録", "タスクを足す"))
	writeFile(t, root, ".vanda/agreements.toml", agreed)
	g.commitAll("約束を置いて承認する", "#1")
	g.run("switch", "-q", "-c", "story-24")
	writeFile(t, root, ".vanda/promises.toml", fpPromises("仕事", "やるべきこと", "仕事の登録", "仕事を足す"))
	writeFile(t, root, ".vanda/agreements.toml", agreed+who("term", "t", fpTermV2, "#24")+who("feature", "f", fpFeatureV2, "#24"))
	g.commitAll("用語と機能の名前と定義を変える", "#24")
	s := startServe(t, bin, root, g.env)

	var detail struct {
		State       string `json:"state"`
		Fingerprint string `json:"fingerprint"`
		Statement   frags  `json:"statement"`
	}
	if code, msg := s.get("/api/claims/c", &detail); code != 200 {
		t.Fatalf("約束の詳細を読めない（%d）: %s", code, msg)
	}
	h := getHistory(t, s, "c")
	if want := sha256Hex([]byte(fpClaimJSON)); detail.Fingerprint != want {
		t.Errorf("約束 c のフィンガープリント %s が、書いたままの文のフィンガープリント %s でない", detail.Fingerprint, want)
	}
	if detail.State != "agreed" || len(h.Agreements) != 1 || !h.Agreements[0].Current {
		t.Errorf("参照先を変えた後の約束 c: state %q、承認 %+v", detail.State, h.Agreements)
	}
	// 見せる文は今の名前に置き換わっている（フィンガープリントは置き換える前の文から作る）
	wantFrags(t, "約束 c の statement", detail.Statement, fragsOf("仕事>t", "は", "仕事の登録>f", "で足し、{@id:x}は文字"))

	var rv refReview
	if code, msg := s.get("/api/review?view=branch&base=main", &rv); code != 200 {
		t.Fatalf("一覧を読めない（%d）: %s", code, msg)
	}
	for _, e := range rv.Entries {
		if e.key() == "c" {
			t.Errorf("参照先の名前と定義だけを変えたのに、約束 c が一覧に出た: %+v", e)
		}
	}
}
