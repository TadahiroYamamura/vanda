package main_test

import (
	"bytes"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/creack/pty"
)

// CMD-11 vanda init。問いかけは擬似端末（pty）で vanda を動かし、問いを読んで答えを書く（docs/plans/init-test.md）。
// vanda init は、必ずテストごとの一時ディレクトリの git のリポジトリで動かす。
//
// 問いの文言は決まっていないので、問いは計画の順（ベースブランチ、ストーリーの置き場所、リポジトリかコマンド、
// ID の書き方、言語ごとのアノテーション、作るか）にたどり、それぞれの問いに出るはずの言葉で確かめる。
// 既定の答えは、計画のとおり、問いの中の [ ] の中から読む。

// ---- 一時的なリポジトリ ----

type initRepo struct {
	t    *testing.T
	root string
	g    *rvGit
}

// newInitRepo は、files を git に追わせたリポジトリを作る。remote が空でなければ origin に足し、originHEAD なら
// origin/HEAD が origin/main を指すようにする。
func newInitRepo(t *testing.T, remote string, originHEAD bool, files ...string) *initRepo {
	t.Helper()
	root := t.TempDir()
	r := &initRepo{t: t, root: root, g: newRVGit(t, root)}
	r.g.run("init", "-q", "-b", "main")
	writeFile(t, root, "README.md", "# 例\n")
	for _, f := range files {
		// Go のファイルは、アノテーションを読むときに Go として読めるものにする
		content := "\n"
		if strings.HasSuffix(f, ".go") {
			content = "package x\n"
		}
		writeFile(t, root, f, content)
	}
	r.g.run("add", "-A")
	r.g.run("commit", "-q", "-m", "はじめ")
	if remote != "" {
		r.g.run("remote", "add", "origin", remote)
	}
	if originHEAD {
		r.g.run("update-ref", "refs/remotes/origin/main", "HEAD")
		r.g.run("symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	}
	return r
}

func (r *initRepo) exists(path string) bool {
	_, err := os.Lstat(filepath.Join(r.root, path))
	return err == nil
}

// ---- 擬似端末で動かす ----

type initSession struct {
	t      *testing.T
	ptmx   *os.File
	cmd    *exec.Cmd
	mu     sync.Mutex
	out    bytes.Buffer
	read   int
	stderr bytes.Buffer
	done   chan struct{}
	err    error
}

// startInit は、標準入力と標準出力を擬似端末にして vanda init を動かす。標準エラーは別に受ける。
func startInit(t *testing.T, bin, dir string, env []string) *initSession {
	t.Helper()
	s := &initSession{t: t, done: make(chan struct{})}
	s.cmd = exec.Command(bin, "init")
	s.cmd.Dir = dir
	s.cmd.Env = append(env, "TERM=dumb")
	s.cmd.Stderr = &lockedWriter{mu: &s.mu, w: &s.stderr}
	ptmx, err := pty.Start(s.cmd)
	if err != nil {
		t.Fatalf("擬似端末で vanda init を動かせない: %v", err)
	}
	s.ptmx = ptmx
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				s.mu.Lock()
				s.out.Write(buf[:n])
				s.mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()
	go func() {
		s.err = s.cmd.Wait()
		close(s.done)
	}()
	t.Cleanup(func() {
		select {
		case <-s.done:
		default:
			_ = s.cmd.Process.Kill()
			<-s.done
		}
		_ = ptmx.Close()
	})
	return s
}

type lockedWriter struct {
	mu *sync.Mutex
	w  *bytes.Buffer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

func (s *initSession) all() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.out.String()
}

// prompt は、前に読んだ所より後に出た文字を、出力が 300 ミリ秒止まるまで待って返す。問いを出して答えを待っている所とみなす。
func (s *initSession) prompt(what string) string {
	s.t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	last, since := -1, time.Now()
	for time.Now().Before(deadline) {
		s.mu.Lock()
		n := s.out.Len()
		s.mu.Unlock()
		if n != last {
			last, since = n, time.Now()
		} else if n > s.read && time.Since(since) > 300*time.Millisecond {
			break
		}
		select {
		case <-s.done:
			s.mu.Lock()
			n = s.out.Len()
			s.mu.Unlock()
			if n == s.read {
				s.t.Fatalf("%s の問いを待っている間に vanda init が終わった\n--- 出力\n%s\n--- 標準エラー\n%s", what, s.all(), s.stderr.String())
			}
		default:
		}
		time.Sleep(50 * time.Millisecond)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.out.Len() == s.read {
		s.t.Fatalf("%s の問いが出ない\n--- 出力\n%s", what, s.out.String())
	}
	chunk := s.out.String()[s.read:]
	s.read = s.out.Len()
	return chunk
}

func (s *initSession) answer(a string) {
	s.t.Helper()
	if _, err := s.ptmx.Write([]byte(a + "\r")); err != nil {
		s.t.Fatalf("答え %q を書けない: %v", a, err)
	}
}

// eof は、行の頭で入力を終える（Ctrl+D）。
func (s *initSession) eof() {
	s.t.Helper()
	if _, err := s.ptmx.Write([]byte{4}); err != nil {
		s.t.Fatalf("Ctrl+D を書けない: %v", err)
	}
}

// wait は、vanda init が終わるのを待ち、終了コードを返す。
func (s *initSession) wait() int {
	s.t.Helper()
	select {
	case <-s.done:
	case <-time.After(15 * time.Second):
		s.t.Fatalf("vanda init が終わらない\n--- 出力\n%s", s.all())
	}
	// 終わった後に残った出力を読み切る
	time.Sleep(100 * time.Millisecond)
	var exitErr *exec.ExitError
	if errors.As(s.err, &exitErr) {
		return exitErr.ExitCode()
	}
	if s.err != nil {
		s.t.Fatalf("vanda init の終わりを待てない: %v", s.err)
	}
	return 0
}

// ---- 問いをたどる ----

// promptLine は、問いの最後の空でない行（答えを待っている行）を返す。
func promptLine(chunk string) string {
	lines := strings.Split(strings.ReplaceAll(chunk, "\r", ""), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) != "" {
			return strings.TrimSpace(lines[i])
		}
	}
	return ""
}

// defaultOf は、答えを待っている行の [ ] の中を返す。既定の答え（#[0-9]+ など）に [ ] が入ることがあるので、
// 行の最初の [ から最後の ] までをとる。ないときは ok が false。
func defaultOf(chunk string) (string, bool) {
	line := promptLine(chunk)
	i, j := strings.Index(line, "["), strings.LastIndex(line, "]")
	if i < 0 || j <= i {
		return "", false
	}
	return strings.TrimSpace(line[i+1 : j]), true
}

var storyChoices = []string{"GitHub Issues", "外部のコマンド", "使わない"}

// choiceNumbers は、選択肢の名前から、問いに出た番号を読む。
func choiceNumbers(t *testing.T, chunk string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, c := range storyChoices {
		m := regexp.MustCompile(`(\d+)\D{0,3}` + regexp.QuoteMeta(c)).FindStringSubmatch(chunk)
		if m == nil {
			t.Fatalf("ストーリーの置き場所の問いに、選択肢「%s」と番号がない\n%s", c, chunk)
		}
		out[c] = m[1]
	}
	return out
}

// initPlan は、問いへの答え。空の文字列は、既定の答えのまま Enter を押すこと。
type initPlan struct {
	base      string
	story     string // 選択肢の名前
	detail    string // リポジトリかコマンド
	idPattern string
	langs     map[string]string // 言語の名前（Go・シナリオ・TLA+・SQL）→ 読むファイルのパターン
	confirm   string
	// 形に合わない答え。問いの名前（story・detail・id）ごとに、正しい答えの前に順に書く
	invalid map[string][]string
}

type initSeen struct {
	baseDefault  string
	storyDefault string // 選択肢の名前
	detailAsked  bool
	detailDef    string
	detailHasDef bool
	idDefault    string
	idHasDefault bool
	langsAsked   []string
	patterns     map[string]string // 言語 → 既定のパターン
}

var langNames = []string{"Go", "シナリオ", "TLA+", "SQL"}

// ask は、keyword を含む問いを読み、形に合わない答えを順に書いて、そのたびに理由と同じ問いが出ることを確かめ、最後に a を書く。
func (s *initSession) ask(what, keyword string, chunk string, invalid []string, a string) {
	s.t.Helper()
	if !strings.Contains(chunk, keyword) {
		s.t.Fatalf("%s の問いに「%s」がない\n%s", what, keyword, chunk)
	}
	line := promptLine(chunk)
	for _, bad := range invalid {
		s.answer(bad)
		again := s.prompt(what + "（聞き直し）")
		if promptLine(again) != line {
			s.t.Errorf("%s に形に合わない答え %q を書いても、同じ問い（%q）に戻らない\n%s", what, bad, line, again)
		}
		if !strings.Contains(again, "。") {
			s.t.Errorf("%s に形に合わない答え %q を書いても、理由が出ない\n%s", what, bad, again)
		}
	}
	s.answer(a)
}

// run は、計画の順に問いをたどり、plan の答えを書く。作るかの問いに答えたところで返す。
func (s *initSession) run(plan initPlan) initSeen {
	s.t.Helper()
	seen := initSeen{patterns: map[string]string{}}

	c := s.prompt("ベースブランチ")
	seen.baseDefault, _ = defaultOf(c)
	s.ask("ベースブランチ", "ベースブランチ", c, nil, plan.base)

	c = s.prompt("ストーリーの置き場所")
	nums := choiceNumbers(s.t, c)
	if d, ok := defaultOf(c); ok {
		seen.storyDefault = d
		for name, n := range nums {
			if d == n || d == name {
				seen.storyDefault = name
			}
		}
	}
	chosen := plan.story
	if chosen == "" {
		chosen = seen.storyDefault
	}
	a := ""
	if plan.story != "" {
		a = nums[plan.story]
	}
	s.ask("ストーリーの置き場所", "ストーリーの置き場所", c, plan.invalid["story"], a)

	switch chosen {
	case "GitHub Issues", "外部のコマンド":
		kw := "リポジトリ"
		if chosen == "外部のコマンド" {
			kw = "コマンド"
		}
		c = s.prompt(kw)
		seen.detailAsked = true
		seen.detailDef, seen.detailHasDef = defaultOf(c)
		s.ask(kw, kw, c, plan.invalid["detail"], plan.detail)
	}

	c = s.prompt("ID の書き方")
	seen.idDefault, seen.idHasDefault = defaultOf(c)
	s.ask("ID の書き方", "ID", c, plan.invalid["id"], plan.idPattern)

	for {
		c = s.prompt("言語か作るか")
		if strings.Contains(c, ".vanda/config.toml") {
			s.ask("作るか", "作", c, nil, plan.confirm)
			return seen
		}
		lang := ""
		for _, n := range langNames {
			if strings.Contains(c, n) {
				lang = n
			}
		}
		if lang == "" {
			s.t.Fatalf("言語の問いに、どの言語かが出ていない\n%s", c)
		}
		// 見つかった言語はいつも読む。問いは読むファイルのパターンだけ
		seen.langsAsked = append(seen.langsAsked, lang)
		seen.patterns[lang], _ = defaultOf(c)
		s.ask(lang+" のパターン", "読むファイル", c, nil, plan.langs[lang])
	}
}

// ---- 作ったファイル ----

type initConfig struct {
	Version    string           `toml:"version"`
	Layer      []map[string]any `toml:"layer"`
	Annotation []struct {
		Paths   []string `toml:"paths"`
		Adapter string   `toml:"adapter"`
		Command []string `toml:"command"`
	} `toml:"annotation"`
	Story *struct {
		IDPattern  string   `toml:"id_pattern"`
		Adapter    string   `toml:"adapter"`
		Repository string   `toml:"repository"`
		Command    []string `toml:"command"`
	} `toml:"story"`
	Gui *struct {
		Base string `toml:"base"`
	} `toml:"gui"`
}

func readInitConfig(t *testing.T, root string) initConfig {
	t.Helper()
	var c initConfig
	md, err := toml.DecodeFile(filepath.Join(root, ".vanda", "config.toml"), &c)
	if err != nil {
		t.Fatalf("作った設定を読めない: %v", err)
	}
	// promises・agreements は既定のままなので書かない。ほかにも決まっていない項目を書かない
	if u := md.Undecoded(); len(u) > 0 {
		t.Errorf("設定に、書かないはずの項目がある: %v", u)
	}
	return c
}

// annotationsOf は、設定の [[annotation]] を「アダプタ パターン」の並びにする（順は問わない）。
func annotationsOf(c initConfig) []string {
	var out []string
	for _, a := range c.Annotation {
		out = append(out, a.Adapter+" "+strings.Join(a.Paths, ",")+strings.Join(a.Command, " "))
	}
	sort.Strings(out)
	return out
}

// 計画の「問い 5 の言語の見つけ方と既定のパターン」の表
var initLangs = []struct {
	name, file, pattern, adapter string
}{
	{"Go", "internal/x/x_test.go", "**/*_test.go", "go"},
	{"シナリオ", "e2e/a.txtar", "**/*.txtar", "txtar"},
	{"TLA+", "spec/S.tla", "**/*.tla", "tla"},
	{"SQL", "db/schema.sql", "**/*.sql", "sql"},
}

// checkNoComments は、作った 4 つのファイルにコメントの行がないことを確かめる。
func checkNoComments(t *testing.T, root string) {
	t.Helper()
	for _, f := range []string{"config.toml", "promises.toml", "agreements.toml", "checks/ci.toml"} {
		data, err := os.ReadFile(filepath.Join(root, ".vanda", f))
		if err != nil {
			t.Errorf(".vanda/%s ができていない: %v", f, err)
			continue
		}
		for i, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "#") {
				t.Errorf(".vanda/%s の %d 行目がコメント: %q", f, i+1, line)
			}
		}
	}
}

