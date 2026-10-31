// Package check は、審査の設定（.vanda/checks/<名前>.toml）を読み、審査を行う。
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

// Names は、審査の名前と、書ける範囲。
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

// LoadSpecs は、審査の設定の中身 src を読む。file はエラーに出す場所。
func LoadSpecs(file string, src []byte) ([]Spec, diag.Errors) {
	var errs []diag.Error
	doc := tomlread.Parse(file, src, &errs)
	if doc == nil {
		return nil, errs
	}
	top := doc.Top()
	ts, _ := top.Tables("check", true)
	if ts != nil && len(ts) == 0 {
		top.ErrAt("check", "審査が 1 つもない", "[[check]] に name と scope を書く")
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
			t.ErrAt("name", "審査 "+quote(name)+" はない", "name を "+strings.Join(names, "・")+" のどれかにする")
			continue
		case scope != ScopeAll && scope != ScopeStory:
			t.ErrAt("scope", "範囲 "+quote(scope)+" は決まった値でない", `scope を "all" か "story" にする`)
			continue
		case !contains(scopes, scope):
			t.ErrAt("scope", "審査 "+name+" は範囲 "+scope+" で行えない", "scope を "+strings.Join(scopes, " か ")+" にする")
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
