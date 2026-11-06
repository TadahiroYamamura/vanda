package review

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/TadahiroYamamura/vanda/internal/agreements"
	"github.com/TadahiroYamamura/vanda/internal/config"
	"github.com/TadahiroYamamura/vanda/internal/glob"
	"github.com/TadahiroYamamura/vanda/internal/repo"
	"github.com/TadahiroYamamura/vanda/internal/snapshot"
)

// Revocation は、合意済の 1 件を、今の人がこのストーリーで取り消せるか（RV-17）。
type Revocation struct {
	Possible bool `json:"possible"`
	// Reason は、取り消せないときの理由。取り消せるときは nil
	Reason *string `json:"reason"`
	// Others は、取り消しても残る、同じストーリーで今の中身への、ほかの人の合意（残れば合意済のまま）
	Others []User `json:"others"`
}

// branchPoint は、「今のブランチ」の始まり（分岐点）を返す。決まらなければ空の文字列。
// レビュー単位がブランチなら、その分岐点。レビュー単位がストーリーなら、選んでいるベースブランチか、設定の既定のもの
func (s *state) branchPoint(mergeBase string) string {
	if mergeBase != "" {
		return mergeBase
	}
	base := s.o.Base
	if base == "" {
		base = s.o.Cfg.GUIBase
	}
	if base == "" {
		return ""
	}
	mb, err := repo.Git(s.o.Root, "merge-base", base, "HEAD")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(mb)
}

// baseRecords は、分岐点の時点の合意の記録を読む。読めない記録は、ないものとみなす（取り消せる側に倒さないよう、呼び出し元で扱う）。
func (s *state) baseRecords(point string) (*agreements.Records, error) {
	snap, err := snapshot.At(s.o.Root, s.o.Cfg, point)
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
	recs, errs := agreements.FromSources(matched, srcs)
	if errs != nil {
		return nil, fmt.Errorf("分岐点の合意の記録を読めません（%d 件。最初の 1 件: %s）", len(errs), errs[0].Error())
	}
	return recs, nil
}

func sameRecord(a, b *agreements.Agreement) bool {
	return a.Kind == b.Kind && a.Key == b.Key && a.Fingerprint == b.Fingerprint && a.Deleted == b.Deleted && a.Story == b.Story &&
		a.AgreedAt.Equal(b.AgreedAt) && a.UserName == b.UserName && a.UserEmail == b.UserEmail
}

// revocable は、kind・key（ファイルなら場所）・fp の今の中身（deleted なら、消したこと）への、ストーリー story の合意の記録のうち、今の人が今のブランチで書いたもの（消すもの）と、
// 取り消せるかの判定を返す。
func (s *state) revocable(kind, key, fp string, deleted bool, point string, base *agreements.Records, user *User) ([]*agreements.Agreement, Revocation) {
	no := func(reason string) Revocation { return Revocation{Reason: ptr(reason), Others: []User{}} }
	story := s.o.Story
	if story == "" {
		return nil, no("ストーリーを選ぶと、取り消せる合意が分かります。")
	}
	if user == nil {
		return nil, no("git の user.name と user.email が設定されていないため、取り消せる合意が分かりません。")
	}
	var ofStory []*agreements.Agreement
	var otherStories []string
	seen := map[string]bool{}
	for _, a := range s.recs.Agreements {
		if a.Kind != kind || a.Target() != key || a.Deleted != deleted || !deleted && a.Fingerprint != fp {
			continue
		}
		if a.Story == story {
			ofStory = append(ofStory, a)
		} else if !seen[a.Story] {
			seen[a.Story] = true
			otherStories = append(otherStories, a.Story)
		}
	}
	if len(ofStory) == 0 {
		what := "今の中身"
		if deleted {
			what = "消したこと"
		}
		return nil, no(fmt.Sprintf("%sへのストーリー %s の合意はありません（ほかのストーリー %s の合意で合意済みです）。", what, story, strings.Join(otherStories, "・")))
	}
	if point == "" {
		return nil, no("ベースブランチが決まっていないため、このブランチで書いた合意か分かりません。ベースブランチを選んでください。")
	}
	if base == nil {
		return nil, no("分岐点の合意の記録を読めないため、このブランチで書いた合意か分かりません。")
	}
	var mine []*agreements.Agreement
	var others []User
	inBase := false
	for _, a := range ofStory {
		if a.UserName != user.Name || a.UserEmail != user.Email {
			others = append(others, User{Name: a.UserName, Email: a.UserEmail})
			continue
		}
		written := true
		for _, b := range base.Agreements {
			if sameRecord(a, b) {
				written = false
				break
			}
		}
		if written {
			mine = append(mine, a)
		} else {
			inBase = true
		}
	}
	if others == nil {
		others = []User{}
	}
	if len(mine) == 0 {
		r := no("ストーリー " + story + " の合意は、ほかの人が書いたものです。")
		if inBase {
			r = no("ストーリー " + story + " の合意は、ベースブランチにすでにあります。")
		}
		r.Others = others
		return nil, r
	}
	return mine, Revocation{Possible: true, Others: others}
}

