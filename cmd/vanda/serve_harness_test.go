package main_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

// API のテストは、ビルドした vanda で `vanda serve --port 0 --no-open` を外から動かして確かめる（監督の決定）。
// 段 1 と同じく、実装の内部の関数に頼らずに書くため。vanda サーバーはテストの中で起動し、テストの終わりに必ず止め、
// 止まったこと（プロセスが終わり、ポートがもう受け付けないこと）も確かめる。

var serveURLPattern = regexp.MustCompile(`http://127\.0\.0\.1:[0-9]+/`)

// serveTB は、*testing.T と *rapid.T の両方で使うためのもの。
type serveTB interface {
	Fatalf(format string, args ...any)
	Errorf(format string, args ...any)
	Cleanup(func())
}

func helper(t serveTB) {
	if h, ok := t.(interface{ Helper() }); ok {
		h.Helper()
	}
}

type vandaServer struct {
	t    serveTB
	cmd  *exec.Cmd
	URL  string // 最後の / を除いたもの
	host string
	done chan error

	mu     sync.Mutex
	output bytes.Buffer // 標準出力と標準エラー（落ちたときに見せる）
	stderr bytes.Buffer
	once   sync.Once
}

func (s *vandaServer) record(line string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.output.WriteString(line + "\n")
}

func (s *vandaServer) Output() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.output.String()
}

func (s *vandaServer) Stderr() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.stderr.String()
}

// serveSlots は、同時に動かす vanda serve の数の上限。テストを並べて動かすので、vanda サーバーが一度に立ちすぎないよう抑える。
var serveSlots = make(chan struct{}, 4)

// startServe は、dir で vanda serve を起動し、標準出力に出た URL を読んで返す。
func startServe(t serveTB, bin, dir string, env []string) *vandaServer {
	helper(t)
	return startServeArgs(t, bin, dir, env, "--port", "0", "--no-open")
}

// startServeArgs は、vanda serve を args の引数で起動する（startServe と同じく、標準出力に出た URL を読んで返す）。
func startServeArgs(t serveTB, bin, dir string, env []string, args ...string) *vandaServer {
	helper(t)
	serveSlots <- struct{}{}
	// 後に足した Cleanup ほど先に動くので、止めた（s.stop）後に枠を返す
	t.Cleanup(func() { <-serveSlots })
	s := &vandaServer{t: t, done: make(chan error, 1)}
	s.cmd = exec.Command(bin, append([]string{"serve"}, args...)...)
	s.cmd.Dir = dir
	s.cmd.Env = env
	stdout, err := s.cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("%v", err)
	}
	stderr, err := s.cmd.StderrPipe()
	if err != nil {
		t.Fatalf("%v", err)
	}
	if err := s.cmd.Start(); err != nil {
		t.Fatalf("vanda serve を起動できない: %v", err)
	}
	t.Cleanup(s.stop)

	found := make(chan string, 1)
	var readers sync.WaitGroup
	readers.Add(2)
	go func() {
		defer readers.Done()
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			s.record(sc.Text())
			if u := serveURLPattern.FindString(sc.Text()); u != "" {
				select {
				case found <- u:
				default:
				}
			}
		}
	}()
	go func() {
		defer readers.Done()
		sc := bufio.NewScanner(stderr)
		for sc.Scan() {
			s.record(sc.Text())
			s.mu.Lock()
			s.stderr.WriteString(sc.Text() + "\n")
			s.mu.Unlock()
		}
	}()
	go func() {
		// 出力を読み終えてから Wait を呼ぶ（os/exec の決まり）
		readers.Wait()
		s.done <- s.cmd.Wait()
	}()

	select {
	case u := <-found:
		s.URL = strings.TrimSuffix(u, "/")
		s.host = strings.TrimPrefix(s.URL, "http://")
	case err := <-s.done:
		s.done <- err
		t.Fatalf("vanda serve が URL を出さずに終わった: %v\n%s", err, s.Output())
	case <-time.After(15 * time.Second):
		t.Fatalf("vanda serve が 15 秒たっても URL を標準出力に出さない\n%s", s.Output())
	}
	return s
}

