// Package story は、ストーリーストア（CM-08）から、ストーリーの題・本文・添付ファイルへのリンクを読む。
package story

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/TadahiroYamamura/vanda/internal/config"
)

type Story struct {
	ID          string       `json:"id"`
	Title       string       `json:"title"`
	Body        string       `json:"body"`
	URL         string       `json:"url"`
	Attachments []Attachment `json:"attachments"`
}

type Attachment struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// Error は、読めなかった理由と、API で返す HTTP のステータスコード。
type Error struct {
	Status  int
	Message string
	Fix     string
}

// Error は、文言の決まりで「。」で終わる message と fix を、そのままつなぐ
func (e *Error) Error() string { return e.Message + e.Fix }

func storeErr(msg, fix string) *Error {
	return &Error{Status: http.StatusBadGateway, Message: msg, Fix: fix}
}

const timeout = 20 * time.Second

// AdapterInfo は、組み込みのストーリーストアのアダプタ。vanda init の選択肢と、ID の書き方の既定に使う
type AdapterInfo struct {
	Name  string // 設定の [story] の adapter に書く名前
	Title string // 画面や質問に出す名前
	// DefaultIDPattern は、ストーリーの ID の書き方（コミットの Story: のトレーラーの値にマッチさせる）の既定
	DefaultIDPattern string
}

// Builtins は、組み込みのストーリーストアのアダプタの一覧。アダプタを足したら、ここにも足す
var Builtins = []AdapterInfo{
	{Name: "github", Title: "GitHub Issues", DefaultIDPattern: config.DefaultIDPattern},
}

// Read は、ストーリー id を、設定のストーリーストアから読む。
func Read(root string, cfg *config.Config, id string) (*Story, *Error) {
	if cfg.IDFull == nil || !cfg.IDFull.MatchString(id) {
		return nil, &Error{Status: http.StatusBadRequest, Message: fmt.Sprintf("ストーリーの ID %s が、書き方（%s）に合いません。", id, cfg.IDPattern), Fix: "設定の id_pattern に合う ID を渡してください。"}
	}
	switch {
	case cfg.Story.User != nil:
		return readUser(root, cfg.Story.User, id)
	case cfg.Story.Adapter == "github":
		return readGitHub(cfg.Story, id)
	}
	return nil, &Error{Status: http.StatusNotFound, Message: "ストーリーストアが設定されていません。", Fix: `.vanda/config.toml の [story] に、adapter = "github" と repository を書くか、.vanda/adapters/ に置いたカスタムアダプタの名前を adapter に書いてください。`}
}

// tokenSource は、api_url のホストに合うトークンの読み先（環境変数の名前と、gh auth token に渡すホスト）を返す。
// api_url はチームで共有する設定に書くので、github.com のトークンを、設定に書かれたほかのホストへ送らないよう、ホストごとに読み先を分ける
func tokenSource(apiURL string) (env, host string, err error) {
	u, err := url.Parse(apiURL)
	if err != nil || u.Host == "" {
		return "", "", fmt.Errorf("api_url %q からホストを読めません", apiURL)
	}
	if strings.EqualFold(u.Host, "api.github.com") {
		return "GH_TOKEN", "github.com", nil
	}
	return "GH_ENTERPRISE_TOKEN", u.Host, nil
}

// token は、トークンを、使うたびに環境変数 env から読み、なければ gh auth token --hostname host の出力を使う。
// 見つからなければ（空も含む）、リクエストを送らないよう、ストーリーストアのエラーを返す。
func token(env, host string) (string, *Error) {
	if t := os.Getenv(env); t != "" {
		return t, nil
	}
	fix := "環境変数 " + env + " にトークンを入れるか、gh auth login --hostname " + host + " でログインしてください（環境変数を変えたときは、vanda を起動し直してください）。"
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "gh", "auth", "token", "--hostname", host)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", storeErr("GitHub（"+host+"）のトークンが見つかりません（"+env+" がなく、gh auth token --hostname "+host+": "+msg+"）。", fix)
	}
	t := strings.TrimSpace(stdout.String())
	if t == "" {
		return "", storeErr("GitHub（"+host+"）のトークンが見つかりません（"+env+" がなく、gh auth token --hostname "+host+" の出力が空です）。", fix)
	}
	return t, nil
}

