package main_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

// vanda と外のもの（カスタムアダプタ、設定に書いたファイル、GitHub の API）との境目の振る舞い。
// ビルドした vanda を一時的な git のリポジトリで動かし、外から確かめる。期待値は約束の文と docs/formats.md から書いた。

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
// @vanda:evidence project-adapter-name
func Test_vanda_adaptersのアダプタは直下のファイルとディレクトリの直下のrunを名前で動かす(t *testing.T) {
	t.Parallel()
	bin := buildVanda(t)
	version, err := runningVersion(bin)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{"ext/a.ext": "@vanda:evidence no-such-claim\n"}
	t.Run("ファイル", func(t *testing.T) {
		t.Parallel()
		root := ifaceRepo(t, version, annotationRule("file-adapter", ""), files, map[string]string{"file-adapter": answerLine1})
		newRVGit(t, root).commitAll("(test) チェックの前にコミットする", "")
		code, _, errs := runVandaIn(t, bin, root, newRVGit(t, root).env, "check", ".vanda/checks/links.toml")
		if code != 1 || !strings.Contains(errs, "ext/a.ext:1: ") || !strings.Contains(errs, "no-such-claim") {
			t.Errorf("ファイルのアダプタが動いていない（%d）\n%s", code, errs)
		}
	})
	t.Run("ディレクトリの直下のrun", func(t *testing.T) {
		t.Parallel()
		root := ifaceRepo(t, version, annotationRule("dir-adapter", ""), files, map[string]string{"dir-adapter/run": answerLine1, "dir-adapter/other": "exit 9\n"})
		newRVGit(t, root).commitAll("(test) チェックの前にコミットする", "")
		code, _, errs := runVandaIn(t, bin, root, newRVGit(t, root).env, "check", ".vanda/checks/links.toml")
		if code != 1 || !strings.Contains(errs, "ext/a.ext:1: ") || !strings.Contains(errs, "no-such-claim") {
			t.Errorf("ディレクトリの run が動いていない（%d）\n%s", code, errs)
		}
	})
}

// CMD-10: アダプタは、リポジトリのルートを作業ディレクトリにし、設定の args をそのまま引数として渡して動かす。
// vanda をリポジトリの下のディレクトリで動かしても、作業ディレクトリはルート
// @vanda:evidence project-adapter-args-and-cwd
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
// @vanda:evidence project-adapter-failure-reason
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
// @vanda:evidence project-adapter-not-annotation
func Test_vanda_adaptersのアダプタがannotationにfalseを返した行は数えず書き方も確かめない(t *testing.T) {
	t.Parallel()
	bin := buildVanda(t)
	version, err := runningVersion(bin)
	if err != nil {
		t.Fatal(err)
	}
	// 1 行目は書き方が誤り（ID が 2 つ・決まっていない種類）、2 行目は約束リストにない ID。どちらも false を返す
	root := ifaceRepo(t, version, annotationRule("not-ann", ""), map[string]string{"ext/a.ext": "@vanda:foo a b\n@vanda:evidence no-such-claim\n"},
		map[string]string{"not-ann": "cat >/dev/null\nprintf '%s' '{\"names\":[{\"line\":1,\"annotation\":false},{\"line\":2,\"annotation\":false}]}'\n"})
	newRVGit(t, root).commitAll("(test) チェックの前にコミットする", "")
	if code, _, errs := runVandaIn(t, bin, root, newRVGit(t, root).env, "check", ".vanda/checks/links.toml"); code != 0 {
		t.Errorf("annotation: false の行で止まった（%d）\n%s", code, errs)
	}
}