// stop は、Ctrl+C と同じ割り込みを送って vanda サーバーを止め、止まったことを確かめる。
func (s *vandaServer) stop() {
	s.once.Do(func() {
		if err := s.cmd.Process.Signal(os.Interrupt); err != nil && !errors.Is(err, os.ErrProcessDone) {
			s.t.Errorf("vanda serve に割り込みを送れない: %v", err)
		}
		select {
		case <-s.done:
		case <-time.After(10 * time.Second):
			s.t.Errorf("vanda serve が、割り込みから 10 秒たっても止まらない（強制的に止める）\n%s", s.Output())
			_ = s.cmd.Process.Kill()
			<-s.done
		}
		if s.host == "" {
			return
		}
		s.checkReleased()
	})
}

// checkReleased は、vanda サーバーがポートを手放したかを確かめる。
// 接続できるかでは確かめない。WSL2 では、誰も待ち受けていないループバックのポートにも接続できてしまうことがあるため。
// 同じポートで待ち受けられれば、vanda サーバーはポートを手放している。
func (s *vandaServer) checkReleased() {
	ln, err := net.Listen("tcp", s.host)
	if err != nil {
		s.t.Errorf("vanda serve を止めた後も、%s を手放していない: %v", s.host, err)
		return
	}
	ln.Close()
}

type apiErrorBody struct {
	Error struct {
		Message string `json:"message"`
		Fix     string `json:"fix"`
	} `json:"error"`
}

// do は、API を呼び、状態コードと中身を返す。out が nil でなければ、200 の中身を out に読む。
// 200 でなければ、誤りの形（{"error": {...}}）で読めることを確かめ、その message を返す。
func (s *vandaServer) do(method, path string, body any, out any) (int, string) {
	helper(s.t)
	var rd io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			s.t.Fatalf("%v", err)
		}
		rd = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, s.URL+path, rd)
	if err != nil {
		s.t.Fatalf("%v", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		s.t.Fatalf("%s %s が失敗した: %v\n%s", method, path, err, s.Output())
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		s.t.Fatalf("%v", err)
	}
	if resp.StatusCode != http.StatusOK {
		var e apiErrorBody
		if err := json.Unmarshal(data, &e); err != nil || e.Error.Message == "" {
			s.t.Errorf("%s %s の誤り（%d）が、決まった形（{\"error\": {\"message\": …}}）でない: %q", method, path, resp.StatusCode, data)
		}
		return resp.StatusCode, e.Error.Message
	}
	if out != nil {
		if err := json.Unmarshal(data, out); err != nil {
			s.t.Fatalf("%s %s の中身を読めない: %v\n%s", method, path, err, data)
		}
	}
	return resp.StatusCode, ""
}

func (s *vandaServer) get(path string, out any) (int, string) {
	helper(s.t)
	return s.do(http.MethodGet, path, nil, out)
}

func (s *vandaServer) post(path string, body, out any) (int, string) {
	helper(s.t)
	return s.do(http.MethodPost, path, body, out)
}

func q(v string) string { return url.QueryEscape(v) }

