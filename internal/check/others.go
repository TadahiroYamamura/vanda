package check

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/TadahiroYamamura/vanda/internal/agreements"
	"github.com/TadahiroYamamura/vanda/internal/config"
	"github.com/TadahiroYamamura/vanda/internal/diag"
	"github.com/TadahiroYamamura/vanda/internal/glob"
	"github.com/TadahiroYamamura/vanda/internal/repo"
)

func matchPath(p, f string) bool { return glob.Match(p, f) }

// storyOrigin は、由来の審査。分かれた点より後の、合流でないコミットのうち、約束リストを変えたものに、ストーリーの ID があるかを確かめる。
func (e *env) storyOrigin() []diag.Error {
	out, err := repo.Git(e.root, "rev-list", "--no-merges", "--reverse", e.mergeBase+"..HEAD")
	if err != nil {
		return []diag.Error{{What: "コミットの一覧を読めません（" + err.Error() + "）", Fix: "git の状態を確かめてください"}}
	}
	var errs []diag.Error
	for _, c := range strings.Fields(out) {
		names, err := repo.Git(e.root, "diff-tree", "--no-commit-id", "--name-only", "-r", "-z", "--root", c)
		if err != nil {
			return append(errs, diag.Error{What: "コミット " + short(c) + " で変わったファイルを読めません（" + err.Error() + "）", Fix: "git の状態を確かめてください"})
		}
		var touched []string
		for _, f := range strings.Split(names, "\x00") {
			if f == "" {
				continue
			}
			for _, p := range e.cfg.Promises {
				if glob.Match(p, f) {
					touched = append(touched, f)
					break
				}
			}
		}
		if len(touched) == 0 {
			continue
		}
		msg, err := repo.Git(e.root, "log", "-1", "--format=%B", c)
		if err != nil {
			return append(errs, diag.Error{What: "コミット " + short(c) + " のメッセージを読めません（" + err.Error() + "）", Fix: "git の状態を確かめてください"})
		}
		subject := strings.SplitN(strings.TrimSpace(msg), "\n", 2)[0]
		id, terr := e.cfg.StoryTrailer(msg)
		switch {
		case terr != nil:
			errs = append(errs, diag.Error{What: fmt.Sprintf("コミット %s「%s」の %v", short(c), subject, terr), Fix: "そのコミットのメッセージの末尾の " + config.StoryTrailerKey + ": を直してください"})
		case id == "":
			errs = append(errs, diag.Error{What: fmt.Sprintf("コミット %s「%s」が約束リスト（%s）を変えていますが、メッセージの末尾に %s: <ストーリーの ID>（%s の形）のトレーラーがありません", short(c), subject, strings.Join(touched, "・"), config.StoryTrailerKey, e.cfg.IDPattern), Fix: "そのコミットのメッセージの末尾に、" + config.StoryTrailerKey + ": <ストーリーの ID> のトレーラーを書いてください"})
		}
	}
	return errs
}

func short(c string) string {
	if len(c) > 7 {
		return c[:7]
	}
	return c
}

// agreement は、合意の審査。今の中身が合意済で、フラグが残っていないことを確かめる。
func (e *env) agreement(scope string) []diag.Error {
	if e.listErrs != nil {
		return cannot("約束リスト", len(e.listErrs), "promise-list-shape")
	}
	if e.recErrs != nil {
		return cannot("合意の記録", len(e.recErrs), "agreement-targets")
	}
	errs := append([]diag.Error{}, e.guardErr...)
	inScope := func(key string, changed map[string]bool) bool {
		return scope == ScopeAll || changed[key]
	}
	// 範囲 story では、そのストーリーが前に合意したもの（外れた合意を探す先）も見る
	storyAgreed := map[string]bool{}
	if scope == ScopeStory {
		for _, a := range e.recs.Agreements {
			if a.Story == e.story {
				storyAgreed[a.Kind+":"+a.Key] = true
				if a.Kind == "file" {
					storyAgreed["path:"+a.Path] = true
				}
			}
		}
	}
	for _, it := range items(e.list) {
		changed := inScope(it.key(), e.changedItems)
		if !changed && !storyAgreed[it.key()] {
			continue
		}
		fp, err := agreements.ItemFingerprint(it.raw)
		if err != nil {
			errs = append(errs, it.table.Err("", kindName[it.kind]+" "+it.id+" の指紋を作れません（"+err.Error()+"）", "約束リストに書ける値（文字列・真偽・配列・表）だけにしてください"))
			continue
		}
		switch {
		case changed && !e.recs.Agreed(it.kind, it.id, fp):
			errs = append(errs, it.table.Err("id", kindName[it.kind]+" "+it.id+" の今の中身に、まだ合意していません（提案中。指紋 "+fp+"）", "中身を確かめてから合意し、合意の記録に書いてください"))
		case storyAgreed[it.key()] && !e.recs.AgreedBy(it.kind, it.id, fp, e.story):
			errs = append(errs, it.table.Err("id", kindName[it.kind]+" "+it.id+" の、ストーリー "+e.story+" の合意が外れています（前に合意した後に中身が変わり、今の中身 "+fp+" に "+e.story+" の合意がありません）", "今の中身を確かめ、ストーリー "+e.story+" で合意し直してください"))
		}
		if !changed {
			continue
		}
		if it.claim != nil {
			if len(it.claim.Flags) > 0 {
				errs = append(errs, it.table.Err("flags", "約束 "+it.id+" にフラグ（"+strings.Join(it.claim.Flags, "・")+"）が残っています", "合意するときに、フラグを外してください（合意の後にフラグだけを立てたなら、中身を確かめ直してから外してください）"))
			}
			if len(it.claim.Guesses) > 0 {
				errs = append(errs, it.table.Err("guesses", "約束 "+it.id+" に推測（guesses）が残っています", "推測を確かめ、合意するときに guesses を外してください"))
			}
		}
	}
	for _, f := range e.targetFiles() {
		changed := inScope(f, e.changedFiles)
		// ファイルの外れた合意は、合意の記録の path で今のファイルを探す（名前を変えたファイルは追わない）
		if !changed && !storyAgreed["path:"+f] {
			continue
		}
		src, err := os.ReadFile(filepath.Join(e.root, filepath.FromSlash(f)))
		if err != nil {
			errs = append(errs, diag.Error{File: f, What: "ファイルを読めません（" + err.Error() + "）", Fix: "ファイルを読む権限を確かめてください"})
			continue
		}
		fp := agreements.FileFingerprint(src)
		switch {
		case changed && !e.recs.Agreed("file", f, fp):
			errs = append(errs, diag.Error{File: f, What: "ファイルの今の中身に、まだ合意していません（提案中。指紋 " + fp + "）", Fix: "中身を確かめてから合意し、合意の記録に書いてください"})
		case storyAgreed["path:"+f] && !e.recs.AgreedBy("file", f, fp, e.story):
			errs = append(errs, diag.Error{File: f, What: "ストーリー " + e.story + " の合意が外れています（前に合意した後に中身が変わり、今の中身 " + fp + " に " + e.story + " の合意がありません）", Fix: "今の中身を確かめ、ストーリー " + e.story + " で合意し直してください"})
		}
	}
	return errs
}