// CMD-10: アダプタがない・実行の権限がない・1 行目に動かし方がない・ディレクトリに run がないときは、場所と直し方を書いた誤り
// @vanda:evidence project-adapter-not-runnable
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
// @vanda:evidence project-adapter-response-errors
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
// @vanda:evidence promise-list-explicit-file-must-exist
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
// a.toml の約束は b.toml の用語を対象にするので、b.toml を読まなければ subjects の先がない誤りになる
// @vanda:evidence promise-list-reads-all-patterns
func Test設定のpromisesのパターンにマッチするファイルをすべて約束リストとして読む(t *testing.T) {
	t.Parallel()
	bin := buildVanda(t)
	version, err := runningVersion(bin)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		".vanda/promises/a.toml":     "[[claim]]\nid = \"task-unique\"\nsubjects = [\"task\"]\nstatement = \"タスクは重ならない\"\nlayers = [\"model\"]\n\n[[claim.evidence]]\nstage = \"implementation\"\ntype = \"unit-test\"\nlayers = [\"model\"]\n",
		".vanda/promises/sub/b.toml": "[[term]]\nid = \"task\"\nname = \"タスク\"\ndefinition = \"やること\"\n",
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
	// c.toml の誤りは出て、b.toml を読んだので a.toml の subjects の誤りは出ない。パターンにマッチしない d.toml は読まない
	if code != 1 || !strings.Contains(errs, ".vanda/promises/c.toml") || strings.Contains(errs, "a.toml") || strings.Contains(errs, "Other_Bad") {
		t.Errorf("パターンにマッチするファイルだけを全部読んでいない（%d）\n%s", code, errs)
	}
}

// CMD-01: vanda doctor は、ストーリーストアが組み込みの github なら、GET <api_url>/repos/<repository> を呼んで、Issues が使えるかを確かめる
// @vanda:evidence doctor-github-issues
func Test_vanda_doctorはGitHubのリポジトリを読んでIssuesが使えるかを確かめる(t *testing.T) {
	t.Parallel()
	bin := buildVanda(t)
	version, err := runningVersion(bin)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		status int
		body   string
		ok     bool
	}{
		{"Issuesが使える", 200, `{"full_name":"owner/app","has_issues":true}`, true},
		{"Issuesが使えない", 200, `{"full_name":"owner/app","has_issues":false}`, false},
		{"リポジトリがない", 404, `{"message":"Not Found"}`, false},
		{"トークンが誤っている", 401, `{"message":"Bad credentials"}`, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			var mu sync.Mutex
			var paths, auths []string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				mu.Lock()
				paths = append(paths, req.Method+" "+req.URL.Path)
				auths = append(auths, req.Header.Get("Authorization"))
				mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(c.status)
				_, _ = w.Write([]byte(c.body))
			}))
			t.Cleanup(srv.Close)
			root := ifaceRepo(t, version, "\n[story]\nadapter = \"github\"\nrepository = \"owner/app\"\napi_url = \""+srv.URL+"\"\n", nil, nil)
			// 偽のサーバー（127.0.0.1）は api.github.com でないので、トークンは GH_ENTERPRISE_TOKEN から読まれる
			env := ghEnv(newRVGit(t, root).env, fakeGH(t, nil), "GH_ENTERPRISE_TOKEN=gho_doctortoken")
			code, out, errs := runVandaIn(t, bin, root, env, "doctor")
			if (code == 0) != c.ok {
				t.Errorf("終了コード %d（通るのは %v のはず）\n%s%s", code, c.ok, out, errs)
			}
			if !c.ok && !strings.Contains(errs, "ください。") {
				t.Errorf("通らないのに、直し方が出ていない\n%s", errs)
			}
			mu.Lock()
			defer mu.Unlock()
			if len(paths) == 0 || paths[0] != "GET /repos/owner/app" {
				t.Errorf("呼んだもの: %v（GET /repos/owner/app のはず）", paths)
			}
			if len(auths) == 0 || !strings.Contains(auths[0], "gho_doctortoken") {
				t.Errorf("トークンを渡していない: %v", auths)
			}
		})
	}
}

