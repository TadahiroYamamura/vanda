package main_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// CMD-10 エビデンスが揃っている審査と、[[annotation]] の types（docs/plans/annotation-types.md の 1）。
// 予定のタイプを types に含む [[annotation]] から読んだアノテーションのうち、タイプに合う種類のものがあれば揃う。
// 種類とタイプの組み合わせは、計画の表から手で書いた。

// typeKinds は、計画の表の「揃っているとするアノテーションの種類」。types に書けるタイプの全部
var typeKinds = map[string]string{
	"model-check":      "claim",
	"sensor":           "sensor",
	"scenario-test":    "evidence",
	"unit-test":        "evidence",
	"cross-check":      "evidence",
	"random-check":     "evidence",
	"interface-test":   "evidence",
	"concurrency-test": "evidence",
	"guardrail":        "evidence",
}

// アノテーションの種類の全部（docs/formats.md）
var annotationKinds = []string{"term", "feature", "claim", "internal", "evidence", "sensor"}

// anyAdapter は、渡された 1 行目のアノテーションに名前を返す .vanda/adapters/ のアダプタ（どの種類も書ける）。
const anyAdapter = "#!/bin/sh\ncat >/dev/null\nprintf '%s' '{\"names\":[{\"line\":1,\"name\":\"x\"}]}'\n"

// typesRepo は、約束 task-id-unique の予定のタイプを planned にし、ext/a.ext に kind のアノテーションを 1 つ置き、
// それを types の [[annotation]] で読むリポジトリを作る。
func typesRepo(t *testing.T, version, planned, kind, types string) string {
	t.Helper()
	root := t.TempDir()
	g := newRVGit(t, root)
	g.run("init", "-q", "-b", "main")
	writeFile(t, root, ".vanda/config.toml", fmt.Sprintf(`version = %q

[[layer]]
id = "model"
name = "モデル"
description = "判定と状態の変化"

[[annotation]]
paths = ["ext/*.ext"]
adapter = "any"
%s
`, version, types))
	writeFile(t, root, ".vanda/checks/present.toml", "[[check]]\nname = \"evidence-present\"\nscope = \"all\"\n")
	writeFile(t, root, ".vanda/promises.toml", fmt.Sprintf(`[[term]]
id = "task"
name = "タスク"
definition = "やること"

[[claim]]
id = "task-id-unique"
subjects = ["task"]
statement = "タスクの ID は重ならない"
layers = ["model"]

[[claim.evidence]]
stage = "test"
type = %q
layers = ["model"]
`, planned))
	writeFile(t, root, ".vanda/agreements.toml", "")
	writeFile(t, root, ".vanda/adapters/any", anyAdapter)
	if err := os.Chmod(filepath.Join(root, ".vanda/adapters/any"), 0o755); err != nil {
		t.Fatal(err)
	}
	ann := "@vanda:" + kind + " task-id-unique"
	if kind == "internal" {
		ann = "@vanda:internal 小さな数の集合"
	}
	writeFile(t, root, "ext/a.ext", ann+"\n")
	g.run("add", "-A")
	g.run("commit", "-q", "-m", "#1 はじめ")
	return root
}

func runCheck(t *testing.T, bin, root string, args ...string) (int, string) {
	t.Helper()
	cmd := exec.Command(bin, append([]string{"check"}, args...)...)
	cmd.Dir = root
	cmd.Env = newRVGit(t, root).env
	out, err := cmd.CombinedOutput()
	if err == nil {
		return 0, string(out)
	}
	if e, ok := err.(*exec.ExitError); ok {
		return e.ExitCode(), string(out)
	}
	t.Fatalf("vanda check を動かせない: %v", err)
	return -1, ""
}

