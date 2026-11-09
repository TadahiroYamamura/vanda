package config

import (
	"fmt"
	"strings"

	"github.com/TadahiroYamamura/vanda/internal/repo"
)

// StoryTrailerKey は、コミットのメッセージの末尾に、ストーリーの ID を書くトレーラーのキー（Co-Authored-By: と同じ Git のトレーラー）
const StoryTrailerKey = "Story"

// StoryTrailer は、コミットのメッセージ msg の末尾のトレーラー Story: <ID> から、ストーリーの ID を読む。なければ空の文字列を返す。
// subject と本文の中の ID は数えない。Story: が 2 つ以上あるとき（1 つのコミットは 1 つのストーリーに当たる）と、
// 値が id_pattern に全体で当たらないときは、誤りを返す。
func (c *Config) StoryTrailer(msg string) (string, error) {
	var ids []string
	for _, l := range trailerLines(msg) {
		k, v, _ := strings.Cut(l, ":")
		if strings.EqualFold(strings.TrimSpace(k), StoryTrailerKey) {
			ids = append(ids, strings.TrimSpace(v))
		}
	}
	switch len(ids) {
	case 0:
		return "", nil
	case 1:
		if c.IDFull == nil || !c.IDFull.MatchString(ids[0]) {
			return "", fmt.Errorf("%s: のトレーラーの値 %s が、ストーリーの ID の書き方（%s）に合いません", StoryTrailerKey, ids[0], c.IDPattern)
		}
		return ids[0], nil
	default:
		return "", fmt.Errorf("%s: のトレーラーが %d つあります（%s）。1 つのコミットに書けるのは 1 つだけです", StoryTrailerKey, len(ids), strings.Join(ids, "・"))
	}
}

// trailerLines は、メッセージの最後の段落が Git のトレーラーの並び（どの行も「キー: 値」か、空白で始まる続きの行）なら、
// その「キー: 値」の行を返す。1 つ目の段落（subject を含む）は、トレーラーとみなさない。
func trailerLines(msg string) []string {
	var paras [][]string
	var cur []string
	for _, l := range strings.Split(strings.ReplaceAll(msg, "\r\n", "\n"), "\n") {
		if strings.TrimSpace(l) == "" {
			if len(cur) > 0 {
				paras = append(paras, cur)
				cur = nil
			}
			continue
		}
		cur = append(cur, l)
	}
	if len(cur) > 0 {
		paras = append(paras, cur)
	}
	if len(paras) < 2 {
		return nil
	}
	var out []string
	for _, l := range paras[len(paras)-1] {
		if l[0] == ' ' || l[0] == '\t' {
			continue
		}
		k, _, ok := strings.Cut(l, ":")
		if !ok || !isTrailerKey(k) {
			return nil
		}
		out = append(out, l)
	}
	return out
}

func isTrailerKey(k string) bool {
	if k == "" {
		return false
	}
	for _, r := range k {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-') {
			return false
		}
	}
	return true
}

// CommitStory は、1 つのコミットと、そのトレーラーから読んだストーリーの ID（なければ空）。Err は、トレーラーの誤り。
type CommitStory struct {
	Hash    string
	Subject string
	Story   string
	Err     error
}

// CommitStories は、revRange（例: <マージベース>..HEAD）のコミットの、トレーラーのストーリーの ID を、git log の順に返す。
func (c *Config) CommitStories(root, revRange string) ([]CommitStory, error) {
	out, err := repo.Git(root, "log", "--format=%H%x00%B%x1e", revRange)
	if err != nil {
		return nil, err
	}
	var res []CommitStory
	for _, rec := range strings.Split(out, "\x1e") {
		rec = strings.TrimLeft(rec, "\n")
		if rec == "" {
			continue
		}
		hash, msg, _ := strings.Cut(rec, "\x00")
		subject, _, _ := strings.Cut(strings.TrimSpace(msg), "\n")
		id, err := c.StoryTrailer(msg)
		res = append(res, CommitStory{Hash: hash, Subject: subject, Story: id, Err: err})
	}
	return res, nil
}
