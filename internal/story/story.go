// Package story は、ストーリーの置き場所（CM-08）から、ストーリーの題・本文・添付ファイルへのリンクを読む。
package story

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
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

// Error は、読めなかった理由と、API で返す HTTP の状態コード。
type Error struct {
	Status  int
	Message string
	Fix     string
}

func (e *Error) Error() string { return e.Message + "。" + e.Fix }

func storeErr(msg, fix string) *Error {
	return &Error{Status: http.StatusBadGateway, Message: msg, Fix: fix}
}

const timeout = 20 * time.Second

// Read は、ストーリー id を、設定の置き場所から読む。
func Read(root string, cfg *config.Config, id string) (*Story, *Error) {
	if cfg.IDFull == nil || !cfg.IDFull.MatchString(id) {
		return nil, &Error{Status: http.StatusBadRequest, Message: fmt.Sprintf("ストーリーの ID %s が、書き方（%s）に合わない", id, cfg.IDPattern), Fix: "設定の id_pattern に合う ID を渡す"}
	}
	switch {
	case cfg.Story.Adapter == "github":
		return readGitHub(cfg.Story, id)
	case len(cfg.Story.Command) > 0:
		return readCommand(root, cfg.Story.Command, id)
	}
	return nil, &Error{Status: http.StatusNotFound, Message: "ストーリーの置き場所が設定にない", Fix: `.vanda/config.toml の [story] に adapter = "github" と repository、または command を書く`}
}

// Token は、GitHub の認証の鍵を、使うたびに読む。環境の変数 GH_TOKEN があればそれを、なければ gh auth token の出力を使う。
func Token() (string, *Error) {
	if t := os.Getenv("GH_TOKEN"); t != "" {
		return t, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "gh", "auth", "token")
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", storeErr("GitHub の認証の鍵を読めない（gh auth token: "+msg+"）", "gh auth login でログインするか、環境の変数 GH_TOKEN に鍵を入れて vanda を起動し直す")
	}
	return strings.TrimSpace(stdout.String()), nil
}

// get は、GitHub の REST API を呼び、JSON を v に読む。404 は、notFound の誤りにする。
func get(st config.StoryStore, path string, v any, notFound *Error) *Error {
	token, e := Token()
	if e != nil {
		return e
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	url := st.APIURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return storeErr("GitHub への要求を作れない（"+err.Error()+"）", ".vanda/config.toml の [story] の api_url と repository を確かめる")
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return storeErr("GitHub（"+url+"）に繋がらない（"+err.Error()+"）", "ネットワークと、[story] の api_url を確かめる")
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return storeErr("GitHub（"+url+"）の応答を読めない（"+err.Error()+"）", "ネットワークを確かめる")
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
		fix := "GitHub の応答の理由を確かめる"
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			fix = "gh auth login でログインし直すか、GH_TOKEN の鍵に、このリポジトリの Issues を読む権限があるかを確かめる"
		}
		return storeErr(fmt.Sprintf("GitHub（%s）が %d を返した（%s）", url, resp.StatusCode, reason), fix)
	}
	if err := json.Unmarshal(body, v); err != nil {
		return storeErr("GitHub（"+url+"）の応答が JSON として読めない（"+err.Error()+"）", "[story] の api_url が GitHub の REST API の URL かを確かめる")
	}
	return nil
}