func initSetup(t *testing.T) (string, string) {
	t.Helper()
	bin := buildVanda(t)
	version, err := runningVersion(bin)
	if err != nil {
		t.Fatal(err)
	}
	return bin, version
}

// ---- テスト ----

// 偽の GitHub。vanda doctor が呼ぶ GET /repos/<owner>/<repo> に、Issues が使えるリポジトリとして応える
const fakeRepoToken = "gho_inittesttoken"

type fakeRepoAPI struct {
	URL  string
	mu   sync.Mutex
	hits int
}

func newFakeRepoAPI(t *testing.T, repo string) *fakeRepoAPI {
	t.Helper()
	f := &fakeRepoAPI{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		auth := req.Header.Get("Authorization")
		if req.Method != http.MethodGet || req.URL.Path != "/repos/"+repo || (auth != "Bearer "+fakeRepoToken && auth != "token "+fakeRepoToken) {
			writeGitHubError(w, http.StatusNotFound, "Not Found")
			return
		}
		f.mu.Lock()
		f.hits++
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write([]byte(`{"full_name":"` + repo + `","has_issues":true}`))
	}))
	t.Cleanup(srv.Close)
	f.URL = srv.URL
	return f
}

func (f *fakeRepoAPI) called() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.hits > 0
}

// CMD-11
func Test既定の答えのまま進めると読み取った値で4つのファイルができdoctorとciの審査が通る(t *testing.T) {
	bin, version := initSetup(t)
	var files []string
	for _, l := range initLangs {
		files = append(files, l.file)
	}
	r := newInitRepo(t, "https://github.com/owner/app.git", true, files...)
	s := startInit(t, bin, r.root, r.g.env)
	seen := s.run(initPlan{})
	if code := s.wait(); code != 0 {
		t.Fatalf("vanda init が %d で終わった\n%s\n%s", code, s.all(), s.stderr.String())
	}
	if seen.baseDefault != "origin/main" || seen.storyDefault != "GitHub Issues" || seen.detailDef != "owner/app" || seen.idDefault != "#[0-9]+" {
		t.Errorf("既定の答え: %+v", seen)
	}

	c := readInitConfig(t, r.root)
	if c.Version != version {
		t.Errorf("version が %q（動いている vanda は %q）", c.Version, version)
	}
	if len(c.Layer) != 0 {
		t.Errorf("[[layer]] を書いている: %v", c.Layer)
	}
	if c.Gui == nil || c.Gui.Base != "origin/main" {
		t.Errorf("[gui] の base: %+v", c.Gui)
	}
	if c.Story == nil || c.Story.Adapter != "github" || c.Story.Repository != "owner/app" || c.Story.IDPattern != "#[0-9]+" || c.Story.Command != nil {
		t.Errorf("[story]: %+v", c.Story)
	}
	var want []string
	for _, l := range initLangs {
		want = append(want, l.adapter+" "+l.pattern)
	}
	sort.Strings(want)
	if got := annotationsOf(c); !reflect.DeepEqual(got, want) {
		t.Errorf("[[annotation]]: %v（思ったもの %v）", got, want)
	}
	for _, f := range []string{"promises.toml", "agreements.toml"} {
		data, err := os.ReadFile(filepath.Join(r.root, ".vanda", f))
		if err != nil || strings.TrimSpace(string(data)) != "" {
			t.Errorf(".vanda/%s が空でない（%v）: %q", f, err, data)
		}
	}
	checkNoComments(t, r.root)
	// 作った後の案内に、[[layer]] を書くことと、CI に足す vanda check が出る
	if !strings.Contains(s.all(), "[[layer]]") {
		t.Errorf("案内に [[layer]] を書くことが出ていない\n%s", s.all())
	}
	if !strings.Contains(s.all(), "vanda check .vanda/checks/ci.toml --base origin/main") {
		t.Errorf("案内に、CI に足す vanda check が出ていない\n%s", s.all())
	}
	// コミットはしない
	if n := r.g.run("rev-list", "--count", "HEAD"); n != "1" {
		t.Errorf("vanda init がコミットした（%s 個）", n)
	}

	// vanda doctor は GitHub に繋がるかを確かめるので、作った設定の api_url だけを偽の GitHub に向ける
	gh := newFakeRepoAPI(t, "owner/app")
	cfgPath := filepath.Join(r.root, ".vanda", "config.toml")
	cfgText, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, r.root, ".vanda/config.toml", strings.Replace(string(cfgText), "[story]\n", "[story]\napi_url = \""+gh.URL+"\"\n", 1))
	env := append(append([]string(nil), r.g.env...), "GH_TOKEN="+fakeRepoToken)
	doctor := exec.Command(bin, "doctor")
	doctor.Dir, doctor.Env = r.root, env
	if out, err := doctor.CombinedOutput(); err != nil {
		t.Errorf("作った設定で vanda doctor が通らない: %v\n%s", err, out)
	}
	if !gh.called() {
		t.Errorf("vanda doctor が、偽の GitHub のリポジトリを読みに来ていない")
	}
	// 作った直後のブランチにはストーリーの ID のあるコミットがないので、--story がなければストーリーが分からず止まる
	check := exec.Command(bin, "check", ".vanda/checks/ci.toml", "--base", "origin/main")
	check.Dir, check.Env = r.root, env
	if out, err := check.CombinedOutput(); err == nil || !strings.Contains(string(out), "ストーリー") {
		t.Errorf("--story なしの vanda check が、ストーリーが分からないとして止まらない: %v\n%s", err, out)
	}
	check = exec.Command(bin, "check", ".vanda/checks/ci.toml", "--base", "origin/main", "--story", "#1")
	check.Dir, check.Env = r.root, env
	if out, err := check.CombinedOutput(); err != nil {
		t.Errorf("作った審査の設定で vanda check --story #1 が通らない: %v\n%s", err, out)
	}
}

