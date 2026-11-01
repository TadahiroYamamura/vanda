package main_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// CM-08 ストーリーの置き場所（GET /api/story）。GitHub Issues のアダプタを、fake_issues_test.go の偽のサーバーに向ける。
// 本物の GitHub には繋がない（api_url を偽のサーバーにし、gh も偽のものを PATH の先頭に置く）。

type storyResponse struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Body        string `json:"body"`
	URL         string `json:"url"`
	Attachments []struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	} `json:"attachments"`
}

// 添付ファイルの決まり（docs/formats.md）の、当たるものと当たらないものを全部の書き方で並べた本文。
const fakeIssueBodyAllAttachments = "当たるもの:\n" +
	"[資料.pdf](https://github.com/user-attachments/files/1/a.pdf)\n" +
	"![画面](https://github.com/user-attachments/assets/b-1)\n" +
	"![](https://github.com/owner/app/assets/1/c.png)\n" +
	"<img width=\"300\" src=\"https://user-images.githubusercontent.com/1/d.png\">\n" +
	"当たらないもの:\n" +
	"[よそ](https://example.com/files/e.pdf)\n" +
	"![ほかのリポジトリ](https://github.com/other/repo/assets/1/f.png)\n" +
	"https://github.com/user-attachments/files/2/bare.pdf\n"

type storySetup struct {
	bin  string
	r    *reviewRepo
	fake *fakeIssues
	gh   string // 偽の gh のディレクトリ
}

// newStorySetup は、設定に [story] を足したリポジトリと偽のサーバーを用意する。env は、vanda serve の環境を返す。
func newStorySetup(t *testing.T, ghToken string) *storySetup {
	t.Helper()
	bin := buildVanda(t)
	version, err := runningVersion(bin)
	if err != nil {
		t.Fatal(err)
	}
	r := newReviewRepo(t, version)
	fake := newFakeIssues(t)
	fake.setIssue("owner/app", 24, "タスクの ID を重ならないようにする", fakeIssueBodyWithAttachments)
	fake.setIssue("owner/app", 26, "添付の書き方", fakeIssueBodyAllAttachments)
	cfg, err := os.ReadFile(filepath.Join(r.Root, ".vanda", "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, r.Root, ".vanda/config.toml", string(cfg)+"\n[story]\nadapter = \"github\"\nrepository = \"owner/app\"\napi_url = \""+fake.URL+"\"\n")
	r.git("commit", "-q", "-am", "設定にストーリーの置き場所を足す")
	return &storySetup{bin: bin, r: r, fake: fake, gh: fakeGH(t, ghToken)}
}

// env は、GH_TOKEN を token にした（空なら除いた）環境を返す。偽の gh を PATH の先頭に置く。
func (st *storySetup) env(token string) []string {
	var env []string
	for _, kv := range st.r.Env {
		if strings.HasPrefix(kv, "GH_TOKEN=") || strings.HasPrefix(kv, "GITHUB_TOKEN=") || strings.HasPrefix(kv, "PATH=") {
			continue
		}
		env = append(env, kv)
	}
	env = append(env, "PATH="+st.gh+string(os.PathListSeparator)+os.Getenv("PATH"))
	if token != "" {
		env = append(env, "GH_TOKEN="+token)
	}
	return env
}

func (st *storySetup) checkOnlyReads(t *testing.T) {
	t.Helper()
	for _, req := range st.fake.Requests() {
		if req.Method != "GET" {
			t.Errorf("段 2 では読むだけなのに、%s %s を送った", req.Method, req.Path)
		}
	}
}

// attachmentList は、添付ファイルを、返した順に「名前 URL」で並べる（並びは本文に出てくる順。監督の決定）。
func attachmentList(got storyResponse) []string {
	var s []string
	for _, a := range got.Attachments {
		s = append(s, a.Name+" "+a.URL)
	}
	return s
}

// CM-08
// @vanda:evidence story-github-read
// @vanda:evidence story-github-token
func Testストーリーの置き場所から題と本文と添付ファイルへのリンクを読める(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		ghTok  string // 偽の gh が出す鍵（空ならログインしていない）
		envTok string // GH_TOKEN
		viaGH  bool
	}{
		{"GH_TOKENの鍵を使う", "", "", false},
		{"GH_TOKENがなければghの鍵を使う", "", "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			st := newStorySetup(t, "")
			if c.viaGH {
				st.gh = fakeGH(t, st.fake.Token)
			}
			envTok := st.fake.Token
			if c.viaGH {
				envTok = ""
			}
			s := startServe(t, st.bin, st.r.Root, st.env(envTok))

			var got storyResponse
			if code, msg := s.get("/api/story?id="+q("#24"), &got); code != 200 {
				t.Fatalf("ストーリーを読めない（%d）: %s", code, msg)
			}
			if got.ID != "#24" || got.Title != "タスクの ID を重ならないようにする" || got.Body != fakeIssueBodyWithAttachments ||
				got.URL != "https://github.com/owner/app/issues/24" {
				t.Errorf("読んだもの: %+v", got)
			}
			want := []string{
				"画面の案 https://github.com/user-attachments/assets/3f0c9a52-0000-4000-8000-000000000001",
				"spec.pdf https://github.com/user-attachments/files/17000001/spec.pdf",
			}
			if a := attachmentList(got); strings.Join(a, "\n") != strings.Join(want, "\n") {
				t.Errorf("添付ファイル: %q", a)
			}
			reqs := st.fake.Requests()
			if len(reqs) == 0 || reqs[len(reqs)-1].Path != "/repos/owner/app/issues/24" || !strings.HasSuffix(reqs[len(reqs)-1].Auth, " "+st.fake.Token) {
				t.Errorf("偽のサーバーへの要求: %+v", reqs)
			}
			calls := fakeGHCalls(t, st.gh)
			if c.viaGH && (len(calls) == 0 || !strings.HasPrefix(calls[0], "auth token")) {
				t.Errorf("GH_TOKEN がないのに gh auth token を呼んでいない: %q", calls)
			}
			if !c.viaGH && len(calls) != 0 {
				t.Errorf("GH_TOKEN があるのに gh を呼んだ: %q", calls)
			}
			st.checkOnlyReads(t)
		})
	}
}

