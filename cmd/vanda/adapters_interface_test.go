package main_test

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// vanda と外のもの（カスタムアダプタ、設定に書いたファイル、GitHub の API）との境目の振る舞い。
// ビルドした vanda を一時的な git のリポジトリで動かし、外から確かめる。期待値は仕様の文と docs/formats.md から書いた。

// ifaceRepo は、設定（[[layer]] の後に extra を足したもの）と、files（場所 → 中身）と、
// .vanda/adapters/ の実行できるアダプタ（名前 → シェルのスクリプトの本体）を置いた git のリポジトリを作る。
func ifaceRepo(t *testing.T, version, extra string, files, adapters map[string]string) string {
	t.Helper()
	root := t.TempDir()
	g := newRVGit(t, root)
	g.run("init", "-q", "-b", "main")
	writeFile(t, root, ".vanda/config.toml", fmt.Sprintf("version = %q\n\n[[layer]]\nid = \"model\"\nname = \"モデル\"\ndescription = \"判定と状態の変化\"\n%s", version, extra))
	writeFile(t, root, ".vanda/checks/links.toml", "[[check]]\nname = \"annotation-links\"\nscope = \"all\"\n")
	writeFile(t, root, ".vanda/checks/shape.toml", "[[check]]\nname = \"promise-list-shape\"\nscope = \"all\"\n")
	writeFile(t, root, ".vanda/agreements.toml", "")
	if _, ok := files[".vanda/promises.toml"]; !ok && !strings.Contains(extra, "promises =") {
		writeFile(t, root, ".vanda/promises.toml", "[[term]]\nid = \"task\"\nname = \"タスク\"\ndefinition = \"やること\"\n")
	}
	for p, b := range files {
		writeFile(t, root, p, b)
	}
	for name, body := range adapters {
		writeFile(t, root, ".vanda/adapters/"+name, "#!/bin/sh\n"+body)
		if err := os.Chmod(filepath.Join(root, ".vanda/adapters", name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// runVandaIn は、dir で vanda を動かし、終了コードと標準出力と標準エラーを返す。
func runVandaIn(t *testing.T, bin, dir string, env []string, args ...string) (int, string, string) {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Env = env
	var out, errb strings.Builder
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	wantNoOldWords(t, "vanda "+strings.Join(args, " ")+" の標準出力", out.String())
	wantNoOldWords(t, "vanda "+strings.Join(args, " ")+" の標準エラー", errb.String())
	if e, ok := err.(*exec.ExitError); ok {
		return e.ExitCode(), out.String(), errb.String()
	}
	if err != nil {
		t.Fatalf("vanda %v を動かせない: %v", args, err)
	}
	return 0, out.String(), errb.String()
}

func annotationRule(adapter, args string) string {
	return fmt.Sprintf("\n[[annotation]]\npaths = [\"ext/*.ext\"]\nadapter = %q\ntypes = [\"unit-test\"]\n%s", adapter, args)
}

// answer は、渡された 1 行目に名前を返すアダプタの本体。
const answerLine1 = "cat >/dev/null\nprintf '%s' '{\"names\":[{\"line\":1,\"name\":\"x\"}]}'\n"

// CMD-10: .vanda/adapters/ の直下のファイルの名前、ディレクトリの名前をアダプタの名前とし、ファイルならそれを、
// ディレクトリなら直下の run を動かす。どちらも動いたことを、返した名前の行のアノテーションの ID が約束リストに
// ないという誤りが出ることで確かめる
// @vanda:evidence annotation-adapter-contract
func Test_vanda_adaptersのアダプタは直下のファイルとディレクトリの直下のrunを名前で動かす(t *testing.T) {
	t.Parallel()
	bin := buildVanda(t)
	version, err := runningVersion(bin)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{"ext/a.ext": "@vanda:evidence no-such-spec\n"}
	t.Run("ファイル", func(t *testing.T) {
		t.Parallel()
		root := ifaceRepo(t, version, annotationRule("file-adapter", ""), files, map[string]string{"file-adapter": answerLine1})
		newRVGit(t, root).commitAll("(test) チェックの前にコミットする", "")
		code, _, errs := runVandaIn(t, bin, root, newRVGit(t, root).env, "check", ".vanda/checks/links.toml")
		if code != 1 || !strings.Contains(errs, "ext/a.ext:1: ") || !strings.Contains(errs, "no-such-spec") {
			t.Errorf("ファイルのアダプタが動いていない（%d）\n%s", code, errs)
		}
	})
	t.Run("ディレクトリの直下のrun", func(t *testing.T) {
		t.Parallel()
		root := ifaceRepo(t, version, annotationRule("dir-adapter", ""), files, map[string]string{"dir-adapter/run": answerLine1, "dir-adapter/other": "exit 9\n"})
		newRVGit(t, root).commitAll("(test) チェックの前にコミットする", "")
		code, _, errs := runVandaIn(t, bin, root, newRVGit(t, root).env, "check", ".vanda/checks/links.toml")
		if code != 1 || !strings.Contains(errs, "ext/a.ext:1: ") || !strings.Contains(errs, "no-such-spec") {
			t.Errorf("ディレクトリの run が動いていない（%d）\n%s", code, errs)
		}
	})
}

// CMD-10: アダプタは、リポジトリのルートを作業ディレクトリにし、設定の args をそのまま引数として渡して動かす。
// vanda をリポジトリの下のディレクトリで動かしても、作業ディレクトリはルート
// @vanda:evidence annotation-adapter-contract
func Test_vanda_adaptersのアダプタはリポジトリのルートでargsをそのまま渡されて動く(t *testing.T) {
	t.Parallel()
	bin := buildVanda(t)
	version, err := runningVersion(bin)
	if err != nil {
		t.Fatal(err)
	}
	// 作業ディレクトリと引数を、失敗の理由として標準エラーに書く
	report := "cat >/dev/null\necho \"cwd=$(pwd -P) argc=$# args=$*\" >&2\nexit 3\n"
	root := ifaceRepo(t, version, annotationRule("report", `args = ["--mode", "two words", "x"]`),
		map[string]string{"ext/a.ext": "@vanda:evidence a\n", "sub/dir/.keep": ""}, map[string]string{"report": report})
	real, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	newRVGit(t, root).commitAll("(test) チェックの前にコミットする", "")
	_, _, errs := runVandaIn(t, bin, filepath.Join(root, "sub", "dir"), newRVGit(t, root).env, "check", "../../.vanda/checks/links.toml")
	if !strings.Contains(errs, "cwd="+real+" ") {
		t.Errorf("アダプタの作業ディレクトリがリポジトリのルート（%s）でない\n%s", real, errs)
	}
	// 空白を含む引数も 1 つのまま渡る（3 つ）
	if !strings.Contains(errs, "argc=3 args=--mode two words x") {
		t.Errorf("args がそのまま渡っていない\n%s", errs)
	}
}

// CMD-10: アダプタが 0 でない終了コードで終わったら、標準エラーに書いた理由を誤りのメッセージに含める
// @vanda:evidence annotation-adapter-contract
func Test_vanda_adaptersのアダプタが失敗すると標準エラーの理由が誤りに入る(t *testing.T) {
	t.Parallel()
	bin := buildVanda(t)
	version, err := runningVersion(bin)
	if err != nil {
		t.Fatal(err)
	}
	root := ifaceRepo(t, version, annotationRule("fails", ""), map[string]string{"ext/a.ext": "@vanda:evidence a\n"},
		map[string]string{"fails": "cat >/dev/null\necho '置き場所が誤っています（3 行目）' >&2\nexit 4\n"})
	newRVGit(t, root).commitAll("(test) チェックの前にコミットする", "")
	code, _, errs := runVandaIn(t, bin, root, newRVGit(t, root).env, "check", ".vanda/checks/links.toml")
	if code != 1 || !strings.Contains(errs, "ext/a.ext") || !strings.Contains(errs, "置き場所が誤っています（3 行目）") {
		t.Errorf("アダプタの理由が誤りに入っていない（%d）\n%s", code, errs)
	}
}

// CMD-10: アダプタが "annotation": false と返した行は、アノテーションとして数えず、書き方の確かめもしない
// @vanda:evidence annotation-adapter-contract
func Test_vanda_adaptersのアダプタがannotationにfalseを返した行は数えず書き方も確かめない(t *testing.T) {
	t.Parallel()
	bin := buildVanda(t)
	version, err := runningVersion(bin)
	if err != nil {
		t.Fatal(err)
	}
	// 1 行目は書き方が誤り（ID が 2 つ・決まっていない種類）、2 行目は約束リストにない ID。どちらも false を返す
	root := ifaceRepo(t, version, annotationRule("not-ann", ""), map[string]string{"ext/a.ext": "@vanda:foo a b\n@vanda:evidence no-such-spec\n"},
		map[string]string{"not-ann": "cat >/dev/null\nprintf '%s' '{\"names\":[{\"line\":1,\"annotation\":false},{\"line\":2,\"annotation\":false}]}'\n"})
	newRVGit(t, root).commitAll("(test) チェックの前にコミットする", "")
	if code, _, errs := runVandaIn(t, bin, root, newRVGit(t, root).env, "check", ".vanda/checks/links.toml"); code != 0 {
		t.Errorf("annotation: false の行で止まった（%d）\n%s", code, errs)
	}
}

// CMD-10: アダプタがない・実行の権限がない・1 行目に動かし方がない・ディレクトリに run がないときは、場所と直し方を書いた誤り
// @vanda:evidence annotation-adapter-contract
func Test_vanda_adaptersのアダプタを動かせないときは場所と直し方を書いた誤りになる(t *testing.T) {
	t.Parallel()
	bin := buildVanda(t)
	version, err := runningVersion(bin)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, adapter, where string
		prepare              func(root string)
	}{
		{"ない", "absent", ".vanda/adapters/absent", func(string) {}},
		{"実行の権限がない", "noexec", ".vanda/adapters/noexec", func(root string) {
			writeFile(t, root, ".vanda/adapters/noexec", "#!/bin/sh\n"+answerLine1)
		}},
		{"1行目に動かし方がない", "noshebang", ".vanda/adapters/noshebang", func(root string) {
			writeFile(t, root, ".vanda/adapters/noshebang", answerLine1)
			if err := os.Chmod(filepath.Join(root, ".vanda/adapters/noshebang"), 0o755); err != nil {
				t.Fatal(err)
			}
		}},
		{"ディレクトリにrunがない", "norun", ".vanda/adapters/norun/run", func(root string) {
			writeFile(t, root, ".vanda/adapters/norun/other", "#!/bin/sh\n"+answerLine1)
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			root := ifaceRepo(t, version, annotationRule(c.adapter, ""), map[string]string{"ext/a.ext": "@vanda:evidence a\n"}, nil)
			c.prepare(root)
			newRVGit(t, root).commitAll("(test) チェックの前にコミットする", "")
			code, _, errs := runVandaIn(t, bin, root, newRVGit(t, root).env, "check", ".vanda/checks/links.toml")
			if code != 1 || !strings.Contains(errs, c.where) || !strings.Contains(errs, "ください。") {
				t.Errorf("誤りに、場所 %s と直し方が出ていない（%d）\n%s", c.where, code, errs)
			}
		})
	}
}

// CMD-10: アダプタの答えに、渡していない行・空の名前・同じ行の 2 つ以上の答え・name と annotation の両方・
// 決まっていない項目があれば誤り。渡した行に答えないのも誤り
// @vanda:evidence annotation-adapter-contract
func Test_vanda_adaptersのアダプタの答えの誤りはそのファイルの誤りになる(t *testing.T) {
	t.Parallel()
	bin := buildVanda(t)
	version, err := runningVersion(bin)
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]string{
		"渡していない行":            `{"names":[{"line":1,"name":"x"},{"line":7,"name":"y"}]}`,
		"空の名前":               `{"names":[{"line":1,"name":""}]}`,
		"同じ行に2つ":             `{"names":[{"line":1,"name":"x"},{"line":1,"name":"y"}]}`,
		"nameとannotationの両方": `{"names":[{"line":1,"name":"x","annotation":false}]}`,
		"決まっていない項目":          `{"names":[{"line":1,"name":"x","extra":1}]}`,
		"決まっていない一番上の項目":      `{"names":[{"line":1,"name":"x"}],"extra":1}`,
		"渡した行に答えない":          `{"names":[]}`,
	}
	for name, answer := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := ifaceRepo(t, version, annotationRule("bad", ""), map[string]string{"ext/a.ext": "@vanda:term task\n"},
				map[string]string{"bad": "cat >/dev/null\nprintf '%s' '" + answer + "'\n"})
			newRVGit(t, root).commitAll("(test) チェックの前にコミットする", "")
			code, _, errs := runVandaIn(t, bin, root, newRVGit(t, root).env, "check", ".vanda/checks/links.toml")
			if code != 1 || !strings.Contains(errs, "ext/a.ext") {
				t.Errorf("答え %s で、ext/a.ext の誤りにならない（%d）\n%s", answer, code, errs)
			}
		})
	}
}