// CMD-10: 種類 × タイプの全部の組み合わせで、揃う・揃わないが計画の表のとおりになる
func Testアノテーションの種類とエビデンスのタイプの全部の組み合わせで揃うかが表のとおりになる(t *testing.T) {
	t.Parallel()
	bin := buildVanda(t)
	version, err := runningVersion(bin)
	if err != nil {
		t.Fatal(err)
	}
	for typ, want := range typeKinds {
		for _, kind := range annotationKinds {
			t.Run(typ+"に"+kind, func(t *testing.T) {
				t.Parallel()
				root := typesRepo(t, version, typ, kind, fmt.Sprintf("types = [%q]", typ))
				code, out := runCheck(t, bin, root, ".vanda/checks/present.toml")
				if ok := kind == want; (code == 0) != ok {
					t.Errorf("予定のタイプ %s に @vanda:%s で、揃うのは %v のはずが、終了コード %d\n%s", typ, kind, ok, code, out)
				}
			})
		}
	}
}

// CMD-10: 種類が合っていても、予定のタイプを types に含まない [[annotation]] のアノテーションでは揃わない
func Test予定のタイプをtypesに含まないannotationのアノテーションでは揃わない(t *testing.T) {
	t.Parallel()
	bin := buildVanda(t)
	version, err := runningVersion(bin)
	if err != nil {
		t.Fatal(err)
	}
	for typ, kind := range typeKinds {
		t.Run(typ, func(t *testing.T) {
			t.Parallel()
			// 同じ種類で揃う、ほかのタイプだけを types に書く
			other := "guardrail"
			if typ == "guardrail" {
				other = "unit-test"
			}
			if kind != "evidence" {
				other = map[string]string{"model-check": "sensor", "sensor": "model-check"}[typ]
			}
			root := typesRepo(t, version, typ, kind, fmt.Sprintf("types = [%q]", other))
			code, out := runCheck(t, bin, root, ".vanda/checks/present.toml")
			if code == 0 {
				t.Errorf("予定のタイプ %s を types（%s）に含まないのに揃った\n%s", typ, other, out)
			}
			if !strings.Contains(out, typ) {
				t.Errorf("揃わないタイプ %s が出ていない\n%s", typ, out)
			}
		})
	}
}

// CMD-10: types は必ず書く。確かめないタイプ（human-review・monitoring）と、今は扱えない proof は書けない
func Test_typesがないときと書けないタイプを書いたときは設定の誤りになる(t *testing.T) {
	t.Parallel()
	bin := buildVanda(t)
	version, err := runningVersion(bin)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct{ name, types, want string }{
		{"typesがない", "", "types"},
		{"human-review", `types = ["human-review"]`, "human-review"},
		{"monitoring", `types = ["monitoring"]`, "monitoring"},
		{"proof", `types = ["proof"]`, "proof"},
		{"決まっていないタイプ", `types = ["smoke-test"]`, "smoke-test"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			root := typesRepo(t, version, "unit-test", "evidence", c.types)
			code, out := runCheck(t, bin, root, ".vanda/checks/present.toml")
			if code == 0 {
				t.Fatalf("設定の誤りにならない\n%s", out)
			}
			if !strings.Contains(out, ".vanda/config.toml") || !strings.Contains(out, c.want) || !strings.Contains(out, "ください。") {
				t.Errorf("誤りに、設定の場所・%s・直し方が出ていない\n%s", c.want, out)
			}
		})
	}
}

// CMD-10: types = [] は誤りにせず、どのタイプも満たさない
func Test_typesが空の配列なら設定の誤りにならずどのタイプも満たさない(t *testing.T) {
	t.Parallel()
	bin := buildVanda(t)
	version, err := runningVersion(bin)
	if err != nil {
		t.Fatal(err)
	}
	root := typesRepo(t, version, "unit-test", "evidence", "types = []")
	code, out := runCheck(t, bin, root, ".vanda/checks/present.toml")
	if code == 0 {
		t.Fatalf("types = [] の [[annotation]] のアノテーションで、unit-test が揃った\n%s", out)
	}
	if !strings.Contains(out, "審査 evidence-present") || !strings.Contains(out, "unit-test") {
		t.Errorf("設定の誤りでなく、unit-test が揃わないとして止まるはず\n%s", out)
	}
}