// CMD-01: vanda doctor は、api_url のホストに合うトークンで GitHub に繋ぎ、見つからなければリクエストを送らずに失敗する。
// api.github.com の場合は、本物の GitHub に繋がないよう、api_url を http://api.github.com にして、HTTP_PROXY で偽のサーバーに向ける
// @vanda:evidence doctor-github-token
// @vanda:evidence story-github-token
func Test_vanda_doctorはapi_urlのホストに合うトークンで繋ぎ見つからなければGitHubにリクエストを送らずに失敗する(t *testing.T) {
	t.Parallel()
	bin := buildVanda(t)
	version, err := runningVersion(bin)
	if err != nil {
		t.Fatal(err)
	}
	type sent struct{ host, auth string }
	for _, h := range []string{"api.github.com", "ほかのホスト"} {
		for _, c := range []struct {
			name string
			// mine は、そのホストに合う読み先（api.github.com なら GH_TOKEN と github.com の gh、ほかなら GH_ENTERPRISE_TOKEN とそのホストの gh）。
			// other は、合わない読み先
			mineEnv, otherEnv bool
			mineGH, otherGH   bool
			wantKey           string // 送るはずのトークン。空なら送らずに失敗する
		}{
			{"合う環境変数と合わない環境変数がある", true, true, true, true, "env_mine"},
			{"合う環境変数がなくghが合うホストにログインしている", false, true, true, true, "gh_mine"},
			{"合う環境変数がなくghも合うホストにログインしていない", false, true, false, true, ""},
		} {
			t.Run(h+"/"+c.name, func(t *testing.T) {
				t.Parallel()
				var mu sync.Mutex
				var got []sent
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					mu.Lock()
					got = append(got, sent{req.Host, req.Header.Get("Authorization")})
					mu.Unlock()
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(`{"full_name":"owner/app","has_issues":true}`))
				}))
				t.Cleanup(srv.Close)
				srvHost := strings.TrimPrefix(srv.URL, "http://")
				apiURL, host, mineEnv, otherEnv, mineGH, otherGH := srv.URL, srvHost, "GH_ENTERPRISE_TOKEN", "GH_TOKEN", srvHost, "github.com"
				if h == "api.github.com" {
					apiURL, host, mineEnv, otherEnv, mineGH, otherGH = "http://api.github.com", "api.github.com", "GH_TOKEN", "GH_ENTERPRISE_TOKEN", "github.com", srvHost
				}
				var kv []string
				if c.mineEnv {
					kv = append(kv, mineEnv+"=env_mine")
				}
				if c.otherEnv {
					kv = append(kv, otherEnv+"=env_other")
				}
				logins := map[string]string{}
				if c.mineGH {
					logins[mineGH] = "gh_mine"
				}
				if c.otherGH {
					logins[otherGH] = "gh_other"
				}
				if h == "api.github.com" {
					kv = append(kv, "HTTP_PROXY="+srv.URL)
				}
				root := ifaceRepo(t, version, "\n[story]\nadapter = \"github\"\nrepository = \"owner/app\"\napi_url = \""+apiURL+"\"\n", nil, nil)
				ghDir := fakeGH(t, logins)
				code, out, errs := runVandaIn(t, bin, root, ghEnv(newRVGit(t, root).env, ghDir, kv...), "doctor")
				mu.Lock()
				defer mu.Unlock()
				if c.wantKey == "" {
					if code != 1 || !strings.Contains(errs, "ください") {
						t.Errorf("終了コード %d（1 で、直し方を出すはず）\n%s%s", code, out, errs)
					}
					if len(got) != 0 {
						t.Errorf("トークンがないのにリクエストを送った: %v", got)
					}
					return
				}
				if code != 0 {
					t.Errorf("終了コード %d（0 のはず）\n%s%s", code, out, errs)
				}
				if want := []sent{{host, "Bearer " + c.wantKey}}; !reflect.DeepEqual(got, want) {
					t.Errorf("送ったリクエスト: %v（%v のはず）", got, want)
				}
			})
		}
	}
}

