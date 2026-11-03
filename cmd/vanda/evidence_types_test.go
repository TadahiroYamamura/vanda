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

// typeKinds は、計画の表の「揃っているとするアノテーションの種類」。types に書けるタイプの全部。
// guardrail と sensor は、2 つのアノテーションが揃ったときだけ揃う（約束 check-evidence-guardrail-needs-definition・
// check-evidence-sensor-needs-check）ので、1 つのアノテーションだけを置く Testアノテーションの種類と… では、どの種類でも揃わない。
// 2 つの揃い方は Test_guardrailの予定は… と Test_sensorの予定は… で確かめる
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
stage = "implementation"
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
	g.commitAll("はじめ", "#1")
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
		if typ == "guardrail" || typ == "sensor" {
			want = ""
		}
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

// CMD-10: types は必ず書く。確かめないタイプ（human-review）と、今は扱えない proof と、決まった値にないタイプ
// （決まった値から外した monitoring を含む）は書けない
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

// pairRepo は、約束 task-id-unique の予定のタイプを planned にし、files（場所 → 1 行目に書くアノテーションの種類）を置いたリポジトリを作る。
// ext/ の下は types が extTypes の [[annotation]] で、def/ の下は types = [] の [[annotation]] で読み、def/ の下はガードレールの定義にする。
// 種類が空のファイルは、アノテーションのないファイル
func pairRepo(t *testing.T, version, planned, extTypes string, files map[string]string) string {
	t.Helper()
	root := typesRepo(t, version, planned, "internal", "types = []")
	g := newRVGit(t, root)
	cfg := readFileString(t, root, ".vanda/config.toml")
	cfg = strings.Replace(cfg, "paths = [\"ext/*.ext\"]\nadapter = \"any\"\ntypes = []", "paths = [\"ext/*.ext\"]\nadapter = \"any\"\n"+extTypes, 1)
	cfg += "\n[[annotation]]\npaths = [\"def/*.ext\"]\nadapter = \"any\"\ntypes = []\n\n[[guardrail]]\nadapter = \"file\"\npaths = [\"def/*.ext\"]\n"
	writeFile(t, root, ".vanda/config.toml", cfg)
	if err := os.Remove(filepath.Join(root, "ext/a.ext")); err != nil {
		t.Fatal(err)
	}
	for p, kind := range files {
		body := "アノテーションなし\n"
		if kind == "internal" {
			body = "@vanda:internal 小さな数の集合\n"
		} else if kind != "" {
			body = "@vanda:" + kind + " task-id-unique\n"
		}
		writeFile(t, root, p, body)
	}
	g.commitAll("(test) 置き直す", "#1")
	return root
}

