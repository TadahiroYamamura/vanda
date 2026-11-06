package agreements_test

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"pgregory.net/rapid"

	"github.com/TadahiroYamamura/vanda/internal/agreements"
	"github.com/TadahiroYamamura/vanda/internal/promises"
)

// CM-03 の指紋のテスト。期待値は docs/formats.md の「指紋」から作る。
// 決まった形の JSON は、実装の CanonicalJSON を使わず、このファイルの canonical で作る。

// @vanda:evidence fingerprint-file-content
func TestFileFingerprintはファイルの中身そのもののSHA256に接頭辞を付ける(t *testing.T) {
	// 期待値は sha256sum で作った値
	cases := map[string]string{
		"":    "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		"abc": "sha256:ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
	}
	for src, want := range cases {
		if got := agreements.FileFingerprint([]byte(src)); got != want {
			t.Errorf("FileFingerprint(%q) = %s, want %s", src, got, want)
		}
	}
}

func TestCanonicalJSONはキーをUTF8のバイトの順に並べ決まった文字だけをエスケープする(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want string
	}{
		{"キーの順", map[string]any{"b": int64(1), "a": "x", "B": true, "あ": false, "_": "y"},
			`{"B":true,"_":"y","a":"x","b":1,"あ":false}`},
		{"エスケープする文字", map[string]any{"s": "\"\\\n\r\t\x01\x1f"},
			`{"s":"\"\\\n\r\t\u0001\u001f"}`},
		{"エスケープしない文字", map[string]any{"s": "<>&日本語'/é\x7f"},
			"{\"s\":\"<>&日本語'/é\x7f\"}"},
		{"配列は書いた順", map[string]any{"a": []any{"z", "a", "m"}},
			`{"a":["z","a","m"]}`},
		{"表の配列は書いた順で、中の表もキーの順", map[string]any{"e": []map[string]any{{"y": "1", "x": "2"}, {"b": "3", "a": "4"}}},
			`{"e":[{"x":"2","y":"1"},{"a":"4","b":"3"}]}`},
		{"整数は10進", map[string]any{"n": int64(-42)}, `{"n":-42}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := agreements.CanonicalJSON(c.in)
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Fatalf("got  %s\nwant %s", got, c.want)
			}
		})
	}
}

func TestCanonicalJSONは浮動小数点数と日時をエラーにする(t *testing.T) {
	for name, v := range map[string]any{"浮動小数点数": 1.5, "日時": time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)} {
		if _, err := agreements.CanonicalJSON(map[string]any{"v": v}); err == nil {
			t.Errorf("%s でエラーにならない", name)
		}
	}
}

// @vanda:evidence fingerprint-excludes-flags
func TestItemFingerprintは手で作った例と同じ値になりflagsとguessesを除く(t *testing.T) {
	// docs/formats.md の例。期待値は {"definition":"やること","id":"task","name":"タスク"} を sha256sum にかけた値
	want := "sha256:8d0b08167c5bf0bbb777f239b51dc94e46f55369dce46bea2ff4dfb23325832d"
	base := map[string]any{"id": "task", "name": "タスク", "definition": "やること"}
	with := map[string]any{"id": "task", "name": "タスク", "definition": "やること", "flags": []any{"ai-proposal"}, "guesses": []any{"推測"}}
	for name, raw := range map[string]map[string]any{"そのまま": base, "flags と guesses 付き": with} {
		got, err := agreements.ItemFingerprint(raw)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("%s: got %s, want %s", name, got, want)
		}
	}
}

// ---- 約束リストの項目の、テストの中の写し ----

type termM struct {
	ID, Name, Definition string
	PartOf               *string
}

type featureM struct {
	ID, Name, Definition string
	Core, Formal         *bool
	Waiver               *string
}

type evidenceM struct {
	Stage, Type string
	Layers      []string
	Detail      *string
}

type claimM struct {
	ID                   string
	Subjects             []string
	Statement            string
	Rationale, Condition *string
	Layers               []string
	Flags, Guesses       []string // nil なら書かない
	Evidence             []evidenceM
}

type field struct {
	key string
	val any // string・bool・[]string・[]obj
}

type obj []field

func (t termM) fields() obj {
	o := obj{{"id", t.ID}, {"name", t.Name}, {"definition", t.Definition}}
	if t.PartOf != nil {
		o = append(o, field{"part_of", *t.PartOf})
	}
	return o
}

func (f featureM) fields() obj {
	o := obj{{"id", f.ID}, {"name", f.Name}, {"definition", f.Definition}}
	if f.Core != nil {
		o = append(o, field{"core", *f.Core})
	}
	if f.Formal != nil {
		o = append(o, field{"formal", *f.Formal})
	}
	if f.Waiver != nil {
		o = append(o, field{"formal_waiver", *f.Waiver})
	}
	return o
}

func (e evidenceM) fields() obj {
	o := obj{{"stage", e.Stage}, {"type", e.Type}, {"layers", e.Layers}}
	if e.Detail != nil {
		o = append(o, field{"detail", *e.Detail})
	}
	return o
}

// fields は、指紋に入る項目だけを返す。flags と guesses は render で別に足す。
func (c claimM) fields() obj {
	o := obj{{"id", c.ID}, {"subjects", c.Subjects}, {"statement", c.Statement}, {"layers", c.Layers}}
	if c.Rationale != nil {
		o = append(o, field{"rationale", *c.Rationale})
	}
	if c.Condition != nil {
		o = append(o, field{"condition", *c.Condition})
	}
	var evs []obj
	for _, e := range c.Evidence {
		evs = append(evs, e.fields())
	}
	return append(o, field{"evidence", evs})
}

func (c claimM) extra() obj {
	var o obj
	if c.Flags != nil {
		o = append(o, field{"flags", c.Flags})
	}
	if c.Guesses != nil {
		o = append(o, field{"guesses", c.Guesses})
	}
	return o
}

// canonical は docs/formats.md の「決まった形の JSON」を、このテストの中で書き直したもの。
func canonical(v any) string {
	var b strings.Builder
	var write func(v any)
	write = func(v any) {
		switch x := v.(type) {
		case string:
			b.WriteByte('"')
			for _, r := range x {
				switch {
				case r == '"':
					b.WriteString(`\"`)
				case r == '\\':
					b.WriteString(`\\`)
				case r == '\n':
					b.WriteString(`\n`)
				case r == '\r':
					b.WriteString(`\r`)
				case r == '\t':
					b.WriteString(`\t`)
				case r < 0x20:
					fmt.Fprintf(&b, `\u%04x`, r)
				default:
					b.WriteRune(r)
				}
			}
			b.WriteByte('"')
		case bool:
			fmt.Fprint(&b, x)
		case []string:
			b.WriteByte('[')
			for i, s := range x {
				if i > 0 {
					b.WriteByte(',')
				}
				write(s)
			}
			b.WriteByte(']')
		case []obj:
			b.WriteByte('[')
			for i, o := range x {
				if i > 0 {
					b.WriteByte(',')
				}
				write(o)
			}
			b.WriteByte(']')
		case obj:
			sorted := append(obj(nil), x...)
			sort.Slice(sorted, func(i, j int) bool { return sorted[i].key < sorted[j].key })
			b.WriteByte('{')
			for i, f := range sorted {
				if i > 0 {
					b.WriteByte(',')
				}
				write(f.key)
				b.WriteByte(':')
				write(f.val)
			}
			b.WriteByte('}')
		default:
			// 指紋が合わなくなり、テストの失敗の JSON に型が出る
			fmt.Fprintf(&b, "<テストの写しに知らない型 %T>", v)
		}
	}
	write(v)
	return b.String()
}