// setRevocations は、合意済の各件に、取り消せるかを付ける。
func (s *state) setRevocations(es []Entry, mergeBase string) error {
	user := GitUser(s.o.Root)
	point := s.branchPoint(mergeBase)
	var base *agreements.Records
	if point != "" && s.o.Story != "" && user != nil {
		b, err := s.baseRecords(point)
		if err == nil {
			base = b
		}
	}
	for i := range es {
		e := &es[i]
		deleted := e.Change == "deleted"
		if e.State == nil || *e.State != "agreed" || e.Fingerprint == nil && !deleted {
			continue
		}
		kind, key, fp := e.Kind, "", ""
		if e.Kind == "file" {
			key = *e.Path
		} else {
			key = *e.ID
		}
		if !deleted {
			fp = *e.Fingerprint
		}
		_, r := s.revocable(kind, key, fp, deleted, point, base, user)
		e.Revoke = &r
	}
	return nil
}

type RevokeRequest struct {
	Kind        string  `json:"kind"`
	ID          *string `json:"id"`
	Path        *string `json:"path"`
	Fingerprint string  `json:"fingerprint"`
	Story       string  `json:"story"`
	Base        *string `json:"base"`
	// Deleted は、消したことへの合意を取り消すか。真なら fingerprint は見ない
	Deleted bool `json:"deleted"`
}

type RevokeResult struct {
	Commit string `json:"commit"`
	// StillAgreed は、取り消した後も合意済か（同じストーリーのほかの人や、ほかのストーリーの合意が残っているとき）
	StillAgreed bool `json:"stillAgreed"`
	Removed     int  `json:"removed"`
}