// CMD-11: 審査の設定には、今ある審査を全部、docs/formats.md の「審査の種類」の順に、審査ごとに決まった範囲で書く
func Test審査の設定には今ある審査が全部docs_formats_mdの順に決まった範囲で入る(t *testing.T) {
	bin, _ := initSetup(t)
	r := newInitRepo(t, "", false)
	s := startInit(t, bin, r.root, r.g.env)
	s.run(initPlan{idPattern: "#[0-9]+"})
	if code := s.wait(); code != 0 {
		t.Fatalf("vanda init が %d で終わった\n%s", code, s.all())
	}
	var doc struct {
		Check []struct {
			Name  string `toml:"name"`
			Scope string `toml:"scope"`
		} `toml:"check"`
	}
	if _, err := toml.DecodeFile(filepath.Join(r.root, ".vanda", "checks", "ci.toml"), &doc); err != nil {
		t.Fatalf("審査の設定を読めない: %v", err)
	}
	var got []string
	for _, c := range doc.Check {
		got = append(got, c.Name+" "+c.Scope)
	}
	// 書ける範囲が 1 つのものはそれ。all と story の両方を書ける agreement と evidence-present は、ストーリーで変わったものを見る story
	want := []string{
		"promise-list-shape all", "annotation-links all", "story-origin story", "agreement story",
		"agreement-targets all", "evidence-present story", "unread-files all",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("審査の設定: %v（思ったもの %v）", got, want)
	}
	checkNoComments(t, r.root)
}