// CM-08: 添付ファイルの決まりの、当たるものと当たらないもの
// @vanda:evidence story-github-attachments
func Testストーリーの添付ファイルは決まったURLで始まるリンクと画像だけで名前はリンクの文字かURLの最後の部分(t *testing.T) {
	t.Parallel()
	st := newStorySetup(t, "")
	s := startServe(t, st.bin, st.r.Root, st.env(st.fake.Token))
	var got storyResponse
	if code, msg := s.get("/api/story?id="+q("#26"), &got); code != 200 {
		t.Fatalf("ストーリーを読めない（%d）: %s", code, msg)
	}
	want := []string{
		"資料.pdf https://github.com/user-attachments/files/1/a.pdf",
		"画面 https://github.com/user-attachments/assets/b-1",
		"c.png https://github.com/owner/app/assets/1/c.png",
		"d.png https://user-images.githubusercontent.com/1/d.png",
	}
	if a := attachmentList(got); strings.Join(a, "\n") != strings.Join(want, "\n") {
		t.Errorf("添付ファイル:\n%s\n--- 思ったもの\n%s", strings.Join(a, "\n"), strings.Join(want, "\n"))
	}
}

// CM-08: 誤りは、黙って空にせず、理由を出す
// @vanda:evidence story-errors
func Testストーリーの置き場所の誤りは理由を画面に出す(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		ghTok    string
		envTok   func(f *fakeIssues) string
		cut      bool
		id       string
		wantCode int
		wantMsg  string // message に含むもの（置き場所が返した理由）。空なら、空でないことだけを求める
	}{
		{"認証の鍵が誤っている", "", func(*fakeIssues) string { return "gho_wrong" }, false, "#24", 502, "Bad credentials"},
		{"ghにログインしていない", "", func(*fakeIssues) string { return "" }, false, "#24", 502, ""},
		{"ネットワークが切れている", "", func(f *fakeIssues) string { return f.Token }, true, "#24", 502, ""},
		{"ないストーリー", "", func(f *fakeIssues) string { return f.Token }, false, "#25", 404, ""},
		{"形の誤ったストーリーのID", "", func(f *fakeIssues) string { return f.Token }, false, "PROJ-1", 400, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			st := newStorySetup(t, c.ghTok)
			s := startServe(t, st.bin, st.r.Root, st.env(c.envTok(st.fake)))
			if c.cut {
				st.fake.cutNetwork()
			}
			code, msg := s.get("/api/story?id="+q(c.id), nil)
			if code != c.wantCode || msg == "" || !strings.Contains(msg, c.wantMsg) {
				t.Errorf("%d（%q）を返した。%d で、理由（%q を含む）を出す", code, msg, c.wantCode, c.wantMsg)
			}
			st.checkOnlyReads(t)
		})
	}
}