// Revoke は、1 件への合意を取り消す（RV-17）。今の人が今のブランチで書いた、ストーリーの今の中身への合意の記録を消し、そのファイルだけを 1 つのコミットにする。
func Revoke(o Options, req RevokeRequest) (*RevokeResult, *Error) {
	if req.Base != nil {
		o.Base = *req.Base
	}
	o.Story = req.Story
	s := &state{o: o, h: newHistory(o.Root, o.Cfg)}
	if e := s.checkStory(req.Story); e != nil {
		return nil, e
	}
	if e := s.loadCurrent(); e != nil {
		return nil, e
	}
	var kind, key, fp, target string
	switch {
	case req.Deleted:
		k, t, e := s.deletedTarget(req.Kind, req.ID, req.Path, "取り消し")
		if e != nil {
			return nil, e
		}
		kind, key, target = req.Kind, k, t
	case req.Kind == "claim" || req.Kind == "term" || req.Kind == "feature":
		if req.ID == nil || *req.ID == "" {
			return nil, badRequest(kindName[req.Kind]+"の取り消しに id がありません。", "id に ID を書いてください。")
		}
		k := req.Kind + ":" + *req.ID
		if _, ok := s.cur.items[k]; !ok {
			return nil, &Error{Status: http.StatusNotFound, Message: kindName[req.Kind] + " " + *req.ID + " は約束リストにありません。", Fix: "一覧を読み直してください。"}
		}
		kind, key, fp, target = req.Kind, *req.ID, s.cur.itemFP[k], kindName[req.Kind]+" "+*req.ID
	case req.Kind == "file":
		if req.Path == nil || *req.Path == "" {
			return nil, badRequest("ファイルの取り消しに path がありません。", "path にファイルの場所を書いてください。")
		}
		if _, ok := s.cur.types[*req.Path]; !ok {
			return nil, &Error{Status: http.StatusNotFound, Message: *req.Path + " は合意の対象のファイルではありません。", Fix: "一覧を読み直してください。"}
		}
		f, err := s.cur.fileFP(*req.Path)
		if err != nil {
			return nil, internalErr(err)
		}
		kind, key, fp, target = "file", *req.Path, f, "ファイル "+*req.Path
	default:
		return nil, badRequest(fmt.Sprintf("対象の種類 %q はありません。", req.Kind), "kind を claim・term・feature・file のどれかにしてください。")
	}
	if !req.Deleted && fp != req.Fingerprint {
		return nil, conflict(target+" の中身が、画面に表示した後に変わりました（画面 "+req.Fingerprint+"、今 "+fp+"）。", "一覧を読み直し、今の中身を確かめてから取り消してください。")
	}
	user := GitUser(o.Root)
	point := s.branchPoint("")
	var base *agreements.Records
	if point != "" && user != nil {
		b, err := s.baseRecords(point)
		if err != nil {
			return nil, internalErr(err)
		}
		base = b
	}
	remove, r := s.revocable(kind, key, fp, req.Deleted, point, base, user)
	if !r.Possible {
		return nil, conflict(target+" の合意を取り消せません。"+*r.Reason, "取り消せるのは、あなたがこのブランチで書いた、このストーリーの合意だけです。")
	}

	byFile := map[string][]*agreements.Agreement{}
	var files []string
	for _, a := range remove {
		f := a.Table.Doc.File
		if _, ok := byFile[f]; !ok {
			files = append(files, f)
		}
		byFile[f] = append(byFile[f], a)
	}
	dirty, err := repo.Git(o.Root, append([]string{"status", "--porcelain", "-z", "--"}, files...)...)
	if err != nil {
		return nil, internalErr(err)
	}
	if dirty != "" {
		return nil, conflict("取り消しで変えるファイル（"+strings.Join(files, "・")+"）に、コミットしていない変更があります。", "そのファイルの変更をコミットするか元に戻してから、取り消してください。")
	}
	originals := map[string][]byte{}
	for _, f := range files {
		b, err := os.ReadFile(filepath.Join(o.Root, filepath.FromSlash(f)))
		if err != nil {
			return nil, internalErr(err)
		}
		originals[f] = b
	}
	fail := func(err error) *Error {
		var errs []error
		for f, b := range originals {
			errs = append(errs, os.WriteFile(filepath.Join(o.Root, filepath.FromSlash(f)), b, 0o644))
		}
		if rerr := errors.Join(errs...); rerr != nil {
			return &Error{Status: http.StatusInternalServerError, Message: fmt.Sprintf("取り消しを書けず（%v）、ファイルを元に戻すこともできませんでした（%v）。", err, rerr), Fix: "git status で " + strings.Join(files, "・") + " を確かめて、直してください。"}
		}
		return &Error{Status: http.StatusInternalServerError, Message: fmt.Sprintf("取り消しを書けませんでした（%v）。ファイルは元に戻しました。", err), Fix: "git の状態を確かめてください。"}
	}
	for _, f := range files {
		out, err := removeRecords(originals[f], byFile[f])
		if err != nil {
			return nil, fail(err)
		}
		if err := os.WriteFile(filepath.Join(o.Root, filepath.FromSlash(f)), out, 0o644); err != nil {
			return nil, fail(err)
		}
	}
	if Hooks.AfterRevokeWrite != nil {
		Hooks.AfterRevokeWrite(o.Root)
	}
	// 読み直して、消したものだけが消え、ほかの記録が変わっていないことを確かめる
	snap, err := snapshot.Working(o.Root, o.Cfg)
	if err != nil {
		return nil, fail(err)
	}
	after, rerrs := agreements.Load(o.Root, snap.Files, o.Cfg.Agreements, o.Cfg.AgreementsDefault)
	if rerrs != nil {
		return nil, fail(fmt.Errorf("取り消した後の合意の記録を読めません（%s）", rerrs[0].Error()))
	}
	if err := checkRevokeOwn(s.recs.Agreements, after.Agreements, user, base); err != nil {
		return nil, fail(err)
	}
	if err := checkRevokeKept(s.recs.Agreements, after.Agreements, kind, key, fp, req.Deleted, req.Story); err != nil {
		return nil, fail(err)
	}
	if err := checkRemoved(s.recs.Agreements, after.Agreements, remove); err != nil {
		return nil, fail(err)
	}
	msg := fmt.Sprintf("(chore) %s で %s の合意を取り消す", req.Story, target)
	if req.Deleted {
		msg = fmt.Sprintf("(chore) %s で %s を消したことへの合意を取り消す", req.Story, target)
	}
	if _, err := repo.Git(o.Root, append([]string{"commit", "-q", "-m", msg, "-m", config.StoryTrailerKey + ": " + req.Story, "--"}, files...)...); err != nil {
		return nil, fail(err)
	}
	head, err := repo.Git(o.Root, "rev-parse", "HEAD")
	if err != nil {
		return nil, internalErr(err)
	}
	if Hooks.AfterRevokeCommit != nil {
		Hooks.AfterRevokeCommit(o.Root)
	}
	if err := checkRevokeCommit(o.Root, files); err != nil {
		return nil, sensorErr("取り消し", strings.TrimSpace(head), err)
	}
	still := after.Agreed(kind, key, fp)
	if req.Deleted {
		still = after.DeletionAgreed(kind, key)
	}
	return &RevokeResult{Commit: strings.TrimSpace(head), StillAgreed: still, Removed: len(remove)}, nil
}

