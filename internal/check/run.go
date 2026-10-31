package check

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/TadahiroYamamura/vanda/internal/agreements"
	"github.com/TadahiroYamamura/vanda/internal/annotation"
	"github.com/TadahiroYamamura/vanda/internal/config"
	"github.com/TadahiroYamamura/vanda/internal/diag"
	"github.com/TadahiroYamamura/vanda/internal/glob"
	"github.com/TadahiroYamamura/vanda/internal/guardrail"
	"github.com/TadahiroYamamura/vanda/internal/promises"
	"github.com/TadahiroYamamura/vanda/internal/repo"
)

type Options struct {
	Root  string
	Cfg   *config.Config
	Specs []Spec
	Base  string
	Story string
	Out   io.Writer
}

// env は、審査に使う、読み込んだもの。
type env struct {
	root  string
	cfg   *config.Config
	files []string

	list     *promises.List
	listErrs diag.Errors
	recs     *agreements.Records
	recErrs  diag.Errors
	anns     []annotation.Annotation
	annErrs  diag.Errors
	guards   []string
	guardErr []diag.Error

	// 範囲 story の審査のためのもの
	mergeBase    string
	story        string
	changedItems map[string]bool // "<種類>:<ID>"
	changedFiles map[string]bool
}

// Run は、審査を設定の順に行い、結果を o.Out に出す。審査を始める前の誤りは error で返す。すべて通れば true。
func Run(o Options) (bool, error) {
	e := &env{root: o.Root, cfg: o.Cfg}
	if err := e.prepare(o); err != nil {
		return false, err
	}
	pass := true
	for _, s := range o.Specs {
		errs := e.run(s)
		diag.Sort(errs)
		scope := s.Scope
		if s.Scope == ScopeStory && e.story != "" && needsStory(s) {
			scope += " " + e.story
		}
		if len(errs) == 0 {
			fmt.Fprintf(o.Out, "審査 %s（%s）: 通った\n", s.Name, scope)
			continue
		}
		pass = false
		fmt.Fprintf(o.Out, "審査 %s（%s）: 失敗（%d 件）\n", s.Name, scope, len(errs))
		for _, err := range errs {
			fmt.Fprintln(o.Out, err.Error())
		}
	}
	return pass, nil
}

func needsStory(s Spec) bool {
	return s.Scope == ScopeStory && (s.Name == "agreement" || s.Name == "evidence-present")
}

func (e *env) prepare(o Options) error {
	files, err := glob.Files(e.root)
	if err != nil {
		return fmt.Errorf("リポジトリのファイルの一覧を作れない（%v）。ファイルを読む権限を確かめる", err)
	}
	e.files = files
	e.list, e.listErrs = promises.Load(e.root, files, e.cfg.Promises, e.cfg.PromisesDefault)
	e.recs, e.recErrs = agreements.Load(e.root, files, e.cfg.Agreements, e.cfg.AgreementsDefault)
	e.anns, e.annErrs = annotation.Read(e.root, files, e.cfg.Annotations)
	e.guards, e.guardErr = guardrail.Files(files, e.cfg.Guardrails)

	var storyChecks, needStory []string
	for _, s := range o.Specs {
		if s.Scope == ScopeStory {
			storyChecks = append(storyChecks, s.Name)
		}
		if needsStory(s) {
			needStory = append(needStory, s.Name)
		}
	}
	if len(storyChecks) == 0 {
		return nil
	}
	if o.Base == "" {
		return fmt.Errorf(`審査の設定に scope = "story" の審査（%s）があるので、ベースブランチが要る。--base <ブランチ> を渡す`, strings.Join(storyChecks, "・"))
	}
	mb, err := repo.Git(e.root, "merge-base", o.Base, "HEAD")
	if err != nil {
		return fmt.Errorf("ベースブランチ %s から分かれた点が分からない（%v）。--base に、今のブランチと履歴のつながったブランチを渡す", o.Base, err)
	}
	e.mergeBase = strings.TrimSpace(mb)
	if len(needStory) > 0 {
		if err := e.decideStory(o.Story, needStory); err != nil {
			return err
		}
	}
	return e.findChanges()
}

