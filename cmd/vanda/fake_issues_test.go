package main_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// CM-08 のテストで使う、ストーリーの置き場所（GitHub Issues）の偽の HTTP サーバーと、偽の gh。
// 本物の GitHub には繋がない。サーバーは httptest で 127.0.0.1 の空いているポートに立て、テストの終わりに止める。
// 偽のサーバーは、GitHub の REST API のうち、段 2 で読む Issue の取得（GET /repos/{owner}/{repo}/issues/{number}）だけを真似る。
// 段 2 は読むだけなので、GET のほかの要求は 405 を返して記録し、テストで「書き込んでいない」ことを確かめられるようにする。

type fakeIssue struct {
	Title string
	Body  string
}

type fakeRequest struct {
	Method string
	Path   string
	Auth   string
}

type fakeIssues struct {
	srv   *httptest.Server
	URL   string
	Token string

	mu       sync.Mutex
	issues   map[string]fakeIssue // 鍵は "owner/repo#24"
	requests []fakeRequest
	cutOff   bool // 要求を受けたら、応えずに接続を切る
}

var fakeIssuePath = regexp.MustCompile(`^/repos/([^/]+)/([^/]+)/issues/([0-9]+)$`)

func newFakeIssues(t *testing.T) *fakeIssues {
	t.Helper()
	f := &fakeIssues{Token: "gho_testtoken" + strconv.Itoa(os.Getpid()), issues: map[string]fakeIssue{}}
	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	f.URL = f.srv.URL
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeIssues) setIssue(repo string, number int, title, body string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.issues[fmt.Sprintf("%s#%d", repo, number)] = fakeIssue{Title: title, Body: body}
}

// cutNetwork は、これからの要求に、応えずに接続を切るようにする（ネットワークの誤り）。
func (f *fakeIssues) cutNetwork() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cutOff = true
}

func (f *fakeIssues) Requests() []fakeRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]fakeRequest(nil), f.requests...)
}

func writeGitHubError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"message": message, "documentation_url": "https://docs.github.com/rest"})
}

func (f *fakeIssues) serve(w http.ResponseWriter, req *http.Request) {
	f.mu.Lock()
	f.requests = append(f.requests, fakeRequest{Method: req.Method, Path: req.URL.Path, Auth: req.Header.Get("Authorization")})
	cut := f.cutOff
	f.mu.Unlock()

	if cut {
		hj, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "接続を切れない", http.StatusInternalServerError)
			return
		}
		conn, _, err := hj.Hijack()
		if err == nil {
			_ = conn.Close()
		}
		return
	}
	if req.Method != http.MethodGet {
		writeGitHubError(w, http.StatusMethodNotAllowed, "段 2 では読むだけ")
		return
	}
	// GitHub は、どちらの書き方の Authorization も受け付ける
	auth := req.Header.Get("Authorization")
	if auth != "Bearer "+f.Token && auth != "token "+f.Token {
		writeGitHubError(w, http.StatusUnauthorized, "Bad credentials")
		return
	}
	m := fakeIssuePath.FindStringSubmatch(req.URL.Path)
	if m == nil {
		writeGitHubError(w, http.StatusNotFound, "Not Found")
		return
	}
	f.mu.Lock()
	is, ok := f.issues[fmt.Sprintf("%s/%s#%s", m[1], m[2], m[3])]
	f.mu.Unlock()
	if !ok {
		writeGitHubError(w, http.StatusNotFound, "Not Found")
		return
	}
	n, _ := strconv.Atoi(m[3])
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"number":   n,
		"title":    is.Title,
		"body":     is.Body,
		"state":    "open",
		"html_url": fmt.Sprintf("https://github.com/%s/%s/issues/%d", m[1], m[2], n),
	})
}

// 添付ファイルへのリンクを含む本文の例。GitHub では、添付は本文の中の user-attachments へのリンクになる。
const fakeIssueBodyWithAttachments = "タスクの ID を重ならないようにする。\n\n" +
	"![画面の案](https://github.com/user-attachments/assets/3f0c9a52-0000-4000-8000-000000000001)\n\n" +
	"仕様の資料: [spec.pdf](https://github.com/user-attachments/files/17000001/spec.pdf)\n"

