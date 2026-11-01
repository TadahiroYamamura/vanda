package review

import (
	"fmt"
	"strings"
	"time"

	"github.com/TadahiroYamamura/vanda/internal/agreements"
	"github.com/TadahiroYamamura/vanda/internal/config"
	"github.com/TadahiroYamamura/vanda/internal/glob"
	"github.com/TadahiroYamamura/vanda/internal/repo"
	"github.com/TadahiroYamamura/vanda/internal/snapshot"
)

// side は、ある時点の、項目と合意の対象のファイル。ファイルの中身は、要るときに読む。
type side struct {
	snap   *snapshot.Snapshot // nil なら、何もない時点（最初のコミットの親）
	items  map[string]snapshot.Entry
	itemFP map[string]string // 指紋を作れない項目は空の文字列
	types  map[string]string // 場所からファイルの種類へ
	texts  map[string][]byte
}

func newSide(s *snapshot.Snapshot) *side {
	sd := &side{snap: s, items: map[string]snapshot.Entry{}, itemFP: map[string]string{}, types: map[string]string{}, texts: map[string][]byte{}}
	if s == nil {
		return sd
	}
	for _, e := range snapshot.Entries(s.List) {
		// 重なった ID は、約束リストの形の審査で誤りになる。ここでは最初のものを使う
		if _, dup := sd.items[e.Key()]; dup {
			continue
		}
		sd.items[e.Key()] = e
		fp, err := agreements.ItemFingerprint(e.Raw)
		if err != nil {
			fp = ""
		}
		sd.itemFP[e.Key()] = fp
	}
	for _, t := range s.Targets {
		sd.types[t.Path] = t.Type
	}
	return sd
}

func (sd *side) text(path string) ([]byte, error) {
	if b, ok := sd.texts[path]; ok {
		return b, nil
	}
	b, err := sd.snap.Read(path)
	if err != nil {
		return nil, err
	}
	sd.texts[path] = b
	return b, nil
}

func (sd *side) fileFP(path string) (string, error) {
	b, err := sd.text(path)
	if err != nil {
		return "", err
	}
	return agreements.FileFingerprint(b), nil
}

// sameItem は、項目 key が、2 つの時点で同じ中身か（どちらにもないときも同じ）を返す。指紋を作れない項目は、変わったとみなす。
func sameItem(a, b *side, key string) bool {
	fa, ina := a.itemFP[key]
	fb, inb := b.itemFP[key]
	if ina != inb {
		return false
	}
	return !ina || fa != "" && fa == fb
}

// sameFile は、場所 path の合意の対象のファイルが、2 つの時点で同じ中身か（どちらでも対象でないときも同じ）を返す。
func sameFile(a, b *side, path string) (bool, error) {
	_, ina := a.types[path]
	_, inb := b.types[path]
	if ina != inb {
		return false, nil
	}
	if !ina {
		return true, nil
	}
	fa, err := a.fileFP(path)
	if err != nil {
		return false, err
	}
	fb, err := b.fileFP(path)
	if err != nil {
		return false, err
	}
	return fa == fb, nil
}

// commit は、HEAD の履歴の、合流でないコミット。
type commit struct {
	Hash    string
	Parent  string // 最初のコミットなら空
	Date    time.Time
	Subject string
	Stories []string
	// StoryErr は、Story: のトレーラーの誤り（2 つ以上、id_pattern に合わない）。誤りなら Stories は空
	StoryErr string
}

// history は、コミットごとの時点を読み、使い回す。
type history struct {
	root    string
	cfg     *config.Config
	sides   map[string]*side
	commits []commit // 古い順
	changes map[string]*changeSet
}

type changeSet struct {
	items map[string]bool
	files map[string]bool
}

func newHistory(root string, cfg *config.Config) *history {
	return &history{root: root, cfg: cfg, sides: map[string]*side{}, changes: map[string]*changeSet{}}
}

func (h *history) sideAt(rev string) (*side, error) {
	if rev == "" {
		return newSide(nil), nil
	}
	if sd, ok := h.sides[rev]; ok {
		return sd, nil
	}
	s, err := snapshot.At(h.root, h.cfg, rev)
	if err != nil {
		return nil, err
	}
	sd := newSide(s)
	h.sides[rev] = sd
	return sd, nil
}

