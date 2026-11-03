package review

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/TadahiroYamamura/vanda/internal/config"
	"github.com/TadahiroYamamura/vanda/internal/glob"
	"github.com/TadahiroYamamura/vanda/internal/promises"
	"github.com/TadahiroYamamura/vanda/internal/repo"
	"github.com/TadahiroYamamura/vanda/internal/tomlwrite"
)

type AgreeRequest struct {
	Kind        string  `json:"kind"`
	ID          *string `json:"id"`
	Path        *string `json:"path"`
	Fingerprint string  `json:"fingerprint"`
	Story       string  `json:"story"`
	// Deleted は、消したことに合意するか。真なら fingerprint は見ない
	Deleted bool `json:"deleted"`
}

type AgreeResult struct {
	Commit    string          `json:"commit"`
	Agreement AgreementRecord `json:"agreement"`
}

type AgreementRecord struct {
	Kind string `json:"kind"`
	Key  string `json:"key"`
	// Fingerprint は、消したことへの合意では nil
	Fingerprint *string `json:"fingerprint"`
	Deleted     bool    `json:"deleted"`
	Path        *string `json:"path"`
	User        User    `json:"user"`
	AgreedAt    string  `json:"agreedAt"`
	Story       string  `json:"story"`
}

// target は、記録の指す先（約束・用語・機能なら ID、ファイルなら場所）。agreements.Agreement の Target と同じ決め方。
func (rec AgreementRecord) target() string {
	if rec.Path != nil {
		return *rec.Path
	}
	return rec.Key
}

func conflict(msg, fix string) *Error {
	return &Error{Status: http.StatusConflict, Message: msg, Fix: fix}
}

func badRequest(msg, fix string) *Error {
	return &Error{Status: http.StatusBadRequest, Message: msg, Fix: fix}
}

