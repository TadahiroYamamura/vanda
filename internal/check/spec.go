// Package check は、チェックの設定（.vanda/checks/<名前>.toml）を読み、チェックする。
package check

import (
	"strings"

	"github.com/TadahiroYamamura/vanda/internal/diag"
	"github.com/TadahiroYamamura/vanda/internal/tomlread"
)

const (
	ScopeAll   = "all"
	ScopeStory = "story"
)

// Names は、チェックの名前と、書けるスコープ。
var Names = []struct {
	Name   string
	Scopes []string
}{
	{"promise-list-shape", []string{ScopeAll}},
	{"annotation-links", []string{ScopeAll}},
	{"story-origin", []string{ScopeStory}},
	{"agreement", []string{ScopeAll, ScopeStory}},
	{"agreement-targets", []string{ScopeAll}},
	{"evidence-present", []string{ScopeAll, ScopeStory}},
	{"unread-files", []string{ScopeAll}},
}

type Spec struct {
	Name  string
	Scope string
}

// LoadSpecs は、チェックの設定の中身 src を読む。file はエラーに出す場所。
func LoadSpecs(file string, src []byte) ([]Spec, diag.Errors) {
	var errs []diag.Error
	doc := tomlread.Parse(file, src, &errs)
	if doc == nil {
		return nil, errs
	}
	top := doc.Top()
	ts, _ := top.Tables("check", true)
	if ts != nil && len(ts) == 0 {
		top.ErrAt("check", "チェックが 1 つもありません", "[[check]] に name と scope を書いてください")
	}
	var names []string
	for _, n := range Names {
		names = append(names, n.Name)
	}
	var specs []Spec
	for _, t := range ts {
		var s Spec
		name, okName := t.Str("name", true)
		scope, okScope := t.Str("scope", true)
		t.Unknown("name, scope")
		if !okName || !okScope {
			continue
		}
		var scopes []string
		for _, n := range Names {
			if n.Name == name {
				scopes = n.Scopes
			}
		}
		switch {
		case scopes == nil:
			t.ErrAt("name", "チェック "+quote(name)+" はありません", "name を "+strings.Join(names, "・")+" のどれかにしてください")
			continue
		case scope != ScopeAll && scope != ScopeStory:
			t.ErrAt("scope", "スコープ "+quote(scope)+" は決まった値ではありません", `scope を "all" か "story" にしてください`)
			continue
		case !contains(scopes, scope):
			t.ErrAt("scope", "チェック "+name+" はスコープ "+scope+" では行えません", "scope を "+strings.Join(scopes, " か ")+" にしてください")
			continue
		}
		s.Name, s.Scope = name, scope
		specs = append(specs, s)
	}
	top.Unknown("check")
	if len(errs) > 0 {
		diag.Sort(errs)
		return nil, errs
	}
	return specs, nil
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
