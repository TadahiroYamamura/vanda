package check

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/TadahiroYamamura/vanda/internal/agreements"
	"github.com/TadahiroYamamura/vanda/internal/annotation"
	"github.com/TadahiroYamamura/vanda/internal/config"
	"github.com/TadahiroYamamura/vanda/internal/diag"
	"github.com/TadahiroYamamura/vanda/internal/guardrail"
	"github.com/TadahiroYamamura/vanda/internal/promises"
	"github.com/TadahiroYamamura/vanda/internal/repo"
	"github.com/TadahiroYamamura/vanda/internal/snapshot"
	"github.com/TadahiroYamamura/vanda/internal/tomlread"
)

type Options struct {
	Root string
	// Tree は、読むコミット（HEAD）。Cfg もこのコミットから読む
	Tree  *repo.Tree
	Cfg   *config.Config
	Specs []Spec
	Base  string
	Story string
	// Out にはチェックごとの結果の行を、Err には誤りの中身を出す
	Out io.Writer
	Err io.Writer
}

// env は、チェックに使う、読み込んだもの。
type env struct {
	root  string
	cfg   *config.Config
	files []string
	read  tomlread.Reader

	list     *promises.List
	listErrs diag.Errors
	recs     *agreements.Records
	recErrs  diag.Errors
	anns     []annotation.Annotation
	annErrs  diag.Errors
	guards   []string
	guardErr []diag.Error

	// スコープ story のチェックのためのもの
	mergeBase    string
	story        string
	changedItems map[string]bool // "<種類>:<ID>"
	changedFiles map[string]bool
	// マージベースにあって今ないもの。今の項目とファイルを回すだけでは見つからないので、別に持つ
	goneItems []gone
	goneFiles []string
}

// gone は、マージベースにあって今ない約束・用語・機能。file はマージベースでその項目があった約束リストのファイル
type gone struct {
	kind, id, file string
}

// Run は、チェックを設定の順に行い、チェックごとの結果の行を o.Out に、誤りの中身を o.Err に出す。チェックを始める前の誤りは error で返す。すべて通れば true。
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
			fmt.Fprintf(o.Out, "チェック %s（%s）: 通った\n", s.Name, scope)
			continue
		}
		pass = false
		fmt.Fprintf(o.Out, "チェック %s（%s）: 失敗（%d 件）\n", s.Name, scope, len(errs))
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
	// --story の形は、どのチェックがあるかによらず確かめる（使わない設定でも、誤った値を黙って受け取らないため）
	if o.Story != "" && (e.cfg.IDFull == nil || !e.cfg.IDFull.MatchString(o.Story)) {
		return fmt.Errorf("--story %s が、ストーリーの ID の書き方（%s）に合いません。設定の id_pattern に合う ID を渡してください。", o.Story, e.cfg.IDPattern)
	}
	t := o.Tree
	if t == nil {
		var err error
		if t, err = repo.HeadTree(e.root); err != nil {
			return err
		}
	}
	files := t.Files
	e.files = files
	e.read = t.Read
	e.list, e.listErrs = promises.LoadWith(t.Read, files, e.cfg.Promises, e.cfg.PromisesDefault)
	e.recs, e.recErrs = agreements.LoadWith(t.Read, files, e.cfg.Agreements)
	e.anns, e.annErrs = annotation.ReadWith(e.root, t.Read, files, e.cfg.Annotations)
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
		return fmt.Errorf(`チェックの設定に scope = "story" のチェック（%s）があるため、ベースブランチが必要です。--base <ブランチ> を渡してください。`, strings.Join(storyChecks, "・"))
	}
	mb, err := repo.Git(e.root, "merge-base", o.Base, "HEAD")
	if err != nil {
		return fmt.Errorf("ベースブランチ %s とのマージベースが分かりません（%v）。--base に、今のブランチと履歴のつながったブランチを渡してください。", o.Base, err)
	}
	e.mergeBase = strings.TrimSpace(mb)
	if len(needStory) > 0 {
		if err := e.decideStory(o.Story, needStory); err != nil {
			return err
		}
	}
	return e.findChanges()
}

// decideStory は、スコープ story のチェックのストーリーを決める。--story（given）を渡しても、マージベースより後のコミットのトレーラーの誤りは、
// チェックを始める前の誤りにする（--story はストーリーを選ぶためのもので、トレーラーの確かめを飛ばすためのものではない。手元と CI で結果をそろえるため）
func (e *env) decideStory(given string, checks []string) error {
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
	sort.Strings(ids)
	if len(ids) == 1 && given != "" && given != ids[0] {
		return fmt.Errorf("--story %s が、マージベースより後のコミットの Story: のトレーラーのストーリー %s と違います（トレーラーと違うストーリーでチェックすると、%s の失効した承認を見ないまま通るため）。--story を外すか、--story %s を渡してください。", given, ids[0], ids[0], ids[0])
	}
	if given != "" {
		e.story = given
		return nil
	}
	switch len(ids) {
	case 1:
		e.story = ids[0]
		return nil
	case 0:
		return fmt.Errorf("スコープ story のチェック（%s）のストーリーが分かりません（マージベースより後のコミットに、Story: のトレーラーがありません）。--story <ID> を渡してください。", strings.Join(checks, "・"))
	default:
		return fmt.Errorf("スコープ story のチェック（%s）のストーリーが 1 つに決まりません（マージベースより後のコミットの Story: のトレーラーに、ストーリーの ID が %s とあります）。--story <ID> で 1 つを渡してください。", strings.Join(checks, "・"), strings.Join(ids, "・"))
	}
}

// findChanges は、マージベースの中身と今の中身（HEAD）を比べ、変わった項目とファイルを求める。
func (e *env) findChanges() error {
	base, err := snapshot.At(e.root, e.cfg, e.mergeBase)
	if err != nil {
		return err
	}
	// マージベースの約束リストの誤りは、ここでは出さない。読めなかった項目は、増えたものとみなす（承認を求める側に倒す）
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
			now[b.Key()] = true // 重複した ID を 2 度数えない
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
			return fmt.Errorf("マージベースの %s を読めない（%v）", f, err)
		}
		cur, err := e.read(f)
		if err != nil || agreements.FileFingerprint(cur) != agreements.FileFingerprint(b) {
			e.changedFiles[f] = true
		}
	}
	return nil
}

// targetFiles は、承認の対象になるファイル（シナリオ、オラクルの定義、ガードレールの定義）を、場所の順で返す。
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
	return []diag.Error{{What: "チェック " + s.Name + " はありません", Fix: "vanda の不具合です。開発者に知らせてください"}}
}

// cannot は、前提の読み込みに失敗したために行えないチェックの、1 行の誤り。
func cannot(what string, n int, owner string) []diag.Error {
	return []diag.Error{{What: fmt.Sprintf("%sを読めないため、このチェックはできません（誤りが %d 件あります）", what, n), Fix: "チェック " + owner + " で誤りを確かめて、直してください"}}
}