func (e *env) decideStory(given string, checks []string) error {
	if given != "" {
		if e.cfg.IDFull == nil || !e.cfg.IDFull.MatchString(given) {
			return fmt.Errorf("--story %s が、ストーリーの ID の書き方（%s）に合わない。設定の id_pattern に合う ID を渡す", given, e.cfg.IDPattern)
		}
		e.story = given
		return nil
	}
	msgs, err := repo.Git(e.root, "log", "--format=%B", e.mergeBase+"..HEAD")
	if err != nil {
		return fmt.Errorf("コミットのメッセージを読めない（%v）", err)
	}
	seen := map[string]bool{}
	var ids []string
	for _, id := range e.cfg.IDPattern.FindAllString(msgs, -1) {
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	switch len(ids) {
	case 1:
		e.story = ids[0]
		return nil
	case 0:
		return fmt.Errorf("範囲 story の審査（%s）のストーリーが分からない（分かれた点より後のコミットのメッセージに、ストーリーの ID がない）。--story <ID> を渡す", strings.Join(checks, "・"))
	default:
		return fmt.Errorf("範囲 story の審査（%s）のストーリーが決まらない（分かれた点より後のコミットのメッセージに、ストーリーの ID が %s とある）。--story <ID> で 1 つを渡す", strings.Join(checks, "・"), strings.Join(ids, "・"))
	}
}

// findChanges は、分かれた点の中身と作業ツリーの今の中身を比べ、変わった項目とファイルを求める。
func (e *env) findChanges() error {
	out, err := repo.Git(e.root, "ls-tree", "-r", "-z", "--name-only", e.mergeBase)
	if err != nil {
		return fmt.Errorf("分かれた点のファイルの一覧を読めない（%v）", err)
	}
	var baseFiles []string
	for _, f := range strings.Split(out, "\x00") {
		if f != "" {
			baseFiles = append(baseFiles, f)
		}
	}
	show := func(f string) ([]byte, error) {
		s, err := repo.Git(e.root, "show", e.mergeBase+":"+f)
		return []byte(s), err
	}
	matched, _ := glob.Select(baseFiles, e.cfg.Promises)
	srcs := map[string][]byte{}
	for _, f := range matched {
		b, err := show(f)
		if err != nil {
			return fmt.Errorf("分かれた点の %s を読めない（%v）", f, err)
		}
		srcs[f] = b
	}
	// 分かれた点の約束リストの誤りは、ここでは出さない。読めなかった項目は、増えたものとみなす（合意を求める側に倒す）
	baseList, _ := promises.FromSources(matched, srcs)
	baseFP := map[string]string{}
	for _, it := range items(baseList) {
		if fp, err := agreements.ItemFingerprint(it.raw); err == nil {
			baseFP[it.key()] = fp
		}
	}
	e.changedItems = map[string]bool{}
	for _, it := range items(e.list) {
		fp, err := agreements.ItemFingerprint(it.raw)
		if err != nil || baseFP[it.key()] != fp {
			e.changedItems[it.key()] = true
		}
	}
	inBase := map[string]bool{}
	for _, f := range baseFiles {
		inBase[f] = true
	}
	e.changedFiles = map[string]bool{}
	for _, f := range e.targetFiles() {
		if !inBase[f] {
			e.changedFiles[f] = true
			continue
		}
		b, err := show(f)
		if err != nil {
			return fmt.Errorf("分かれた点の %s を読めない（%v）", f, err)
		}
		cur, err := os.ReadFile(filepath.Join(e.root, filepath.FromSlash(f)))
		if err != nil || agreements.FileFingerprint(cur) != agreements.FileFingerprint(b) {
			e.changedFiles[f] = true
		}
	}
	return nil
}

// targetFiles は、合意の対象になるファイル（シナリオ、オラクルの定義、ガードレールの定義）を、場所の順で返す。
func (e *env) targetFiles() []string {
	seen := map[string]bool{}
	var out []string
	for _, f := range e.files {
		for _, r := range e.cfg.Annotations {
			if r.Adapter != "txtar" && r.Adapter != "tla" {
				continue
			}
			for _, p := range r.Paths {
				if glob.Match(p, f) && !seen[f] {
					seen[f] = true
					out = append(out, f)
				}
			}
		}
	}
	for _, f := range e.guards {
		if !seen[f] {
			seen[f] = true
			out = append(out, f)
		}
	}
	sort.Strings(out)
	return out
}

func (e *env) run(s Spec) []diag.Error {
	switch s.Name {
	case "promise-list-shape":
		return e.promiseListShape()
	case "annotation-links":
		return e.annotationLinks()
	case "story-origin":
		return e.storyOrigin()
	case "agreement":
		return e.agreement(s.Scope)
	case "agreement-targets":
		return e.agreementTargets()
	case "evidence-present":
		return e.evidencePresent(s.Scope)
	case "unread-files":
		return e.unreadFiles()
	}
	return []diag.Error{{What: "審査 " + s.Name + " はない", Fix: "vanda の不具合なので、知らせる"}}
}

// cannot は、前提の読み込みに失敗したために行えない審査の、1 行の誤り。
func cannot(what string, n int, owner string) []diag.Error {
	return []diag.Error{{What: fmt.Sprintf("%sが読めないため行えない（誤りが %d 件ある）", what, n), Fix: "審査 " + owner + " で誤りを出して直す"}}
}
