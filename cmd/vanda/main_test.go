package main

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/creack/pty"
)

func gitRepo(t *testing.T, config string) string {
	t.Helper()
	dir := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", dir).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	if config != "" {
		if err := os.MkdirAll(filepath.Join(dir, ".vanda"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, ".vanda", "config.toml"), []byte(config), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// commitIn は、dir のリポジトリの作業ツリーのすべてを 1 つのコミットにする（審査は HEAD のコミットの中身だけを読むため）
func commitIn(t *testing.T, dir, subject string) {
	t.Helper()
	for _, args := range [][]string{
		{"rm", "-r", "-q", "--cached", "--ignore-unmatch", "."},
		{"add", "-A"},
		{"-c", "user.name=T", "-c", "user.email=t@example.com", "-c", "commit.gpgsign=false", "commit", "-q", "--allow-empty", "-m", subject},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}

func runIn(t *testing.T, dir string, args ...string) (int, string, string) {
	t.Helper()
	t.Chdir(dir)
	var out, errb bytes.Buffer
	code := run(args, &out, &errb)
	return code, out.String(), errb.String()
}

// @vanda:evidence version-always-runs
func TestVersionは版が合わなくても版を出す(t *testing.T) {
	dir := gitRepo(t, `version = "9.9.9"`)
	code, out, _ := runIn(t, dir, "version")
	if code != 0 || out != "0.1.0\n" {
		t.Errorf("code %d, out %q", code, out)
	}
}

// @vanda:evidence config-version-mismatch
func TestDoctorは版が合わなければ止まる(t *testing.T) {
	dir := gitRepo(t, `version = "9.9.9"`)
	code, out, errs := runIn(t, dir, "doctor")
	// 誤りの終わりの「。」（直し方の後にも付ける）を、CLI の出力で確かめる 1 か所。ほかは文体に頼らず意味の要で照合する
	if code != 1 || out != "" || !strings.Contains(errs, ".vanda/config.toml:1: 動いている vanda（0.1.0）が、設定の版（9.9.9）より古いです。vanda を 9.9.9 に上げてください。\n") {
		t.Errorf("code %d, stderr %q", code, errs)
	}
}

// @vanda:evidence doctor-ok-line
func TestDoctorはリポジトリの根の設定を読む(t *testing.T) {
	dir := gitRepo(t, `version = "0.1.0"`)
	sub := filepath.Join(dir, "a", "b")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	code, out, errs := runIn(t, sub, "doctor")
	if code != 0 || !strings.HasPrefix(out, "設定を確かめ") || !strings.Contains(out, ".vanda/config.toml") || strings.Count(out, "\n") != 1 || errs != "" {
		t.Errorf("code %d, out %q, stderr %q", code, out, errs)
	}
}

// @vanda:evidence outside-git-repository
func TestDoctorはgitのリポジトリの外では誤りにする(t *testing.T) {
	code, _, errs := runIn(t, t.TempDir(), "doctor")
	if code != 1 || !strings.Contains(errs, "git のリポジトリの外") {
		t.Errorf("code %d, stderr %q", code, errs)
	}
}

// @vanda:evidence check-config-required
func TestCheckは審査の設定のファイルを渡さなければ誤りにする(t *testing.T) {
	dir := gitRepo(t, `version = "0.1.0"`)
	code, _, errs := runIn(t, dir, "check")
	if code != 1 || !strings.Contains(errs, "審査の設定のファイル") || !strings.Contains(errs, "既定の審査") {
		t.Errorf("code %d, stderr %q", code, errs)
	}
}

// @vanda:evidence check-runs-in-order-and-fails
// @vanda:evidence check-output-lines
func TestCheckは審査の設定のファイルをカレントディレクトリから読み失敗なら終了コード1にする(t *testing.T) {
	dir := gitRepo(t, `version = "0.1.0"`)
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(filepath.Join(sub, "checks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "checks", "push.toml"), []byte("[[check]]\nname = \"unread-files\"\nscope = \"all\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".vanda", "memo.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	commitIn(t, dir, "memo を足す")
	code, out, errs := runIn(t, sub, "check", "checks/push.toml")
	if code != 1 || out != "審査 unread-files（all）: 失敗（1 件）\n" || !strings.HasPrefix(errs, ".vanda/memo.txt: ") || !strings.Contains(errs, "読まれて") || !strings.Contains(errs, "promises か agreements") || strings.Count(errs, "\n") != 1 {
		t.Errorf("code %d, out %q, stderr %q", code, out, errs)
	}
	if err := os.Remove(filepath.Join(dir, ".vanda", "memo.txt")); err != nil {
		t.Fatal(err)
	}
	commitIn(t, dir, "memo を消す")
	code, out, _ = runIn(t, sub, "check", "checks/push.toml")
	if code != 0 || out != "審査 unread-files（all）: 通った\n" {
		t.Errorf("code %d, out %q", code, out)
	}
}

// @vanda:evidence version-always-runs
func TestVersionはvandaのディレクトリがなくてもgitのリポジトリの外でも版を1行で出す(t *testing.T) {
	for name, dir := range map[string]string{"vandaのディレクトリがない": gitRepo(t, ""), "gitのリポジトリの外": t.TempDir()} {
		code, out, errs := runIn(t, dir, "version")
		if code != 0 || out != "0.1.0\n" || errs != "" {
			t.Errorf("%s: code %d, out %q, stderr %q", name, code, out, errs)
		}
	}
}

// withoutGH は、どのホストにもログインしていない偽の gh を PATH の先頭に置く。本物の gh の認証キーを読ませないため
func withoutGH(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte("#!/bin/sh\necho 'no oauth token found' >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// @vanda:evidence doctor-github-issues
func TestDoctorはストーリーストアが組込みのgithubならリポジトリを読みIssuesが使えるかを確かめる(t *testing.T) {
	for _, c := range []struct {
		name      string
		hasIssues bool
		wantCode  int
	}{{"Issuesが使える", true, 0}, {"Issuesが使えない", false, 1}} {
		var hit string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			hit = req.Method + " " + req.URL.Path + " " + req.Header.Get("Authorization")
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"full_name":"o/r","has_issues":%v}`, c.hasIssues)
		}))
		// 偽のサーバー（127.0.0.1）は api.github.com でないので、認証キーは GH_ENTERPRISE_TOKEN から読まれる
		t.Setenv("GH_ENTERPRISE_TOKEN", "tok123")
		t.Setenv("GH_TOKEN", "")
		withoutGH(t)
		dir := gitRepo(t, "version = \"0.1.0\"\n[story]\nadapter = \"github\"\nrepository = \"o/r\"\napi_url = \""+srv.URL+"\"\n")
		code, out, errs := runIn(t, dir, "doctor")
		srv.Close()
		if !strings.HasPrefix(hit, "GET /repos/o/r ") || !strings.Contains(hit, "tok123") {
			t.Errorf("%s: GET <api_url>/repos/<repository> を鍵を付けて呼んでいない: %q", c.name, hit)
		}
		if code != c.wantCode {
			t.Errorf("%s: code %d（%d のはず）, out %q, stderr %q", c.name, code, c.wantCode, out, errs)
		}
	}
}

// @vanda:evidence doctor-project-adapter
func TestDoctorはストーリーストアがカスタムアダプタならあって実行できるかを確かめる(t *testing.T) {
	cfg := "version = \"0.1.0\"\n[story]\nadapter = \"jira\"\n"
	for _, c := range []struct {
		name     string
		mode     os.FileMode
		create   bool
		wantCode int
	}{{"あって実行できる", 0o755, true, 0}, {"実行の権限がない", 0o644, true, 1}, {"ない", 0, false, 1}} {
		dir := gitRepo(t, cfg)
		if c.create {
			if err := os.MkdirAll(filepath.Join(dir, ".vanda", "adapters"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, ".vanda", "adapters", "jira"), []byte("#!/bin/sh\n"), c.mode); err != nil {
				t.Fatal(err)
			}
		}
		code, out, errs := runIn(t, dir, "doctor")
		if code != c.wantCode {
			t.Errorf("%s: code %d（%d のはず）, out %q, stderr %q", c.name, code, c.wantCode, out, errs)
		}
		if c.wantCode != 0 && !strings.Contains(errs, ".vanda/adapters/jira") {
			t.Errorf("%s: 誤りにカスタムアダプタの場所が出ていない: %q", c.name, errs)
		}
	}
}

// @vanda:evidence config-file-required
func Test設定のファイルがなければdoctorもcheckも場所の出る誤りにする(t *testing.T) {
	dir := gitRepo(t, "")
	if err := os.WriteFile(filepath.Join(dir, "c.toml"), []byte("[[check]]\nname = \"unread-files\"\nscope = \"all\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"doctor"}, {"check", "c.toml"}} {
		code, _, errs := runIn(t, dir, args...)
		if code != 1 || !strings.Contains(errs, ".vanda/config.toml") {
			t.Errorf("%v: code %d, stderr %q", args, code, errs)
		}
	}
}

// @vanda:evidence init-terminal-streams
func Test端末かどうかは端末の属性で決まり_dev_nullもパイプもファイルも端末でない(t *testing.T) {
	ptmx, tty, err := pty.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer ptmx.Close()
	defer tty.Close()
	devNull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer devNull.Close()
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer pr.Close()
	defer pw.Close()
	file, err := os.Create(filepath.Join(t.TempDir(), "out.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	for _, c := range []struct {
		name string
		f    *os.File
		want bool
	}{
		{"擬似端末", tty, true},
		{"/dev/null", devNull, false},
		{"パイプの読む側", pr, false},
		{"パイプの書く側", pw, false},
		{"ふつうのファイル", file, false},
	} {
		if got := isTerminal(c.f); got != c.want {
			t.Errorf("%s: 端末か %v（%v のはず）", c.name, got, c.want)
		}
	}
}

// @vanda:evidence check-errors-before-run
func TestCheckは審査を始める前の誤りを標準エラーに出し結果の行を出さずに終了コード1で終わる(t *testing.T) {
	const okConfig = "version = \"0.1.0\"\n"
	const storyChecks = "[[check]]\nname = \"agreement\"\nscope = \"story\"\n"
	const allChecks = "[[check]]\nname = \"unread-files\"\nscope = \"all\"\n"
	cases := []struct {
		name, config, checks string
		args                 []string
		want                 string // 標準エラーに出るはずのもの
	}{
		{"設定の誤り", okConfig + "extra = 1\n", allChecks, nil, ".vanda/config.toml:2: "},
		{"版の違い", "version = \"9.9.9\"\n", allChecks, nil, "9.9.9"},
		{"審査の設定の誤り", okConfig, "[[check]]\nname = \"nope\"\nscope = \"all\"\n", nil, "c.toml:2: "},
		{"--baseがない", okConfig, storyChecks, nil, "--base"},
		{"--baseのブランチがない", okConfig, storyChecks, []string{"--base", "no-such-branch"}, "no-such-branch"},
		{"--storyの形の誤り", okConfig, allChecks, []string{"--story", "24"}, "--story 24"},
		{"--storyがトレーラーと違う", okConfig, storyChecks, []string{"--base", "main", "--story", "#9"}, "#9"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := gitRepo(t, c.config)
			if out, err := exec.Command("git", "-C", dir, "branch", "-m", "main").CombinedOutput(); err != nil {
				t.Fatalf("git branch: %v %s", err, out)
			}
			if err := os.WriteFile(filepath.Join(dir, "c.toml"), []byte(c.checks), 0o644); err != nil {
				t.Fatal(err)
			}
			commitIn(t, dir, "はじめ")
			if out, err := exec.Command("git", "-C", dir, "checkout", "-q", "-b", "work").CombinedOutput(); err != nil {
				t.Fatalf("git checkout: %v %s", err, out)
			}
			commitIn(t, dir, "作業\n\nStory: #2")
			code, out, errs := runIn(t, dir, append([]string{"check", "c.toml"}, c.args...)...)
			if code != 1 || out != "" || !strings.Contains(errs, c.want) {
				t.Errorf("code %d（1 のはず）, 標準出力 %q（空のはず）, 標準エラー %q（%q を含むはず）", code, out, errs, c.want)
			}
			if strings.HasPrefix(errs, "審査 ") || strings.Contains(errs, "\n審査 ") {
				t.Errorf("標準エラーに審査の結果の行が出た: %q", errs)
			}
		})
	}
}