// CMD-11: remote の URL の形ごとの、ストーリーの置き場所とリポジトリの既定の答え
func Test_remoteのURLの形ごとにストーリーの置き場所とリポジトリの既定の答えが決まる(t *testing.T) {
	bin, _ := initSetup(t)
	cases := []struct {
		name, url string
		github    bool
	}{
		{"httpsの形", "https://github.com/owner/app.git", true},
		{"httpsの形で.gitのないもの", "https://github.com/owner/app", true},
		{"sshの形", "git@github.com:owner/app.git", true},
		{"ssh_URLの形", "ssh://git@github.com/owner/app.git", true},
		{"sshの別名のホスト", "git@github.com_work:owner/app.git", true},
		{"GitHubでないホストのhttps", "https://gitlab.com/owner/app.git", false},
		{"GitHubでないホストのssh", "git@gitlab.com:owner/app.git", false},
		{"remoteなし", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := newInitRepo(t, c.url, c.url != "")
			// 既定の置き場所のまま進める。GitHub でないときは、GitHub Issues を選び直して、リポジトリに既定がないことを見る
			s := startInit(t, bin, r.root, r.g.env)
			plan := initPlan{story: "GitHub Issues", detail: "owner/app"}
			seen := s.run(plan)
			s.wait()
			wantStory := "使わない"
			if c.github {
				wantStory = "GitHub Issues"
			}
			if seen.storyDefault != wantStory {
				t.Errorf("ストーリーの置き場所の既定: %q（思ったもの %q）", seen.storyDefault, wantStory)
			}
			if c.github && (!seen.detailHasDef || seen.detailDef != "owner/app") {
				t.Errorf("リポジトリの既定: %q（思ったもの owner/app）", seen.detailDef)
			}
			if !c.github && seen.detailHasDef {
				t.Errorf("GitHub でない remote なのに、リポジトリに既定がある: %q", seen.detailDef)
			}
			wantBase := "main"
			if c.url != "" {
				wantBase = "origin/main"
			}
			if seen.baseDefault != wantBase {
				t.Errorf("ベースブランチの既定: %q（思ったもの %q）", seen.baseDefault, wantBase)
			}
		})
	}
}