// removeRecords は、合意の記録のファイルの中身 src から、records の [[agreement]] の表（見出しから、次の表の見出しの前までのうち、
// 終わりの空行とコメントの行を除いたもの）と、その直前の空行 1 つを消す。ほかの行は変えない。
func removeRecords(src []byte, records []*agreements.Agreement) ([]byte, error) {
	lines := strings.SplitAfter(string(src), "\n")
	drop := map[int]bool{}
	isHeader := func(l string) bool {
		t := strings.TrimSpace(l)
		return strings.HasPrefix(t, "[") && !strings.HasPrefix(t, "[agreement.")
	}
	for _, a := range records {
		start := a.Table.Line("") - 1
		if start < 0 || start >= len(lines) || !strings.HasPrefix(strings.TrimSpace(lines[start]), "[[") {
			return nil, fmt.Errorf("%s: 合意の記録の見出しの行が見つかりません", a.Table.Doc.File)
		}
		end := start
		for j := start + 1; j < len(lines) && !isHeader(lines[j]); j++ {
			t := strings.TrimSpace(lines[j])
			if t != "" && !strings.HasPrefix(t, "#") {
				end = j
			}
		}
		for j := start; j <= end; j++ {
			drop[j] = true
		}
		if start > 0 && strings.TrimSpace(lines[start-1]) == "" {
			drop[start-1] = true
		}
	}
	var b strings.Builder
	for i, l := range lines {
		if !drop[i] {
			b.WriteString(l)
		}
	}
	return []byte(b.String()), nil
}

// checkRemoved は、取り消しの後の記録が、前の記録から remove を除いたものとちょうど同じかを確かめる。
func checkRemoved(before, after, remove []*agreements.Agreement) error {
	var want []*agreements.Agreement
	for _, a := range before {
		gone := false
		for _, r := range remove {
			if a == r {
				gone = true
			}
		}
		if !gone {
			want = append(want, a)
		}
	}
	if len(want) != len(after) {
		return fmt.Errorf("取り消した後の合意の記録が %d 件になりました（%d 件のはずです）", len(after), len(want))
	}
	for i := range want {
		if !sameRecord(want[i], after[i]) {
			return fmt.Errorf("取り消した後の合意の記録の %d 件目が、取り消す前と違います", i+1)
		}
	}
	return nil
}

// deletedTarget は、消したことへの合意と取り消しの対象を確かめ、記録の key（約束・用語・機能なら ID、ファイルなら場所）と、
// 文言に使う名前を返す。今あるものは消していないので断る（画面に表示した後に足し直されたときも、ここで断る）。
func (s *state) deletedTarget(kind string, id, path *string, what string) (string, string, *Error) {
	switch kind {
	case "claim", "term", "feature":
		if id == nil || *id == "" {
			return "", "", badRequest(kindName[kind]+"の"+what+"に id がありません。", "id に ID を書いてください。")
		}
		if _, ok := s.cur.items[kind+":"+*id]; ok {
			return "", "", conflict(kindName[kind]+" "+*id+" は約束リストにあり、消していません。", "一覧を読み直してください。")
		}
		return *id, kindName[kind] + " " + *id, nil
	case "file":
		if path == nil || *path == "" {
			return "", "", badRequest("ファイルの"+what+"に path がありません。", "path にファイルの場所を書いてください。")
		}
		if _, ok := s.cur.types[*path]; ok {
			return "", "", conflict(*path+" は合意の対象のファイルとして今もあり、消していません。", "一覧を読み直してください。")
		}
		if len(snapshot.Targets([]string{*path}, s.o.Cfg)) == 0 {
			return "", "", &Error{Status: http.StatusNotFound, Message: *path + " は合意の対象のファイルの場所ではありません。", Fix: "一覧を読み直してください。"}
		}
		return *path, "ファイル " + *path, nil
	}
	return "", "", badRequest(fmt.Sprintf("対象の種類 %q はありません。", kind), "kind を claim・term・feature・file のどれかにしてください。")
}