// CM-09: 設定にはっきり書いた、ワイルドカードのない場所のファイルがなければ誤り。ワイルドカードのあるパターンが
// 何にもマッチしないのは誤りにしない
// @vanda:evidence mistakes-reported-with-fix
func Test設定にはっきり書いた約束リストのファイルがなければ誤りでワイルドカードなら誤りにしない(t *testing.T) {
	t.Parallel()
	bin := buildVanda(t)
	version, err := runningVersion(bin)
	if err != nil {
		t.Fatal(err)
	}
	root := ifaceRepo(t, version, "", nil, nil)
	cfg := readFileString(t, root, ".vanda/config.toml")
	writeFile(t, root, ".vanda/config.toml", strings.Replace(cfg, "\n[[layer]]", "promises = [\".vanda/specs.toml\"]\n\n[[layer]]", 1))
	if err := os.Remove(filepath.Join(root, ".vanda/promises.toml")); err != nil {
		t.Fatal(err)
	}
	newRVGit(t, root).commitAll("(test) チェックの前にコミットする", "")
	code, _, errs := runVandaIn(t, bin, root, newRVGit(t, root).env, "check", ".vanda/checks/shape.toml")
	if code != 1 || !strings.Contains(errs, ".vanda/specs.toml") {
		t.Errorf("はっきり書いたファイルがないのに、その場所の誤りにならない（%d）\n%s", code, errs)
	}
	writeFile(t, root, ".vanda/config.toml", strings.Replace(cfg, "\n[[layer]]", "promises = [\".vanda/specs/*.toml\"]\n\n[[layer]]", 1))
	newRVGit(t, root).commitAll("(test) チェックの前にコミットする", "")
	if code, _, errs := runVandaIn(t, bin, root, newRVGit(t, root).env, "check", ".vanda/checks/shape.toml"); code != 0 {
		t.Errorf("ワイルドカードのパターンが何にもマッチしないのを誤りにした（%d）\n%s", code, errs)
	}
}

