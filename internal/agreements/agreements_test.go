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

// 期待値の指紋は、手で書いた JSON を sha256sum に通して作った
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
			t.Errorf("書き方 %d の指紋: %s, want %s", i, got, want)
		}
	}
}

func TestItemFingerprintは約束のエビデンスを含めフラグを除く(t *testing.T) {
	const want = "sha256:e6b2ed903b3426210fb3ef2b666199300db863e4ee9ca95bb0ca12f6ecef6f62"
	root, files := writeFiles(t, map[string]string{".vanda/promises.toml": `[[claim]]
id = "c"
subjects = ["task"]
statement = "s"
layers = ["model"]
flags = ["ai-proposal"]
guesses = ["g"]

[[claim.evidence]]
type = "unit-test"
stage = "test"
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
		t.Error("name が違うのに指紋が同じ")
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

func TestFileFingerprintは中身そのものから作る(t *testing.T) {
	if got := FileFingerprint([]byte("hello\n")); got != "sha256:5891b5b522d5df086d0ff0b110fbd9d21bb4fc7163af34d08286a2e846f6be03" {
		t.Errorf("got %s", got)
	}
}

const fpA = "sha256:5891b5b522d5df086d0ff0b110fbd9d21bb4fc7163af34d08286a2e846f6be03"
const fpB = "sha256:ead0b1ff85473e3117de7dfb1514f5d4d749cc30a0eb431c015014c47a59bbe5"

func TestAgreedは種類と鍵と指紋がすべて合う合意があるときだけ真(t *testing.T) {
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
	r, errs := Load(root, files, []string{".vanda/agreements.toml"}, false)
	if errs != nil {
		t.Fatal(errs)
	}
	cases := []struct {
		name          string
		kind, key, fp string
		want          bool
	}{
		{"すべて合えば合意済", "claim", "c", fpA, true},
		{"指紋が違えば提案中", "claim", "c", fpB, false},
		{"鍵が違えば提案中", "claim", "d", fpA, false},
		{"種類が違えば提案中", "term", "c", fpA, false},
		{"ファイルは指紋を鍵にして合意済", "file", fpB, fpB, true},
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

func TestLoadは合意の記録の誤りを行の付いたエラーで全部返す(t *testing.T) {
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
	_, errs := Load(root, files, []string{".vanda/agreements.toml"}, false)
	want := []string{
		`.vanda/agreements.toml:2: kind "story" は決まった値ではありません。`,
		`.vanda/agreements.toml:4: fingerprint "md5:abc" が指紋の形ではありません。`,
		`.vanda/agreements.toml:5: 対象が story の合意に、path があります。`,
		`.vanda/agreements.toml:1: agreement の 1 件目 の user がありません。`,
		`.vanda/agreements.toml:6: agreed_at に時差がありません。`,
		`.vanda/agreements.toml:9: 対象がファイルの合意に、path がありません。`,
		`.vanda/agreements.toml:11: 対象がファイルの合意の key が、fingerprint と違います。`,
		`.vanda/agreements.toml:13: agreement の 2 件目 の user の email がありません。`,
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