// CMD-11: ストーリーの置き場所の選択肢の全部で、問い 3・4 の出方と既定と、書かれる [story] が合う
func Testストーリーの置き場所の選択肢ごとに問いの出方と既定とstoryの中身が合う(t *testing.T) {
	bin, _ := initSetup(t)
	cases := []struct {
		choice      string
		detail      string
		idPattern   string
		detailAsked bool
		detailKind  string
		idDefault   string // 空なら既定なし
		adapter     string
		repository  string
		command     []string
		wantPattern string
	}{
		{"GitHub Issues", "", "", true, "リポジトリ", "#[0-9]+", "github", "owner/app", nil, "#[0-9]+"},
		{"外部のコマンド", "tools/story-adapter --strict", "PROJ-[0-9]+", true, "コマンド", "", "", "", []string{"tools/story-adapter", "--strict"}, "PROJ-[0-9]+"},
		{"使わない", "", "PROJ-[0-9]+", false, "", "", "", "", nil, "PROJ-[0-9]+"},
	}
	for _, c := range cases {
		t.Run(c.choice, func(t *testing.T) {
			r := newInitRepo(t, "https://github.com/owner/app.git", true)
			s := startInit(t, bin, r.root, r.g.env)
			seen := s.run(initPlan{story: c.choice, detail: c.detail, idPattern: c.idPattern})
			if code := s.wait(); code != 0 {
				t.Fatalf("vanda init が %d で終わった\n%s", code, s.all())
			}
			if seen.detailAsked != c.detailAsked {
				t.Errorf("リポジトリかコマンドの問いが出たか: %v（思ったもの %v）", seen.detailAsked, c.detailAsked)
			}
			if c.choice == "外部のコマンド" && seen.detailHasDef {
				t.Errorf("外部のコマンドの問いに既定がある: %q", seen.detailDef)
			}
			if c.idDefault == "" && seen.idHasDefault {
				t.Errorf("ID の書き方に既定がある: %q（%s は既定を持たない）", seen.idDefault, c.choice)
			}
			if c.idDefault != "" && seen.idDefault != c.idDefault {
				t.Errorf("ID の書き方の既定: %q（思ったもの %q）", seen.idDefault, c.idDefault)
			}
			cfg := readInitConfig(t, r.root)
			st := cfg.Story
			if st == nil || st.Adapter != c.adapter || st.Repository != c.repository || !reflect.DeepEqual(st.Command, c.command) || st.IDPattern != c.wantPattern {
				t.Errorf("[story]: %+v（思ったもの adapter %q、repository %q、command %v、id_pattern %q）", st, c.adapter, c.repository, c.command, c.wantPattern)
			}
		})
	}
}

