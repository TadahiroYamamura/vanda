package annotation

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/TadahiroYamamura/vanda/internal/config"
)

func setup(t *testing.T, files map[string]string) (string, []string) {
	t.Helper()
	root := t.TempDir()
	var names []string
	for name, body := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		mode := os.FileMode(0o644)
		if strings.HasSuffix(name, ".sh") {
			mode = 0o755
		}
		if err := os.WriteFile(p, []byte(body), mode); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return root, names
}

func show(as []Annotation) string {
	var b strings.Builder
	for _, a := range as {
		b.WriteString(a.File + ":" + fmt.Sprintf("%02d", a.Line) + " " + a.Kind + " " + a.Value + " -> " + a.Name + "\n")
	}
	return b.String()
}

// @vanda:evidence annotation-go-func-doc
// @vanda:evidence annotation-sql-statement
// @vanda:evidence annotation-tla-definition
// @vanda:evidence annotation-txtar-comment-lines
func TestReadはアダプタごとに付いているものの名前を読み取る(t *testing.T) {
	root, files := setup(t, map[string]string{
		"x/a_test.go": `package x

// @vanda:evidence task-id-unique
// ほかの説明
func TestTaskID(t *testing.T) {}

/*
@vanda:sensor task-id-unique
*/
func (s *Store[T]) Add() {}
`,
		"e2e/add.txtar": `# @vanda:evidence add-task
exec vanda version
-- a.txt --
x
`,
		"spec/Task.tla": `---- MODULE Task ----
\* @vanda:term task
Tasks == {1, 2}

(* @vanda:claim task-id-unique *)
(* 補助の説明
   (* 入れ子 *) *)
LOCAL Unique(s, f(_)) == TRUE

\* @vanda:internal 状態の型
\* @vanda:feature add-task
Add(t) == t \* 足す

\* @vanda:claim ids
ids[t \in Tasks] == t
====
`,
		"db/schema.sql": `-- @vanda:evidence task-id-unique

-- 説明
CREATE UNIQUE INDEX task_id ON task(id);
`,
	})
	rules := []config.AnnotationRule{
		{Paths: []string{"**/*_test.go"}, Adapter: "go"},
		{Paths: []string{"e2e/*.txtar"}, Adapter: "txtar"},
		{Paths: []string{"spec/*.tla"}, Adapter: "tla"},
		{Paths: []string{"db/*.sql"}, Adapter: "sql"},
	}
	as, errs := Read(root, files, rules)
	if errs != nil {
		t.Fatal(errs)
	}
	want := `db/schema.sql:01 evidence task-id-unique -> CREATE UNIQUE INDEX task_id ON task(id);
e2e/add.txtar:01 evidence add-task -> add
spec/Task.tla:02 term task -> Tasks
spec/Task.tla:05 claim task-id-unique -> Unique
spec/Task.tla:10 internal 状態の型 -> Add
spec/Task.tla:11 feature add-task -> Add
spec/Task.tla:14 claim ids -> ids
x/a_test.go:03 evidence task-id-unique -> TestTaskID
x/a_test.go:08 sensor task-id-unique -> Store.Add
`
	if got := show(as); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// @vanda:evidence annotation-syntax-errors
// @vanda:evidence annotation-go-func-doc
// @vanda:evidence annotation-tla-definition
// @vanda:evidence annotation-one-rule-per-file
// @vanda:evidence annotation-adapter-required
func TestReadは書き方と置き場所の誤りを行の付いたエラーで全部返す(t *testing.T) {
	root, files := setup(t, map[string]string{
		"x/a_test.go": `package x

// @vanda:evidence a b
func TestA(t *testing.T) {}

// @vanda:claimx a
func TestB(t *testing.T) {}

// @vanda:evidence a

func TestC(t *testing.T) {}

var s = "@vanda:evidence a"

// @vanda:internal
func TestD(t *testing.T) {}

// @vanda:sensor
// @vanda:term a @vanda:term b
func TestE(t *testing.T) {}
`,
		"e2e/a.txtar": "-- a.txt --\n# @vanda:evidence a\n",
		"spec/A.tla": `---- MODULE A ----
X == "\* @vanda:term a"
\* @vanda:term a
VARIABLE x
\* @vanda:term b
`,
		"db/a.sql":   "CREATE TABLE t (id int); /* @vanda:evidence a */\n",
		"other/a.md": "<!-- @vanda:evidence a -->\n",
	})
	rules := []config.AnnotationRule{
		{Paths: []string{"**/*_test.go"}, Adapter: "go", Line: 3},
		{Paths: []string{"e2e/*.txtar"}, Adapter: "txtar", Line: 7},
		{Paths: []string{"spec/*.tla"}, Adapter: "tla", Line: 9},
		{Paths: []string{"db/*.sql", "**/a.sql"}, Adapter: "sql", Line: 11},
		{Paths: []string{"other/*.md"}, Line: 13},
		{Paths: []string{"db/*"}, Adapter: "sql", Line: 15},
	}
	_, errs := Read(root, files, rules)
	want := [][]string{
		{"x/a_test.go:3: ", "@vanda:evidence", "ID が 2 つ以上"},
		{"x/a_test.go:6: ", "@vanda:claimx", "決まった種類"},
		{"x/a_test.go:9: ", "アノテーション", "func の宣言の直前のコメント"},
		{"x/a_test.go:15: ", "@vanda:internal", "理由"},
		{"x/a_test.go:18: ", "@vanda:sensor", "ID"},
		{"x/a_test.go:19: ", "1 行に @vanda: が 2 つ以上"},
		{"spec/A.tla:3: ", "アノテーションの次の行（4 行目）", "Name ==・Name(…) ==・Name[…] == の定義"},
		{"spec/A.tla:5: ", "アノテーションの後に", "定義"},
		{"db/a.sql: ", ".vanda/config.toml の [[annotation]]（11 行目・15 行目）", "2 つ以上のパターン"},
		{"other/a.md: ", ".vanda/config.toml の 13 行目の [[annotation]] のパターン", "アダプタ"},
	}
	got := errs.Error()
	for _, w := range want {
		if !lineWith(got, w[0], w[1:]...) {
			t.Errorf("エラーに、%q で始まり %q を含む行がない:\n%s", w[0], w[1:], got)
		}
	}
	if n := strings.Count(got, "\n") + 1; n != len(want) {
		t.Errorf("エラーの数 %d, want %d:\n%s", n, len(want), got)
	}
}

// @vanda:evidence annotation-outside-comment-ignored
func TestReadはその言語のコメントの外にあるアノテーションを数えない(t *testing.T) {
	root, files := setup(t, map[string]string{
		"x/a_test.go": "package x\n\nvar s = \"@vanda:evidence a b\"\nvar r = `\n// @vanda:claimx\n`\n",
		"e2e/a.txtar": "# 説明\n-- .vanda/promises.toml --\n# @vanda:evidence a b\n",
		"spec/A.tla":  "---- MODULE A ----\nX == \"\\* @vanda:term a b\"\n====\n",
		"db/a.sql":    "CREATE TABLE t (id int); /* @vanda:evidence a b */\nSELECT '-- @vanda:evidence a b';\n",
	})
	rules := []config.AnnotationRule{
		{Paths: []string{"**/*_test.go"}, Adapter: "go"},
		{Paths: []string{"e2e/*.txtar"}, Adapter: "txtar"},
		{Paths: []string{"spec/*.tla"}, Adapter: "tla"},
		{Paths: []string{"db/*.sql"}, Adapter: "sql"},
	}
	as, errs := Read(root, files, rules)
	if errs != nil || len(as) != 0 {
		t.Errorf("as = %v, errs = %v", as, errs)
	}
}

// 作業ツリーのファイルには書きかけを置き、読む中身（read が返すもの。審査では HEAD のコミットの中身）と食い違わせる。
// アダプタは受け取った標準入力を in.json に書き写すだけにし、JSON の形と中身はテストの側で確かめる
//
// @vanda:evidence project-adapter-reads-committed-content
func TestReadはvanda_adaptersのアダプタにJSONで場所と読む中身と行を渡し名前を受け取る(t *testing.T) {
	const committed = "x\n<!-- @vanda:evidence a -->\n<!-- @vanda:evidence b -->\n"
	root, files := setup(t, map[string]string{
		"docs/a.md":  "書きかけ\n\n\n\n<!-- @vanda:evidence c -->\n",
		"adapter.sh": "#!/bin/sh\ncat > in.json\necho '{\"names\":[{\"line\":2,\"name\":\"A\"},{\"line\":3,\"name\":\"B\"}]}'\n",
	})
	read := func(file string) ([]byte, error) {
		if file == "docs/a.md" {
			return []byte(committed), nil
		}
		return os.ReadFile(filepath.Join(root, filepath.FromSlash(file)))
	}
	as, errs := ReadWith(root, read, files, []config.AnnotationRule{{Paths: []string{"docs/*.md"}, User: &config.UserAdapter{Name: "adapter", Path: "adapter.sh"}}})
	if errs != nil {
		t.Fatal(errs)
	}
	if got := show(as); got != "docs/a.md:02 evidence a -> A\ndocs/a.md:03 evidence b -> B\n" {
		t.Errorf("got:\n%s", got)
	}
	raw, err := os.ReadFile(filepath.Join(root, "in.json"))
	if err != nil {
		t.Fatal(err)
	}
	var in map[string]any
	if err := json.Unmarshal(raw, &in); err != nil {
		t.Fatalf("標準入力が JSON でない: %v\n%s", err, raw)
	}
	want := map[string]any{
		"path":        "docs/a.md",
		"content":     committed,
		"annotations": []any{map[string]any{"line": float64(2)}, map[string]any{"line": float64(3)}},
	}
	if !reflect.DeepEqual(in, want) {
		t.Errorf("標準入力:\n%s\nwant: %v", raw, want)
	}
}

// @vanda:evidence project-adapter-failure-reason
// @vanda:evidence project-adapter-response-errors
func TestReadはvanda_adaptersのアダプタの失敗と返し漏れを誤りにする(t *testing.T) {
	root, files := setup(t, map[string]string{
		"docs/a.md":  "<!-- @vanda:evidence a -->\n<!-- @vanda:evidence b -->\n",
		"docs/b.md":  "<!-- @vanda:evidence a -->\n",
		"partial.sh": "#!/bin/sh\ncat >/dev/null\necho '{\"names\":[{\"line\":1,\"name\":\"A\"},{\"line\":5,\"name\":\"X\"}]}'\n",
		"fail.sh":    "#!/bin/sh\ncat >/dev/null\necho 'わからない' >&2\nexit 2\n",
	})
	_, errs := Read(root, files, []config.AnnotationRule{
		{Paths: []string{"docs/a.md"}, User: &config.UserAdapter{Name: "partial", Path: "partial.sh"}},
		{Paths: []string{"docs/b.md"}, User: &config.UserAdapter{Name: "fail", Path: "fail.sh"}},
	})
	want := [][]string{
		// アダプタは名前で指す（指し方の細かい文言は決まっていないので、名前が入ることだけを見る）
		{"docs/a.md: ", "partial", "渡していない 5 行目"},
		{"docs/a.md:2: ", "partial", "答えを返しませんでした"},
		{"docs/b.md: ", "fail", "（わからない）"},
	}
	got := errs.Error()
	for _, w := range want {
		if !lineWith(got, w[0], w[1:]...) {
			t.Errorf("エラーに、%q で始まり %q を含む行がない:\n%s", w[0], w[1:], got)
		}
	}
	if n := strings.Count(got, "\n") + 1; n != len(want) {
		t.Errorf("エラーの数 %d, want %d:\n%s", n, len(want), got)
	}
}

// @vanda:evidence annotation-kinds-per-builtin-adapter
func TestReadは言語ごとに決まった種類のほかを誤りにする(t *testing.T) {
	root, files := setup(t, map[string]string{
		"x/a_test.go": "package x\n\n// @vanda:claim a\nfunc TestA(t *testing.T) {}\n",
		"e2e/a.txtar": "# @vanda:sensor a\n-- a --\n",
		"spec/A.tla":  "\\* @vanda:evidence a\nX == 1\n",
		"db/a.sql":    "-- @vanda:term a\nCREATE TABLE t (id int);\n",
		"docs/a.md":   "<!-- @vanda:term a -->\n",
		"adapter.sh":  "#!/bin/sh\ncat >/dev/null\necho '{\"names\":[{\"line\":1,\"name\":\"A\"}]}'\n",
	})
	rules := []config.AnnotationRule{
		{Paths: []string{"**/*_test.go"}, Adapter: "go"},
		{Paths: []string{"e2e/*.txtar"}, Adapter: "txtar"},
		{Paths: []string{"spec/*.tla"}, Adapter: "tla"},
		{Paths: []string{"db/*.sql"}, Adapter: "sql"},
		{Paths: []string{"docs/*.md"}, User: &config.UserAdapter{Name: "adapter", Path: "adapter.sh"}},
	}
	as, errs := Read(root, files, rules)
	want := [][]string{
		{"db/a.sql:1: ", "@vanda:term", "sql のアダプタのファイル", "書けるのは evidence"},
		{"e2e/a.txtar:1: ", "@vanda:sensor", "txtar のアダプタのファイル", "書けるのは evidence"},
		{"spec/A.tla:1: ", "@vanda:evidence", "tla のアダプタのファイル", "書けるのは term・feature・claim・internal"},
		{"x/a_test.go:3: ", "@vanda:claim", "go のアダプタのファイル", "書けるのは evidence・sensor"},
	}
	got := errs.Error()
	if len(strings.Split(got, "\n")) != len(want) {
		t.Errorf("エラーの数が違う:\n%s", got)
	}
	for _, w := range want {
		if !lineWith(got, w[0], w[1:]...) {
			t.Errorf("エラーに、%q で始まり %q を含む行がない:\n%s", w[0], w[1:], got)
		}
	}
	if len(as) != 1 || as[0].File != "docs/a.md" {
		t.Errorf("カスタムアダプタでは、どの種類も書ける: %v", as)
	}
}

// lineWith は、got の行のうち、prefix で始まり、frags をこの順に含むものがあるかを返す。
// 誤りの文は、です・ます調などの文体の細部に頼らず、骨組み（場所と行）と、ID・意味の要の語で照合する。
func lineWith(got, prefix string, frags ...string) bool {
	for _, line := range strings.Split(got, "\n") {
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		rest, ok := line[len(prefix):], true
		for _, f := range frags {
			i := strings.Index(rest, f)
			if i < 0 {
				ok = false
				break
			}
			rest = rest[i+len(f):]
		}
		if ok {
			return true
		}
	}
	return false
}

// ---- 約束の文から作ったテスト（docs/plans/self-hosting.md・evidence.md の見直しで足した） ----

// @vanda:evidence annotation-backquoted-ignored
func TestReadはコメントの中の逆引用符で囲んだ書き方を数えず1行に2つあるかを見るときも数えない(t *testing.T) {
	root, files := setup(t, map[string]string{
		"x/a_test.go": "package x\n\n// 書き方は `@vanda:foo a b` のとおり\ntype T struct{}\n\n// @vanda:evidence c1\n// `@vanda:evidence c2` とは書かない\nfunc TestA(t *testing.T) {}\n",
		"e2e/a.txtar": "# @vanda:evidence c1\n# 説明: `@vanda:foo a b`\nexec echo\n-- a --\n",
		"spec/A.tla":  "---- MODULE A ----\n\\* 説明: `@vanda:foo a b`\n(* @vanda:internal 理由。`@vanda:term` は付けない *)\nX == 1\n====\n",
		"db/a.sql":    "-- @vanda:evidence c1\n-- 説明: `@vanda:foo a b`\nCREATE TABLE t (id int);\n",
		"w/a.test.ts": "import { test } from \"vitest\";\n\n// 説明: `@vanda:foo a b`\nconst x = 1;\n// @vanda:evidence c1\ntest(\"t\", () => {});\n",
	})
	rules := []config.AnnotationRule{
		{Paths: []string{"**/*_test.go"}, Adapter: "go"},
		{Paths: []string{"e2e/*.txtar"}, Adapter: "txtar"},
		{Paths: []string{"spec/*.tla"}, Adapter: "tla"},
		{Paths: []string{"db/*.sql"}, Adapter: "sql"},
		{Paths: []string{"w/*.test.ts"}, Adapter: "ts"},
	}
	as, errs := Read(root, files, rules)
	if errs != nil {
		t.Fatalf("逆引用符で囲んだものを数えた: %v", errs)
	}
	if len(as) != 5 {
		t.Errorf("読んだアノテーション（逆引用符の外のものだけ、5 つ）: %s", show(as))
	}
	// 逆引用符を外すと、同じ所で数える（上が読み飛ばしでないことの確かめ）
	root, files = setup(t, map[string]string{"x/a_test.go": "package x\n\n// 書き方は @vanda:foo a b のとおり\ntype T struct{}\n"})
	if _, errs := Read(root, files, rules[:1]); errs == nil {
		t.Error("逆引用符で囲まない @vanda: を数えていない")
	}
}

// @vanda:evidence annotation-txtar-comment-lines
func TestReadはtxtarの最初の区切りより前でもシャープで始まらない命令の行を読まない(t *testing.T) {
	root, files := setup(t, map[string]string{"e2e/add.txtar": "# @vanda:evidence c1\nexec echo '@vanda:foo two ids'\n! exec false @vanda:evidence c2\n-- a --\n@vanda:evidence c3\n"})
	as, errs := Read(root, files, []config.AnnotationRule{{Paths: []string{"e2e/*.txtar"}, Adapter: "txtar"}})
	if errs != nil {
		t.Fatal(errs)
	}
	if got := show(as); got != "e2e/add.txtar:01 evidence c1 -> add\n" {
		t.Errorf("got:\n%s", got)
	}
}

// tsCases は、組込みの ts のアダプタの書ける所と、付いているものの名前（計画 annotation-types.md の 2）。
//
// @vanda:evidence annotation-ts-test-call
// @vanda:evidence annotation-ts-fixed-name
func TestReadはtsのテストの呼び出しの直前だけを受け付け名前を最後の呼び出しの最初の引数から外側のdescribeとつなぐ(t *testing.T) {
	src := "import { describe, it, test } from \"vitest\";\n\n" +
		"// @vanda:evidence c1\ntest(\"外\", () => {});\n\n" +
		"describe(\"画面\", () => {\n" +
		"  // @vanda:evidence c2\n  it(\"出す\", () => {});\n" +
		"  test.describe(\"中\", () => {\n" +
		"    /* @vanda:evidence c3 */\n    test(\"飛ばさない\", () => {});\n" +
		"    // @vanda:evidence c4\n    test.each([1, 2])(\"番号 %i\", (n) => {});\n" +
		"    // @vanda:evidence c5\n    it(`${op} をする`, () => {});\n" +
		"  });\n});\n" +
		"const s = \"@vanda:foo two ids\";\nconst u = `@vanda:evidence no ${s}`;\n"
	root, files := setup(t, map[string]string{"w/a.test.ts": src})
	as, errs := Read(root, files, []config.AnnotationRule{{Paths: []string{"w/*.test.ts"}, Adapter: "ts"}})
	if errs != nil {
		t.Fatal(errs)
	}
	var names []string
	for _, a := range as {
		names = append(names, a.Value+" "+a.Name)
	}
	want := []string{"c1 外", "c2 画面 > 出す", "c3 画面 > 中 > 飛ばさない", "c4 画面 > 中 > 番号 %i", "c5 画面 > 中 > ${op} をする"}
	if strings.Join(names, "\n") != strings.Join(want, "\n") {
		t.Errorf("got:\n%s\nwant:\n%s", strings.Join(names, "\n"), strings.Join(want, "\n"))
	}
}

// @vanda:evidence annotation-ts-fixed-name
// @vanda:evidence annotation-ts-hooks-rejected
// @vanda:evidence annotation-ts-test-call
// @vanda:evidence annotation-kinds-per-builtin-adapter
func TestReadはtsのフックとstepと変数の名前とテストでない呼び出しの前のアノテーションを誤りにする(t *testing.T) {
	cases := map[string]string{
		"beforeEach":     "// @vanda:evidence c1\nbeforeEach(() => {});\n",
		"afterEach":      "// @vanda:evidence c1\nafterEach(() => {});\n",
		"beforeAll":      "// @vanda:evidence c1\nbeforeAll(() => {});\n",
		"afterAll":       "// @vanda:evidence c1\nafterAll(() => {});\n",
		"step":           "test(\"t\", async () => {\n// @vanda:evidence c1\nawait test.step(\"s\", async () => {});\n});\n",
		"変数の名前":          "const n = \"t\";\n// @vanda:evidence c1\ntest(n, () => {});\n",
		"変数の宣言":          "// @vanda:evidence c1\nconst n = 1;\n",
		"空行をはさむ":         "// @vanda:evidence c1\n\ntest(\"t\", () => {});\n",
		"expect":         "test(\"t\", () => {\n// @vanda:evidence c1\nexpect(1).toBe(1);\n});\n",
		"evidenceのほかの種類": "// @vanda:claim c1\ntest(\"t\", () => {});\n",
	}
	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			root, files := setup(t, map[string]string{"w/a.test.ts": src})
			if _, errs := Read(root, files, []config.AnnotationRule{{Paths: []string{"w/*.test.ts"}, Adapter: "ts"}}); errs == nil || !strings.HasPrefix(errs.Error(), "w/a.test.ts:") {
				t.Errorf("誤りにならない、または場所がない: %v", errs)
			}
		})
	}
}

