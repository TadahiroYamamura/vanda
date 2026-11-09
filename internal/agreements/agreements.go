// Package agreements は、承認の記録（既定は .vanda/agreements.toml）を読み、承認済かを判定する。
package agreements

import (
	"encoding/hex"
	"strings"
	"time"

	"github.com/TadahiroYamamura/vanda/internal/diag"
	"github.com/TadahiroYamamura/vanda/internal/glob"
	"github.com/TadahiroYamamura/vanda/internal/tomlread"
)

var Kinds = []string{"claim", "term", "feature", "file"}

type Agreement struct {
	Table       *tomlread.Table
	Kind        string
	Key         string
	Fingerprint string
	Path        string // 対象がファイルのときだけ。承認したときの場所
	// Deleted は、消したことへの承認か。真なら Fingerprint は空で、ファイルなら Key は場所
	Deleted   bool
	UserName  string
	UserEmail string
	AgreedAt  time.Time
	Story     string
}

type Records struct {
	Files      []string
	Agreements []*Agreement
}

// Load は、files（リポジトリの直下からの場所の一覧）のうち patterns に当たるものを、承認の記録として読む。
// ワイルドカードのないパターンのファイルがなくても誤りにしない。そのファイルは、はじめて承認したときに作るため。
func Load(root string, files, patterns []string) (*Records, diag.Errors) {
	return LoadWith(tomlread.OSReader(root), files, patterns)
}

// LoadWith は、Load と同じく読む。ファイルの中身は read で読む。
func LoadWith(read tomlread.Reader, files, patterns []string) (*Records, diag.Errors) {
	var errs []diag.Error
	matched, _ := glob.Select(files, patterns)
	r := &Records{Files: matched}
	for _, f := range matched {
		doc := tomlread.LoadWith(read, f, &errs)
		if doc == nil {
			continue
		}
		r.read(doc.Top())
	}
	if len(errs) > 0 {
		diag.Sort(errs)
		return r, errs
	}
	return r, nil
}

func (r *Records) read(top *tomlread.Table) {
	ts, _ := top.Tables("agreement", false)
	for _, t := range ts {
		a := &Agreement{Table: t}
		if k, ok := t.Str("kind", true); ok {
			if !contains(Kinds, k) {
				t.ErrAt("kind", "kind "+quote(k)+" は決まった値ではありません", "kind を "+strings.Join(Kinds, "・")+" のどれかにしてください")
			}
			a.Kind = k
		}
		a.Key, _ = t.Str("key", true)
		if d, ok := t.Bool("deleted", false); ok {
			if !d {
				t.ErrAt("deleted", "deleted = false と書いてあります", "消したことへの承認なら deleted = true に、中身への承認なら deleted を消して fingerprint を書いてください")
			}
			a.Deleted = d
		}
		if a.Deleted {
			if t.Has("fingerprint") {
				t.ErrAt("fingerprint", "消したことへの承認の記録に、fingerprint があります", "消したものには中身がないので、fingerprint を消してください")
			}
		} else if fp, ok := t.Str("fingerprint", true); ok {
			if !isFingerprint(fp) {
				t.ErrAt("fingerprint", "fingerprint "+quote(fp)+" がフィンガープリントの形ではありません", "sha256: に続けて、16 進の小文字 64 文字で書いてください")
			}
			a.Fingerprint = fp
		}
		path, hasPath := t.Str("path", false)
		a.Path = path
		if a.Kind == "file" {
			if !t.Has("path") {
				t.ErrAt("", "対象がファイルの承認に、path がありません", `path = "…" に、承認したときのファイルの場所を書いてください`)
			}
			if a.Deleted && t.Has("path") && a.Key != "" && a.Key != a.Path {
				t.ErrAt("key", "消したファイルへの承認の記録の key が、path と違います", "key にも、消したファイルの場所を書いてください")
			}
			if !a.Deleted && a.Key != "" && a.Fingerprint != "" && a.Key != a.Fingerprint {
				t.ErrAt("key", "対象がファイルの承認の key が、fingerprint と違います", "key にも、ファイルの中身のフィンガープリントを書いてください")
			}
		} else if hasPath && a.Kind != "" {
			t.ErrAt("path", "対象が "+a.Kind+" の承認に、path があります", "path は対象がファイルのときだけ書けます。消してください")
		}
		if u, ok := t.Table("user", true); ok {
			a.UserName, _ = u.Str("name", true)
			a.UserEmail, _ = u.Str("email", true)
			u.Unknown("name, email")
		}
		a.AgreedAt, _ = t.Time("agreed_at", true)
		a.Story, _ = t.Str("story", true)
		t.Unknown("kind, key, fingerprint, deleted, path, user, agreed_at, story")
		r.Agreements = append(r.Agreements, a)
	}
	top.Unknown("agreement")
}