// CMD-11: 言語のあるなしの 16 通りで、問いに出る言語と書かれる [[annotation]] が合う
func Test言語のあるなしの16通りで問いに出る言語と書かれるannotationが合う(t *testing.T) {
	bin, _ := initSetup(t)
	for mask := 0; mask < 1<<len(initLangs); mask++ {
		var files, names, want []string
		for i, l := range initLangs {
			if mask&(1<<i) != 0 {
				files = append(files, l.file)
				names = append(names, l.name)
				want = append(want, l.adapter+" "+l.pattern)
			}
		}
		label := strings.Join(names, "と")
		if label == "" {
			label = "なし"
		}
		t.Run(label, func(t *testing.T) {
			r := newInitRepo(t, "", false, files...)
			s := startInit(t, bin, r.root, r.g.env)
			seen := s.run(initPlan{idPattern: "#[0-9]+"})
			if code := s.wait(); code != 0 {
				t.Fatalf("vanda init が %d で終わった\n%s", code, s.all())
			}
			asked := append([]string(nil), seen.langsAsked...)
			sort.Strings(asked)
			wantNames := append([]string(nil), names...)
			sort.Strings(wantNames)
			if !reflect.DeepEqual(asked, wantNames) {
				t.Errorf("問いに出た言語: %v（思ったもの %v）", asked, wantNames)
			}
			for _, l := range initLangs {
				if p, ok := seen.patterns[l.name]; ok && p != l.pattern {
					t.Errorf("%s のパターンの既定: %q（思ったもの %q）", l.name, p, l.pattern)
				}
			}
			sort.Strings(want)
			if got := annotationsOf(readInitConfig(t, r.root)); !reflect.DeepEqual(got, want) && !(len(got) == 0 && len(want) == 0) {
				t.Errorf("[[annotation]]: %v（思ったもの %v）", got, want)
			}
		})
	}
}

