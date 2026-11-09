package agreements

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TadahiroYamamura/vanda/internal/promises"
)

func writeFiles(t *testing.T, files map[string]string) (string, []string) {
	t.Helper()
	root := t.TempDir()
	var names []string
	for name, body := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		names = append(names, name)
	}
	return root, names
}

// 期待値のフィンガープリントは、手で書いた JSON を sha256sum に通して作った
// @vanda:evidence fingerprint-ignores-formatting
// @vanda:evidence fingerprint-excludes-flags
func TestItemFingerprintは書き方の違いで変わらずflagsとguessesを含めない(t *testing.T) {
	const want = "sha256:ead0b1ff85473e3117de7dfb1514f5d4d749cc30a0eb431c015014c47a59bbe5"
	srcs := []string{
		"[[term]]\nid = \"task\"\nname = \"タスク\"\ndefinition = \"a<b & \\\"c\\\"\\n\"\npart_of = \"x\"\n",
		"[[term]]\n  part_of='x'\n  definition = \"\"\"\na<b & \"c\"\n\"\"\"\n  name   =   \"タスク\"\n  id = \"task\"\n",
	}
	for i, src := range srcs {
		root, files := writeFiles(t, map[string]string{".vanda/promises.toml": src})
		l, errs := promises.Load(root, files, []string{".vanda/promises.toml"}, false)
		if errs != nil {
			t.Fatal(errs)
		}
		got, err := ItemFingerprint(l.Terms[0].Raw)
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("書き方 %d のフィンガープリント: %s, want %s", i, got, want)
		}
	}
}