func readGitHub(st config.StoryStore, id string) (*Story, *Error) {
	var nums []string
	for _, f := range strings.FieldsFunc(id, func(r rune) bool { return r < '0' || r > '9' }) {
		nums = append(nums, f)
	}
	if len(nums) == 0 {
		return nil, &Error{Status: http.StatusBadRequest, Message: "ストーリーの ID " + id + " に、GitHub の Issue の番号（数字）がない", Fix: "id_pattern を、番号を含む書き方（例: #[0-9]+）にする"}
	}
	num := nums[len(nums)-1]
	var issue struct {
		Title   string  `json:"title"`
		Body    *string `json:"body"`
		HTMLURL string  `json:"html_url"`
	}
	notFound := &Error{Status: http.StatusNotFound, Message: "GitHub の " + st.Repository + " に Issue " + num + " がない", Fix: "ストーリーの ID と、[story] の repository を確かめる"}
	if e := get(st, "/repos/"+st.Repository+"/issues/"+num, &issue, notFound); e != nil {
		return nil, e
	}
	body := ""
	if issue.Body != nil {
		body = *issue.Body
	}
	atts, err := Attachments(st.Repository, body)
	if err != nil {
		return nil, &Error{Status: http.StatusInternalServerError, Message: err.Error(), Fix: "vanda の不具合なので、知らせる"}
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
		return nil, fmt.Errorf("添付ファイルを探す正規表現が読めない（%v）", err)
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

type commandRequest struct {
	Action string `json:"action"`
	Story  string `json:"story"`
}

type commandResponse struct {
	Title       *string      `json:"title"`
	Body        *string      `json:"body"`
	URL         *string      `json:"url"`
	Attachments []Attachment `json:"attachments"`
}

func readCommand(root string, command []string, id string) (*Story, *Error) {
	in, err := json.Marshal(commandRequest{Action: "read", Story: id})
	if err != nil {
		return nil, storeErr("外部のアダプタに渡す JSON を作れない（"+err.Error()+"）", "vanda の不具合なので、知らせる")
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	cmd.Dir = root
	cmd.Stdin = bytes.NewReader(in)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	name := strings.Join(command, " ")
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, storeErr("ストーリーの置き場所のアダプタ（"+name+"）が "+id+" を読めなかった（"+msg+"）", "アダプタが返した理由を確かめる")
	}
	var resp commandResponse
	dec := json.NewDecoder(&stdout)
	dec.DisallowUnknownFields()
	bad := func(what string) *Error {
		return storeErr("ストーリーの置き場所のアダプタ（"+name+"）が返した JSON の形が誤っている（"+what+"）", "アダプタを docs/formats.md の形に合わせて直す")
	}
	if err := dec.Decode(&resp); err != nil {
		return nil, bad(err.Error())
	}
	if dec.More() {
		return nil, bad("JSON の後に続きがある")
	}
	switch {
	case resp.Title == nil:
		return nil, bad("title がない")
	case resp.Body == nil:
		return nil, bad("body がない")
	case resp.URL == nil:
		return nil, bad("url がない")
	}
	atts := []Attachment{}
	for i, a := range resp.Attachments {
		if a.Name == "" || a.URL == "" {
			return nil, bad(fmt.Sprintf("attachments の %d 件目に name か url がない", i+1))
		}
		atts = append(atts, a)
	}
	return &Story{ID: id, Title: *resp.Title, Body: *resp.Body, URL: *resp.URL, Attachments: atts}, nil
}

// Check は、ストーリーの置き場所に繋がるかを確かめ、確かめたことを 1 文で返す（CMD-01）。置き場所が設定になければ、空の文を返す。
func Check(root string, cfg *config.Config) (string, error) {
	st := cfg.Story
	switch {
	case st.Adapter == "github":
		var repo struct {
			HasIssues bool `json:"has_issues"`
		}
		notFound := storeErr("GitHub に "+st.Repository+" がないか、今の認証の鍵では見えない", "[story] の repository と、鍵の権限を確かめる")
		if e := get(st, "/repos/"+st.Repository, &repo, notFound); e != nil {
			return "", e
		}
		if !repo.HasIssues {
			return "", errors.New("GitHub の " + st.Repository + " で Issues が使えない。リポジトリの設定で Issues を有効にする")
		}
		return "ストーリーの置き場所 GitHub Issues（" + st.Repository + "）に繋がった", nil
	case len(st.Command) > 0:
		if _, err := exec.LookPath(command0(root, st.Command[0])); err != nil {
			return "", fmt.Errorf("ストーリーの置き場所のアダプタ %s が見つからない（%v）。[story] の command を直す", st.Command[0], err)
		}
		return "ストーリーの置き場所のアダプタ " + st.Command[0] + " がある", nil
	}
	return "", nil
}

// command0 は、/ を含む相対の場所のコマンドを、リポジトリの根からの場所にする（アダプタはリポジトリの根で動かすため）。
func command0(root, c string) string {
	if strings.Contains(c, "/") && !strings.HasPrefix(c, "/") {
		return root + "/" + c
	}
	return c
}