// CMD-11: 答えを変えると、その値で書かれる（ベースブランチ、リポジトリ、ID の書き方、言語ごとのパターン。コマンドは
// ストーリーの置き場所の選択肢のテストで変える）
func Test答えを変えるとその値で書かれる(t *testing.T) {
	bin, _ := initSetup(t)
	r := newInitRepo(t, "https://github.com/owner/app.git", true, "internal/x/x_test.go", "e2e/a.txtar")
	r.g.run("branch", "develop")
	s := startInit(t, bin, r.root, r.g.env)
	s.run(initPlan{
		base: "develop", detail: "other/repo", idPattern: "[A-Z]+-[0-9]+",
		langs: map[string]string{"Go": "internal/**/*_test.go", "シナリオ": "e2e/**/*.txtar"},
	})
	if code := s.wait(); code != 0 {
		t.Fatalf("vanda init が %d で終わった\n%s", code, s.all())
	}
	c := readInitConfig(t, r.root)
	if c.Gui == nil || c.Gui.Base != "develop" {
		t.Errorf("[gui] の base: %+v", c.Gui)
	}
	if c.Story == nil || c.Story.Repository != "other/repo" || c.Story.IDPattern != "[A-Z]+-[0-9]+" {
		t.Errorf("[story]: %+v", c.Story)
	}
	if got, want := annotationsOf(c), []string{"go internal/**/*_test.go", "txtar e2e/**/*.txtar"}; !reflect.DeepEqual(got, want) {
		t.Errorf("[[annotation]]: %v（思ったもの %v）", got, want)
	}
}

// CMD-11: 形に合わない答えは、理由が出て同じ問いに戻る
func Test形に合わない答えは理由が出て同じ問いに戻る(t *testing.T) {
	bin, _ := initSetup(t)
	t.Run("選択肢にない番号と既定のないIDの書き方の空と正規表現の誤りと空の文字列に当たる正規表現", func(t *testing.T) {
		r := newInitRepo(t, "", false)
		s := startInit(t, bin, r.root, r.g.env)
		s.run(initPlan{
			story: "外部のコマンド", detail: "tools/story-adapter", idPattern: "PROJ-[0-9]+",
			invalid: map[string][]string{"story": {"9", "0"}, "id": {"", "[", "a*"}},
		})
		if code := s.wait(); code != 0 {
			t.Fatalf("vanda init が %d で終わった\n%s", code, s.all())
		}
		if c := readInitConfig(t, r.root); c.Story == nil || c.Story.IDPattern != "PROJ-[0-9]+" || !reflect.DeepEqual(c.Story.Command, []string{"tools/story-adapter"}) {
			t.Errorf("聞き直した後の答えで書かれていない: %+v", c.Story)
		}
	})
	t.Run("リポジトリの形", func(t *testing.T) {
		r := newInitRepo(t, "https://github.com/owner/app.git", true)
		s := startInit(t, bin, r.root, r.g.env)
		s.run(initPlan{detail: "other/repo", invalid: map[string][]string{"detail": {"owner", "owner/", "/repo", "a/b/c"}}})
		if code := s.wait(); code != 0 {
			t.Fatalf("vanda init が %d で終わった\n%s", code, s.all())
		}
		if c := readInitConfig(t, r.root); c.Story == nil || c.Story.Repository != "other/repo" {
			t.Errorf("聞き直した後の答えで書かれていない: %+v", c.Story)
		}
	})
}

// CMD-11: 作る前の確かめで n と答えると、何も作られない
func Test作る前の確かめでnと答えると何も作られない(t *testing.T) {
	bin, _ := initSetup(t)
	r := newInitRepo(t, "https://github.com/owner/app.git", true, "e2e/a.txtar")
	s := startInit(t, bin, r.root, r.g.env)
	s.run(initPlan{confirm: "n"})
	if code := s.wait(); code != 1 {
		t.Errorf("n と答えたときの終了コードが %d（1 のはず）", code)
	}
	if r.exists(".vanda") {
		t.Errorf("n と答えたのに .vanda/ ができた")
	}
}

// CMD-11: 途中で入力を終える（Ctrl+D）と、何も作られない。問いごとに回す
func Test途中で入力を終えると何も作られない(t *testing.T) {
	bin, _ := initSetup(t)
	steps := []string{"ベースブランチ", "ストーリーの置き場所", "リポジトリ", "ID の書き方", "言語", "作るか"}
	for i, step := range steps {
		t.Run(step+"で終える", func(t *testing.T) {
			r := newInitRepo(t, "https://github.com/owner/app.git", true, "e2e/a.txtar")
			s := startInit(t, bin, r.root, r.g.env)
			for j := 0; j < i; j++ {
				s.prompt(steps[j])
				s.answer("")
			}
			s.prompt(step)
			s.eof()
			code := s.wait()
			if r.exists(".vanda") {
				t.Errorf("%s で入力を終えたのに .vanda/ ができた", step)
			}
			if code == 0 {
				t.Errorf("%s で入力を終えたのに、終了コードが 0", step)
			}
		})
	}
}