// Agree は、1 件に合意する（RV-06）。合意の記録を書き足し、約束ならフラグを外し、その 2 つのファイルだけを 1 つのコミットにする。
func Agree(o Options, req AgreeRequest, now time.Time) (*AgreeResult, *Error) {
	s := &state{o: o, h: newHistory(o.Root, o.Cfg)}
	if e := s.checkStory(req.Story); e != nil {
		return nil, e
	}
	user := GitUser(o.Root)
	if user == nil {
		return nil, badRequest("git の user.name か user.email が設定されていないため、誰が合意したかを記録できません。", "git config user.name と git config user.email を設定してください。")
	}
	if e := s.loadCurrent(); e != nil {
		return nil, e
	}

	rec := AgreementRecord{Kind: req.Kind, User: *user, AgreedAt: now.Format(time.RFC3339), Story: req.Story}
	var target string // 一覧の名前（コミットのメッセージに書く）
	var claim *promises.Claim
	var source string // 合意の対象が書いてあるファイル。消した約束・用語・機能では空
	var fp string
	switch {
	case req.Deleted:
		key, t, e := s.deletedTarget(req.Kind, req.ID, req.Path, "合意")
		if e != nil {
			return nil, e
		}
		rec.Key, rec.Deleted, target = key, true, t
		if req.Kind == "file" {
			rec.Path, source = &key, key
		}
	case req.Kind == "claim" || req.Kind == "term" || req.Kind == "feature":
		if req.ID == nil || *req.ID == "" {
			return nil, badRequest(kindName[req.Kind]+"の合意に id がありません。", "id に ID を書いてください。")
		}
		key := req.Kind + ":" + *req.ID
		cur, ok := s.cur.items[key]
		if !ok {
			return nil, &Error{Status: http.StatusNotFound, Message: kindName[req.Kind] + " " + *req.ID + " は約束リストにありません。", Fix: "一覧を読み直してください。"}
		}
		rec.Key, fp = cur.ID, s.cur.itemFP[key]
		if fp == "" {
			return nil, conflict(kindName[req.Kind]+" "+cur.ID+" の指紋を作れません。", "vanda check の promise-list-shape で誤りを確かめて、直してください。")
		}
		target = kindName[req.Kind] + " " + cur.ID
		claim = cur.Claim
		source = cur.Table.Doc.File
	case req.Kind == "file":
		if req.Path == nil || *req.Path == "" {
			return nil, badRequest("ファイルの合意に path がありません。", "path にファイルの場所を書いてください。")
		}
		p := *req.Path
		if _, ok := s.cur.types[p]; !ok {
			return nil, &Error{Status: http.StatusNotFound, Message: p + " は合意の対象のファイルではありません。", Fix: "一覧を読み直してください。"}
		}
		var err error
		if fp, err = s.cur.fileFP(p); err != nil {
			return nil, internalErr(err)
		}
		rec.Key, rec.Path = fp, &p
		target = "ファイル " + p
		source = p
	default:
		return nil, badRequest(fmt.Sprintf("対象の種類 %q はありません。", req.Kind), "kind を claim・term・feature・file のどれかにしてください。")
	}

	agreementsFile, create, e := s.agreementsFile()
	if e != nil {
		return nil, e
	}
	files := []string{agreementsFile}
	var promiseFile string
	if claim != nil && (claim.Table.Has("flags") || claim.Table.Has("guesses")) {
		promiseFile = claim.Table.Doc.File
		files = append(files, promiseFile)
	}
	// 合意の対象のファイルにコミットしていない変更があると、合意のコミットの時点のリポジトリが、合意した中身と合わなくなる
	sources := []string{source}
	if source == "" {
		// 消した約束・用語・機能には書いてあるファイルがないので、約束リストのどのファイル（消したファイルを含む）にも、
		// コミットしていない変更がないことを求める
		dirty, err := dirtyMatching(o.Root, o.Cfg.Promises)
		if err != nil {
			return nil, internalErr(err)
		}
		if len(dirty) > 0 {
			return nil, conflict("約束リスト（"+strings.Join(dirty, "・")+"）に、コミットしていない変更があります（このまま合意すると、消したことと、合意のコミットの時点のリポジトリが合わなくなります）。", "先に約束リストの変更をコミットしてから、合意してください。")
		}
		sources, _ = glob.Select(s.cur.snap.Files, o.Cfg.Promises)
	} else {
		dirty, err := repo.Git(o.Root, "status", "--porcelain", "-z", "--", source)
		if err != nil {
			return nil, internalErr(err)
		}
		if dirty != "" {
			return nil, conflict(target+" が書いてある "+source+" に、コミットしていない変更があります（このまま合意すると、合意した中身と、合意のコミットの時点のリポジトリが合わなくなります）。", "先に "+source+" の変更をコミットしてから、合意してください。")
		}
	}
	dirty, err := repo.Git(o.Root, append([]string{"status", "--porcelain", "-z", "--"}, files...)...)
	if err != nil {
		return nil, internalErr(err)
	}
	if dirty != "" {
		return nil, conflict("合意で変えるファイル（"+strings.Join(files, "・")+"）に、コミットしていない変更があります。", "そのファイルの変更をコミットするか元に戻してから、合意してください。")
	}

	// 書きかけの確かめを指紋の比べより先にする。今の中身は HEAD のコミットなので、書きかけがあると指紋の違いで断ることになり、先にコミットするよう知らせられないため
	if req.Deleted {
		if s.recs.DeletionAgreedBy(rec.Kind, rec.target(), req.Story) {
			return nil, conflict(target+" を消したことには、ストーリー "+req.Story+" ですでに合意しています。", "一覧を読み直してください。")
		}
	} else {
		rec.Fingerprint = &fp
		if fp != req.Fingerprint {
			return nil, conflict(target+" の中身が、画面に表示した後に変わりました（画面 "+req.Fingerprint+"、今 "+fp+"）。", "一覧を読み直し、今の中身を確かめてから合意してください。")
		}
		if s.recs.AgreedBy(rec.Kind, rec.target(), fp, req.Story) {
			return nil, conflict(target+" の今の中身には、ストーリー "+req.Story+" ですでに合意しています。", "一覧を読み直してください。")
		}
	}

	// 書く前の中身を持っておき、途中で失敗したら戻す
	originals := map[string][]byte{}
	for _, f := range files {
		if f == agreementsFile && create {
			continue
		}
		b, err := os.ReadFile(filepath.Join(o.Root, filepath.FromSlash(f)))
		if err != nil {
			return nil, internalErr(err)
		}
		originals[f] = b
	}
	restore := func() error {
		var errs []error
		for _, f := range files {
			abs := filepath.Join(o.Root, filepath.FromSlash(f))
			if b, ok := originals[f]; ok {
				errs = append(errs, os.WriteFile(abs, b, 0o644))
			} else {
				if _, err := repo.Git(o.Root, "rm", "-q", "--cached", "--ignore-unmatch", "--", f); err != nil {
					errs = append(errs, err)
				}
				errs = append(errs, os.Remove(abs))
			}
		}
		return errors.Join(errs...)
	}
	fail := func(err error) *Error {
		if rerr := restore(); rerr != nil {
			return &Error{Status: http.StatusInternalServerError, Message: fmt.Sprintf("合意を書けず（%v）、ファイルを元に戻すこともできませんでした（%v）。", err, rerr), Fix: "git status で " + strings.Join(files, "・") + " を確かめて、直してください。"}
		}
		return &Error{Status: http.StatusInternalServerError, Message: fmt.Sprintf("合意を書けませんでした（%v）。ファイルは元に戻しました。", err), Fix: "git の状態を確かめてください。"}
	}

	if promiseFile != "" {
		out, err := removeFlags(originals[promiseFile], claim)
		if err != nil {
			return nil, conflict(err.Error(), "flags と guesses を [[claim]] の表の行として書き直すか、手で消してから合意してください。")
		}
		if err := os.WriteFile(filepath.Join(o.Root, filepath.FromSlash(promiseFile)), out, 0o644); err != nil {
			return nil, fail(err)
		}
	}
	abs := filepath.Join(o.Root, filepath.FromSlash(agreementsFile))
	if create {
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			return nil, fail(err)
		}
	}
	if err := os.WriteFile(abs, appendRecord(originals[agreementsFile], rec), 0o644); err != nil {
		return nil, fail(err)
	}
	if Hooks.AfterAgreeWrite != nil {
		Hooks.AfterAgreeWrite(o.Root)
	}
	if err := s.verify(rec, claim); err != nil {
		return nil, fail(err)
	}
	if create {
		if _, err := repo.Git(o.Root, "add", "--", agreementsFile); err != nil {
			return nil, fail(err)
		}
	}
	msg := fmt.Sprintf("(chore) %s で %s に合意する", req.Story, target)
	if req.Deleted {
		msg = fmt.Sprintf("(chore) %s で %s を消したことに合意する", req.Story, target)
	}
	// パスを渡すと、索引に足してあるほかのファイルの変更を入れず、渡したファイルだけをコミットする
	if _, err := repo.Git(o.Root, append([]string{"commit", "-q", "-m", msg, "-m", config.StoryTrailerKey + ": " + req.Story, "--"}, files...)...); err != nil {
		return nil, fail(err)
	}
	head, err := repo.Git(o.Root, "rev-parse", "HEAD")
	if err != nil {
		return nil, internalErr(err)
	}
	if Hooks.AfterAgreeCommit != nil {
		Hooks.AfterAgreeCommit(o.Root)
	}
	if err := checkAgreeCommit(o.Root, files, sources); err != nil {
		return nil, sensorErr("合意", strings.TrimSpace(head), err)
	}
	return &AgreeResult{Commit: strings.TrimSpace(head), Agreement: rec}, nil
}

