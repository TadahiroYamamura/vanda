package annotation

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

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
	want := []string{
		"x/a_test.go:3: @vanda:evidence に ID が 2 つ以上ある。",
		"x/a_test.go:6: @vanda:claimx は決まった種類でない。",
		"x/a_test.go:9: アノテーションが func の宣言の直前のコメントにない。",
		"x/a_test.go:15: @vanda:internal に理由がない。",
		"x/a_test.go:18: @vanda:sensor に ID がない。",
		"x/a_test.go:19: 1 行に @vanda: が 2 つ以上ある。",
		"spec/A.tla:3: アノテーションの次の行（4 行目）が、Name ==・Name(…) ==・Name[…] == の定義でない。",
		"spec/A.tla:5: アノテーションの後に定義がない。",
		"db/a.sql: .vanda/config.toml の [[annotation]]（11 行目・15 行目）の 2 つ以上のパターンに当たる。",
		"other/a.md: .vanda/config.toml の 13 行目の [[annotation]] のパターンに当たるが、使うアダプタが決まっていない。",
	}
	got := errs.Error()
	for _, w := range want {
		if !strings.Contains(got, w) {
			t.Errorf("エラーに %q がない:\n%s", w, got)
		}
	}
	if n := strings.Count(got, "\n") + 1; n != len(want) {
		t.Errorf("エラーの数 %d, want %d:\n%s", n, len(want), got)
	}
}

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

func TestReadは外部のコマンドのアダプタとJSONで受け渡す(t *testing.T) {
	root, files := setup(t, map[string]string{
		"docs/a.md": "x\n<!-- @vanda:evidence a -->\n<!-- @vanda:evidence b -->\n",
		"adapter.sh": `#!/bin/sh
in=$(cat)
case "$in" in
  '{"path":"docs/a.md","annotations":[{"line":2},{"line":3}]}') echo '{"names":[{"line":2,"name":"A"},{"line":3,"name":"B"}]}' ;;
  *) echo "unexpected: $in" >&2; exit 3 ;;
esac
`,
	})
	as, errs := Read(root, files, []config.AnnotationRule{{Paths: []string{"docs/*.md"}, Command: []string{"./adapter.sh"}}})
	if errs != nil {
		t.Fatal(errs)
	}
	if got := show(as); got != "docs/a.md:02 evidence a -> A\ndocs/a.md:03 evidence b -> B\n" {
		t.Errorf("got:\n%s", got)
	}
}

func TestReadは外部のコマンドのアダプタの失敗と返し漏れを誤りにする(t *testing.T) {
	root, files := setup(t, map[string]string{
		"docs/a.md":  "<!-- @vanda:evidence a -->\n<!-- @vanda:evidence b -->\n",
		"docs/b.md":  "<!-- @vanda:evidence a -->\n",
		"partial.sh": "#!/bin/sh\ncat >/dev/null\necho '{\"names\":[{\"line\":1,\"name\":\"A\"},{\"line\":5,\"name\":\"X\"}]}'\n",
		"fail.sh":    "#!/bin/sh\ncat >/dev/null\necho 'わからない' >&2\nexit 2\n",
	})
	_, errs := Read(root, files, []config.AnnotationRule{
		{Paths: []string{"docs/a.md"}, Command: []string{"./partial.sh"}},
		{Paths: []string{"docs/b.md"}, Command: []string{"./fail.sh"}},
	})
	want := []string{
		"docs/a.md: アダプタ（./partial.sh）が、アノテーションのない 5 行目の名前を返した。",
		"docs/a.md:2: アダプタ（./partial.sh）が、このアノテーションの名前を返さなかった。",
		"docs/b.md: アノテーションの読み取りのアダプタ（./fail.sh）が失敗した（わからない）。",
	}
	got := errs.Error()
	for _, w := range want {
		if !strings.Contains(got, w) {
			t.Errorf("エラーに %q がない:\n%s", w, got)
		}
	}
	if n := strings.Count(got, "\n") + 1; n != len(want) {
		t.Errorf("エラーの数 %d, want %d:\n%s", n, len(want), got)
	}
}