// テストの中で起動した vanda サーバーが、テストの終わりに止まることを確かめる（stop が確かめる）。
// あわせて、Host が 127.0.0.1:<待ち受けているポート>・localhost:<待ち受けているポート> でない要求を断ることも確かめる
// （CM-01: 自分のマシンの中でだけ開く）。
// @vanda:evidence serve-rejects-foreign-host
// @vanda:evidence serve-listens-localhost
// @vanda:evidence serve-prints-url
func Test起動したvanda_serveは止めると止まりほかのHostの要求を断る(t *testing.T) {
	t.Parallel()
	bin := buildVanda(t)
	version, err := runningVersion(bin)
	if err != nil {
		t.Fatal(err)
	}
	r := newReviewRepo(t, version)
	s := startServe(t, bin, r.Root, r.Env)

	_, port, err := net.SplitHostPort(s.host)
	if err != nil {
		t.Fatal(err)
	}
	other := "1"
	if port == "1" {
		other = "2"
	}
	cases := []struct {
		host string
		want int
	}{
		{"127.0.0.1:" + port, http.StatusOK},
		{"localhost:" + port, http.StatusOK},
		// 待ち受けているポートと違うポート
		{"127.0.0.1:" + other, http.StatusForbidden},
		{"localhost:" + other, http.StatusForbidden},
		// ポートのない Host
		{"127.0.0.1", http.StatusForbidden},
		{"localhost", http.StatusForbidden},
		// ほかのホストの名前
		{"example.com", http.StatusForbidden},
		{"example.com:" + port, http.StatusForbidden},
	}
	for _, c := range cases {
		req, err := http.NewRequest(http.MethodGet, s.URL+"/api/context", nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Host = c.host
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != c.want {
			t.Errorf("%s で待ち受けているとき、Host が %q の要求に %d を返した（%d を返す）", port, c.host, resp.StatusCode, c.want)
		}
	}

	s.stop()
	if s.cmd.ProcessState == nil {
		t.Errorf("vanda serve のプロセスが終わっていない")
	}
}

// GET 以外の要求で、Origin があって http://<Host> でなければ 403 で断る（ブラウザーのほかのサイトからの書き込みを防ぐ）
// @vanda:evidence serve-rejects-foreign-origin
func Testほかのサイトから来たGETでない要求は403で断りOriginがないか同じなら断らない(t *testing.T) {
	t.Parallel()
	bin := buildVanda(t)
	version, err := runningVersion(bin)
	if err != nil {
		t.Fatal(err)
	}
	r := newReviewRepo(t, version)
	s := startServe(t, bin, r.Root, r.Env)
	head := r.git("rev-parse", "HEAD")
	body := `{"kind":"claim","id":"task-id-unique","path":null,"fingerprint":"sha256:0","story":"#24"}`
	send := func(method, origin string) int {
		t.Helper()
		var rd io.Reader
		if method != http.MethodGet {
			rd = strings.NewReader(body)
		}
		path := "/api/agree"
		if method == http.MethodGet {
			path = "/api/context"
		}
		req, err := http.NewRequest(method, s.URL+path, rd)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		if origin != "" {
			req.Header.Set("Origin", origin)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	for _, origin := range []string{"http://example.com", "https://" + s.host, "http://" + s.host + ".example.com", "null"} {
		if code := send(http.MethodPost, origin); code != http.StatusForbidden {
			t.Errorf("Origin %s の POST に %d を返した（403 で断る）", origin, code)
		}
	}
	// 同じ Origin か、Origin のない POST は断らない（指紋が違うので 409 になる）
	for _, origin := range []string{"", "http://" + s.host} {
		if code := send(http.MethodPost, origin); code == http.StatusForbidden {
			t.Errorf("Origin %q の POST を 403 で断った", origin)
		}
	}
	// GET は Origin がほかのサイトでも断らない
	if code := send(http.MethodGet, "http://example.com"); code != http.StatusOK {
		t.Errorf("ほかのサイトの Origin の GET に %d を返した（断らない）", code)
	}
	if r.git("rev-parse", "HEAD") != head {
		t.Errorf("断った要求で、コミットができた")
	}
}

// runServeToEnd は、vanda serve を動かし、終わるまで待って、終了コードと標準出力と標準エラーを返す。
// 待ち受けを始めてしまうと終わらないので、15 秒で止めて落とす。
func runServeToEnd(t *testing.T, bin, dir string, env []string, args ...string) (int, string, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, append([]string{"serve"}, args...)...)
	cmd.Dir = dir
	cmd.Env = env
	var stdout, stderr strings.Builder
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if ctx.Err() != nil {
		t.Fatalf("vanda serve %v が 15 秒たっても終わらない（止めた）\n標準出力: %s\n標準エラー: %s", args, stdout.String(), stderr.String())
	}
	var exitErr *exec.ExitError
	switch {
	case err == nil:
		return 0, stdout.String(), stderr.String()
	case errors.As(err, &exitErr):
		return exitErr.ExitCode(), stdout.String(), stderr.String()
	default:
		t.Fatalf("vanda serve %v を動かせない: %v", args, err)
		return 0, "", ""
	}
}

// @vanda:evidence serve-config-error-stops
func Test設定に誤りがあればvanda_serveは待ち受けを始めずに誤りを標準エラーに出し終了コード1で終わる(t *testing.T) {
	t.Parallel()
	bin := buildVanda(t)
	version, err := runningVersion(bin)
	if err != nil {
		t.Fatal(err)
	}
	newer, _, err := neighborVersions(version)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name   string
		change func(r *reviewRepo)
	}{
		{"決まっていない項目がある", func(r *reviewRepo) {
			writeFile(t, r.Root, ".vanda/config.toml", fmt.Sprintf(reviewConfig, version)+"\nno_such_entry = 1\n")
		}},
		{"版が合わない", func(r *reviewRepo) {
			writeFile(t, r.Root, ".vanda/config.toml", fmt.Sprintf(reviewConfig, newer))
		}},
		{"設定のファイルがない", func(r *reviewRepo) {
			if err := os.Remove(filepath.Join(r.Root, ".vanda", "config.toml")); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			r := newReviewRepo(t, version)
			c.change(r)
			r.g.commitTracked("設定を変える", reviewStory)
			code, stdout, stderr := runServeToEnd(t, bin, r.Root, r.Env, "--port", "0", "--no-open")
			if code != 1 {
				t.Errorf("終了コード %d（1 のはず）\n標準出力: %s\n標準エラー: %s", code, stdout, stderr)
			}
			if !strings.Contains(stderr, ".vanda/config.toml") {
				t.Errorf("標準エラーに、設定の誤りが出ない: %q", stderr)
			}
			if serveURLPattern.MatchString(stdout) || serveURLPattern.MatchString(stderr) {
				t.Errorf("待ち受けを始めて URL を出した\n標準出力: %s\n標準エラー: %s", stdout, stderr)
			}
		})
	}
}

// wantPortAdvice は、待ち受けられなかった vanda serve が、終了コード 1 で終わり、空いているポートか --port 0 を渡すよう伝えたかを確かめる。
func wantPortAdvice(t *testing.T, where string, code int, stdout, stderr string) {
	t.Helper()
	if code != 1 {
		t.Errorf("%s: 終了コード %d（1 のはず）\n標準出力: %s\n標準エラー: %s", where, code, stdout, stderr)
	}
	for _, w := range []string{"空いているポート", "--port 0"} {
		if !strings.Contains(stderr, w) {
			t.Errorf("%s: 標準エラーに「%s」がない: %q", where, w, stderr)
		}
	}
	if serveURLPattern.MatchString(stdout) {
		t.Errorf("%s: 待ち受けられないのに、起動した URL を出した: %q", where, stdout)
	}
}

// @vanda:evidence serve-port-unavailable
func Test待ち受けられないポートを渡すとvanda_serveは空いているポートか__port_0を渡すよう伝えて終了コード1で終わる(t *testing.T) {
	t.Parallel()
	bin := buildVanda(t)
	version, err := runningVersion(bin)
	if err != nil {
		t.Fatal(err)
	}
	r := newReviewRepo(t, version)
	for _, port := range []string{"65536", "70000", "-1"} {
		code, stdout, stderr := runServeToEnd(t, bin, r.Root, r.Env, "--port="+port, "--no-open")
		wantPortAdvice(t, "--port="+port, code, stdout, stderr)
	}
}

// ほかのプロセスが待ち受けている本物のポートを渡す
// @vanda:evidence serve-port-unavailable
func Testほかのものが待ち受けているポートを渡すとvanda_serveは空いているポートか__port_0を渡すよう伝えて終了コード1で終わりそのポートを奪わない(t *testing.T) {
	t.Parallel()
	bin := buildVanda(t)
	version, err := runningVersion(bin)
	if err != nil {
		t.Fatal(err)
	}
	r := newReviewRepo(t, version)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	accepted := make(chan struct{}, 1)
	go func() {
		c, err := ln.Accept()
		if err == nil {
			c.Close()
			accepted <- struct{}{}
		}
	}()
	_, port, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	code, stdout, stderr := runServeToEnd(t, bin, r.Root, r.Env, "--port", port, "--no-open")
	wantPortAdvice(t, "待ち受けているポート "+port, code, stdout, stderr)
	// 前から待ち受けていたものは、そのまま受け付ける
	c, err := net.DialTimeout("tcp", ln.Addr().String(), 5*time.Second)
	if err != nil {
		t.Fatalf("vanda serve を動かした後に、前から待ち受けていたポートに繋がらない: %v", err)
	}
	c.Close()
	select {
	case <-accepted:
	case <-time.After(5 * time.Second):
		t.Errorf("前から待ち受けていたものが、繋いだ要求を受け付けない")
	}
}

// fakeOpenerPath は、git だけを置いた PATH のディレクトリを作る。fail なら、ブラウザを開くコマンド（xdg-open・open）を、失敗して終わるものにして置く。
// fail でなければ、ブラウザを開くコマンドは PATH にない。
func fakeOpenerPath(t *testing.T, fail bool) string {
	t.Helper()
	dir := t.TempDir()
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(git, filepath.Join(dir, "git")); err != nil {
		t.Fatal(err)
	}
	if fail {
		for _, name := range []string{"xdg-open", "open"} {
			if err := os.WriteFile(filepath.Join(dir, name), []byte("#!/bin/sh\necho ブラウザがない >&2\nexit 3\n"), 0o755); err != nil {
				t.Fatal(err)
			}
		}
	}
	return dir
}

// @vanda:evidence serve-browser-open-failure
func Testブラウザを開けなければvanda_serveは開いたURLとそれをブラウザで開くよう促す文を標準エラーに出し動き続ける(t *testing.T) {
	t.Parallel()
	bin := buildVanda(t)
	version, err := runningVersion(bin)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		fail bool
	}{
		{"ブラウザを開くコマンドがない", false},
		{"ブラウザを開くコマンドが失敗する", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			r := newReviewRepo(t, version)
			var env []string
			for _, kv := range r.Env {
				if !strings.HasPrefix(kv, "PATH=") {
					env = append(env, kv)
				}
			}
			env = append(env, "PATH="+fakeOpenerPath(t, c.fail))
			s := startServeArgs(t, bin, r.Root, env, "--port", "0")
			url := s.URL + "/"
			deadline := time.Now().Add(15 * time.Second)
			for !(strings.Contains(s.Stderr(), url) && strings.Contains(s.Stderr(), "ブラウザで開いて")) {
				if time.Now().After(deadline) {
					t.Fatalf("15 秒たっても、標準エラーに %s とそれをブラウザで開くよう促す文が出ない\n標準エラー: %s", url, s.Stderr())
				}
				time.Sleep(20 * time.Millisecond)
			}
			// ブラウザを開けなくても、vanda サーバーは動き続ける
			var got contextResponse
			if code, msg := s.get("/api/context", &got); code != 200 {
				t.Errorf("ブラウザを開けなかった後の vanda サーバーが答えない（%d）: %s", code, msg)
			}
			select {
			case err := <-s.done:
				s.done <- err
				t.Errorf("ブラウザを開けなかった後に、vanda serve が終わった: %v\n%s", err, s.Output())
			default:
			}
		})
	}
}