// agreementTargets は、合意の記録の指す先の審査。
func (e *env) agreementTargets() []diag.Error {
	errs := append([]diag.Error{}, e.recErrs...)
	if e.recs == nil {
		return errs
	}
	if e.listErrs != nil {
		return append(errs, cannot("約束リスト", len(e.listErrs), "promise-list-shape")...)
	}
	have := map[string]bool{}
	for _, it := range items(e.list) {
		have[it.key()] = true
	}
	for _, a := range e.recs.Agreements {
		if a.Kind == "file" || a.Kind == "" || a.Key == "" {
			continue
		}
		if !have[a.Kind+":"+a.Key] {
			errs = append(errs, a.Table.Err("key", "合意の記録が、約束リストにない"+kindName[a.Kind]+" "+quote(a.Key)+" を指しています", "約束リストに戻すか、この合意の記録を消してください"))
		}
	}
	return errs
}

// evidencePresent は、エビデンスが揃っているかの審査。
func (e *env) evidencePresent(scope string) []diag.Error {
	if e.listErrs != nil {
		return cannot("約束リスト", len(e.listErrs), "promise-list-shape")
	}
	if e.annErrs != nil {
		return cannot("アノテーション", len(e.annErrs), "annotation-links")
	}
	var errs []diag.Error
	for _, c := range e.list.Claims {
		if scope == ScopeStory && !e.changedItems["claim:"+c.ID] {
			continue
		}
		for _, ev := range c.Evidence {
			switch ev.Type {
			case "proof":
				errs = append(errs, ev.Table.Err("type", "タイプ proof は、まだ扱えません（対応するアダプタがありません）", "別のタイプのエビデンスにしてください"))
				continue
			case "human-review":
				continue
			}
			kind := config.EvidenceKind(ev.Type)
			if kind == "" {
				continue // 決まった値でないタイプは、約束リストの形の審査で出す
			}
			found := false
			for _, a := range e.anns {
				if a.Satisfies(c.ID, ev.Type) {
					found = true
					break
				}
			}
			if !found {
				errs = append(errs, ev.Table.Err("type", fmt.Sprintf("約束 %s の %s のエビデンスがありません（types に %s を含む [[annotation]] で読んだファイルに、@vanda:%s %s がありません）", c.ID, ev.Type, ev.Type, kind, c.ID), fmt.Sprintf("エビデンスを作り、@vanda:%s %s を付けてください。そのファイルが、types に %s を含む [[annotation]] のパターンに当たるようにしてください", kind, c.ID, ev.Type)))
			}
		}
	}
	return errs
}

// unreadFiles は、.vanda/ にあるのに、設定のどのパターンにも当たらないファイルを見つける。
func (e *env) unreadFiles() []diag.Error {
	var errs []diag.Error
	for _, f := range e.files {
		// .vanda/adapters/ の下のアダプタは、設定の adapter の名前で動かすもので、パターンで読むものではない
		if !strings.HasPrefix(f, ".vanda/") || f == config.File || glob.Match(".vanda/checks/*.toml", f) || strings.HasPrefix(f, ".vanda/adapters/") {
			continue
		}
		read := false
		for _, p := range append(append([]string{}, e.cfg.Promises...), e.cfg.Agreements...) {
			if glob.Match(p, f) {
				read = true
				break
			}
		}
		if !read {
			errs = append(errs, diag.Error{File: f, What: ".vanda/ にありますが、設定のどのパターンにも当たらないため、読まれていません", Fix: ".vanda/config.toml の promises か agreements のパターンに当たるようにするか、ファイルを消してください"})
		}
	}
	return errs
}
