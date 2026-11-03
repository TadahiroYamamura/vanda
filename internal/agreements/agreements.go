// Package agreements は、合意の記録（既定は .vanda/agreements.toml）を読み、合意済かを判定する。
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
	Path        string // 対象がファイルのときだけ。合意したときの場所
	UserName    string
	UserEmail   string
	AgreedAt    time.Time
	Story       string
}

type Records struct {
	Files      []string
	Agreements []*Agreement
}

// Load は、files（リポジトリの直下からの場所の一覧）のうち patterns に当たるものを、合意の記録として読む。allowMissing なら、ワイルドカードのないパターンのファイルがなくても誤りにしない（設定に書かなかった既定のファイルのため）。
func Load(root string, files, patterns []string, allowMissing bool) (*Records, diag.Errors) {
	var errs []diag.Error
	matched, missing := glob.Select(files, patterns)
	if allowMissing {
		missing = nil
	}
	for _, m := range missing {
		errs = append(errs, diag.Error{File: m, What: "合意の記録のファイルがありません", Fix: "ファイルを作るか、.vanda/config.toml の agreements を直してください"})
	}
	r := &Records{Files: matched}
	for _, f := range matched {
		doc := tomlread.Load(root, f, &errs)
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
		if fp, ok := t.Str("fingerprint", true); ok {
			if !isFingerprint(fp) {
				t.ErrAt("fingerprint", "fingerprint "+quote(fp)+" が指紋の形ではありません", "sha256: に続けて、16 進の小文字 64 文字で書いてください")
			}
			a.Fingerprint = fp
		}
		path, hasPath := t.Str("path", false)
		a.Path = path
		if a.Kind == "file" {
			if !t.Has("path") {
				t.ErrAt("", "対象がファイルの合意に、path がありません", `path = "…" に、合意したときのファイルの場所を書いてください`)
			}
			if a.Key != "" && a.Fingerprint != "" && a.Key != a.Fingerprint {
				t.ErrAt("key", "対象がファイルの合意の key が、fingerprint と違います", "key にも、ファイルの中身の指紋を書いてください")
			}
		} else if hasPath && a.Kind != "" {
			t.ErrAt("path", "対象が "+a.Kind+" の合意に、path があります", "path は対象がファイルのときだけ書けます。消してください")
		}
		if u, ok := t.Table("user", true); ok {
			a.UserName, _ = u.Str("name", true)
			a.UserEmail, _ = u.Str("email", true)
			u.Unknown("name, email")
		}
		a.AgreedAt, _ = t.Time("agreed_at", true)
		a.Story, _ = t.Str("story", true)
		t.Unknown("kind, key, fingerprint, path, user, agreed_at, story")
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

// Agreed は、対象の種類 kind・指す先 target（約束・用語・機能なら ID、ファイルなら場所）の今の中身の指紋 fp に合う合意が 1 つでもあるかを返す。
// ファイルは場所でも見分ける。指紋だけで当てると、合意済のファイルを写した先が、誰も読まないまま合意済になる
func (r *Records) Agreed(kind, target, fp string) bool {
	for _, a := range r.Agreements {
		if a.Kind == kind && a.Target() == target && a.Fingerprint == fp {
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

// AgreedBy は、ストーリー story の合意のうち、kind・target・fp に合うものがあるかを返す。
func (r *Records) AgreedBy(kind, target, fp, story string) bool {
	for _, a := range r.Agreements {
		if a.Kind == kind && a.Target() == target && a.Fingerprint == fp && a.Story == story {
			return true
		}
	}
	return false
}

// FromSources は、読み込み済みの中身 srcs（場所から中身へ）を、names の順に合意の記録として読む。
// 過去のコミットの合意の記録を読むときに使う。
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