// storyIDs は、コミットのメッセージの末尾の Story: のトレーラーにあるストーリーの ID を返す（0 か 1 つ）。
// トレーラーの誤り（2 つ以上、id_pattern に合わない）のコミットは、ストーリーを空にする。履歴に 1 つ誤ったコミットがあるだけで
// 画面を開けなくならないよう、誤りで止めずに、理由を /api/context の trailerErrors で画面に出す
func storyIDs(cfg *config.Config, msg string) []string {
	id, err := cfg.StoryTrailer(msg)
	if err != nil || id == "" {
		return nil
	}
	return []string{id}
}

// storyIDsIn は、revRange のコミットのトレーラーにあるストーリーの ID を、出てきた順に、重ねずに返す（誤りの扱いは storyIDs と同じ）。
func storyIDsIn(root string, cfg *config.Config, revRange string) ([]string, error) {
	commits, err := cfg.CommitStories(root, revRange)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []string
	for _, c := range commits {
		if c.Err == nil && c.Story != "" && !seen[c.Story] {
			seen[c.Story] = true
			out = append(out, c.Story)
		}
	}
	return out, nil
}

func hasStory(c commit, story string) bool {
	for _, s := range c.Stories {
		if s == story {
			return true
		}
	}
	return false
}

// loadCommits は、HEAD の履歴の、合流でないコミットを古い順に読む。コミットのない新しいリポジトリでは空。
func (h *history) loadCommits() ([]commit, error) {
	if h.commits != nil {
		return h.commits, nil
	}
	if _, err := repo.Git(h.root, "rev-parse", "--verify", "-q", "HEAD"); err != nil {
		h.commits = []commit{}
		return h.commits, nil
	}
	out, err := repo.Git(h.root, "log", "--no-merges", "--reverse", "--format=%H%x00%P%x00%cI%x00%B%x1e", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("コミットの履歴を読めません（%v）", err)
	}
	h.commits = []commit{}
	for _, rec := range strings.Split(out, "\x1e") {
		rec = strings.TrimLeft(rec, "\n")
		if rec == "" {
			continue
		}
		f := strings.SplitN(rec, "\x00", 4)
		if len(f) != 4 {
			return nil, fmt.Errorf("コミットの履歴の形を読めません（%q）", rec)
		}
		date, err := time.Parse(time.RFC3339, f[2])
		if err != nil {
			return nil, fmt.Errorf("コミット %s の日時 %q を読めません（%v）", f[0], f[2], err)
		}
		parent, _, _ := strings.Cut(f[1], " ")
		msg := strings.TrimSpace(f[3])
		subject, _, _ := strings.Cut(msg, "\n")
		c := commit{Hash: f[0], Parent: parent, Date: date, Subject: subject, Stories: storyIDs(h.cfg, msg)}
		if _, err := h.cfg.StoryTrailer(msg); err != nil {
			c.StoryErr = err.Error() + "。"
		}
		h.commits = append(h.commits, c)
	}
	return h.commits, nil
}

// changed は、コミット c が、1 つ目の親から変えた項目の鍵と、合意の対象のファイルの場所を返す。
func (h *history) changed(c commit) (*changeSet, error) {
	if cs, ok := h.changes[c.Hash]; ok {
		return cs, nil
	}
	cs := &changeSet{items: map[string]bool{}, files: map[string]bool{}}
	h.changes[c.Hash] = cs
	names, err := repo.Git(h.root, "diff-tree", "--no-commit-id", "--name-only", "-r", "-z", "--root", c.Hash)
	if err != nil {
		return nil, fmt.Errorf("コミット %s で変わったファイルを読めません（%v）", short(c.Hash), err)
	}
	var touched []string
	promisesTouched := false
	for _, f := range strings.Split(names, "\x00") {
		if f == "" {
			continue
		}
		touched = append(touched, f)
		for _, p := range h.cfg.Promises {
			if glob.Match(p, f) {
				promisesTouched = true
			}
		}
	}
	if len(touched) == 0 {
		return cs, nil
	}
	before, err := h.sideAt(c.Parent)
	if err != nil {
		return nil, err
	}
	after, err := h.sideAt(c.Hash)
	if err != nil {
		return nil, err
	}
	if promisesTouched {
		for k := range union(before.itemFP, after.itemFP) {
			if !sameItem(before, after, k) {
				cs.items[k] = true
			}
		}
	}
	for _, f := range touched {
		same, err := sameFile(before, after, f)
		if err != nil {
			return nil, err
		}
		if !same {
			cs.files[f] = true
		}
	}
	return cs, nil
}

func union(a, b map[string]string) map[string]bool {
	out := map[string]bool{}
	for k := range a {
		out[k] = true
	}
	for k := range b {
		out[k] = true
	}
	return out
}

func short(c string) string {
	if len(c) > 7 {
		return c[:7]
	}
	return c
}