// Target は、記録の指す先。約束・用語・機能なら ID、ファイルなら場所（path）。
func (a *Agreement) Target() string {
	if a.Kind == "file" {
		return a.Path
	}
	return a.Key
}

// Agreed は、対象の種類 kind・指す先 target（約束・用語・機能なら ID、ファイルなら場所）の今の中身のフィンガープリント fp に合う承認が 1 つでもあるかを返す。
// ファイルは場所でも見分ける。フィンガープリントだけで当てると、承認済のファイルを写した先が、誰も読まないまま承認済になる
func (r *Records) Agreed(kind, target, fp string) bool {
	for _, a := range r.Agreements {
		if a.Kind == kind && a.Target() == target && !a.Deleted && a.Fingerprint == fp {
			return true
		}
	}
	return false
}

func isFingerprint(s string) bool {
	h, ok := strings.CutPrefix(s, fingerprintPrefix)
	if !ok || len(h) != 64 || strings.ToLower(h) != h {
		return false
	}
	_, err := hex.DecodeString(h)
	return err == nil
}

func contains(xs []string, s string) bool {
	for _, x := range xs {
		if x == s {
			return true
		}
	}
	return false
}

func quote(s string) string { return `"` + s + `"` }

// AgreedBy は、ストーリー story の承認のうち、kind・target・fp に合うものがあるかを返す。
func (r *Records) AgreedBy(kind, target, fp, story string) bool {
	for _, a := range r.Agreements {
		if a.Kind == kind && a.Target() == target && !a.Deleted && a.Fingerprint == fp && a.Story == story {
			return true
		}
	}
	return false
}

// DeletionAgreed は、kind・target を消したことへの承認が 1 つでもあるかを返す。
func (r *Records) DeletionAgreed(kind, target string) bool {
	for _, a := range r.Agreements {
		if a.Kind == kind && a.Target() == target && a.Deleted {
			return true
		}
	}
	return false
}

// DeletionAgreedBy は、ストーリー story の承認のうち、kind・target を消したことへの承認があるかを返す。
func (r *Records) DeletionAgreedBy(kind, target, story string) bool {
	for _, a := range r.Agreements {
		if a.Kind == kind && a.Target() == target && a.Deleted && a.Story == story {
			return true
		}
	}
	return false
}

// EverAgreed は、kind・target の中身を承認したことがあるかを返す（消したことへの承認は数えない）。
func (r *Records) EverAgreed(kind, target string) bool {
	for _, a := range r.Agreements {
		if a.Kind == kind && a.Target() == target && !a.Deleted {
			return true
		}
	}
	return false
}

// FromSources は、読み込み済みの中身 srcs（場所から中身へ）を、names の順に承認の記録として読む。
// 過去のコミットの承認の記録を読むときに使う。
func FromSources(names []string, srcs map[string][]byte) (*Records, diag.Errors) {
	var errs []diag.Error
	r := &Records{Files: names}
	for _, f := range names {
		doc := tomlread.Parse(f, srcs[f], &errs)
		if doc == nil {
			continue
		}
		r.read(doc.Top())
	}
	if len(errs) > 0 {
		diag.Sort(errs)
		return r, errs
	}
	return r, nil
}
