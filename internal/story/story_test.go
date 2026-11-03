package story

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/TadahiroYamamura/vanda/internal/config"
)

// 組込みの github のアダプタが、api_url のホストに合う認証キーを選ぶこと。
// 本物の GitHub に繋がないよう、http.DefaultClient の Transport を、要求を記録して決まった答えを返すものに替える。
// 本物の gh の認証キーを読まないよう、偽の gh を PATH の先頭に置く。どちらもプロセス全体に効くので、並べて動かさない。

type fakeTransport struct {
	mu   sync.Mutex
	reqs []*http.Request
}

func (f *fakeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	f.mu.Lock()
	f.reqs = append(f.reqs, req)
	f.mu.Unlock()
	// ストーリーを読む（Issue）ときにも、doctor が確かめる（リポジトリ）ときにも足りる中身
	body := `{"title":"題","body":"本文","html_url":"https://github.com/owner/app/issues/24","full_name":"owner/app","has_issues":true}`
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
}

func (f *fakeTransport) auths() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var s []string
	for _, r := range f.reqs {
		s = append(s, r.URL.Host+" "+r.Header.Get("Authorization"))
	}
	return s
}

func useFakeGitHub(t *testing.T) *fakeTransport {
	t.Helper()
	f := &fakeTransport{}
	old := http.DefaultClient.Transport
	http.DefaultClient.Transport = f
	t.Cleanup(func() { http.DefaultClient.Transport = old })
	return f
}

// useFakeGH は、偽の gh を PATH の先頭に置き、呼ばれた引数を書くファイルの場所を返す。
// tokens は、ホスト → そのホストにログインした認証キー。本物の gh と同じく、--hostname がなければ github.com のものを出し、
// ログインしていないホストなら終了コード 1 で終わる。認証キーが空のホストには、何も出さずに終了コード 0 で終わる。
func useFakeGH(t *testing.T, tokens map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "calls.log")
	var cases strings.Builder
	for host, token := range tokens {
		fmt.Fprintf(&cases, "    '%s') printf '%%s' '%s'; [ -n '%s' ] && echo; exit 0 ;;\n", host, token, token)
	}
	script := fmt.Sprintf(`#!/bin/sh
echo "$*" >> '%s'
if [ "$1" = "auth" ] && [ "$2" = "token" ]; then
  host=github.com
  if [ "$3" = "--hostname" ]; then
    host="$4"
  fi
  case "$host" in
%s  esac
  echo "no oauth token found for $host" >&2
  exit 1
fi
exit 2
`, log, cases.String())
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return log
}

func ghCalls(t *testing.T, log string) []string {
	t.Helper()
	data, err := os.ReadFile(log)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimRight(string(data), "\n"), "\n")
}

// setTokens は、GH_TOKEN と GH_ENTERPRISE_TOKEN を、env に書いたものだけにする（書いていないものは空）。
func setTokens(t *testing.T, env map[string]string) {
	t.Helper()
	for _, k := range []string{"GH_TOKEN", "GH_ENTERPRISE_TOKEN"} {
		t.Setenv(k, env[k])
	}
}

func githubConfig(apiURL string) *config.Config {
	return &config.Config{
		IDPattern: regexp.MustCompile(`#[0-9]+`),
		IDFull:    regexp.MustCompile(`^(?:#[0-9]+)$`),
		Story:     config.StoryStore{Adapter: "github", Repository: "owner/app", APIURL: apiURL},
	}
}

// 認証キーを使う 2 つの所。画面でストーリーを読むときと、vanda doctor が繋がるかを確かめるとき
var githubOps = []struct {
	name string
	run  func(cfg *config.Config) error
}{
	{"ストーリーを読む", func(cfg *config.Config) error {
		if _, e := Read(".", cfg, "#24"); e != nil {
			return e
		}
		return nil
	}},
	{"doctorで確かめる", func(cfg *config.Config) error {
		_, err := Check(".", cfg)
		return err
	}},
}

// 偽の gh がログインしているホストと認証キー。どのホストにも、違う認証キーを持たせる
var ghLogins = map[string]string{
	"github.com":                 "gho_gh_dotcom",
	"ghe.example.com":            "ghe_gh_example",
	"api.github.com.example.net": "ghe_gh_lookalike",
}