// writeUserAdapter は、.vanda/adapters/ のアダプタ（シェルのスクリプト）を、実行の権限を付けて書く。
func writeUserAdapter(t *testing.T, root, path, body string) {
	t.Helper()
	writeFile(t, root, path, "#!/bin/sh\n"+body)
	if err := os.Chmod(filepath.Join(root, path), 0o755); err != nil {
		t.Fatal(err)
	}
}

// ストーリーの置き場所の、.vanda/adapters/ のアダプタ。標準入力をそのまま本文に、引数を題に入れて返す（何が渡ったかを見るため）
const userStoryAdapter = `in=$(cat)
in=$(printf '%s' "$in" | sed 's/"/\\"/g')
printf '{"title":"args: %s","body":"%s","url":""}' "$*" "$in"
`

// CM-08: ストーリーの置き場所の.vanda/adapters/ のアダプタは、ファイルの形でもディレクトリの形（直下の run）でも動き、args が渡る。
// 組込みのアダプタ（github）と同じ名前なら、.vanda/adapters/ のものを使う
// @vanda:evidence story-project-adapter
func Testストーリーの置き場所のvanda_adaptersのアダプタはファイルとディレクトリの形で動きargsが渡り組込みのものより先に使われる(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, adapter, path, args, wantTitle string
	}{
		{"ファイルの形", "jira-story", ".vanda/adapters/jira-story", `args = ["--project", "APP"]`, "args: --project APP"},
		{"ディレクトリの形", "jira-dir", ".vanda/adapters/jira-dir/run", `args = ["--dir"]`, "args: --dir"},
		{"argsなし", "jira-story", ".vanda/adapters/jira-story", "", "args: "},
		// 組込みの github を使うなら、repository がないので誤りになるか、GitHub に繋ぎに行く
		{"組込みのgithubと同じ名前", "github", ".vanda/adapters/github", `args = ["--mine"]`, "args: --mine"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			bin := buildVanda(t)
			version, err := runningVersion(bin)
			if err != nil {
				t.Fatal(err)
			}
			r := newReviewRepo(t, version)
			cfg := fmt.Sprintf(reviewConfig, version) + "\n[story]\nadapter = \"" + c.adapter + "\"\n" + c.args + "\n"
			writeFile(t, r.Root, ".vanda/config.toml", cfg)
			writeUserAdapter(t, r.Root, c.path, userStoryAdapter)
			r.git("add", "-A")
			r.git("commit", "-q", "-m", ".vanda/adapters/ のアダプタを置く")
			s := startServe(t, bin, r.Root, r.Env)
			var got storyResponse
			if code, msg := s.get("/api/story?id="+q("#24"), &got); code != 200 {
				t.Fatalf("ストーリーを読めない（%d）: %s", code, msg)
			}
			if got.Title != c.wantTitle {
				t.Errorf("題: %q（思ったもの %q。.vanda/adapters/ のアダプタに args が渡っていない）", got.Title, c.wantTitle)
			}
			// 標準入力には、docs/formats.md の形で、読むストーリーの ID が来る
			if !strings.Contains(got.Body, `"action"`) || !strings.Contains(got.Body, `"read"`) || !strings.Contains(got.Body, `#24`) {
				t.Errorf(".vanda/adapters/ のアダプタの標準入力: %q", got.Body)
			}
		})
	}
}
