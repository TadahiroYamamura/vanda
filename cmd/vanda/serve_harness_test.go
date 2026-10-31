package main_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

// API のテストは、ビルドした vanda で `vanda serve --port 0 --no-open` を外から動かして確かめる（監督の決定）。
// 段 1 と同じく、実装の内部の関数に頼らずに書くため。サーバーはテストの中で起動し、テストの終わりに必ず止め、
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

// startServe は、dir で vanda serve を起動し、標準出力に出た URL を読んで返す。
func startServe(t serveTB, bin, dir string, env []string) *vandaServer {
	helper(t)
	s := &vandaServer{t: t, done: make(chan error, 1)}
	s.cmd = exec.Command(bin, "serve", "--port", "0", "--no-open")
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

// stop は、Ctrl+C と同じ割り込みを送ってサーバーを止め、止まったことを確かめる。
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

// checkReleased は、サーバーがポートを手放したかを確かめる。
// 接続できるかでは確かめない。WSL2 では、誰も待ち受けていないループバックのポートにも接続できてしまうことがあるため。
// 同じポートで待ち受けられれば、サーバーはポートを手放している。
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

// テストの中で起動したサーバーが、テストの終わりに止まることを確かめる（stop が確かめる）。
// あわせて、ほかのホストの名前で来た要求を断ることも確かめる（CM-01: 自分のマシンの中でだけ開く）。
func Test起動したvanda_serveは止めると止まりほかのHostの要求を断る(t *testing.T) {
	bin := buildVanda(t)
	version, err := runningVersion(bin)
	if err != nil {
		t.Fatal(err)
	}
	r := newReviewRepo(t, version)
	s := startServe(t, bin, r.Root, r.Env)

	req, err := http.NewRequest(http.MethodGet, s.URL+"/api/context", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Host = "example.com"
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("Host が example.com の要求に %d を返した（403 で断る）", resp.StatusCode)
	}

	s.stop()
	if s.cmd.ProcessState == nil {
		t.Errorf("vanda serve のプロセスが終わっていない")
	}
}