// get は、GitHub の REST API を呼び、JSON を v に読む。404 は、notFound の誤りにする。
func get(st config.StoryStore, path string, v any, notFound *Error) *Error {
	env, host, err := tokenSource(st.APIURL)
	if err != nil {
		return storeErr("GitHub のトークンを選べません（"+err.Error()+"）。", ".vanda/config.toml の [story] の api_url を確かめてください。")
	}
	key, e := token(env, host)
	if e != nil {
		return e
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	url := st.APIURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return storeErr("GitHub へのリクエストを作れません（"+err.Error()+"）。", ".vanda/config.toml の [story] の api_url と repository を確かめてください。")
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	req.Header.Set("Authorization", "Bearer "+key)
	// http を認めるのは、サンドボックスなどで http の API に送ることがあるため。トークンが平文で流れることに気づけるよう、送るたびに知らせる
	if strings.HasPrefix(strings.ToLower(st.APIURL), "http://") {
		fmt.Fprintf(os.Stderr, "api_url（%s）が http:// で始まるため、GitHub のトークンを暗号化せずに送っています。暗号化して送るには、api_url を https:// にしてください。\n", st.APIURL)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return storeErr("GitHub（"+url+"）に繋がりません（"+err.Error()+"）。", "ネットワークと、[story] の api_url を確かめてください。")
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return storeErr("GitHub（"+url+"）のレスポンスを読めません（"+err.Error()+"）。", "ネットワークを確かめてください。")
	}
	switch {
	case resp.StatusCode == http.StatusNotFound && notFound != nil:
		return notFound
	case resp.StatusCode != http.StatusOK:
		var m struct {
			Message string `json:"message"`
		}
		reason := strings.TrimSpace(string(body))
		if json.Unmarshal(body, &m) == nil && m.Message != "" {
			reason = m.Message
		}
		fix := "GitHub が返した理由を確かめてください。"
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			fix = "gh auth login --hostname " + host + " でログインし直すか、" + env + " のトークンに、このリポジトリの Issues を読む権限があるかを確かめてください。"
		}
		return storeErr(fmt.Sprintf("GitHub（%s）が %d を返しました（%s）。", url, resp.StatusCode, reason), fix)
	}
	if err := json.Unmarshal(body, v); err != nil {
		return storeErr("GitHub（"+url+"）のレスポンスを JSON として読めません（"+err.Error()+"）。", "[story] の api_url が GitHub の REST API の URL かを確かめてください。")
	}
	return nil
}

func readGitHub(st config.StoryStore, id string) (*Story, *Error) {
	var nums []string
	for _, f := range strings.FieldsFunc(id, func(r rune) bool { return r < '0' || r > '9' }) {
		nums = append(nums, f)
	}
	if len(nums) == 0 {
		return nil, &Error{Status: http.StatusBadRequest, Message: "ストーリーの ID " + id + " に、GitHub の Issue の番号（数字）がありません。", Fix: "id_pattern を、番号を含む書き方（例: #[0-9]+）にしてください。"}
	}
	num := nums[len(nums)-1]
	var issue struct {
		Title   string  `json:"title"`
		Body    *string `json:"body"`
		HTMLURL string  `json:"html_url"`
	}
	notFound := &Error{Status: http.StatusNotFound, Message: "GitHub の " + st.Repository + " に Issue " + num + " はありません。", Fix: "ストーリーの ID と、[story] の repository を確かめてください。"}
	if e := get(st, "/repos/"+st.Repository+"/issues/"+num, &issue, notFound); e != nil {
		return nil, e
	}
	body := ""
	if issue.Body != nil {
		body = *issue.Body
	}
	atts, err := Attachments(st.Repository, body)
	if err != nil {
		return nil, &Error{Status: http.StatusInternalServerError, Message: err.Error() + "。", Fix: "vanda の不具合です。開発者に知らせてください。"}
	}
	return &Story{ID: id, Title: issue.Title, Body: body, URL: issue.HTMLURL, Attachments: atts}, nil
}

// 添付ファイルを探す書き方。Markdown のリンクと画像、HTML の img と、その src・alt
const (
	mdLinkPattern  = `!?\[([^\]]*)\]\(([^) ]+)( +"[^"]*")?\)`
	imgTagPattern  = `<img[^>]*>`
	srcAttrPattern = `src="([^"]*)"`
	altAttrPattern = `alt="([^"]*)"`
)

func compileAll(patterns ...string) ([]*regexp.Regexp, error) {
	out := make([]*regexp.Regexp, len(patterns))
	for i, p := range patterns {
		re, err := regexp.Compile(p)
		if err != nil {
			return nil, err
		}
		out[i] = re
	}
	return out, nil
}