func expectedFingerprint(o obj) string {
	sum := sha256.Sum256([]byte(canonical(o)))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// ---- TOML の書き方 ----

// style は、指紋に関わらない書き方の違い。plainStyle はいつも同じ書き方をする。
type style interface {
	perm(n int) []int
	eq() string
	quote(s string) string
	inlineEvidence() bool
	multilineArray() bool
	comment() bool
}

type plainStyle struct{}

func (plainStyle) perm(n int) []int {
	p := make([]int, n)
	for i := range p {
		p[i] = i
	}
	return p
}
func (plainStyle) eq() string            { return " = " }
func (plainStyle) quote(s string) string { return basicString(s) }
func (plainStyle) inlineEvidence() bool  { return false }
func (plainStyle) multilineArray() bool  { return false }
func (plainStyle) comment() bool         { return false }

type randomStyle struct{ t *rapid.T }

func (s randomStyle) perm(n int) []int {
	return rapid.Permutation(plainStyle{}.perm(n)).Draw(s.t, "順")
}
func (s randomStyle) eq() string {
	return rapid.SampledFrom([]string{" = ", "=", "   =  ", "\t=\t"}).Draw(s.t, "=")
}
func (s randomStyle) quote(str string) string {
	if canLiteral(str) && rapid.Bool().Draw(s.t, "引用符") {
		return "'" + str + "'"
	}
	return basicString(str)
}
func (s randomStyle) inlineEvidence() bool {
	return rapid.Bool().Draw(s.t, "インラインのエビデンス")
}
func (s randomStyle) multilineArray() bool { return rapid.Bool().Draw(s.t, "複数行の配列") }
func (s randomStyle) comment() bool        { return rapid.Bool().Draw(s.t, "コメント") }

func canLiteral(s string) bool {
	for _, r := range s {
		if r == '\'' || (r < 0x20 && r != '\t') || r == 0x7f {
			return false
		}
	}
	return true
}

func basicString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"':
			b.WriteString(`\"`)
		case r == '\\':
			b.WriteString(`\\`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, `\u%04X`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

func renderValue(st style, v any, inline bool) string {
	switch x := v.(type) {
	case string:
		return st.quote(x)
	case bool:
		return fmt.Sprint(x)
	case []string:
		parts := make([]string, len(x))
		for i, s := range x {
			parts[i] = st.quote(s)
		}
		if len(x) > 0 && !inline && st.multilineArray() {
			return "[\n  " + strings.Join(parts, ",\n  ") + ",\n]"
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case obj:
		parts := make([]string, len(x))
		for i, j := range st.perm(len(x)) {
			parts[i] = x[j].key + st.eq() + renderValue(st, x[j].val, true)
		}
		return "{ " + strings.Join(parts, ", ") + " }"
	case []obj:
		parts := make([]string, len(x))
		for i, o := range x {
			parts[i] = renderValue(st, o, true)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	}
	// TOML として読めなくなり、テストの失敗に型が出る
	return fmt.Sprintf("<知らない型 %T>", v)
}

func renderTable(b *strings.Builder, st style, header string, o obj) {
	fmt.Fprintf(b, "[[%s]]\n", header)
	var subtables []obj
	for _, j := range st.perm(len(o)) {
		f := o[j]
		if evs, ok := f.val.([]obj); ok && f.key == "evidence" && !st.inlineEvidence() {
			subtables = evs
			continue
		}
		if st.comment() {
			b.WriteString("# 指紋に入らないコメント\n")
		}
		fmt.Fprintf(b, "%s%s%s\n", f.key, st.eq(), renderValue(st, f.val, false))
	}
	b.WriteString("\n")
	for _, e := range subtables {
		renderTable(b, st, header+".evidence", e)
	}
}

type promiseList struct {
	terms    []termM
	features []featureM
	claims   []claimM
}

func render(st style, l promiseList) string {
	var tables []func(*strings.Builder)
	for _, x := range l.terms {
		tables = append(tables, func(b *strings.Builder) { renderTable(b, st, "term", x.fields()) })
	}
	for _, x := range l.features {
		tables = append(tables, func(b *strings.Builder) { renderTable(b, st, "feature", x.fields()) })
	}
	for _, x := range l.claims {
		tables = append(tables, func(b *strings.Builder) { renderTable(b, st, "claim", append(x.fields(), x.extra()...)) })
	}
	var b strings.Builder
	for _, j := range st.perm(len(tables)) {
		tables[j](&b)
	}
	return b.String()
}

// load は、TOML を約束リストとして読み、ID ごとの指紋を返す。
func load(t interface {
	Helper()
	Fatalf(string, ...any)
}, root, src string) map[string]string {
	t.Helper()
	path := filepath.Join(root, ".vanda", "promises.toml")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("%v", err)
	}
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatalf("%v", err)
	}
	list, errs := promises.Load(root, []string{".vanda/promises.toml"}, []string{".vanda/promises.toml"}, false)
	if len(errs) > 0 {
		t.Fatalf("約束リストを読めない: %v\n%s", errs, src)
	}
	fps := map[string]string{}
	add := func(id string, raw map[string]any) {
		fp, err := agreements.ItemFingerprint(raw)
		if err != nil {
			t.Fatalf("%s の指紋を作れない: %v", id, err)
		}
		fps[id] = fp
	}
	for _, x := range list.Terms {
		add(x.ID, x.Raw)
	}
	for _, x := range list.Features {
		add(x.ID, x.Raw)
	}
	for _, x := range list.Claims {
		add(x.ID, x.Raw)
	}
	return fps
}

func expected(l promiseList) map[string]string {
	m := map[string]string{}
	for _, x := range l.terms {
		m[x.ID] = expectedFingerprint(x.fields())
	}
	for _, x := range l.features {
		m[x.ID] = expectedFingerprint(x.fields())
	}
	for _, x := range l.claims {
		m[x.ID] = expectedFingerprint(x.fields())
	}
	return m
}

// ---- ランダムチェック ----

var stages = []string{"plan", "implementation", "release", "operation"}
var evidenceTypes = []string{"model-check", "proof", "cross-check", "random-check", "scenario-test", "unit-test",
	"interface-test", "concurrency-test", "guardrail", "sensor", "human-review"}

// 書き方で扱いの変わる文字（引用符、\、改行、タブ、制御文字、< > &、日本語）を多めに混ぜる。
func textGen() *rapid.Generator[string] {
	chars := []string{"a", "Z", "0", " ", "あ", "漢", "é", "\"", "'", "\\", "\n", "\t", "\x01", "<", ">", "&", "#", "=", "[", "{"}
	return rapid.Custom(func(t *rapid.T) string {
		parts := rapid.SliceOfN(rapid.SampledFrom(chars), 0, 12).Draw(t, "文字")
		return strings.Join(parts, "")
	})
}

func optional[T any](t *rapid.T, g *rapid.Generator[T], label string) *T {
	if !rapid.Bool().Draw(t, label+"を書くか") {
		return nil
	}
	v := g.Draw(t, label)
	return &v
}

func listGen(t *rapid.T) promiseList {
	text := textGen()
	ids := rapid.SliceOfNDistinct(rapid.StringMatching(`[a-z][a-z0-9]{0,4}(-[a-z0-9]{1,4}){0,2}`), 3, 3, rapid.ID[string]).Draw(t, "ID")
	layers := rapid.SliceOfNDistinct(rapid.SampledFrom([]string{"model", "cli", "db"}), 1, 3, rapid.ID[string])
	term := termM{ID: ids[0], Name: text.Draw(t, "用語の名前"), Definition: text.Draw(t, "用語の定義"), PartOf: optional(t, text, "part_of")}
	core := optional(t, rapid.Bool(), "core")
	formal := optional(t, rapid.Bool(), "formal")
	feature := featureM{ID: ids[1], Name: text.Draw(t, "機能の名前"), Definition: text.Draw(t, "機能の定義"), Core: core, Formal: formal}
	if core != nil && *core && formal != nil && !*formal {
		w := text.Draw(t, "formal_waiver")
		feature.Waiver = &w
	}
	claimLayers := layers.Draw(t, "約束のレイヤー")
	evs := rapid.SliceOfN(rapid.Custom(func(t *rapid.T) evidenceM {
		return evidenceM{
			Stage:  rapid.SampledFrom(stages).Draw(t, "ステージ"),
			Type:   rapid.SampledFrom(evidenceTypes).Draw(t, "タイプ"),
			Layers: rapid.SliceOfNDistinct(rapid.SampledFrom(claimLayers), 1, len(claimLayers), rapid.ID[string]).Draw(t, "エビデンスのレイヤー"),
			Detail: optional(t, text, "detail"),
		}
	}), 1, 3).Draw(t, "エビデンス")
	claim := claimM{
		ID: ids[2], Subjects: []string{ids[0], ids[1]}[:rapid.IntRange(1, 2).Draw(t, "対象の数")],
		Statement: text.Draw(t, "文"), Rationale: optional(t, text, "理由"), Condition: optional(t, text, "条件"),
		Layers: claimLayers, Evidence: evs,
	}
	if rapid.Bool().Draw(t, "flags") {
		claim.Flags = []string{"ai-proposal"}
	}
	if rapid.Bool().Draw(t, "guesses") {
		claim.Guesses = rapid.SliceOfN(text, 0, 2).Draw(t, "推測")
	}
	return promiseList{terms: []termM{term}, features: []featureM{feature}, claims: []claimM{claim}}
}

// @vanda:evidence fingerprint-ignores-formatting
func Test約束リストの項目の順と空白と引用符とコメントの違いでは指紋が変わらず決まった形のJSONの指紋になる(t *testing.T) {
	root := t.TempDir()
	rapid.Check(t, func(rt *rapid.T) {
		l := listGen(rt)
		want := expected(l)
		plain := load(rt, root, render(plainStyle{}, l))
		other := load(rt, root, render(randomStyle{rt}, l))
		for id, w := range want {
			if plain[id] != w {
				rt.Fatalf("%s: 決まった書き方の指紋 %s が、決まった形の JSON の指紋 %s と違う\nJSON: %s", id, plain[id], w, jsonOf(l, id))
			}
			if other[id] != w {
				rt.Fatalf("%s: 書き方を変えると指紋が %s になった（%s のはず）", id, other[id], w)
			}
		}
	})
}

func jsonOf(l promiseList, id string) string {
	for _, x := range l.terms {
		if x.ID == id {
			return canonical(x.fields())
		}
	}
	for _, x := range l.features {
		if x.ID == id {
			return canonical(x.fields())
		}
	}
	for _, x := range l.claims {
		if x.ID == id {
			return canonical(x.fields())
		}
	}
	return ""
}

// ---- 項目を全部回す ----

func strp(s string) *string { return &s }
func boolp(b bool) *bool    { return &b }

func baseList() promiseList {
	return promiseList{
		terms:    []termM{{ID: "task", Name: "タスク", Definition: "やること"}},
		features: []featureM{{ID: "task-add", Name: "タスクの登録", Definition: "タスクを足す"}},
		claims: []claimM{{
			ID: "task-id-unique", Subjects: []string{"task"}, Statement: "タスクの ID は重ならない",
			Rationale: strp("ID で指すため"), Condition: strp("いつも"), Layers: []string{"model", "cli"},
			Evidence: []evidenceM{
				{Stage: "implementation", Type: "unit-test", Layers: []string{"model"}, Detail: strp("task_test.go")},
				{Stage: "implementation", Type: "scenario-test", Layers: []string{"cli"}},
			},
		}},
	}
}

// @vanda:evidence fingerprint-excludes-flags
func Test指紋に入る項目のどれを変えても指紋が変わりflagsとguessesでは変わらない(t *testing.T) {
	type change struct {
		name    string
		id      string
		mutate  func(*promiseList)
		changes bool
	}
	changes := []change{
		{"用語の名前", "task", func(l *promiseList) { l.terms[0].Name = "タスク2" }, true},
		{"用語の定義", "task", func(l *promiseList) { l.terms[0].Definition = "すること" }, true},
		{"用語の part_of を書く", "task", func(l *promiseList) { l.terms[0].PartOf = strp("work") }, true},
		{"機能の名前", "task-add", func(l *promiseList) { l.features[0].Name = "登録" }, true},
		{"機能の定義", "task-add", func(l *promiseList) { l.features[0].Definition = "足す" }, true},
		{"機能の core を書く", "task-add", func(l *promiseList) { l.features[0].Core = boolp(false) }, true},
		{"機能の formal を書く", "task-add", func(l *promiseList) { l.features[0].Formal = boolp(false) }, true},
		{"機能の formal_waiver を書く", "task-add", func(l *promiseList) {
			l.features[0].Core, l.features[0].Formal, l.features[0].Waiver = boolp(true), boolp(false), strp("理由")
		}, true},
		{"約束の文", "task-id-unique", func(l *promiseList) { l.claims[0].Statement = "重ならない" }, true},
		{"約束の理由", "task-id-unique", func(l *promiseList) { l.claims[0].Rationale = strp("別の理由") }, true},
		{"約束の理由を省く", "task-id-unique", func(l *promiseList) { l.claims[0].Rationale = nil }, true},
		{"約束の条件", "task-id-unique", func(l *promiseList) { l.claims[0].Condition = strp("登録のとき") }, true},
		{"約束の対象", "task-id-unique", func(l *promiseList) { l.claims[0].Subjects = []string{"task", "task-add"} }, true},
		{"約束のレイヤーの順", "task-id-unique", func(l *promiseList) { l.claims[0].Layers = []string{"cli", "model"} }, true},
		{"約束のレイヤー", "task-id-unique", func(l *promiseList) { l.claims[0].Layers = []string{"model", "cli", "db"} }, true},
		{"エビデンスのステージ", "task-id-unique", func(l *promiseList) { l.claims[0].Evidence[0].Stage = "release" }, true},
		{"エビデンスのタイプ", "task-id-unique", func(l *promiseList) { l.claims[0].Evidence[0].Type = "random-check" }, true},
		{"エビデンスのレイヤー", "task-id-unique", func(l *promiseList) { l.claims[0].Evidence[0].Layers = []string{"model", "cli"} }, true},
		{"エビデンスの detail", "task-id-unique", func(l *promiseList) { l.claims[0].Evidence[0].Detail = strp("other_test.go") }, true},
		{"エビデンスの detail を省く", "task-id-unique", func(l *promiseList) { l.claims[0].Evidence[0].Detail = nil }, true},
		{"エビデンスの順", "task-id-unique", func(l *promiseList) {
			e := l.claims[0].Evidence
			l.claims[0].Evidence = []evidenceM{e[1], e[0]}
		}, true},
		{"エビデンスを足す", "task-id-unique", func(l *promiseList) {
			l.claims[0].Evidence = append(l.claims[0].Evidence, evidenceM{Stage: "release", Type: "human-review", Layers: []string{"model"}})
		}, true},
		{"flags を立てる", "task-id-unique", func(l *promiseList) { l.claims[0].Flags = []string{"ai-proposal"} }, false},
		{"guesses を書く", "task-id-unique", func(l *promiseList) { l.claims[0].Guesses = []string{"推測"} }, false},
		{"guesses を空で書く", "task-id-unique", func(l *promiseList) { l.claims[0].Guesses = []string{} }, false},
	}
	root := t.TempDir()
	before := load(t, root, render(plainStyle{}, baseList()))
	for _, c := range changes {
		t.Run(c.name, func(t *testing.T) {
			l := baseList()
			c.mutate(&l)
			after := load(t, root, render(plainStyle{}, l))
			if c.changes && after[c.id] == before[c.id] {
				t.Fatalf("%s を変えても指紋が変わらない", c.name)
			}
			if !c.changes && after[c.id] != before[c.id] {
				t.Fatalf("%s を変えると指紋が変わった", c.name)
			}
			for id, fp := range before {
				if id != c.id && after[id] != fp {
					t.Fatalf("変えていない %s の指紋が変わった", id)
				}
			}
		})
	}
}

// @vanda:evidence changed-id-is-new
func Test約束リストのIDを変えると指紋が変わる(t *testing.T) {
	root := t.TempDir()
	before := load(t, root, render(plainStyle{}, baseList()))
	l := baseList()
	l.claims[0].ID = "task-ids-unique"
	after := load(t, root, render(plainStyle{}, l))
	if after["task-ids-unique"] == before["task-id-unique"] {
		t.Fatal("ID を変えても指紋が変わらない")
	}
}

// @vanda:evidence fingerprint-as-written
func Test_formalを省いたものとformalをtrueと書いたものは別の指紋になる(t *testing.T) {
	root := t.TempDir()
	a := baseList()
	a.features[0].Core = boolp(true)
	b := baseList()
	b.features[0].Core, b.features[0].Formal = boolp(true), boolp(true)
	fa := load(t, root, render(plainStyle{}, a))["task-add"]
	fb := load(t, root, render(plainStyle{}, b))["task-add"]
	if fa == fb {
		t.Fatal("formal を省いたものと formal = true が同じ指紋になった")
	}
}

// 期待値は sha256sum で作った値。どれも sha256: に続けて 16 進の小文字 64 文字
//
// @vanda:evidence fingerprint-form
func Test指紋は元のバイト列のSHA256をsha256と16進の小文字64文字で書いたもの(t *testing.T) {
	files := map[string]string{
		"":           "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		"abc":        "sha256:ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
		"hello\n":    "sha256:5891b5b522d5df086d0ff0b110fbd9d21bb4fc7163af34d08286a2e846f6be03",
		"\x00\xff\n": "sha256:712450d3c4a79eea9509e75dc1dacdeff58034df538536cfae2da882bd8a0c50",
		"日本語":        "sha256:77710aedc74ecfa33685e33a6c7df5cc83004da1bdcef7fb280f5c2b2e97e0a5",
	}
	for src, want := range files {
		if got := agreements.FileFingerprint([]byte(src)); got != want {
			t.Errorf("ファイル %q の指紋: %s（思ったもの %s）", src, got, want)
		}
	}
	// 項目の指紋の元のバイト列は、決まった形の JSON {"definition":"やること","id":"task","name":"タスク"}
	got, err := agreements.ItemFingerprint(map[string]any{"id": "task", "name": "タスク", "definition": "やること"})
	if err != nil {
		t.Fatal(err)
	}
	if want := "sha256:8d0b08167c5bf0bbb777f239b51dc94e46f55369dce46bea2ff4dfb23325832d"; got != want {
		t.Errorf("項目の指紋: %s（思ったもの %s）", got, want)
	}
}

// 項目の指紋の期待値は、手で書いた決まった形の JSON のバイト列の指紋（ファイルの指紋と同じ作り方）にする
//
// @vanda:evidence fingerprint-item-json
func Test約束と用語と機能の指紋はキーをバイトの順に並べ配列を書いた順にし空白と最後の改行のないJSONから作る(t *testing.T) {
	src := `[[term]]
part_of = "whole"
name = "タスク"
id = "task"
definition = "やること"

[[feature]]
formal_waiver = "状態を持たないため"
formal = false
core = true
name = "タスクの登録"
id = "add-task"
definition = "タスクを足す"

[[claim]]
statement = "タスクの ID は重ならない"
subjects = ["task", "add-task"]
rationale = "見分けるため"
layers = ["model", "db"]
id = "task-unique"
condition = "いつも"

[[claim.evidence]]
type = "unit-test"
stage = "implementation"
layers = ["model"]
detail = "TestUnique"

[[claim.evidence]]
stage = "plan"
layers = ["db", "model"]
type = "guardrail"
`
	want := map[string]string{
		"task":     `{"definition":"やること","id":"task","name":"タスク","part_of":"whole"}`,
		"add-task": `{"core":true,"definition":"タスクを足す","formal":false,"formal_waiver":"状態を持たないため","id":"add-task","name":"タスクの登録"}`,
		"task-unique": `{"condition":"いつも","evidence":[{"detail":"TestUnique","layers":["model"],"stage":"implementation","type":"unit-test"},` +
			`{"layers":["db","model"],"stage":"plan","type":"guardrail"}],"id":"task-unique","layers":["model","db"],` +
			`"rationale":"見分けるため","statement":"タスクの ID は重ならない","subjects":["task","add-task"]}`,
	}
	got := load(t, t.TempDir(), src)
	for id, j := range want {
		if fp := agreements.FileFingerprint([]byte(j)); got[id] != fp {
			t.Errorf("%s の指紋 %s が、次の JSON の指紋 %s でない\n%s", id, got[id], fp, j)
		}
		// 空白や最後の改行を入れたものとは違う
		for _, other := range []string{j + "\n", strings.Replace(j, ",", ", ", 1), strings.Replace(j, ":", ": ", 1)} {
			if got[id] == agreements.FileFingerprint([]byte(other)) {
				t.Errorf("%s の指紋が、空白か最後の改行を入れた JSON の指紋と同じ\n%s", id, other)
			}
		}
	}
}

// @vanda:evidence fingerprint-json-strings
func Test決まった形のJSONの文字列は決まった文字だけをエスケープし真偽と整数はそのまま書く(t *testing.T) {
	// U+0000〜U+001F の全部と、エスケープするほかの 2 文字、エスケープしない文字
	escaped := map[rune]string{'"': `\"`, '\\': `\\`, '\n': `\n`, '\r': `\r`, '\t': `\t`}
	for r := rune(0); r < 0x20; r++ {
		if _, ok := escaped[r]; !ok {
			escaped[r] = fmt.Sprintf(`\u%04x`, r)
		}
	}
	kept := []rune{'<', '>', '&', '\'', '/', 'あ', '漢', '😀', 0x7f, 0x80, 0x2028, 0xfeff, ' '}
	check := func(r rune, want string) {
		t.Helper()
		raw := map[string]any{"id": "t", "name": "n", "definition": "a" + string(r) + "b"}
		j := `{"definition":"a` + want + `b","id":"t","name":"n"}`
		got, err := agreements.ItemFingerprint(raw)
		if err != nil {
			t.Fatal(err)
		}
		if fp := agreements.FileFingerprint([]byte(j)); got != fp {
			t.Errorf("U+%04X の指紋が、次の JSON の指紋でない\n%s", r, j)
		}
	}
	for r, want := range escaped {
		check(r, want)
	}
	for _, r := range kept {
		check(r, string(r))
	}
	// 真偽は true・false、整数は 10 進
	raw := map[string]any{"id": "f", "core": true, "formal": false, "n": int64(42), "m": int64(-7), "z": int64(0)}
	got, err := agreements.ItemFingerprint(raw)
	if err != nil {
		t.Fatal(err)
	}
	if j := `{"core":true,"formal":false,"id":"f","m":-7,"n":42,"z":0}`; got != agreements.FileFingerprint([]byte(j)) {
		t.Errorf("真偽と整数の指紋が、次の JSON の指紋でない\n%s", j)
	}
}
