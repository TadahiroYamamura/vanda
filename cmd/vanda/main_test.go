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
	code, out, errs := runIn(t, sub, "check", "checks/push.toml")
	if code != 1 || out != "審査 unread-files（all）: 失敗（1 件）\n" || !strings.HasPrefix(errs, ".vanda/memo.txt: ") || !strings.Contains(errs, "読まれて") || !strings.Contains(errs, "promises か agreements") || strings.Count(errs, "\n") != 1 {
		t.Errorf("code %d, out %q, stderr %q", code, out, errs)
	}
	if err := os.Remove(filepath.Join(dir, ".vanda", "memo.txt")); err != nil {
		t.Fatal(err)
	}
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

// @vanda:evidence doctor-github-issues
func TestDoctorは置き場所が組込みのgithubならリポジトリを読みIssuesが使えるかを確かめる(t *testing.T) {
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
		t.Setenv("GH_TOKEN", "tok123")
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
func TestDoctorは置き場所がvanda_adaptersのアダプタならあって実行できるかを確かめる(t *testing.T) {
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
			t.Errorf("%s: 誤りにアダプタの場所が出ていない: %q", c.name, errs)
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
