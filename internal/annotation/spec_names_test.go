package annotation_test

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/TadahiroYamamura/vanda/internal/annotation"
	"github.com/TadahiroYamamura/vanda/internal/config"
)

// 期待値は docs/formats.md の「組み込みのアダプタ」の表から、例ごとに手で書いた。

type want struct {
	Line  int
	Kind  string
	Value string
	Name  string
}

func readOne(t *testing.T, path, src, adapter string) ([]want, []string) {
	t.Helper()
	root := t.TempDir()
	abs := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	rules := []config.AnnotationRule{{Paths: []string{path}, Adapter: adapter}}
	anns, errs := annotation.Read(root, []string{path}, rules)
	var got []want
	for _, a := range anns {
		if a.File != path {
			t.Errorf("File = %q, want %q", a.File, path)
		}
		got = append(got, want{a.Line, a.Kind, a.Value, a.Name})
	}
	var msgs []string
	for _, e := range errs {
		msgs = append(msgs, e.Error())
	}
	return got, msgs
}

func checkRead(t *testing.T, path, src, adapter string, expected []want) {
	t.Helper()
	got, errs := readOne(t, path, src, adapter)
	if len(errs) > 0 {
		t.Fatalf("エラーが出た:\n%s", strings.Join(errs, "\n"))
	}
	if !slices.Equal(got, expected) {
		t.Fatalf("読んだもの:\n got  %+v\n want %+v", got, expected)
	}
}

func checkError(t *testing.T, path, src, adapter string, line int) {
	t.Helper()
	_, errs := readOne(t, path, src, adapter)
	if len(errs) == 0 {
		t.Fatalf("エラーが出ない")
	}
	prefix := path + ":" + strconv.Itoa(line) + ": "
	for _, e := range errs {
		if strings.HasPrefix(e, prefix) {
			return
		}
	}
	t.Fatalf("%q で始まるエラーがない:\n%s", prefix, strings.Join(errs, "\n"))
}

// @vanda:evidence annotation-go-func-doc
func TestGoのアノテーションは次のfuncの名前に付きメソッドは型と名前をつなぐ(t *testing.T) {
	src := `package task

import "testing"

// @vanda:evidence task-id-unique
func TestAddKeepsIdsUnique(t *testing.T) {}

type Store struct{}

// Add はタスクを足す。
//
// @vanda:sensor task-id-unique
// @vanda:sensor task-title-required
func (s *Store) Add(id string) error { return nil }

// @vanda:sensor task-id-unique
func (s Store) Len() int { return 0 }

// @vanda:sensor task-id-unique
func helper() {}
`
	checkRead(t, "task_test.go", src, "go", []want{
		{5, "evidence", "task-id-unique", "TestAddKeepsIdsUnique"},
		{12, "sensor", "task-id-unique", "Store.Add"},
		{13, "sensor", "task-title-required", "Store.Add"},
		{16, "sensor", "task-id-unique", "Store.Len"},
		{19, "sensor", "task-id-unique", "helper"},
	})
}

// @vanda:evidence annotation-txtar-comment-lines
func Test_txtarのアノテーションはファイルからtxtarを除いた名前に付く(t *testing.T) {
	src := `# タスクを足すと、ID が重ならない
# @vanda:evidence task-id-unique
# @vanda:evidence task-title-required
exec vanda add
-- want.txt --
ok
`
	checkRead(t, "e2e/task/add-task.txtar", src, "txtar", []want{
		{2, "evidence", "task-id-unique", "add-task"},
		{3, "evidence", "task-title-required", "add-task"},
	})
}

// @vanda:evidence annotation-txtar-comment-lines
func Test_txtarの最初の区切りより後のアノテーションは中のファイルの中身なので数えず誤りにもしない(t *testing.T) {
	src := `# @vanda:evidence task-id-unique
exec vanda add
-- want.txt --
# @vanda:evidence not-counted
# @vanda:foo two ids
ok
`
	checkRead(t, "e2e/add-task.txtar", src, "txtar", []want{
		{1, "evidence", "task-id-unique", "add-task"},
	})
}

// @vanda:evidence annotation-tla-definition
func TestTLAのアノテーションは後の空行でもコメントでもない最初の行の定義の名前に付く(t *testing.T) {
	src := `---- MODULE Task ----
\* @vanda:term task
TaskIds == {"t1", "t2"}

(* @vanda:internal 検査のための小さな数の集合 *)
Small == {0, 1, 2}

\* @vanda:feature task-add

\* 足す操作
(* 2 行目の説明 *)
Add(t, u) == t

\* @vanda:claim task-id-unique
LOCAL Unique == TRUE

(* @vanda:claim task-done *)
Done == TRUE
====
`
	checkRead(t, "spec/Task.tla", src, "tla", []want{
		{2, "term", "task", "TaskIds"},
		{5, "internal", "検査のための小さな数の集合", "Small"},
		{8, "feature", "task-add", "Add"},
		{14, "claim", "task-id-unique", "Unique"},
		{17, "claim", "task-done", "Done"},
	})
}