// CM-09: 設定の promises のパターンにマッチするファイルを、すべて約束リストとして読む。
// a.toml の仕様は b.toml の機能を refines するので、b.toml を読まなければ refines の先がない誤りになる
// @vanda:evidence mistakes-reported-with-fix
func Test設定のpromisesのパターンにマッチするファイルをすべて約束リストとして読む(t *testing.T) {
	t.Parallel()
	bin := buildVanda(t)
	version, err := runningVersion(bin)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		".vanda/promises/a.toml":     "[[spec]]\nid = \"task-unique\"\nrefines = [\"add-task\"]\nstatement = \"タスクは重ならない\"\nlayers = [\"model\"]\n\n[[spec.evidence]]\nstage = \"implementation\"\ntype = \"unit-test\"\nlayers = [\"model\"]\n",
		".vanda/promises/sub/b.toml": "[[term]]\nid = \"task\"\nname = \"タスク\"\ndefinition = \"やること\"\n\n[[feature]]\nid = \"add-task\"\nname = \"タスクの登録\"\ndefinition = \"タスクを足す\"\nrefines = [\"no-lost-task\"]\n\n[[property]]\nid = \"no-lost-task\"\nstatement = \"登録したタスクは失われない\"\n",
		".vanda/promises/c.toml":     "[[term]]\nid = \"Bad_ID\"\nname = \"誤った ID\"\ndefinition = \"読まれていれば誤りになる\"\n",
		".vanda/not-promises/d.toml": "[[term]]\nid = \"Other_Bad\"\nname = \"マッチしない\"\ndefinition = \"読まれない\"\n",
		".vanda/promises/readme.md":  "パターンにマッチしない\n",
	}
	extra := "promises = [\".vanda/promises/**/*.toml\"]\n"
	root := ifaceRepo(t, version, "", files, nil)
	cfg := readFileString(t, root, ".vanda/config.toml")
	writeFile(t, root, ".vanda/config.toml", strings.Replace(cfg, "\n[[layer]]", extra+"\n[[layer]]", 1))
	newRVGit(t, root).commitAll("(test) チェックの前にコミットする", "")
	code, _, errs := runVandaIn(t, bin, root, newRVGit(t, root).env, "check", ".vanda/checks/shape.toml")
	// c.toml の誤りは出て、b.toml を読んだので a.toml の refines の誤りは出ない。パターンにマッチしない d.toml は読まない
	if code != 1 || !strings.Contains(errs, ".vanda/promises/c.toml") || strings.Contains(errs, "a.toml") || strings.Contains(errs, "Other_Bad") {
		t.Errorf("パターンにマッチするファイルだけを全部読んでいない（%d）\n%s", code, errs)
	}
}