// CMD-11: 止める場合は、何も書かれず、理由が標準エラーに出て、終了コードが 1
func Test止める場合は何も書かず理由を標準エラーに出し終了コード1で終わる(t *testing.T) {
	bin, _ := initSetup(t)
	expectStop := func(t *testing.T, r *initRepo, cmd *exec.Cmd, stderr *bytes.Buffer, wait func() int, keyword string, before map[string]string) {
		t.Helper()
		if code := wait(); code != 1 {
			t.Errorf("終了コードが %d（1 のはず）", code)
		}
		if !strings.Contains(stderr.String(), keyword) || !strings.Contains(stderr.String(), "。") {
			t.Errorf("標準エラーに理由（%s）が出ていない: %q", keyword, stderr.String())
		}
		if got := snapshotDir(t, r.root); !reflect.DeepEqual(got, before) {
			t.Errorf("止めたのに、ファイルが変わった\n前 %v\n後 %v", before, got)
		}
	}

	// 端末でない。標準入力と標準出力の片方か両方が端末でない、の 3 通り
	for _, c := range []struct {
		name          string
		stdin, stdout bool // 端末か
	}{{"標準入力が端末でない", false, true}, {"標準出力が端末でない", true, false}, {"どちらも端末でない", false, false}} {
		t.Run(c.name, func(t *testing.T) {
			r := newInitRepo(t, "https://github.com/owner/app.git", true)
			before := snapshotDir(t, r.root)
			ptmx, tty, err := pty.Open()
			if err != nil {
				t.Fatal(err)
			}
			defer ptmx.Close()
			defer tty.Close()
			go func() { _, _ = bytes.NewBuffer(nil).ReadFrom(ptmx) }()
			cmd := exec.Command(bin, "init")
			cmd.Dir, cmd.Env = r.root, r.g.env
			var stdout, stderr bytes.Buffer
			cmd.Stdin, cmd.Stdout, cmd.Stderr = bytes.NewReader(nil), &stdout, &stderr
			if c.stdin {
				cmd.Stdin = tty
			}
			if c.stdout {
				cmd.Stdout = tty
			}
			err = cmd.Start()
			if err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			wait := func() int {
				select {
				case err := <-done:
					var exitErr *exec.ExitError
					if errors.As(err, &exitErr) {
						return exitErr.ExitCode()
					}
					if err != nil {
						t.Fatal(err)
					}
					return 0
				case <-time.After(15 * time.Second):
					_ = cmd.Process.Kill()
					t.Fatalf("端末でないのに vanda init が止まらない（問いかけて待っている）")
					return -1
				}
			}
			expectStop(t, r, cmd, &stderr, wait, "端末", before)
		})
	}

	runPty := func(t *testing.T, r *initRepo, dir string, keyword string) {
		t.Helper()
		before := snapshotDir(t, r.root)
		s := startInit(t, bin, dir, r.g.env)
		expectStop(t, r, s.cmd, &s.stderr, s.wait, keyword, before)
	}
	t.Run("gitの作業ツリーの中でない", func(t *testing.T) {
		root := t.TempDir()
		r := &initRepo{t: t, root: root, g: newRVGit(t, root)}
		runPty(t, r, root, "git")
	})
	t.Run(".vanda_がディレクトリである", func(t *testing.T) {
		r := newInitRepo(t, "https://github.com/owner/app.git", true)
		writeFile(t, r.root, ".vanda/config.toml", "version = \"0.0.1\"\n")
		runPty(t, r, r.root, ".vanda")
	})
	t.Run(".vanda_がファイルである", func(t *testing.T) {
		r := newInitRepo(t, "https://github.com/owner/app.git", true)
		writeFile(t, r.root, ".vanda", "ファイル\n")
		runPty(t, r, r.root, ".vanda")
	})
}

// snapshotDir は、dir の下のファイル（.git を除く）の場所と中身を返す。
func snapshotDir(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		if rel == ".git" && info.IsDir() {
			return filepath.SkipDir
		}
		if info.IsDir() {
			out[rel+"/"] = ""
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		out[rel] = string(data)
		return nil
	})
	if err != nil {
		t.Fatal(fmt.Errorf("%s を読めない: %w", dir, err))
	}
	return out
}
