package review

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/TadahiroYamamura/vanda/internal/agreements"
	"github.com/TadahiroYamamura/vanda/internal/glob"
	"github.com/TadahiroYamamura/vanda/internal/repo"
	"github.com/TadahiroYamamura/vanda/internal/snapshot"
)

// ClaimAt は、あるコミットの時点の約束の中身（CL-08）。
type ClaimAt struct {
	ID      string       `json:"id"`
	Commit  string       `json:"commit"`
	Date    string       `json:"date"`
	Subject string       `json:"subject"`
	Stories []string     `json:"stories"`
	Content ClaimContent `json:"content"`
}

// ClaimAtCommit は、コミット rev の時点の、約束 id の中身を返す。
func ClaimAtCommit(o Options, id, rev string) (*ClaimAt, *Error) {
	// git の引数として読まれないよう、コミットのハッシュの形のものだけを受け付ける
	if !isHash(rev) {
		return nil, &Error{Status: http.StatusBadRequest, Message: "コミット " + rev + " が、コミットのハッシュ（16 進の 4〜64 文字）の形ではありません。", Fix: "由来か合意の履歴から、もう一度選んでください。"}
	}
	full, err := repo.Git(o.Root, "rev-parse", "--verify", "-q", rev+"^{commit}")
	if err != nil {
		return nil, &Error{Status: http.StatusNotFound, Message: "コミット " + rev + " はありません。", Fix: "由来か合意の履歴から、もう一度選んでください。"}
	}
	full = strings.TrimSpace(full)
	info, err := repo.Git(o.Root, "log", "-1", "--format=%cI%x00%B", full)
	if err != nil {
		return nil, internalErr(err)
	}
	date, msg, _ := strings.Cut(strings.TrimRight(info, "\n"), "\x00")
	msg = strings.TrimSpace(msg)
	subject, _, _ := strings.Cut(msg, "\n")
	snap, err := snapshot.At(o.Root, o.Cfg, full)
	if err != nil {
		return nil, internalErr(err)
	}
	if snap.ListErrs != nil {
		return nil, &Error{Status: http.StatusInternalServerError, Message: fmt.Sprintf("コミット %s の時点の約束リストを読めません（%d 件。最初の 1 件: %s）。", short(full), len(snap.ListErrs), snap.ListErrs[0].Error()), Fix: "その時点の約束リストに誤りがあります。ほかの時点を選んでください。"}
	}
	for _, e := range snapshot.Entries(snap.List) {
		if e.Kind == "claim" && e.ID == id {
			return &ClaimAt{ID: id, Commit: full, Date: date, Subject: subject, Stories: strs(storyIDs(o.Cfg, msg)), Content: Content(e).(ClaimContent)}, nil
		}
	}
	return nil, &Error{Status: http.StatusNotFound, Message: fmt.Sprintf("コミット %s の時点には、約束 %s はありません。", short(full), id), Fix: "由来か合意の履歴から、ほかの時点を選んでください。"}
}

// agreementCommits は、合意の記録ごとに、その記録を書き足したコミットを探す。
// HEAD の履歴を古い順にたどり、合意の記録のファイルを変えたコミットの時点の記録に、同じ記録（種類・鍵・指紋・ストーリー・日時・誰が）が初めて現れたコミットとする。
// まだコミットしていない記録は、見つからない（空の文字列）。
func (s *state) agreementCommits(list []*agreements.Agreement) (map[*agreements.Agreement]string, error) {
	out := map[*agreements.Agreement]string{}
	if len(list) == 0 {
		return out, nil
	}
	commits, err := s.h.loadCommits()
	if err != nil {
		return nil, err
	}
	same := func(a, b *agreements.Agreement) bool {
		return a.Kind == b.Kind && a.Key == b.Key && a.Fingerprint == b.Fingerprint && a.Story == b.Story &&
			a.AgreedAt.Equal(b.AgreedAt) && a.UserName == b.UserName && a.UserEmail == b.UserEmail
	}
	for _, c := range commits {
		if len(out) == len(list) {
			break
		}
		names, err := repo.Git(s.o.Root, "diff-tree", "--no-commit-id", "--name-only", "-r", "-z", "--root", c.Hash)
		if err != nil {
			return nil, fmt.Errorf("コミット %s で変わったファイルを読めません（%v）", short(c.Hash), err)
		}
		touched := false
		for _, f := range strings.Split(names, "\x00") {
			if f == "" {
				continue
			}
			for _, p := range s.o.Cfg.Agreements {
				if glob.Match(p, f) {
					touched = true
				}
			}
		}
		if !touched {
			continue
		}
		snap, err := snapshot.At(s.o.Root, s.o.Cfg, c.Hash)
		if err != nil {
			return nil, err
		}
		matched, _ := glob.Select(snap.Files, s.o.Cfg.Agreements)
		srcs := map[string][]byte{}
		for _, f := range matched {
			b, err := snap.Read(f)
			if err != nil {
				return nil, err
			}
			srcs[f] = b
		}
		// その時点の合意の記録に誤りがあっても、読めた記録だけで探す
		recs, _ := agreements.FromSources(matched, srcs)
		for _, want := range list {
			if _, done := out[want]; done {
				continue
			}
			for _, got := range recs.Agreements {
				if same(want, got) {
					out[want] = c.Hash
					break
				}
			}
		}
	}
	return out, nil
}

func isHash(s string) bool {
	if len(s) < 4 || len(s) > 64 {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