// CMD-10: アダプタには、アノテーションを読むファイルの HEAD のコミットの中身を、標準入力の JSON の content で渡す。
// 作業ツリーには、行のずれる書きかけ（コミットしていない変更）と、git add しただけの変更の 2 つを試す。
// アダプタは受け取った標準入力を .in.json に書き写し、1 行目に名前を返す
// @vanda:evidence annotation-adapter-contract
// @vanda:evidence screen-matches-check
func Test_vanda_adaptersのアダプタはコミットしていない変更があってもHEADの中身をcontentで受け取る(t *testing.T) {
	t.Parallel()
	bin := buildVanda(t)
	version, err := runningVersion(bin)
	if err != nil {
		t.Fatal(err)
	}
	const committed = "@vanda:term task\n本文\n"
	for _, staged := range []bool{false, true} {
		t.Run(fmt.Sprintf("git_addした=%v", staged), func(t *testing.T) {
			t.Parallel()
			root := ifaceRepo(t, version, annotationRule("echo-in", ""), map[string]string{"ext/a.ext": committed},
				map[string]string{"echo-in": "cat > .in.json\nprintf '%s' '{\"names\":[{\"line\":1,\"name\":\"x\"}]}'\n"})
			g := newRVGit(t, root)
			g.commitAll("(test) はじめ", "")
			writeFile(t, root, "ext/a.ext", "書きかけ\n@vanda:term no-such-term\n")
			if staged {
				g.run("add", "ext/a.ext")
			}
			code, _, errs := runVandaIn(t, bin, root, g.env, "check", ".vanda/checks/links.toml")
			if code != 0 {
				t.Errorf("HEAD の中身なら通るはず（%d）\n%s", code, errs)
			}
			var in struct {
				Path        string           `json:"path"`
				Content     *string          `json:"content"`
				Annotations []map[string]int `json:"annotations"`
			}
			if err := json.Unmarshal([]byte(readFileString(t, root, ".in.json")), &in); err != nil {
				t.Fatal(err)
			}
			if in.Path != "ext/a.ext" || in.Content == nil || *in.Content != committed || !reflect.DeepEqual(in.Annotations, []map[string]int{{"line": 1}}) {
				t.Errorf("渡したもの: path %q content %v annotations %v", in.Path, in.Content, in.Annotations)
			}
		})
	}
}