// @vanda:evidence story-github-token
// @vanda:evidence doctor-github-token
func Test認証キーはapi_urlのホストがapi_github_comならGH_TOKENかgithub_comのghのものでほかのホストならGH_ENTERPRISE_TOKENかそのホストのghのもの(t *testing.T) {
	both := map[string]string{"GH_TOKEN": "env_dotcom", "GH_ENTERPRISE_TOKEN": "env_enterprise"}
	cases := []struct {
		name    string
		apiURL  string
		env     map[string]string
		wantKey string
		wantGH  string // 呼ばれるはずの gh の引数。空なら呼ばれない
	}{
		{"api.github.comで両方の環境変数がある", "https://api.github.com", both, "env_dotcom", ""},
		{"api.github.comでGH_TOKENがない", "https://api.github.com", map[string]string{"GH_ENTERPRISE_TOKEN": "env_enterprise"}, "gho_gh_dotcom", "auth token --hostname github.com"},
		{"GHEで両方の環境変数がある", "https://ghe.example.com/api/v3", both, "env_enterprise", ""},
		{"GHEでGH_ENTERPRISE_TOKENがない", "https://ghe.example.com/api/v3", map[string]string{"GH_TOKEN": "env_dotcom"}, "ghe_gh_example", "auth token --hostname ghe.example.com"},
		// 名前が api.github.com で始まっても、ほかのホスト
		{"api.github.comに似たホストで両方の環境変数がある", "https://api.github.com.example.net", both, "env_enterprise", ""},
		{"api.github.comに似たホストでGH_ENTERPRISE_TOKENがない", "https://api.github.com.example.net", map[string]string{"GH_TOKEN": "env_dotcom"}, "ghe_gh_lookalike", "auth token --hostname api.github.com.example.net"},
	}
	for _, op := range githubOps {
		for _, c := range cases {
			t.Run(op.name+"/"+c.name, func(t *testing.T) {
				f := useFakeGitHub(t)
				log := useFakeGH(t, ghLogins)
				setTokens(t, c.env)
				if err := op.run(githubConfig(c.apiURL)); err != nil {
					t.Fatalf("失敗した: %v", err)
				}
				host := strings.TrimPrefix(strings.TrimPrefix(c.apiURL, "https://"), "http://")
				host = strings.SplitN(host, "/", 2)[0]
				if got := f.auths(); len(got) != 1 || got[0] != host+" Bearer "+c.wantKey {
					t.Errorf("送った要求: %q（%s に Bearer %s を 1 回のはず）", got, host, c.wantKey)
				}
				calls := ghCalls(t, log)
				if c.wantGH == "" && len(calls) != 0 {
					t.Errorf("環境変数に認証キーがあるのに gh を呼んだ: %q", calls)
				}
				if c.wantGH != "" && (len(calls) != 1 || calls[0] != c.wantGH) {
					t.Errorf("gh の呼ばれ方: %q（%q のはず）", calls, c.wantGH)
				}
			})
		}
	}
}

// @vanda:evidence story-github-token-missing
// @vanda:evidence doctor-github-token
func Test_api_urlのホストに合う認証キーが見つからなければ要求を送らずにストーリーストアのエラーにする(t *testing.T) {
	cases := []struct {
		name   string
		apiURL string
		env    map[string]string
		logins map[string]string
	}{
		{"api.github.comでGH_TOKENがなくghもgithub_comにログインしていない", "https://api.github.com",
			map[string]string{"GH_ENTERPRISE_TOKEN": "env_enterprise"}, map[string]string{"ghe.example.com": "ghe_gh_example"}},
		{"api.github.comでGH_TOKENがなくghのgithub_comの認証キーが空", "https://api.github.com",
			map[string]string{"GH_ENTERPRISE_TOKEN": "env_enterprise"}, map[string]string{"github.com": "", "ghe.example.com": "ghe_gh_example"}},
		{"GHEでGH_ENTERPRISE_TOKENがなくghもそのホストにログインしていない", "https://ghe.example.com/api/v3",
			map[string]string{"GH_TOKEN": "env_dotcom"}, map[string]string{"github.com": "gho_gh_dotcom"}},
		{"GHEでGH_ENTERPRISE_TOKENがなくghのそのホストの認証キーが空", "https://ghe.example.com/api/v3",
			map[string]string{"GH_TOKEN": "env_dotcom"}, map[string]string{"github.com": "gho_gh_dotcom", "ghe.example.com": ""}},
	}
	for _, op := range githubOps {
		for _, c := range cases {
			t.Run(op.name+"/"+c.name, func(t *testing.T) {
				f := useFakeGitHub(t)
				useFakeGH(t, c.logins)
				setTokens(t, c.env)
				err := op.run(githubConfig(c.apiURL))
				var se *Error
				if !errors.As(err, &se) || se.Status != http.StatusBadGateway || !strings.Contains(se.Fix, "ください") {
					t.Errorf("ストーリーストアのエラー（502 と直し方）にならない: %#v", err)
				}
				if got := f.auths(); len(got) != 0 {
					t.Errorf("認証キーがないのに要求を送った: %q", got)
				}
			})
		}
	}
}

// captureStderr は、fn を動かすあいだに標準エラーに出たものを返す。
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = w
	done := make(chan string)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()
	defer func() { os.Stderr = old }()
	fn()
	os.Stderr = old
	w.Close()
	return <-done
}

// @vanda:evidence story-github-http-warning
func Test_api_urlがhttpなら認証キーを送るたびに暗号化されずに流れることを標準エラーに出す(t *testing.T) {
	cases := []struct {
		name      string
		apiURL    string
		env       map[string]string
		wantWarns int // 2 回読んだときに出る知らせの数
	}{
		{"httpで認証キーがある", "http://ghe.example.com/api/v3", map[string]string{"GH_ENTERPRISE_TOKEN": "env_enterprise"}, 2},
		{"httpsで認証キーがある", "https://ghe.example.com/api/v3", map[string]string{"GH_ENTERPRISE_TOKEN": "env_enterprise"}, 0},
		// 送らなければ、流れない
		{"httpで認証キーがない", "http://ghe.example.com/api/v3", nil, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := useFakeGitHub(t)
			useFakeGH(t, nil)
			setTokens(t, c.env)
			cfg := githubConfig(c.apiURL)
			errs := captureStderr(t, func() {
				for i := 0; i < 2; i++ {
					_, _ = Read(".", cfg, "#24")
				}
			})
			var warns int
			for _, line := range strings.Split(strings.TrimRight(errs, "\n"), "\n") {
				if strings.Contains(line, c.apiURL) && strings.Contains(line, "暗号化") {
					warns++
				}
			}
			if warns != c.wantWarns {
				t.Errorf("知らせが %d 回（%d 回のはず）。標準エラー: %q", warns, c.wantWarns, errs)
			}
			if c.wantWarns > 0 && len(f.auths()) != c.wantWarns {
				t.Errorf("送った要求: %q（知らせと同じ %d 回のはず）", f.auths(), c.wantWarns)
			}
		})
	}
}
