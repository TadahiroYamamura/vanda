package main_test

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// 組み込みの ts のアダプタと、vanda のリポジトリの .vanda/adapters/playwright が付ける名前
// （docs/plans/annotation-types.md の 2・3）。名前は、呼び出しの最初の引数の文字列で、describe の中なら外側の
// describe の名前から順に " > " でつなぐ。約束の詳細（GET /api/claims/<ID>）の found の名前で確かめる。

// tsNamesFile は、入れ子の describe を含むテストのファイル。期待する名前は tsNamesWant。
// 組み込みの ts は .skip・.todo を数えず .only を誤りにする（annotation_ts.txtar で確かめる）ので、ここには書かない
const tsNamesFile = `import { describe, it, test } from "vitest";

// @vanda:evidence task-id-unique
test("いちばん外", () => {});

describe("ReviewPage", () => {
  // @vanda:evidence task-id-unique
  it("一覧を出す", () => {});

  describe("承認", () => {
    // @vanda:evidence task-id-unique
    test("承認すると承認済になる", () => {});

    /* @vanda:evidence task-id-unique */
    it("取り消すと提案中に戻る", () => {});
  });
});

// @vanda:evidence task-id-unique
describe("まとまり", () => {});

test.describe("Playwright の形", () => {
  // @vanda:evidence task-id-unique
  test.each(["承認", "取り消し"])("%s をする", (op) => {});

  // @vanda:evidence task-id-unique
  test(` + "`${op} を承認する`" + `, () => {});
});
`

var tsNamesWant = []string{
	"いちばん外",
	"ReviewPage > 一覧を出す",
	"ReviewPage > 承認 > 承認すると承認済になる",
	"ReviewPage > 承認 > 取り消すと提案中に戻る",
	"まとまり",
	// .each(表)("名前", …) は、最後の呼び出しの最初の引数
	"Playwright の形 > %s をする",
	// テンプレート文字列は、書いてあるとおりの文字
	"Playwright の形 > ${op} を承認する",
}

// foundNames は、annotation の設定と、ファイル path に中身 body を置いたリポジトリを作り、約束の詳細の found の名前を返す。
func foundNames(t *testing.T, annotation string, files map[string]string, adapters map[string]string) ([]string, string) {
	t.Helper()
	bin := buildVanda(t)
	version, err := runningVersion(bin)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	g := newRVGit(t, root)
	g.run("init", "-q", "-b", "main")
	writeFile(t, root, ".vanda/config.toml", fmt.Sprintf("version = %q\n\n[[layer]]\nid = \"model\"\nname = \"モデル\"\ndescription = \"判定と状態の変化\"\n\n%s", version, annotation))
	writeFile(t, root, ".vanda/promises.toml", "[[term]]\nid = \"task\"\nname = \"タスク\"\ndefinition = \"やること\"\n\n[[claim]]\nid = \"task-id-unique\"\nsubjects = [\"task\"]\nstatement = \"タスクの ID は重ならない\"\nlayers = [\"model\"]\n\n[[claim.evidence]]\nstage = \"implementation\"\ntype = \"scenario-test\"\nlayers = [\"model\"]\n\n[[claim.evidence]]\nstage = \"implementation\"\ntype = \"unit-test\"\nlayers = [\"model\"]\n")
	writeFile(t, root, ".vanda/agreements.toml", "")
	for p, b := range files {
		writeFile(t, root, p, b)
	}
	for p, src := range adapters {
		data, err := os.ReadFile(src)
		if err != nil {
			t.Fatalf("アダプタ %s を読めない: %v", src, err)
		}
		writeFile(t, root, p, string(data))
		if err := os.Chmod(filepath.Join(root, p), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	g.commitAll("はじめ", "#1")
	s := startServe(t, bin, root, g.env)
	var got claimDetail
	if code, msg := s.get("/api/claims/task-id-unique", &got); code != 200 {
		t.Fatalf("約束の詳細を読めない（%d）: %s", code, msg)
	}
	var names []string
	for _, e := range got.Evidence {
		for _, f := range e.Found {
			names = append(names, f.Name)
		}
	}
	sort.Strings(names)
	return names, root
}

// 組み込みの ts は、入れ子の describe の名前を " > " でつなぐ
func Test組み込みのtsのアダプタは呼び出しの最初の引数を名前にし入れ子のdescribeをつなぐ(t *testing.T) {
	t.Parallel()
	names, _ := foundNames(t, "[[annotation]]\npaths = [\"**/*.test.ts\"]\nadapter = \"ts\"\ntypes = [\"unit-test\"]\n",
		map[string]string{"web/src/a.test.ts": tsNamesFile}, nil)
	want := append([]string(nil), tsNamesWant...)
	sort.Strings(want)
	if !reflect.DeepEqual(names, want) {
		t.Errorf("名前: %v（思ったもの %v）", names, want)
	}
}

// vanda のリポジトリの .vanda/adapters/playwright は、E2E にアノテーションを付けるとシナリオテストとして揃え、
// 組み込みの ts と同じく名前を " > " でつなぐ
func Test_vanda_adaptersのplaywrightはE2Eのアノテーションをシナリオテストとして揃え名前をつなぐ(t *testing.T) {
	t.Parallel()
	adapter := filepath.Join("..", "..", ".vanda", "adapters", "playwright")
	e2e := `import { test, expect } from "@playwright/test";

test.describe("レビューの画面", () => {
  // @vanda:evidence task-id-unique
  test("承認する", async ({ page }) => {});
});
`
	cfg := "[[annotation]]\npaths = [\"web/e2e/**/*.spec.ts\"]\nadapter = \"playwright\"\ntypes = [\"scenario-test\"]\n"
	names, root := foundNames(t, cfg, map[string]string{"web/e2e/review.spec.ts": e2e}, map[string]string{".vanda/adapters/playwright": adapter})
	if want := []string{"レビューの画面 > 承認する"}; !reflect.DeepEqual(names, want) {
		t.Errorf("名前: %v（思ったもの %v）", names, want)
	}
	// シナリオテストの予定は、E2E のアノテーションで揃う（unit-test の予定は揃わない）
	writeFile(t, root, ".vanda/checks/present.toml", "[[check]]\nname = \"evidence-present\"\nscope = \"all\"\n")
	bin := buildVanda(t)
	code, out := runCheck(t, bin, root, ".vanda/checks/present.toml")
	if code == 0 {
		t.Fatalf("unit-test の予定まで揃った\n%s", out)
	}
	if !containsLine(out, "unit-test") || containsLine(out, "scenario-test") {
		t.Errorf("揃わないのが unit-test だけでない（scenario-test は E2E で揃うはず）\n%s", out)
	}
}

func containsLine(out, s string) bool {
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, s) {
			return true
		}
	}
	return false
}