// CMD-10: guardrail の予定は、ガードレールの定義（[[guardrail]] の paths）の evidence と、types に guardrail を含む
// [[annotation]] で読んだ、定義でないファイルの evidence の両方があるときだけ揃う。定義だけ・検証だけ・両方の 3 通りと、
// 両方あるときの片方の種類の全部を回す
// @vanda:evidence check-evidence-guardrail-needs-definition
func Test_guardrailの予定はガードレールの定義と拒むことの確かめの両方のevidenceがあるときだけ揃う(t *testing.T) {
	t.Parallel()
	bin := buildVanda(t)
	version, err := runningVersion(bin)
	if err != nil {
		t.Fatal(err)
	}
	type c struct {
		files map[string]string
		ok    bool
	}
	cases := map[string]c{
		"定義だけ": {map[string]string{"def/schema.ext": "evidence", "ext/reject.ext": ""}, false},
		"検証だけ": {map[string]string{"def/schema.ext": "", "ext/reject.ext": "evidence"}, false},
		"両方":   {map[string]string{"def/schema.ext": "evidence", "ext/reject.ext": "evidence"}, true},
	}
	for _, k := range annotationKinds {
		if k == "evidence" {
			continue
		}
		cases["定義が"+k] = c{map[string]string{"def/schema.ext": k, "ext/reject.ext": "evidence"}, false}
		cases["検証が"+k] = c{map[string]string{"def/schema.ext": "evidence", "ext/reject.ext": k}, false}
	}
	for name, cs := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := pairRepo(t, version, "guardrail", `types = ["guardrail"]`, cs.files)
			code, out := runCheck(t, bin, root, ".vanda/checks/present.toml")
			if (code == 0) != cs.ok {
				t.Errorf("揃うのは %v のはずが、終了コード %d\n%s", cs.ok, code, out)
			}
			if !cs.ok && !strings.Contains(out, "guardrail") {
				t.Errorf("揃わない guardrail の予定が出ていない\n%s", out)
			}
		})
	}
	// 定義のファイルを types に guardrail を含む [[annotation]] で読んでも、定義でないファイルの代わりにはならない
	t.Run("定義のファイルをtypesにguardrailを含むannotationで読む", func(t *testing.T) {
		t.Parallel()
		root := pairRepo(t, version, "guardrail", `types = ["guardrail"]`, map[string]string{"ext/schema.ext": "evidence"})
		cfg := readFileString(t, root, ".vanda/config.toml")
		writeFile(t, root, ".vanda/config.toml", strings.Replace(cfg, "adapter = \"file\"\npaths = [\"def/*.ext\"]", "adapter = \"file\"\npaths = [\"ext/schema.ext\"]", 1))
		newRVGit(t, root).commitAll("(test) 定義を ext に置く", "#1")
		if code, out := runCheck(t, bin, root, ".vanda/checks/present.toml"); code == 0 {
			t.Errorf("定義のファイルの evidence だけで揃った\n%s", out)
		}
	})
}

// CMD-10: sensor の予定は、types に sensor を含む [[annotation]] で読んだ sensor（センサーの場所）と evidence（センサーの検証）
// の両方があるときだけ揃う。場所だけ・検証だけ・両方の 3 通りと、両方あるときの片方の種類の全部を回す
// @vanda:evidence check-evidence-sensor-needs-check
func Test_sensorの予定はセンサーの場所と検証の両方があるときだけ揃う(t *testing.T) {
	t.Parallel()
	bin := buildVanda(t)
	version, err := runningVersion(bin)
	if err != nil {
		t.Fatal(err)
	}
	type c struct {
		files map[string]string
		ok    bool
	}
	cases := map[string]c{
		"場所だけ": {map[string]string{"ext/loc.ext": "sensor", "ext/check.ext": ""}, false},
		"検証だけ": {map[string]string{"ext/loc.ext": "", "ext/check.ext": "evidence"}, false},
		"両方":   {map[string]string{"ext/loc.ext": "sensor", "ext/check.ext": "evidence"}, true},
	}
	for _, k := range annotationKinds {
		if k != "sensor" {
			cases["場所が"+k] = c{map[string]string{"ext/loc.ext": k, "ext/check.ext": "evidence"}, false}
		}
		if k != "evidence" {
			cases["検証が"+k] = c{map[string]string{"ext/loc.ext": "sensor", "ext/check.ext": k}, false}
		}
	}
	for name, cs := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := pairRepo(t, version, "sensor", `types = ["sensor"]`, cs.files)
			code, out := runCheck(t, bin, root, ".vanda/checks/present.toml")
			if (code == 0) != cs.ok {
				t.Errorf("揃うのは %v のはずが、終了コード %d\n%s", cs.ok, code, out)
			}
			if !cs.ok && !strings.Contains(out, "sensor") {
				t.Errorf("揃わない sensor の予定が出ていない\n%s", out)
			}
		})
	}
	// 2 つとも、types に sensor を含まない [[annotation]] で読んだものは数えない
	t.Run("typesにsensorを含まないannotationで読む", func(t *testing.T) {
		t.Parallel()
		root := pairRepo(t, version, "sensor", `types = ["unit-test"]`, map[string]string{"ext/loc.ext": "sensor", "ext/check.ext": "evidence"})
		if code, out := runCheck(t, bin, root, ".vanda/checks/present.toml"); code == 0 {
			t.Errorf("types に sensor を含まない [[annotation]] のアノテーションで揃った\n%s", out)
		}
	})
}
