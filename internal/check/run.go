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
	"github.com/TadahiroYamamura/vanda/internal/snapshot"
)

type Options struct {
	Root  string
	Cfg   *config.Config
	Specs []Spec
	Base  string
	Story string
	// Out には審査ごとの結果の行を、Err には誤りの中身を出す
	Out io.Writer
	Err io.Writer
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
	// 分かれた点にあって今ないもの。今の項目とファイルを回すだけでは見つからないので、別に持つ
	goneItems []gone
	goneFiles []string
}

// gone は、分かれた点にあって今ない約束・用語・機能。file は分かれた点でその項目があった約束リストのファイル
type gone struct {
	kind, id, file string
}

// Run は、審査を設定の順に行い、審査ごとの結果の行を o.Out に、誤りの中身を o.Err に出す。審査を始める前の誤りは error で返す。すべて通れば true。
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
			fmt.Fprintln(o.Err, err.Error())
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
		return fmt.Errorf("リポジトリのファイルの一覧を作れません（%v）。ファイルを読む権限を確かめてください。", err)
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
		return fmt.Errorf(`審査の設定に scope = "story" の審査（%s）があるため、ベースブランチが必要です。--base <ブランチ> を渡してください。`, strings.Join(storyChecks, "・"))
	}
	mb, err := repo.Git(e.root, "merge-base", o.Base, "HEAD")
	if err != nil {
		return fmt.Errorf("ベースブランチ %s から分かれた点が分かりません（%v）。--base に、今のブランチと履歴のつながったブランチを渡してください。", o.Base, err)
	}
	e.mergeBase = strings.TrimSpace(mb)
	if len(needStory) > 0 {
		if err := e.decideStory(o.Story, needStory); err != nil {
			return err
		}
	}
	return e.findChanges()
}

// decideStory は、範囲 story の審査のストーリーを決める。--story（given）を渡しても、分かれた点より後のコミットのトレーラーの誤りは、
// 審査を始める前の誤りにする（--story はストーリーを選ぶためのもので、トレーラーの確かめを飛ばすためのものではない。手元と CI で結果をそろえるため）
func (e *env) decideStory(given string, checks []string) error {
	if given != "" && (e.cfg.IDFull == nil || !e.cfg.IDFull.MatchString(given)) {
		return fmt.Errorf("--story %s が、ストーリーの ID の書き方（%s）に合いません。設定の id_pattern に合う ID を渡してください。", given, e.cfg.IDPattern)
	}
	commits, err := e.cfg.CommitStories(e.root, e.mergeBase+"..HEAD")
	if err != nil {
		return fmt.Errorf("コミットのメッセージを読めません（%v）。", err)
	}
	seen := map[string]bool{}
	var ids []string
	for _, c := range commits {
		if c.Err != nil {
			return fmt.Errorf("コミット %s「%s」の %v。コミットのメッセージの末尾の %s: を直してください。", short(c.Hash), c.Subject, c.Err, config.StoryTrailerKey)
		}
		if c.Story != "" && !seen[c.Story] {
			seen[c.Story] = true
			ids = append(ids, c.Story)
		}
	}
	if given != "" {
		e.story = given
		return nil
	}
	sort.Strings(ids)
	switch len(ids) {
	case 1:
		e.story = ids[0]
		return nil
	case 0:
		return fmt.Errorf("範囲 story の審査（%s）のストーリーが分かりません（分かれた点より後のコミットに、Story: のトレーラーがありません）。--story <ID> を渡してください。", strings.Join(checks, "・"))
	default:
		return fmt.Errorf("範囲 story の審査（%s）のストーリーが 1 つに決まりません（分かれた点より後のコミットの Story: のトレーラーに、ストーリーの ID が %s とあります）。--story <ID> で 1 つを渡してください。", strings.Join(checks, "・"), strings.Join(ids, "・"))
	}
}

// findChanges は、分かれた点の中身と作業ツリーの今の中身を比べ、変わった項目とファイルを求める。
func (e *env) findChanges() error {
	base, err := snapshot.At(e.root, e.cfg, e.mergeBase)
	if err != nil {
		return err
	}
	// 分かれた点の約束リストの誤りは、ここでは出さない。読めなかった項目は、増えたものとみなす（合意を求める側に倒す）
	baseFP := base.ItemFingerprints()
	e.changedItems = map[string]bool{}
	now := map[string]bool{}
	for _, it := range items(e.list) {
		now[it.key()] = true
		fp, err := agreements.ItemFingerprint(it.raw)
		if err != nil || baseFP[it.key()] != fp {
			e.changedItems[it.key()] = true
		}
	}
	for _, b := range snapshot.Entries(base.List) {
		if !now[b.Key()] {
			now[b.Key()] = true // 重なった ID を 2 度数えない
			e.goneItems = append(e.goneItems, gone{kind: b.Kind, id: b.ID, file: b.Table.Doc.File})
		}
	}
	e.changedFiles = map[string]bool{}
	cur := map[string]bool{}
	for _, f := range e.targetFiles() {
		cur[f] = true
	}
	for _, t := range base.Targets {
		if !cur[t.Path] {
			e.goneFiles = append(e.goneFiles, t.Path)
		}
	}
	for _, f := range e.targetFiles() {
		if !base.Has(f) {
			e.changedFiles[f] = true
			continue
		}
		b, err := base.Read(f)
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
	var out []string
	for _, t := range snapshot.Targets(e.files, e.cfg) {
		out = append(out, t.Path)
	}
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
	return []diag.Error{{What: "審査 " + s.Name + " はありません", Fix: "vanda の不具合です。開発者に知らせてください"}}
}

// cannot は、前提の読み込みに失敗したために行えない審査の、1 行の誤り。
func cannot(what string, n int, owner string) []diag.Error {
	return []diag.Error{{What: fmt.Sprintf("%sを読めないため、この審査を行えません（誤りが %d 件あります）", what, n), Fix: "審査 " + owner + " で誤りを確かめて、直してください"}}
}
