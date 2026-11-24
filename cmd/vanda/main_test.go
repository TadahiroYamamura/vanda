package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/creack/pty"

	"github.com/TadahiroYamamura/vanda/internal/initcmd"
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

// commitIn は、dir のリポジトリの作業ツリーのすべてを 1 つのコミットにする（チェックは HEAD のコミットの中身だけを読むため）
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

func writeChecks(t *testing.T, dir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "c.toml"), []byte("[[check]]\nname = \"unread-files\"\nscope = \"all\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// @vanda:evidence config-version-mismatch
func TestCheckはバージョンが合わなければ止まる(t *testing.T) {
	dir := gitRepo(t, `version = "9.9.9"`)
	writeChecks(t, dir)
	commitIn(t, dir, "設定を置く")
	code, out, errs := runIn(t, dir, "check", "c.toml")
	// 誤りの終わりの「。」（直し方の後にも付ける）を、CLI の出力で確かめる 1 か所。ほかは文体に頼らず意味の要で照合する
	if code != 1 || out != "" || !strings.Contains(errs, ".vanda/config.toml:1: 動いている vanda（0.1.0）が、設定のバージョン（9.9.9）より古いです。vanda を 9.9.9 に上げてください。\n") {
		t.Errorf("code %d, stderr %q", code, errs)
	}
}

// @vanda:evidence outside-git-repository
func TestCheckはgitのリポジトリの外では誤りにする(t *testing.T) {
	dir := t.TempDir()
	writeChecks(t, dir)
	code, _, errs := runIn(t, dir, "check", "c.toml")
	if code != 1 || !strings.Contains(errs, "git のリポジトリの外") {
		t.Errorf("code %d, stderr %q", code, errs)
	}
}

// @vanda:evidence check-config-required
func TestCheckはチェックの設定のファイルを渡さなければ誤りにする(t *testing.T) {
	dir := gitRepo(t, `version = "0.1.0"`)
	code, _, errs := runIn(t, dir, "check")
	if code != 1 || !strings.Contains(errs, "チェックの設定のファイル") || !strings.Contains(errs, "既定のチェック") {
		t.Errorf("code %d, stderr %q", code, errs)
	}
}

// @vanda:evidence check-runs-in-order-and-fails
// @vanda:evidence check-output-lines
func TestCheckはチェックの設定のファイルをカレントディレクトリから読み失敗なら終了コード1にする(t *testing.T) {
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
	if code != 1 || out != "チェック unread-files（all）: 失敗（1 件）\n" || !strings.HasPrefix(errs, ".vanda/memo.txt: ") || !strings.Contains(errs, "読まれて") || !strings.Contains(errs, "promises か agreements") || strings.Count(errs, "\n") != 1 {
		t.Errorf("code %d, out %q, stderr %q", code, out, errs)
	}
	if err := os.Remove(filepath.Join(dir, ".vanda", "memo.txt")); err != nil {
		t.Fatal(err)
	}
	commitIn(t, dir, "memo を消す")
	code, out, _ = runIn(t, sub, "check", "checks/push.toml")
	if code != 0 || out != "チェック unread-files（all）: 通った\n" {
		t.Errorf("code %d, out %q", code, out)
	}
}

// @vanda:evidence config-file-required
func Test設定のファイルがなければcheckは場所の出る誤りにする(t *testing.T) {
	dir := gitRepo(t, "")
	writeChecks(t, dir)
	code, _, errs := runIn(t, dir, "check", "c.toml")
	if code != 1 || !strings.Contains(errs, ".vanda/config.toml") {
		t.Errorf("code %d, stderr %q", code, errs)
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
func TestCheckはチェックを始める前の誤りを標準エラーに出し結果の行を出さずに終了コード1で終わる(t *testing.T) {
	const okConfig = "version = \"0.1.0\"\n"
	const storyChecks = "[[check]]\nname = \"agreement\"\nscope = \"story\"\n"
	const allChecks = "[[check]]\nname = \"unread-files\"\nscope = \"all\"\n"
	cases := []struct {
		name, config, checks string
		args                 []string
		want                 string // 標準エラーに出るはずのもの
	}{
		{"設定の誤り", okConfig + "extra = 1\n", allChecks, nil, ".vanda/config.toml:2: "},
		{"バージョンの違い", "version = \"9.9.9\"\n", allChecks, nil, "9.9.9"},
		{"チェックの設定の誤り", okConfig, "[[check]]\nname = \"nope\"\nscope = \"all\"\n", nil, "c.toml:2: "},
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
			if strings.HasPrefix(errs, "チェック ") || strings.Contains(errs, "\nチェック ") {
				t.Errorf("標準エラーにチェックの結果の行が出た: %q", errs)
			}
		})
	}
}

// @vanda:evidence init-check-failure-keeps
func Test_vanda_initは作った設定を読み直して誤りがあれば_vandaを残して直すべき誤りを返す(t *testing.T) {
	files := [][2]string{
		{".vanda/config.toml", "version = \"0.1.0\"\n\n[story]\nid_pattern = \"(\"\n"},
		{".vanda/promises.toml", ""},
		{".vanda/agreements.toml", ""},
	}
	root := gitRepo(t, "")
	if err := initcmd.Write(root, files); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err := report(&out, root, files, false)
	if err == nil || !strings.Contains(err.Error(), ".vanda/config.toml:4: ") || !strings.Contains(err.Error(), "id_pattern") {
		t.Errorf("作った設定の誤り（4 行目の id_pattern）を返さない: %v", err)
	}
	if strings.Contains(out.String(), "作りました") {
		t.Errorf("誤りがあるのに、作り終えた案内を出した: %q", out.String())
	}
	for _, f := range files {
		if b, rerr := os.ReadFile(filepath.Join(root, f[0])); rerr != nil || string(b) != f[1] {
			t.Errorf("作った %s が残っていない: %v", f[0], rerr)
		}
	}
}
