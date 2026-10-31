// Package snapshot は、ある時点（コミットか作業ツリー）の、約束リストと合意の対象のファイルを読む。
// 審査（vanda check）の「1 つのストーリーで変わったもの」と、画面の一覧が、同じ比べ方を使うためにある。
package snapshot

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/TadahiroYamamura/vanda/internal/agreements"
	"github.com/TadahiroYamamura/vanda/internal/config"
	"github.com/TadahiroYamamura/vanda/internal/diag"
	"github.com/TadahiroYamamura/vanda/internal/glob"
	"github.com/TadahiroYamamura/vanda/internal/promises"
	"github.com/TadahiroYamamura/vanda/internal/repo"
	"github.com/TadahiroYamamura/vanda/internal/tomlread"
)

// Entry は、約束リストの項目（用語・機能・約束）を、種類を問わずに扱うためのもの。
type Entry struct {
	Kind    string // term・feature・claim
	ID      string
	Table   *tomlread.Table
	Raw     map[string]any
	Term    *promises.Term
	Feature *promises.Feature
	Claim   *promises.Claim
}

// Key は、種類と ID をつないだ鍵。ID は種類をまたいで重ならない決まりだが、重なった誤りがあっても別のものとして数えるため、種類も入れる。
func (e Entry) Key() string { return e.Kind + ":" + e.ID }

// Entries は、用語・機能・約束の順に返す。
func Entries(l *promises.List) []Entry {
	if l == nil {
		return nil
	}
	var out []Entry
	for _, t := range l.Terms {
		out = append(out, Entry{Kind: "term", ID: t.ID, Table: t.Table, Raw: t.Raw, Term: t})
	}
	for _, f := range l.Features {
		out = append(out, Entry{Kind: "feature", ID: f.ID, Table: f.Table, Raw: f.Raw, Feature: f})
	}
	for _, c := range l.Claims {
		out = append(out, Entry{Kind: "claim", ID: c.ID, Table: c.Table, Raw: c.Raw, Claim: c})
	}
	return out
}

// ファイルの種類
const (
	Scenario  = "scenario"
	Oracle    = "oracle"
	Guardrail = "guardrail"
)

// Target は、合意の対象のファイル。
type Target struct {
	Path string
	Type string
}

// Targets は、files のうち合意の対象になるファイル（txtar・tla のアノテーションのパターンと、[[guardrail]] の paths に当たるもの）を、場所の順で返す。
// 2 つに当たるファイルは、アノテーションのほうの種類にする。
func Targets(files []string, cfg *config.Config) []Target {
	kind := map[string]string{}
	for _, f := range files {
		for _, r := range cfg.Annotations {
			t := map[string]string{"txtar": Scenario, "tla": Oracle}[r.Adapter]
			if t == "" || kind[f] != "" {
				continue
			}
			for _, p := range r.Paths {
				if glob.Match(p, f) {
					kind[f] = t
					break
				}
			}
		}
	}
	for _, g := range cfg.Guardrails {
		matched, _ := glob.Select(files, g.Paths)
		for _, f := range matched {
			if kind[f] == "" {
				kind[f] = Guardrail
			}
		}
	}
	out := make([]Target, 0, len(kind))
	for f, t := range kind {
		out = append(out, Target{Path: f, Type: t})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

// Snapshot は、ある時点のファイルの一覧と、約束リストと、合意の対象のファイル。
type Snapshot struct {
	Files    []string
	List     *promises.List
	ListErrs diag.Errors
	Targets  []Target
	has      map[string]bool
	read     func(path string) ([]byte, error)
}

// Working は、作業ツリーの今の中身を読む。
func Working(root string, cfg *config.Config) (*Snapshot, error) {
	files, err := glob.Files(root)
	if err != nil {
		return nil, fmt.Errorf("リポジトリのファイルの一覧を作れない（%v）。ファイルを読む権限を確かめる", err)
	}
	s := newSnapshot(files, cfg, func(f string) ([]byte, error) {
		return os.ReadFile(filepath.Join(root, filepath.FromSlash(f)))
	})
	s.List, s.ListErrs = promises.Load(root, files, cfg.Promises, cfg.PromisesDefault)
	return s, nil
}

// At は、コミット rev の中身を読む。パターンは今の設定のものを当てる。
// 約束リストの誤りは ListErrs に入れ、読めた項目だけを List に入れる。
func At(root string, cfg *config.Config, rev string) (*Snapshot, error) {
	out, err := repo.Git(root, "ls-tree", "-r", "-z", "--name-only", rev)
	if err != nil {
		return nil, fmt.Errorf("コミット %s のファイルの一覧を読めない（%v）", rev, err)
	}
	var files []string
	for _, f := range strings.Split(out, "\x00") {
		if f != "" {
			files = append(files, f)
		}
	}
	sort.Strings(files)
	s := newSnapshot(files, cfg, func(f string) ([]byte, error) {
		b, err := repo.Git(root, "show", rev+":"+f)
		return []byte(b), err
	})
	matched, _ := glob.Select(files, cfg.Promises)
	srcs := map[string][]byte{}
	for _, f := range matched {
		b, err := s.read(f)
		if err != nil {
			return nil, fmt.Errorf("コミット %s の %s を読めない（%v）", rev, f, err)
		}
		srcs[f] = b
	}
	s.List, s.ListErrs = promises.FromSources(matched, srcs)
	return s, nil
}

func newSnapshot(files []string, cfg *config.Config, read func(string) ([]byte, error)) *Snapshot {
	has := make(map[string]bool, len(files))
	for _, f := range files {
		has[f] = true
	}
	return &Snapshot{Files: files, Targets: Targets(files, cfg), has: has, read: read}
}

// Has は、その時点に path のファイルがあるかを返す。
func (s *Snapshot) Has(path string) bool { return s.has[path] }

// Read は、その時点の path のファイルの中身を返す。
func (s *Snapshot) Read(path string) ([]byte, error) {
	if !s.has[path] {
		return nil, fmt.Errorf("%s がない", path)
	}
	return s.read(path)
}

// ItemFingerprints は、項目の鍵から指紋への表を返す。指紋を作れない項目は入れない。
func (s *Snapshot) ItemFingerprints() map[string]string {
	out := map[string]string{}
	for _, e := range Entries(s.List) {
		if fp, err := agreements.ItemFingerprint(e.Raw); err == nil {
			out[e.Key()] = fp
		}
	}
	return out
}