// @vanda:evidence project-adapter-args-and-cwd
func TestReadはvanda_adaptersのアダプタをリポジトリの根でargsをそのまま渡して動かす(t *testing.T) {
	root, files := setup(t, map[string]string{
		"docs/a.md": "<!-- @vanda:evidence a -->\n",
		// 作業のディレクトリにある印のファイルと、引数をそのまま名前に入れて返す
		"mark.txt": "根",
		"ad.sh":    "#!/bin/sh\ncat >/dev/null\nm=$(cat mark.txt)\nprintf '{\"names\":[{\"line\":1,\"name\":\"%s|%s|%s\"}]}' \"$m\" \"$1\" \"$2\"\n",
	})
	sub := filepath.Join(root, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(sub)
	as, errs := Read(root, files, []config.AnnotationRule{{Paths: []string{"docs/*.md"}, User: &config.UserAdapter{Name: "ad", Path: "ad.sh", Args: []string{"--a b", "c"}}}})
	if errs != nil {
		t.Fatal(errs)
	}
	if len(as) != 1 || as[0].Name != "根|--a b|c" {
		t.Errorf("作業のディレクトリか引数が違う: %s", show(as))
	}
}

// @vanda:evidence project-adapter-not-annotation
// @vanda:evidence project-adapter-response-errors
func TestReadはvanda_adaptersのアダプタの答えでannotationがfalseの行を数えず誤った答えを誤りにする(t *testing.T) {
	ans := func(json string) string {
		return "#!/bin/sh\ncat >/dev/null\necho '" + json + "'\n"
	}
	run := func(json string) ([]Annotation, error) {
		root, files := setup(t, map[string]string{"docs/a.md": "@vanda:evidence a b\n@vanda:evidence c\n", "ad.sh": ans(json)})
		as, errs := Read(root, files, []config.AnnotationRule{{Paths: []string{"docs/*.md"}, User: &config.UserAdapter{Name: "ad", Path: "ad.sh"}}})
		if errs != nil {
			return as, errs
		}
		return as, nil
	}
	// 1 行目は書き方が誤り（ID が 2 つ）だが、annotation: false なので数えず確かめない
	as, err := run(`{"names":[{"line":1,"annotation":false},{"line":2,"name":"C"}]}`)
	if err != nil || len(as) != 1 || as[0].Value != "c" {
		t.Errorf("annotation: false の行: %v %s", err, show(as))
	}
	// 名前を返した行は、書き方を確かめる
	if _, err := run(`{"names":[{"line":1,"name":"A"},{"line":2,"name":"C"}]}`); err == nil || !strings.Contains(err.Error(), "docs/a.md:1: ") {
		t.Errorf("名前を返した行の書き方の誤りが出ない: %v", err)
	}
	for name, json := range map[string]string{
		"渡していない行":          `{"names":[{"line":1,"annotation":false},{"line":2,"name":"C"},{"line":5,"name":"X"}]}`,
		"行を返さない":           `{"names":[{"line":1,"annotation":false}]}`,
		"空の名前":             `{"names":[{"line":1,"annotation":false},{"line":2,"name":""}]}`,
		"同じ行に2つ":           `{"names":[{"line":1,"annotation":false},{"line":2,"name":"C"},{"line":2,"name":"D"}]}`,
		"名前とannotationの両方": `{"names":[{"line":1,"annotation":false},{"line":2,"name":"C","annotation":false}]}`,
		"annotationにtrue":  `{"names":[{"line":1,"annotation":false},{"line":2,"annotation":true}]}`,
		"決まっていない項目":        `{"names":[{"line":1,"annotation":false},{"line":2,"name":"C","note":"x"}]}`,
		"どちらもない":           `{"names":[{"line":1,"annotation":false},{"line":2}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := run(json); err == nil || !strings.Contains(err.Error(), "docs/a.md") {
				t.Errorf("誤りにならない: %v", err)
			}
		})
	}
}

// @vanda:evidence project-adapter-not-runnable
func TestReadは1行目に動かし方がないvanda_adaptersのアダプタを場所と直し方の誤りにする(t *testing.T) {
	root, files := setup(t, map[string]string{"docs/a.md": "@vanda:evidence a\n", "ad.sh": "cat >/dev/null\necho '{\"names\":[{\"line\":1,\"name\":\"A\"}]}'\n"})
	_, errs := Read(root, files, []config.AnnotationRule{{Paths: []string{"docs/*.md"}, User: &config.UserAdapter{Name: "ad", Path: "ad.sh"}}})
	if errs == nil || !strings.Contains(errs.Error(), "ad.sh") || !strings.Contains(errs.Error(), "ください") {
		t.Errorf("1 行目に動かし方がないアダプタが、場所と直し方の誤りにならない: %v", errs)
	}
}

// @vanda:evidence annotation-ts-skipped-not-counted
func TestReadはtsのskipとtodoを付けた呼び出しとその中のアノテーションを数えない(t *testing.T) {
	for _, callee := range []string{"test", "it", "describe"} {
		for _, mod := range []string{"skip", "todo"} {
			t.Run(callee+"."+mod, func(t *testing.T) {
				// 飛ばす呼び出しの前（describe ならその中のテストにも）アノテーションを付け、外のテストのものだけが残ることを確かめる。
				// テストの中にテストを書くことはテストの道具が許さないので、中の呼び出しは describe でだけ確かめる
				body := "() => {}"
				if callee == "describe" {
					body = "() => {\n  // @vanda:evidence c-in\n  test(\"中\", () => {});\n}"
				}
				src := "// @vanda:evidence c-out\ntest(\"外\", () => {});\n\n" +
					"// @vanda:evidence c-skip\n" + callee + "." + mod + "(\"飛ばす\", " + body + ");\n"
				root, files := setup(t, map[string]string{"w/a.test.ts": src})
				as, errs := Read(root, files, []config.AnnotationRule{{Paths: []string{"w/*.test.ts"}, Adapter: "ts"}})
				if errs != nil {
					t.Fatal(errs)
				}
				if got := show(as); got != "w/a.test.ts:01 evidence c-out -> 外\n" {
					t.Errorf("got:\n%s", got)
				}
			})
		}
	}
}

// @vanda:evidence annotation-ts-only-rejected
func TestReadはtsのonlyを付けた呼び出しをアノテーションの有無によらずその行を指して誤りにする(t *testing.T) {
	for _, callee := range []string{"test", "it", "describe"} {
		for name, src := range map[string]string{
			"アノテーションあり": "// @vanda:evidence c1\ntest(\"外\", () => {});\n\n// @vanda:evidence c2\n" + callee + ".only(\"だけ\", () => {});\n",
			"アノテーションなし": "// @vanda:evidence c1\ntest(\"外\", () => {});\n\n\n" + callee + ".only(\"だけ\", () => {});\n",
		} {
			t.Run(callee+"."+name, func(t *testing.T) {
				root, files := setup(t, map[string]string{"w/a.test.ts": src})
				_, errs := Read(root, files, []config.AnnotationRule{{Paths: []string{"w/*.test.ts"}, Adapter: "ts"}})
				if errs == nil || !lineWith(errs.Error(), "w/a.test.ts:5: ", ".only") {
					t.Errorf("5 行目の .only を誤りにしていない: %v", errs)
				}
			})
		}
	}
}

// アダプタは標準入力を読んだ後、止まったままになる。待つのは timeout の 1 秒で、sleep の 30 秒は待たない
//
// @vanda:evidence project-adapter-annotation-timeout
func TestReadはvanda_adaptersのアダプタがtimeoutを過ぎたらどのアダプタかを書いたタイムアウトの誤りにする(t *testing.T) {
	root, files := setup(t, map[string]string{
		"docs/a.md": "<!-- @vanda:evidence a -->\n",
		"slow.sh":   "#!/bin/sh\ncat >/dev/null\nsleep 30\n",
	})
	start := time.Now()
	_, errs := Read(root, files, []config.AnnotationRule{{Paths: []string{"docs/*.md"}, User: &config.UserAdapter{Name: "slow", Path: "slow.sh"}, Timeout: time.Second}})
	if d := time.Since(start); d > 10*time.Second {
		t.Errorf("timeout の 1 秒で止めず、%v 待った", d)
	}
	if errs == nil || !lineWith(errs.Error(), "docs/a.md: ", "slow", "タイムアウト") {
		t.Errorf("どのアダプタかを書いたタイムアウトの誤りでない: %v", errs)
	}
}