// CMD-01: vanda doctor は、ストーリーストアがカスタムアダプタなら、.vanda/adapters/ にあって実行できるかを確かめる
// @vanda:evidence doctor-project-adapter
// @vanda:evidence project-adapter-not-runnable
func Test_vanda_doctorはストーリーストアのカスタムアダプタがあって実行できるかを確かめる(t *testing.T) {
	t.Parallel()
	bin := buildVanda(t)
	version, err := runningVersion(bin)
	if err != nil {
		t.Fatal(err)
	}
	story := "cat >/dev/null\necho '{\"title\":\"t\",\"body\":\"\",\"url\":\"\"}'\n"
	cases := []struct {
		name     string
		adapters map[string]string
		noexec   bool
		ok       bool
		where    string
		// noShebang は、アダプタを 1 行目の動かし方（#!）なしで、実行の権限を付けて置き直す
		noShebang bool
		reason    string
	}{
		{"ファイルがあって実行できる", map[string]string{"jira": story}, false, true, "", false, ""},
		{"ディレクトリのrunがあって実行できる", map[string]string{"jira/run": story}, false, true, "", false, ""},
		{"ない", nil, false, false, ".vanda/adapters/jira", false, ""},
		{"実行の権限がない", map[string]string{"jira": story}, true, false, ".vanda/adapters/jira", false, ""},
		{"ディレクトリにrunがない", map[string]string{"jira/other": story}, false, false, ".vanda/adapters/jira/run", false, ""},
		{"1行目に動かし方がない", map[string]string{"jira": story}, false, false, ".vanda/adapters/jira", true, "1 行目に、動かし方がありません"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			root := ifaceRepo(t, version, "\n[story]\nadapter = \"jira\"\n", nil, c.adapters)
			if c.noexec {
				if err := os.Chmod(filepath.Join(root, ".vanda/adapters/jira"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if c.noShebang {
				if err := os.WriteFile(filepath.Join(root, ".vanda/adapters/jira"), []byte(story), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			code, out, errs := runVandaIn(t, bin, root, newRVGit(t, root).env, "doctor")
			if c.reason != "" && !strings.Contains(errs, c.reason) {
				t.Errorf("誤りに「%s」が出ていない\n%s", c.reason, errs)
			}
			if (code == 0) != c.ok {
				t.Errorf("終了コード %d（通るのは %v のはず）\n%s%s", code, c.ok, out, errs)
			}
			if c.ok && !strings.Contains(out, "ストーリーストアのカスタムアダプタ jira（") {
				t.Errorf("確かめたカスタムアダプタが出力に出ていない\n%s", out)
			}
			// ないときは、設定を読む所で、組み込みにも .vanda/adapters/ にもないという誤りになる
			if !c.ok && c.adapters != nil && !strings.Contains(errs, "ストーリーストアのカスタムアダプタ jira（") {
				t.Errorf("誤りに、どのカスタムアダプタかが出ていない\n%s", errs)
			}
			if !c.ok && (!strings.Contains(errs, c.where) || !strings.Contains(errs, "ください。")) {
				t.Errorf("誤りに、場所 %s と直し方が出ていない\n%s", c.where, errs)
			}
		})
	}
}

// CMD-10: アダプタには、アノテーションを読むファイルの HEAD のコミットの中身を、標準入力の JSON の content で渡す。
// 作業ツリーには、行のずれる書きかけ（コミットしていない変更）と、git add しただけの変更の 2 つを試す。
// アダプタは受け取った標準入力を .in.json に書き写し、1 行目に名前を返す
// @vanda:evidence project-adapter-reads-committed-content
// @vanda:evidence reads-committed-files-only
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
// @vanda:evidence project-adapter-annotation-timeout
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
// （レスポンスを返さずに待ち続けることがない）。約束の詳細はエビデンスを探すのでアノテーションを読む
// @vanda:evidence project-adapter-annotation-timeout
func Test_vanda_adaptersのアダプタがtimeoutを過ぎたら約束の詳細のAPIがどのアダプタかを書いたタイムアウトの誤りを返す(t *testing.T) {
	t.Parallel()
	bin := buildVanda(t)
	version, err := runningVersion(bin)
	if err != nil {
		t.Fatal(err)
	}
	promises := "[[term]]\nid = \"task\"\nname = \"タスク\"\ndefinition = \"やること\"\n\n[[claim]]\nid = \"task-id-unique\"\nsubjects = [\"task\"]\nstatement = \"タスクの ID は重ならない\"\nlayers = [\"model\"]\n\n[[claim.evidence]]\nstage = \"implementation\"\ntype = \"unit-test\"\nlayers = [\"model\"]\n"
	root := ifaceRepo(t, version, annotationRule("slow-adapter", "timeout = 1\n")+"\n[gui]\nbase = \"main\"\n",
		map[string]string{".vanda/promises.toml": promises, "ext/a.ext": "@vanda:evidence task-id-unique\n"},
		map[string]string{"slow-adapter": "cat >/dev/null\nsleep 30\n"})
	g := newRVGit(t, root)
	g.commitAll("(test) はじめ", "#1")
	s := startServe(t, bin, root, g.env)
	start := time.Now()
	code, msg := s.get("/api/claims/task-id-unique", nil)
	if d := time.Since(start); d > 15*time.Second {
		t.Errorf("timeout の 1 秒で止めず、%v 待った", d)
	}
	if code == 200 || !strings.Contains(msg, "slow-adapter") || !strings.Contains(msg, "タイムアウト") {
		t.Errorf("どのアダプタかを書いたタイムアウトの誤りでない（%d）: %s", code, msg)
	}
}

// @vanda:evidence project-adapter-missing-answer
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