// @vanda:evidence serve-reloads-config
func Test起動したvanda_serveはAPIの要求ごとに設定を読み直し読めなければ誤りの数と最初の1件を書いた500を返す(t *testing.T) {
	t.Parallel()
	bin := buildVanda(t)
	version, err := runningVersion(bin)
	if err != nil {
		t.Fatal(err)
	}
	r := newReviewRepo(t, version)
	s := startServe(t, bin, r.Root, r.Env)
	layerNames := func() []string {
		t.Helper()
		var got contextResponse
		if code, msg := s.get("/api/context", &got); code != 200 {
			t.Fatalf("画面で選べるものを読めない（%d）: %s", code, msg)
		}
		var names []string
		for _, l := range got.Layers {
			names = append(names, l.ID+" "+l.Name)
		}
		return names
	}
	if got := strings.Join(layerNames(), "・"); got != "model モデル・db DB" {
		t.Fatalf("起動したときのレイヤー: %s", got)
	}

	// 起動した後に設定を書き換えると、起動し直さなくても次の要求から効く
	writeFile(t, r.Root, ".vanda/config.toml", strings.Replace(fmt.Sprintf(reviewConfig, version), `name = "モデル"`, `name = "判定のモデル"`, 1))
	r.g.commitTracked("レイヤーの名前を変える", reviewStory)
	if got := strings.Join(layerNames(), "・"); got != "model 判定のモデル・db DB" {
		t.Errorf("設定を書き換えた後のレイヤー: %s（model 判定のモデル・db DB のはず）", got)
	}

	// 決まっていない項目を 3 つ書いた設定。最初の誤りは、1 つ目の項目の行
	lines := strings.Split(fmt.Sprintf(reviewConfig, version), "\n")
	broken := append([]string{lines[0], "no_such_first = 1", "", "no_such_second = 2", "no_such_third = 3"}, lines[1:]...)
	writeFile(t, r.Root, ".vanda/config.toml", strings.Join(broken, "\n"))
	r.g.commitTracked("設定を壊す", reviewStory)
	count := regexp.MustCompile(`(^|[^0-9])3 ?件`)
	requests := []struct{ method, path string }{
		{http.MethodGet, "/api/context"},
		{http.MethodGet, "/api/review?view=branch&base=main"},
		{http.MethodGet, "/api/claims/task-id-unique"},
		{http.MethodGet, "/api/claims/task-id-unique/at/" + r.git("rev-parse", "main")},
		{http.MethodGet, "/api/file?path=" + q("e2e/add-task.txtar")},
		{http.MethodGet, "/api/story?id=" + q(reviewStory)},
		{http.MethodPost, "/api/agree"},
		{http.MethodPost, "/api/revoke"},
	}
	for _, req := range requests {
		var body any
		switch req.path {
		case "/api/agree":
			body = agreeClaim("task-id-unique", reviewClaimFP)
		case "/api/revoke":
			body = revokeClaim("task-id-unique", reviewClaimFP)
		}
		code, msg := s.do(req.method, req.path, body, nil)
		where := req.method + " " + req.path
		if code != http.StatusInternalServerError {
			t.Errorf("%s: 読めない設定で %d を返した（500 のはず）: %s", where, code, msg)
			continue
		}
		if !count.MatchString(msg) {
			t.Errorf("%s: 誤りの数（3 件）が message にない: %s", where, msg)
		}
		if !strings.Contains(msg, "no_such_first") || strings.Contains(msg, "no_such_second") || strings.Contains(msg, "no_such_third") {
			t.Errorf("%s: message に、最初の 1 件（no_such_first）だけを書いていない: %s", where, msg)
		}
	}
}