// agreementsFile は、合意の記録を書き足すファイルを返す。設定の agreements のパターンの順で、最初に当たったファイル。
// どれにも当たらず、最初のパターンにワイルドカードがなければ、そのファイルを作る（create が真）。
func (s *state) agreementsFile() (string, bool, *Error) {
	for _, p := range s.o.Cfg.Agreements {
		for _, f := range s.cur.snap.Files {
			if glob.Match(p, f) {
				return f, false, nil
			}
		}
	}
	first := s.o.Cfg.Agreements[0]
	if glob.HasMeta(first) {
		return "", false, conflict("合意の記録のファイルがなく、設定の agreements の最初のパターン "+first+" にワイルドカードがあるため、どこに作るかを決められません。", "合意の記録のファイルを作ってから、合意してください。")
	}
	return first, true, nil
}

// removeFlags は、約束の表から flags と guesses の行（複数行の配列なら、その全部の行）を消す。ほかの行は変えない。
func removeFlags(src []byte, claim *promises.Claim) ([]byte, error) {
	lines := strings.SplitAfter(string(src), "\n")
	drop := map[int]bool{}
	for _, key := range []string{"flags", "guesses"} {
		if !claim.Table.Has(key) {
			continue
		}
		start := claim.Table.Line(key)
		if start == 0 || !startsWithKey(lines[start-1], key) {
			return nil, fmt.Errorf("%s: 約束 %s の %s がインラインの表の中にあるため、消せません。", claim.Table.Doc.File, claim.ID, key)
		}
		end, err := valueEnd(lines, start-1)
		if err != nil {
			return nil, fmt.Errorf("%s:%d: 約束 %s の %s の終わりが分かりません（%v）。", claim.Table.Doc.File, start, claim.ID, key, err)
		}
		for i := start - 1; i <= end; i++ {
			drop[i] = true
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

// startsWithKey は、行が key の項目（key = …。キーを引用符で囲んだ "key" = … と 'key' = … を含む）から始まるかを返す。
// インラインの表の中に書いた項目は、行が key で始まらない
func startsWithKey(line, key string) bool {
	t := strings.TrimLeft(line, " \t")
	for _, k := range []string{key, `"` + key + `"`, "'" + key + "'"} {
		if rest, ok := strings.CutPrefix(t, k); ok && strings.HasPrefix(strings.TrimLeft(rest, " \t"), "=") {
			return true
		}
	}
	return false
}

// valueEnd は、行 i（0 から数える）の `key = [ … ]` の値が終わる行を返す。文字列とコメントの中の括弧は数えない。
func valueEnd(lines []string, i int) (int, error) {
	depth := 0
	started := false
	inValue := false
	for n := i; n < len(lines); n++ {
		l := lines[n]
		for j := 0; j < len(l); j++ {
			c := l[j]
			if !inValue {
				if c == '=' {
					inValue = true
				}
				continue
			}
			switch {
			case strings.HasPrefix(l[j:], `"""`) || strings.HasPrefix(l[j:], `'''`):
				return 0, errors.New("複数行の文字列は扱えません")
			case c == '"':
				j++
				for j < len(l) && l[j] != '"' {
					if l[j] == '\\' {
						j++
					}
					j++
				}
			case c == '\'':
				j++
				for j < len(l) && l[j] != '\'' {
					j++
				}
			case c == '#':
				j = len(l)
			case c == '[':
				depth++
				started = true
			case c == ']':
				depth--
			}
		}
		if started && depth == 0 {
			return n, nil
		}
	}
	return 0, errors.New("配列が閉じていません")
}

// appendRecord は、合意の記録のファイルの中身 src の終わりに、1 件の [[agreement]] を書き足す。
func appendRecord(src []byte, rec AgreementRecord) []byte {
	var b strings.Builder
	b.Write(src)
	if len(src) > 0 {
		if src[len(src)-1] != '\n' {
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	b.WriteString("[[agreement]]\n")
	fmt.Fprintf(&b, "kind = %s\n", tomlwrite.String(rec.Kind))
	fmt.Fprintf(&b, "key = %s\n", tomlwrite.String(rec.Key))
	if rec.Deleted {
		b.WriteString("deleted = true\n")
	} else if rec.Fingerprint != nil {
		fmt.Fprintf(&b, "fingerprint = %s\n", tomlwrite.String(*rec.Fingerprint))
	}
	if rec.Path != nil {
		fmt.Fprintf(&b, "path = %s\n", tomlwrite.String(*rec.Path))
	}
	fmt.Fprintf(&b, "user = { name = %s, email = %s }\n", tomlwrite.String(rec.User.Name), tomlwrite.String(rec.User.Email))
	fmt.Fprintf(&b, "agreed_at = %s\n", rec.AgreedAt)
	fmt.Fprintf(&b, "story = %s\n", tomlwrite.String(rec.Story))
	return []byte(b.String())
}

// dirtyMatching は、patterns に当たるファイルのうち、コミットしていない変更（消したもの・索引に足したものを含む）のあるものを返す。
func dirtyMatching(root string, patterns []string) ([]string, error) {
	out, err := repo.Git(root, "status", "--porcelain", "-z", "--untracked-files=all")
	if err != nil {
		return nil, err
	}
	var dirty []string
	fields := strings.Split(out, "\x00")
	for i := 0; i < len(fields); i++ {
		f := fields[i]
		if len(f) < 4 {
			continue
		}
		paths := []string{f[3:]}
		// 名前の変更と写しは、次の欄に元の場所が続く
		if f[0] == 'R' || f[0] == 'C' {
			if i+1 < len(fields) {
				paths = append(paths, fields[i+1])
			}
			i++
		}
		for _, p := range paths {
			for _, pat := range patterns {
				if glob.Match(pat, p) {
					dirty = append(dirty, p)
					break
				}
			}
		}
	}
	return dirty, nil
}

// Agreeable は、1 件に、今の人がこのストーリーで合意できるか。合意済でも、このストーリーの合意がなければ合意できる
// （ストーリー s の合意が外れた後に、ほかのストーリーが今の中身に合意していても、s が合意し直すまで外れたままのため）。
type Agreeable struct {
	Possible bool `json:"possible"`
	// Reason は、合意できないときの理由。合意できるときは nil
	Reason *string `json:"reason"`
}

// setAgreeables は、各件に、合意できるかを付ける。判定は Agree が断る条件と合わせる。
func (s *state) setAgreeables(es []Entry) {
	story := s.o.Story
	user := GitUser(s.o.Root)
	for i := range es {
		e := &es[i]
		no := func(reason string) { e.Agree = &Agreeable{Reason: ptr(reason)} }
		target := ""
		if e.Kind == "file" {
			target = *e.Path
		} else {
			target = *e.ID
		}
		switch {
		case story == "":
			no("ストーリーを選ぶと、合意できます。")
		case user == nil:
			no("git の user.name と user.email を設定すると、合意できます。")
		case e.Change == "deleted":
			if s.recs.DeletionAgreedBy(e.Kind, target, story) {
				no("消したことには、ストーリー " + story + " ですでに合意しています。")
			} else {
				e.Agree = &Agreeable{Possible: true}
			}
		case e.Fingerprint == nil:
			no("指紋を作れないため、合意できません。vanda check の promise-list-shape で誤りを確かめて、直してください。")
		case s.recs.AgreedBy(e.Kind, target, *e.Fingerprint, story):
			no("今の中身には、ストーリー " + story + " ですでに合意しています。")
		default:
			e.Agree = &Agreeable{Possible: true}
		}
	}
}