// .vanda/adapters/playwright は、E2E の文字列の中の `@vanda:` に "annotation": false を返すので、それは数えられず、
// 書き方が誤っていても誤りにならない。vanda の E2E（web/e2e/review.spec.ts）を、そのまま読ませても止まらない
func Test_vanda_adaptersのplaywrightはE2Eの文字列の中の書き方をアノテーションとしないのでvandaのE2Eで止まらない(t *testing.T) {
	t.Parallel()
	adapter := filepath.Join("..", "..", ".vanda", "adapters", "playwright")
	strs := "import { test } from \"@playwright/test\";\n\nconst bad = \"@vanda:foo two ids\";\nconst tpl = `# @vanda:evidence no-such-id ${bad}`;\n\n// @vanda:evidence task-id-unique\ntest(\"承認する\", async () => {});\n"
	spec, err := os.ReadFile(filepath.Join("..", "..", "web", "e2e", "review.spec.ts"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := "[[annotation]]\npaths = [\"web/e2e/**/*.spec.ts\"]\nadapter = \"playwright\"\ntypes = [\"scenario-test\"]\n"
	names, root := foundNames(t, cfg, map[string]string{"web/e2e/strings.spec.ts": strs, "web/e2e/review.spec.ts": string(spec)}, map[string]string{".vanda/adapters/playwright": adapter})
	if want := []string{"承認する"}; !reflect.DeepEqual(names, want) {
		t.Errorf("名前: %v（思ったもの %v。文字列の中の @vanda: を数えている）", names, want)
	}
	writeFile(t, root, ".vanda/checks/links.toml", "[[check]]\nname = \"annotation-links\"\nscope = \"all\"\n")
	// vanda の E2E のアノテーションは vanda の約束を指すので、この一時的なリポジトリでは「約束リストにない」で止まる。
	// それのほかの誤り（文字列の中の書き方を読んだことによる、種類や ID の数の誤り）が出ないことを見る
	_, out := runCheck(t, buildVanda(t), root, ".vanda/checks/links.toml")
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, "web/e2e/") && !strings.Contains(l, "約束リストの約束にありません") {
			t.Errorf("E2E の文字列の中の書き方を、アノテーションとして読んだ: %s", l)
		}
	}
}