// Attachments は、GitHub の Issue の本文から、添付ファイルへのリンクを、出てきた順に、重ねずに返す。
// GitHub の API には添付の一覧がないので、GitHub が添付を置く URL へのリンクと画像を、添付とみなす。
func Attachments(repository, body string) ([]Attachment, error) {
	res, err := compileAll(mdLinkPattern, imgTagPattern, srcAttrPattern, altAttrPattern)
	if err != nil {
		return nil, fmt.Errorf("添付ファイルを探す正規表現を読めません（%v）", err)
	}
	mdLink, imgTag, srcAttr, altAttr := res[0], res[1], res[2], res[3]
	prefixes := []string{"https://github.com/user-attachments/", "https://github.com/" + repository + "/assets/", "https://user-images.githubusercontent.com/"}
	type hit struct {
		at   int
		name string
		url  string
	}
	var hits []hit
	for _, m := range mdLink.FindAllStringSubmatchIndex(body, -1) {
		hits = append(hits, hit{m[0], body[m[2]:m[3]], body[m[4]:m[5]]})
	}
	for _, m := range imgTag.FindAllStringIndex(body, -1) {
		tag := body[m[0]:m[1]]
		src := srcAttr.FindStringSubmatch(tag)
		if src == nil {
			continue
		}
		name := ""
		if alt := altAttr.FindStringSubmatch(tag); alt != nil {
			name = alt[1]
		}
		hits = append(hits, hit{m[0], name, src[1]})
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].at < hits[j].at })
	out := []Attachment{}
	seen := map[string]bool{}
	for _, h := range hits {
		ok := false
		for _, p := range prefixes {
			if strings.HasPrefix(h.url, p) {
				ok = true
			}
		}
		if !ok || seen[h.url] {
			continue
		}
		seen[h.url] = true
		name := strings.TrimSpace(h.name)
		if name == "" {
			name = h.url[strings.LastIndex(h.url, "/")+1:]
		}
		out = append(out, Attachment{Name: name, URL: h.url})
	}
	return out, nil
}

type userRequest struct {
	Action string `json:"action"`
	Story  string `json:"story"`
}

type userResponse struct {
	Title       *string      `json:"title"`
	Body        *string      `json:"body"`
	URL         *string      `json:"url"`
	Attachments []Attachment `json:"attachments"`
}

func readUser(root string, u *config.UserAdapter, id string) (*Story, *Error) {
	in, err := json.Marshal(userRequest{Action: "read", Story: id})
	if err != nil {
		return nil, storeErr("アダプタに渡す JSON を作れません（"+err.Error()+"）。", "vanda の不具合です。開発者に知らせてください。")
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := u.Cmd(ctx, root)
	cmd.Stdin = bytes.NewReader(in)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		if e := u.StartError(root, err); e != nil {
			return nil, storeErr("ストーリーストアの"+e.What+"。", e.Fix+"。")
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, storeErr("ストーリーストアの"+u.Label()+"が "+id+" を読めませんでした（"+msg+"）。", "アダプタが返した理由を確かめてください。")
	}
	var resp userResponse
	dec := json.NewDecoder(&stdout)
	dec.DisallowUnknownFields()
	bad := func(what string) *Error {
		return storeErr("ストーリーストアの"+u.Label()+"が返した JSON の形が誤っています（"+what+"）。", "アダプタを docs/formats.md の形に合わせて直してください。")
	}
	if err := dec.Decode(&resp); err != nil {
		return nil, bad(err.Error())
	}
	if dec.More() {
		return nil, bad("JSON の後に続きがあります")
	}
	switch {
	case resp.Title == nil:
		return nil, bad("title がありません")
	case resp.Body == nil:
		return nil, bad("body がありません")
	case resp.URL == nil:
		return nil, bad("url がありません")
	}
	atts := []Attachment{}
	for i, a := range resp.Attachments {
		if a.Name == "" || a.URL == "" {
			return nil, bad(fmt.Sprintf("attachments の %d 件目に name か url がありません", i+1))
		}
		atts = append(atts, a)
	}
	return &Story{ID: id, Title: *resp.Title, Body: *resp.Body, URL: *resp.URL, Attachments: atts}, nil
}

// Check は、ストーリーストアに繋がるかを確かめ、確かめたことを 1 文で返す（CMD-01）。ストーリーストアが設定になければ、空の文を返す。
func Check(root string, cfg *config.Config) (string, error) {
	st := cfg.Story
	switch {
	case st.User != nil:
		if e := st.User.Check(root); e != nil {
			return "", fmt.Errorf("ストーリーストアの%w", e)
		}
		return "ストーリーストアの" + st.User.Label() + "があり、実行できます", nil
	case st.Adapter == "github":
		var repo struct {
			HasIssues bool `json:"has_issues"`
		}
		notFound := storeErr("GitHub に "+st.Repository+" がないか、今のトークンでは見えません。", "[story] の repository と、トークンの権限を確かめてください。")
		if e := get(st, "/repos/"+st.Repository, &repo, notFound); e != nil {
			return "", e
		}
		if !repo.HasIssues {
			return "", errors.New("GitHub の " + st.Repository + " で Issues が使えません。リポジトリの設定で Issues を有効にしてください。")
		}
		return "ストーリーストア GitHub Issues（" + st.Repository + "）に繋がりました", nil
	}
	return "", nil
}