// CMD-10: アダプタが [[annotation]] の timeout（秒）を過ぎても終わらなければ、止めて、どのアダプタかを書いたタイムアウトの誤りにする。
// sleep の 30 秒を待たずに、timeout の 1 秒で終わることも見る
// @vanda:evidence annotation-adapter-contract
func Test_vanda_adaptersのアダプタがtimeoutを過ぎたらどのアダプタかを書いたタイムアウトの誤りで止まる(t *testing.T) {
	t.Parallel()
	bin := buildVanda(t)
	version, err := runningVersion(bin)
	if err != nil {
		t.Fatal(err)
	}
	root := ifaceRepo(t, version, annotationRule("slow-adapter", "timeout = 1\n"), map[string]string{"ext/a.ext": "@vanda:term task\n"},
		map[string]string{"slow-adapter": "cat >/dev/null\nsleep 30\n"})
	g := newRVGit(t, root)
	g.commitAll("(test) はじめ", "")
	start := time.Now()
	code, _, errs := runVandaIn(t, bin, root, g.env, "check", ".vanda/checks/links.toml")
	if d := time.Since(start); d > 15*time.Second {
		t.Errorf("timeout の 1 秒で止めず、%v 待った", d)
	}
	if code != 1 || !strings.Contains(errs, "ext/a.ext") || !strings.Contains(errs, "slow-adapter") || !strings.Contains(errs, "タイムアウト") {
		t.Errorf("どのアダプタかを書いたタイムアウトの誤りでない（%d）\n%s", code, errs)
	}
}

