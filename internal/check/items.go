package check

import (
	"github.com/TadahiroYamamura/vanda/internal/promises"
	"github.com/TadahiroYamamura/vanda/internal/tomlread"
)

// item は、約束リストの項目（用語・機能・約束）を、種類を問わずに扱うためのもの。
type item struct {
	kind  string // term・feature・claim
	id    string
	table *tomlread.Table
	raw   map[string]any
	claim *promises.Claim
}

func (it item) key() string { return it.kind + ":" + it.id }

func items(l *promises.List) []item {
	if l == nil {
		return nil
	}
	var out []item
	for _, t := range l.Terms {
		out = append(out, item{kind: "term", id: t.ID, table: t.Table, raw: t.Raw})
	}
	for _, f := range l.Features {
		out = append(out, item{kind: "feature", id: f.ID, table: f.Table, raw: f.Raw})
	}
	for _, c := range l.Claims {
		out = append(out, item{kind: "claim", id: c.ID, table: c.Table, raw: c.Raw, claim: c})
	}
	return out
}

var kindName = map[string]string{"term": "用語", "feature": "機能", "claim": "約束"}