// @vanda:evidence fingerprint-excludes-flags
func TestItemFingerprintは約束のエビデンスを含めフラグを除く(t *testing.T) {
	const want = "sha256:8229b8e3daf4b1dd844d0abb996706e57d70d90b8737c38ed3b612cabce12195"
	root, files := writeFiles(t, map[string]string{".vanda/promises.toml": `[[claim]]
id = "c"
subjects = ["task"]
statement = "s"
layers = ["model"]
flags = ["ai-proposal"]
guesses = ["g"]

[[claim.evidence]]
type = "unit-test"
stage = "implementation"
layers = ["model"]
`})
	l, errs := promises.Load(root, files, []string{".vanda/promises.toml"}, false)
	if errs != nil {
		t.Fatal(errs)
	}
	got, err := ItemFingerprint(l.Claims[0].Raw)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestItemFingerprintは中身が変われば変わる(t *testing.T) {
	a, _ := ItemFingerprint(map[string]any{"id": "x", "name": "a"})
	b, _ := ItemFingerprint(map[string]any{"id": "x", "name": "b"})
	if a == b {
		t.Error("name が違うのにフィンガープリントが同じ")
	}
}

func TestCanonicalJSONは制御文字だけをエスケープする(t *testing.T) {
	got, err := CanonicalJSON(map[string]any{"b": "\x01\t\"\\é<", "a": []any{true, int64(-3)}})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"a":[true,-3],"b":"\u0001\t\"\\é<"}`
	if got != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

// @vanda:evidence fingerprint-file-content
func TestFileFingerprintは中身そのものから作る(t *testing.T) {
	if got := FileFingerprint([]byte("hello\n")); got != "sha256:5891b5b522d5df086d0ff0b110fbd9d21bb4fc7163af34d08286a2e846f6be03" {
		t.Errorf("got %s", got)
	}
}

const fpA = "sha256:5891b5b522d5df086d0ff0b110fbd9d21bb4fc7163af34d08286a2e846f6be03"
const fpB = "sha256:ead0b1ff85473e3117de7dfb1514f5d4d749cc30a0eb431c015014c47a59bbe5"

// @vanda:evidence changed-id-is-new
// @vanda:evidence agreed-when-fingerprint-matches
func TestAgreedは種類とキーとフィンガープリントがすべて合いファイルなら場所も合う承認があるときだけ真(t *testing.T) {
	root, files := writeFiles(t, map[string]string{".vanda/agreements.toml": `[[agreement]]
kind = "claim"
key = "c"
fingerprint = "` + fpA + `"
user = { name = "PO", email = "po@example.com" }
agreed_at = 2026-10-31T12:00:00+09:00
story = "#24"

[[agreement]]
kind = "file"
key = "` + fpB + `"
fingerprint = "` + fpB + `"
path = "e2e/a.txtar"
agreed_at = 2026-10-31T12:00:00Z
story = "#25"

[agreement.user]
name = "PO"
email = "po@example.com"
`})
	r, errs := Load(root, files, []string{".vanda/agreements.toml"})
	if errs != nil {
		t.Fatal(errs)
	}
	cases := []struct {
		name          string
		kind, key, fp string
		want          bool
	}{
		{"すべて合えば承認済", "claim", "c", fpA, true},
		{"フィンガープリントが違えば提案中", "claim", "c", fpB, false},
		{"キーが違えば提案中", "claim", "d", fpA, false},
		{"種類が違えば提案中", "term", "c", fpA, false},
		{"ファイルは場所とフィンガープリントが合えば承認済", "file", "e2e/a.txtar", fpB, true},
		{"ファイルはフィンガープリントが合っても場所が違えば提案中", "file", "e2e/copy.txtar", fpB, false},
		{"ファイルは場所が合ってもフィンガープリントが違えば提案中", "file", "e2e/a.txtar", fpA, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := r.Agreed(c.kind, c.key, c.fp); got != c.want {
				t.Errorf("Agreed = %v", got)
			}
		})
	}
	if a := r.Agreements[1]; a.Path != "e2e/a.txtar" || a.UserEmail != "po@example.com" || a.Story != "#25" {
		t.Errorf("2 件目: %+v", a)
	}
}

// @vanda:evidence agreements-file-record-path
func TestLoadは承認の記録の誤りを行の付いたエラーで全部返す(t *testing.T) {
	root, files := writeFiles(t, map[string]string{".vanda/agreements.toml": `[[agreement]]
kind = "story"
key = "c"
fingerprint = "md5:abc"
path = "x"
agreed_at = 2026-10-31T12:00:00
story = "#24"

[[agreement]]
kind = "file"
key = "` + fpA + `"
fingerprint = "` + fpB + `"
user = { name = "PO" }
agreed_at = 2026-10-31T12:00:00Z
story = "#24"
`})
	_, errs := Load(root, files, []string{".vanda/agreements.toml"})
	want := [][]string{
		{".vanda/agreements.toml:2: ", `kind "story"`, "決まった値"},
		{".vanda/agreements.toml:4: ", `fingerprint "md5:abc"`, "フィンガープリントの形"},
		{".vanda/agreements.toml:5: ", "対象が story の承認", "path"},
		{".vanda/agreements.toml:1: ", "agreement の 1 件目 の user"},
		{".vanda/agreements.toml:6: ", "agreed_at", "タイムゾーン"},
		{".vanda/agreements.toml:9: ", "対象がファイルの承認", "path"},
		{".vanda/agreements.toml:11: ", "対象がファイルの承認の key", "fingerprint"},
		{".vanda/agreements.toml:13: ", "agreement の 2 件目 の user の email"},
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

const userLine = "user = { name = \"PO\", email = \"po@example.com\" }\nagreed_at = 2026-10-31T12:00:00Z\nstory = \"#24\"\n"

// @vanda:evidence deletion-agreement-record
func TestLoadは消したことへの承認の記録をfingerprintの代わりにdeletedで読み中身への承認と分けて数える(t *testing.T) {
	root, files := writeFiles(t, map[string]string{".vanda/agreements.toml": "[[agreement]]\nkind = \"file\"\nkey = \"e2e/old.txtar\"\ndeleted = true\npath = \"e2e/old.txtar\"\n" + userLine +
		"\n[[agreement]]\nkind = \"claim\"\nkey = \"gone\"\ndeleted = true\n" + userLine})
	r, errs := Load(root, files, []string{".vanda/agreements.toml"})
	if errs != nil {
		t.Fatal(errs)
	}
	if len(r.Agreements) != 2 {
		t.Fatalf("記録の数 %d", len(r.Agreements))
	}
	if a := r.Agreements[0]; !a.Deleted || a.Fingerprint != "" || a.Kind != "file" || a.Key != "e2e/old.txtar" || a.Path != "e2e/old.txtar" || a.Story != "#24" || a.UserEmail != "po@example.com" {
		t.Errorf("ファイルを消したことへの承認の記録: %+v", a)
	}
	if a := r.Agreements[1]; !a.Deleted || a.Fingerprint != "" || a.Kind != "claim" || a.Key != "gone" || a.Path != "" {
		t.Errorf("約束を消したことへの承認の記録: %+v", a)
	}
	cases := []struct {
		name string
		got  bool
		want bool
	}{
		{"消したファイルの場所への、消したことへの承認", r.DeletionAgreed("file", "e2e/old.txtar"), true},
		{"消した約束への、消したことへの承認", r.DeletionAgreed("claim", "gone"), true},
		{"ほかの場所には、消したことへの承認がない", r.DeletionAgreed("file", "e2e/other.txtar"), false},
		{"種類が違えば、消したことへの承認がない", r.DeletionAgreed("term", "gone"), false},
		{"消したことへの承認は、中身への承認に数えない（ファイル）", r.Agreed("file", "e2e/old.txtar", fpA), false},
		{"消したことへの承認は、中身への承認に数えない（約束）", r.Agreed("claim", "gone", fpA), false},
		{"消したことへの承認は、承認したことに数えない", r.EverAgreed("claim", "gone"), false},
	}
	for _, c := range cases {
		if c.got != c.want {
			t.Errorf("%s: %v（思ったもの %v）", c.name, c.got, c.want)
		}
	}
}

// @vanda:evidence deletion-agreement-record
// @vanda:evidence agreements-file-record-path
func TestLoadは消したことへの承認の記録の形の誤りを行の付いたエラーで返す(t *testing.T) {
	cases := []struct {
		name, body string
		line       string
		frags      []string
	}{
		{"ファイルの記録の key が path と違う", "[[agreement]]\nkind = \"file\"\nkey = \"e2e/a.txtar\"\ndeleted = true\npath = \"e2e/b.txtar\"\n" + userLine, ":3: ", []string{"key"}},
		{"ファイルの記録の key がフィンガープリント", "[[agreement]]\nkind = \"file\"\nkey = \"" + fpA + "\"\ndeleted = true\npath = \"e2e/a.txtar\"\n" + userLine, ":3: ", []string{"key"}},
		{"fingerprint と deleted の両方がある", "[[agreement]]\nkind = \"claim\"\nkey = \"c\"\nfingerprint = \"" + fpA + "\"\ndeleted = true\n" + userLine, "", []string{"deleted"}},
		{"deleted が false", "[[agreement]]\nkind = \"claim\"\nkey = \"c\"\ndeleted = false\n" + userLine, ":4: ", []string{"deleted"}},
		{"約束の記録に path がある", "[[agreement]]\nkind = \"claim\"\nkey = \"c\"\ndeleted = true\npath = \"x\"\n" + userLine, ":5: ", []string{"path"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root, files := writeFiles(t, map[string]string{".vanda/agreements.toml": c.body})
			_, errs := Load(root, files, []string{".vanda/agreements.toml"})
			if errs == nil {
				t.Fatalf("誤りにならない:\n%s", c.body)
			}
			if got := errs.Error(); !lineWith(got, ".vanda/agreements.toml"+c.line, c.frags...) {
				t.Errorf("エラーに、.vanda/agreements.toml%s で始まり %q を含む行がない:\n%s", c.line, c.frags, got)
			}
		})
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