// fakeGH は、PATH の先頭に置く偽の gh を作り、そのディレクトリを返す。
// `gh auth token` に、token を出して終わる。token が空なら、gh にログインしていないときのように、終了コード 1 で終わる。
// ほかの引数で呼ばれたら、終了コード 2 で終わる。呼ばれた引数は、ディレクトリの calls.log に 1 行ずつ書く。
func fakeGH(t *testing.T, token string) string {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "calls.log")
	answer := fmt.Sprintf("echo '%s'\n  exit 0", token)
	if token == "" {
		answer = "echo 'no oauth token found for github.com' >&2\n  exit 1"
	}
	script := fmt.Sprintf(`#!/bin/sh
echo "$*" >> '%s'
if [ "$1" = "auth" ] && [ "$2" = "token" ]; then
  %s
fi
echo "偽の gh: 思っていない引数: $*" >&2
exit 2
`, log, answer)
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func fakeGHCalls(t *testing.T, dir string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "calls.log"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimRight(string(data), "\n"), "\n")
}

// --- 偽のサーバーと偽の gh が、思ったとおりに応えることを確かめる ---

func fakeGet(t *testing.T, url, auth string) (int, map[string]any, error) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, nil, err
	}
	var v map[string]any
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("JSON でない応え: %q", data)
	}
	return resp.StatusCode, v, nil
}

func Test偽のストーリーの置き場所は正しい鍵でだけIssueを返し誤りをGitHubと同じ形で返す(t *testing.T) {
	t.Parallel()
	f := newFakeIssues(t)
	f.setIssue("owner/app", 24, "タスクの ID", fakeIssueBodyWithAttachments)
	url := f.URL + "/repos/owner/app/issues/24"

	for _, auth := range []string{"Bearer " + f.Token, "token " + f.Token} {
		code, v, err := fakeGet(t, url, auth)
		if err != nil || code != 200 || v["title"] != "タスクの ID" || v["body"] != fakeIssueBodyWithAttachments || v["number"] != float64(24) {
			t.Errorf("%s: code %d, %v, %v", auth, code, v, err)
		}
	}
	for _, auth := range []string{"", "Bearer wrong", "Bearer " + f.Token + "x"} {
		if code, v, err := fakeGet(t, url, auth); err != nil || code != 401 || v["message"] != "Bad credentials" {
			t.Errorf("鍵 %q: code %d, %v, %v", auth, code, v, err)
		}
	}
	for _, u := range []string{f.URL + "/repos/owner/app/issues/25", f.URL + "/repos/owner/other/issues/24"} {
		if code, v, err := fakeGet(t, u, "Bearer "+f.Token); err != nil || code != 404 || v["message"] != "Not Found" {
			t.Errorf("%s: code %d, %v, %v", u, code, v, err)
		}
	}
	resp, err := http.Post(url, "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 405 {
		t.Errorf("POST に %d を返した", resp.StatusCode)
	}
	reqs := f.Requests()
	if last := reqs[len(reqs)-1]; last.Method != "POST" || last.Path != "/repos/owner/app/issues/24" {
		t.Errorf("POST が記録されていない: %v", last)
	}

	f.cutNetwork()
	if _, _, err := fakeGet(t, url, "Bearer "+f.Token); err == nil {
		t.Errorf("接続を切るようにしたのに、応えが返った")
	}
}

func Test偽のghはauth_tokenに鍵を出しログインしていなければ失敗する(t *testing.T) {
	t.Parallel()
	dir := fakeGH(t, "gho_abc")
	out, err := exec.Command(filepath.Join(dir, "gh"), "auth", "token").Output()
	if err != nil || string(out) != "gho_abc\n" {
		t.Errorf("out %q, err %v", out, err)
	}
	if err := exec.Command(filepath.Join(dir, "gh"), "issue", "comment", "24").Run(); err == nil {
		t.Errorf("思っていない引数で成功した")
	}
	if calls := fakeGHCalls(t, dir); len(calls) != 2 || calls[0] != "auth token" || calls[1] != "issue comment 24" {
		t.Errorf("呼ばれた引数の記録: %q", calls)
	}

	none := fakeGH(t, "")
	cmd := exec.Command(filepath.Join(none, "gh"), "auth", "token")
	var stderr strings.Builder
	cmd.Stderr = &stderr
	var exitErr *exec.ExitError
	if err := cmd.Run(); !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 || !strings.Contains(stderr.String(), "no oauth token") {
		t.Errorf("ログインしていない gh: err %v, stderr %q", err, stderr.String())
	}
}
