package check

import (
	"github.com/TadahiroYamamura/vanda/internal/promises"
	"github.com/TadahiroYamamura/vanda/internal/snapshot"
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
	var out []item
	for _, e := range snapshot.Entries(l) {
		out = append(out, item{kind: e.Kind, id: e.ID, table: e.Table, raw: e.Raw, claim: e.Claim})
	}
	return out
}

var kindName = map[string]string{"term": "用語", "feature": "機能", "claim": "約束"}
