package review

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"time"

	"github.com/TadahiroYamamura/vanda/internal/agreements"
	"github.com/TadahiroYamamura/vanda/internal/glob"
	"github.com/TadahiroYamamura/vanda/internal/promises"
	"github.com/TadahiroYamamura/vanda/internal/repo"
	"github.com/TadahiroYamamura/vanda/internal/snapshot"
)

type AgreeRequest struct {
	Kind        string  `json:"kind"`
	ID          *string `json:"id"`
	Path        *string `json:"path"`
	Fingerprint string  `json:"fingerprint"`
	Story       string  `json:"story"`
}

type AgreeResult struct {
	Commit    string          `json:"commit"`
	Agreement AgreementRecord `json:"agreement"`
}

type AgreementRecord struct {
	Kind        string  `json:"kind"`
	Key         string  `json:"key"`
	Fingerprint string  `json:"fingerprint"`
	Path        *string `json:"path"`
	User        User    `json:"user"`
	AgreedAt    string  `json:"agreedAt"`
	Story       string  `json:"story"`
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
		return nil, badRequest("git の user.name か user.email がないので、誰が合意したかを記録できない", "git config user.name と git config user.email を設定する")
	}
	if e := s.loadCurrent(); e != nil {
		return nil, e
	}

	rec := AgreementRecord{Kind: req.Kind, User: *user, AgreedAt: now.Format(time.RFC3339), Story: req.Story}
	var target string // 一覧の名前（コミットのメッセージに書く）
	var claim *promises.Claim
	switch req.Kind {
	case "claim", "term", "feature":
		if req.ID == nil || *req.ID == "" {
			return nil, badRequest(kindName[req.Kind]+"の合意に id がない", "id に ID を書く")
		}
		key := req.Kind + ":" + *req.ID
		cur, ok := s.cur.items[key]
		if !ok {
			return nil, &Error{Status: http.StatusNotFound, Message: kindName[req.Kind] + " " + *req.ID + " が約束リストにない", Fix: "一覧を読み直す"}
		}
		rec.Key, rec.Fingerprint = cur.ID, s.cur.itemFP[key]
		if rec.Fingerprint == "" {
			return nil, conflict(kindName[req.Kind]+" "+cur.ID+" の指紋を作れない", "vanda check の promise-list-shape で誤りを出して直す")
		}
		target = kindName[req.Kind] + " " + cur.ID
		claim = cur.Claim
	case "file":
		if req.Path == nil || *req.Path == "" {
			return nil, badRequest("ファイルの合意に path がない", "path にファイルの場所を書く")
		}
		p := *req.Path
		if _, ok := s.cur.types[p]; !ok {
			return nil, &Error{Status: http.StatusNotFound, Message: p + " は合意の対象のファイルでない", Fix: "一覧を読み直す"}
		}
		fp, err := s.cur.fileFP(p)
		if err != nil {
			return nil, internalErr(err)
		}
		rec.Key, rec.Fingerprint, rec.Path = fp, fp, &p
		target = "ファイル " + p
	default:
		return nil, badRequest(fmt.Sprintf("対象の種類 %q はない", req.Kind), "kind を claim・term・feature・file のどれかにする")
	}
	if rec.Fingerprint != req.Fingerprint {
		return nil, conflict(target+" の中身が、画面に出した後に変わった（画面 "+req.Fingerprint+"、今 "+rec.Fingerprint+"）", "一覧を読み直し、今の中身を確かめてから合意する")
	}
	if s.recs.AgreedBy(rec.Kind, rec.Key, rec.Fingerprint, req.Story) {
		return nil, conflict(target+" の今の中身には、ストーリー "+req.Story+" でもう合意している", "一覧を読み直す")
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
	dirty, err := repo.Git(o.Root, append([]string{"status", "--porcelain", "-z", "--"}, files...)...)
	if err != nil {
		return nil, internalErr(err)
	}
	if dirty != "" {
		return nil, conflict("合意で変えるファイル（"+strings.Join(files, "・")+"）に、コミットしていない変更がある", "そのファイルの変更をコミットするか、元に戻してから合意する")
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
			return &Error{Status: http.StatusInternalServerError, Message: fmt.Sprintf("合意を書けず（%v）、元に戻すこともできなかった（%v）", err, rerr), Fix: "git status で、" + strings.Join(files, "・") + " を確かめて直す"}
		}
		return &Error{Status: http.StatusInternalServerError, Message: fmt.Sprintf("合意を書けなかった（%v）。ファイルは元に戻した", err), Fix: "git の状態を確かめる"}
	}

	if promiseFile != "" {
		out, err := removeFlags(originals[promiseFile], claim)
		if err != nil {
			return nil, conflict(err.Error(), "flags と guesses を [[claim]] の表の行として書き直すか、手で消してから合意する")
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
	if err := s.verify(rec, claim); err != nil {
		return nil, fail(err)
	}
	if create {
		if _, err := repo.Git(o.Root, "add", "--", agreementsFile); err != nil {
			return nil, fail(err)
		}
	}
	msg := fmt.Sprintf("(chore) %s で %s に合意する", req.Story, target)
	// パスを渡すと、索引に足してあるほかのファイルの変更を入れず、渡したファイルだけをコミットする
	if _, err := repo.Git(o.Root, append([]string{"commit", "-q", "-m", msg, "--"}, files...)...); err != nil {
		return nil, fail(err)
	}
	head, err := repo.Git(o.Root, "rev-parse", "HEAD")
	if err != nil {
		return nil, internalErr(err)
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
		return "", false, conflict("合意の記録のファイルがなく、設定の agreements の最初のパターン "+first+" にワイルドカードがあるので、どこに作るか決まらない", "合意の記録のファイルを作ってから合意する")
	}
	return first, true, nil
}

// verify は、書いた後の合意の記録と約束リストを読み直し、合意が今の中身に合い、フラグが外れたかを確かめる。
func (s *state) verify(rec AgreementRecord, claim *promises.Claim) error {
	snap, err := snapshot.Working(s.o.Root, s.o.Cfg)
	if err != nil {
		return err
	}
	if snap.ListErrs != nil {
		return fmt.Errorf("フラグを外した後の約束リストが読めない（%s）", snap.ListErrs[0].Error())
	}
	recs, rerrs := agreements.Load(s.o.Root, snap.Files, s.o.Cfg.Agreements, s.o.Cfg.AgreementsDefault)
	if rerrs != nil {
		return fmt.Errorf("書き足した後の合意の記録が読めない（%s）", rerrs[0].Error())
	}
	if !recs.AgreedBy(rec.Kind, rec.Key, rec.Fingerprint, rec.Story) {
		return errors.New("書き足した合意が、読み直した合意の記録にない")
	}
	if claim == nil {
		return nil
	}
	after := newSide(snap)
	key := "claim:" + claim.ID
	if after.itemFP[key] != rec.Fingerprint {
		return errors.New("フラグを外したら、約束の指紋が変わった")
	}
	want := map[string]any{}
	for k, v := range claim.Raw {
		if k != "flags" && k != "guesses" {
			want[k] = v
		}
	}
	if !reflect.DeepEqual(after.items[key].Raw, want) {
		return errors.New("フラグを外したら、flags・guesses のほかの項目まで変わった")
	}
	return nil
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
		if start == 0 || !strings.HasPrefix(strings.TrimLeft(lines[start-1], " \t"), key) {
			return nil, fmt.Errorf("%s: 約束 %s の %s が、インラインの表の中にあるので消せない", claim.Table.Doc.File, claim.ID, key)
		}
		end, err := valueEnd(lines, start-1)
		if err != nil {
			return nil, fmt.Errorf("%s:%d: 約束 %s の %s の終わりが分からない（%v）", claim.Table.Doc.File, start, claim.ID, key, err)
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
				return 0, errors.New("複数行の文字列は扱わない")
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
	return 0, errors.New("配列が閉じていない")
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
	fmt.Fprintf(&b, "kind = %s\n", tomlString(rec.Kind))
	fmt.Fprintf(&b, "key = %s\n", tomlString(rec.Key))
	fmt.Fprintf(&b, "fingerprint = %s\n", tomlString(rec.Fingerprint))
	if rec.Path != nil {
		fmt.Fprintf(&b, "path = %s\n", tomlString(*rec.Path))
	}
	fmt.Fprintf(&b, "user = { name = %s, email = %s }\n", tomlString(rec.User.Name), tomlString(rec.User.Email))
	fmt.Fprintf(&b, "agreed_at = %s\n", rec.AgreedAt)
	fmt.Fprintf(&b, "story = %s\n", tomlString(rec.Story))
	return []byte(b.String())
}

// tomlString は、TOML の基本の文字列（"…"）にする。
func tomlString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"':
			b.WriteString(`\"`)
		case r == '\\':
			b.WriteString(`\\`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\t':
			b.WriteString(`\t`)
		case r < 0x20 || r == 0x7f:
			fmt.Fprintf(&b, `\u%04X`, r)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}