// RV: 画面の API でも、アダプタが timeout を過ぎたら止めて、どのアダプタかを書いたタイムアウトの誤りを返す
// （レスポンスを返さずに待ち続けることがない）。仕様の詳細はエビデンスを探すのでアノテーションを読む
// @vanda:evidence annotation-adapter-contract
func Test_vanda_adaptersのアダプタがtimeoutを過ぎたら仕様の詳細のAPIがどのアダプタかを書いたタイムアウトの誤りを返す(t *testing.T) {
	t.Parallel()
	bin := buildVanda(t)
	version, err := runningVersion(bin)
	if err != nil {
		t.Fatal(err)
	}
	promises := "[[term]]\nid = \"task\"\nname = \"タスク\"\ndefinition = \"やること\"\n\n[[spec]]\nid = \"task-id-unique\"\nrefines = [\"add-task\"]\nstatement = \"タスクの ID は重ならない\"\nlayers = [\"model\"]\n\n[[spec.evidence]]\nstage = \"implementation\"\ntype = \"unit-test\"\nlayers = [\"model\"]\n\n[[feature]]\nid = \"add-task\"\nname = \"タスクの登録\"\ndefinition = \"タスクを足す\"\nrefines = [\"no-lost-task\"]\n\n[[property]]\nid = \"no-lost-task\"\nstatement = \"登録したタスクは失われない\"\n"
	root := ifaceRepo(t, version, annotationRule("slow-adapter", "timeout = 1\n")+"\n[gui]\nbase = \"main\"\n",
		map[string]string{".vanda/promises.toml": promises, "ext/a.ext": "@vanda:evidence task-id-unique\n"},
		map[string]string{"slow-adapter": "cat >/dev/null\nsleep 30\n"})
	g := newRVGit(t, root)
	g.commitAll("(test) はじめ", "#1")
	s := startServe(t, bin, root, g.env)
	start := time.Now()
	code, msg := s.get("/api/specs/task-id-unique", nil)
	if d := time.Since(start); d > 15*time.Second {
		t.Errorf("timeout の 1 秒で止めず、%v 待った", d)
	}
	if code == 200 || !strings.Contains(msg, "slow-adapter") || !strings.Contains(msg, "タイムアウト") {
		t.Errorf("どのアダプタかを書いたタイムアウトの誤りでない（%d）: %s", code, msg)
	}
}

// @vanda:evidence annotation-adapter-contract
func Test_vanda_adaptersのアダプタが渡した行のどれかに答えなければvanda_checkがその行を指して誤りにする(t *testing.T) {
	t.Parallel()
	bin := buildVanda(t)
	version, err := runningVersion(bin)
	if err != nil {
		t.Fatal(err)
	}
	// 1・3・4 行目を渡し、3 行目だけに答えない
	root := ifaceRepo(t, version, annotationRule("partial", ""), map[string]string{"ext/a.ext": "@vanda:term task\n本文\n@vanda:term task\n@vanda:term task\n"},
		map[string]string{"partial": "cat >/dev/null\nprintf '%s' '{\"names\":[{\"line\":1,\"name\":\"x\"},{\"line\":4,\"annotation\":false}]}'\n"})
	newRVGit(t, root).commitAll("(test) チェックの前にコミットする", "")
	code, out, errs := runVandaIn(t, bin, root, newRVGit(t, root).env, "check", ".vanda/checks/links.toml")
	if code != 1 || !strings.Contains(out, "チェック annotation-links（all）: 失敗（1 件）") || !strings.HasPrefix(errs, "ext/a.ext:3: ") || strings.Count(errs, "\n") != 1 {
		t.Errorf("答えのない 3 行目だけを指す 1 件の誤りにならない（%d）\n%s%s", code, out, errs)
	}
}
