package check

import (
	"github.com/TadahiroYamamura/vanda/internal/promises"
	"github.com/TadahiroYamamura/vanda/internal/snapshot"
	"github.com/TadahiroYamamura/vanda/internal/tomlread"
)

// item は、約束リストの項目（用語・機能・仕様・性質・前提）を、種類を問わずに扱うためのもの。
type item struct {
	kind  string // term・feature・spec・property・truth
	id    string
	table *tomlread.Table
	raw   map[string]any
	spec  *promises.Spec
}

func (it item) key() string { return it.kind + ":" + it.id }

func items(l *promises.List) []item {
	var out []item
	for _, e := range snapshot.Entries(l) {
		out = append(out, item{kind: e.Kind, id: e.ID, table: e.Table, raw: e.Raw, spec: e.Spec})
	}
	return out
}

var kindName = map[string]string{"property": "性質", "truth": "前提", "term": "用語", "feature": "機能", "spec": "仕様"}