func TestTLAの関数の定義に付けたアノテーションは関数の名前に付く(t *testing.T) {
	src := `---- MODULE Task ----
\* @vanda:claim task-title-required
Count[x \in Small] == x + 1
====
`
	checkRead(t, "spec/Task.tla", src, "tla", []want{{2, "claim", "task-title-required", "Count"}})
}

// @vanda:evidence annotation-outside-comment-ignored
func Test文字列の中の書き方はアノテーションとして数えず誤りにもしない(t *testing.T) {
	t.Run("go", func(t *testing.T) {
		checkRead(t, "task.go", "package task\n\nconst usage = \"@vanda:evidence no-such-id\"\n\nvar raw = `@vanda:foo x y`\n", "go", nil)
	})
	t.Run("tla", func(t *testing.T) {
		checkRead(t, "spec/Task.tla", "---- MODULE Task ----\nMsg == \"@vanda:claim no-such-id\"\n====\n", "tla", nil)
	})
	t.Run("sql", func(t *testing.T) {
		checkRead(t, "db/schema.sql", "INSERT INTO task VALUES ('@vanda:evidence no-such-id');\n", "sql", nil)
	})
}

// @vanda:evidence annotation-sql-statement
func TestSQLのアノテーションは後の空行でもコメントでもない最初の行の中身に付く(t *testing.T) {
	src := `-- @vanda:evidence task-id-unique

-- タスクの表
   CREATE TABLE task (id TEXT PRIMARY KEY);  	
`
	checkRead(t, "db/schema.sql", src, "sql", []want{
		{1, "evidence", "task-id-unique", "CREATE TABLE task (id TEXT PRIMARY KEY);"},
	})
}

func Test_internalの理由は空白を含めて行の終わりかコメントの閉じまでを読む(t *testing.T) {
	src := `---- MODULE Task ----
\* @vanda:internal 検査 のための 集合
A == {1}

(* @vanda:internal 閉じの前まで *)
B == {2}
====
`
	checkRead(t, "spec/Task.tla", src, "tla", []want{
		{2, "internal", "検査 のための 集合", "A"},
		{5, "internal", "閉じの前まで", "B"},
	})
}

// 言語ごとに書ける種類（監督の決定）: Go は evidence・sensor、txtar は evidence、TLA+ は term・feature・claim・internal、SQL は evidence。
// 6 つの種類 × 4 つの言語を全部回し、書けない組み合わせは誤り、書ける組み合わせは読めることを確かめる。
// @vanda:evidence annotation-kinds-per-builtin-adapter
func Test言語ごとに書ける種類だけを読みほかの組み合わせはその行で誤りにする(t *testing.T) {
	allowed := map[string][]string{
		"go":    {"evidence", "sensor"},
		"txtar": {"evidence"},
		"tla":   {"term", "feature", "claim", "internal"},
		"sql":   {"evidence"},
	}
	srcOf := map[string]func(ann string) (path, src string, line int, name string){
		"go": func(ann string) (string, string, int, string) {
			return "task.go", "package task\n\n// " + ann + "\nfunc Add() {}\n", 3, "Add"
		},
		"txtar": func(ann string) (string, string, int, string) {
			return "e2e/add.txtar", "# " + ann + "\nexec true\n", 1, "add"
		},
		"tla": func(ann string) (string, string, int, string) {
			return "spec/Task.tla", "---- MODULE Task ----\n\\* " + ann + "\nAdd == TRUE\n====\n", 2, "Add"
		},
		"sql": func(ann string) (string, string, int, string) {
			return "db/schema.sql", "-- " + ann + "\nCREATE TABLE task (id TEXT);\n", 1, "CREATE TABLE task (id TEXT);"
		},
	}
	for _, lang := range []string{"go", "txtar", "tla", "sql"} {
		for _, kind := range annotationKinds {
			t.Run(lang+"/"+kind, func(t *testing.T) {
				value := "some-id"
				if kind == "internal" {
					value = "理由"
				}
				path, src, line, name := srcOf[lang]("@vanda:" + kind + " " + value)
				if slices.Contains(allowed[lang], kind) {
					checkRead(t, path, src, lang, []want{{line, kind, value, name}})
				} else {
					checkError(t, path, src, lang, line)
				}
			})
		}
	}
}

// 種類の一覧は docs/formats.md から手で書いた（実装の annotation.Kinds から作らない）。
var annotationKinds = []string{"term", "feature", "claim", "internal", "evidence", "sensor"}
