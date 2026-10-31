package check

import (
	"fmt"
	"strings"

	"github.com/TadahiroYamamura/vanda/internal/config"
	"github.com/TadahiroYamamura/vanda/internal/diag"
)

var Stages = []string{"plan", "implementation", "test", "release", "operation"}

var Types = []string{"model-check", "proof", "cross-check", "random-check", "scenario-test", "unit-test", "interface-test", "concurrency-test", "guardrail", "sensor", "monitoring", "human-review"}

var Flags = []string{"ai-proposal"}

// promiseListShape は、約束リストの形の審査。読み込みの誤りに加えて、値・ID・指す先を確かめる。
func (e *env) promiseListShape() []diag.Error {
	errs := append([]diag.Error{}, e.listErrs...)
	if e.list == nil {
		return errs
	}
	all := items(e.list)
	where := map[string]item{}
	for _, it := range all {
		if it.id == "" {
			continue
		}
		if !config.IsID(it.id) {
			errs = append(errs, it.table.Err("id", kindName[it.kind]+"の ID "+quote(it.id)+" が決まった形でない", "英小文字で始め、英小文字・数字をハイフンでつないだ形（例: task-id）にする"))
			continue
		}
		if prev, dup := where[it.id]; dup {
			errs = append(errs, it.table.Err("id", fmt.Sprintf("ID %s が、%s の%s（%s）と重なっている", quote(it.id), place(prev), kindName[prev.kind], prev.id), "用語・機能・約束のどれとも重ならない ID にする"))
			continue
		}
		where[it.id] = it
	}
	kindOf := func(id string) string {
		if it, ok := where[id]; ok {
			return it.kind
		}
		return ""
	}

	layers := map[string]bool{}
	var layerIDs []string
	for _, l := range e.cfg.Layers {
		layers[l.ID] = true
		layerIDs = append(layerIDs, l.ID)
	}

	for _, t := range e.list.Terms {
		if t.PartOf == "" {
			continue
		}
		if k := kindOf(t.PartOf); k != "term" {
			errs = append(errs, t.Table.Err("part_of", "part_of の先 "+quote(t.PartOf)+" が用語にない"+notKind(k), "part_of に、約束リストにある用語の ID を書く"))
		}
	}
	errs = append(errs, e.partOfCycles()...)

	for _, f := range e.list.Features {
		waiver := f.Table.Has("formal_waiver")
		switch {
		case f.Core && !f.Formal && !waiver:
			errs = append(errs, f.Table.Err("formal", "コア機能で formal = false なのに、形式手法を使わない理由（formal_waiver）がない", `formal_waiver = "…" に理由を書くか、formal = true にする`))
		case waiver && !(f.Core && !f.Formal):
			errs = append(errs, f.Table.Err("formal_waiver", "formal_waiver を書けるのは core = true かつ formal = false のときだけ", "formal_waiver を消す"))
		}
	}

	for _, c := range e.list.Claims {
		for _, s := range c.Subjects {
			if k := kindOf(s); k != "term" && k != "feature" {
				errs = append(errs, c.Table.Err("subjects", "subjects の "+quote(s)+" が用語にも機能にもない"+notKind(k), "subjects に、約束リストにある用語か機能の ID を書く"))
			}
		}
		for _, l := range c.Layers {
			if !layers[l] {
				errs = append(errs, c.Table.Err("layers", "レイヤー "+quote(l)+" が設定にない", "layers を "+orNone(layerIDs)+" から選ぶか、.vanda/config.toml の [[layer]] に足す"))
			}
		}
		for _, fl := range c.Flags {
			if !contains(Flags, fl) {
				errs = append(errs, c.Table.Err("flags", "フラグ "+quote(fl)+" は決まった値でない", "flags に書けるのは ai-proposal だけ"))
			}
		}
		covered := map[string]bool{}
		for _, ev := range c.Evidence {
			if ev.Table.Has("stage") && !contains(Stages, ev.Stage) {
				errs = append(errs, ev.Table.Err("stage", "ステージ "+quote(ev.Stage)+" は決まった値でない", "stage を "+strings.Join(Stages, "・")+" のどれかにする"))
			}
			if ev.Table.Has("type") && !contains(Types, ev.Type) {
				errs = append(errs, ev.Table.Err("type", "タイプ "+quote(ev.Type)+" は決まった値でない", "type を "+strings.Join(Types, "・")+" のどれかにする"))
			}
			for _, l := range ev.Layers {
				if !contains(c.Layers, l) {
					errs = append(errs, ev.Table.Err("layers", "エビデンスのレイヤー "+quote(l)+" が、約束の layers にない", "エビデンスの layers を、約束の layers（"+strings.Join(c.Layers, "・")+"）から選ぶ"))
					continue
				}
				covered[l] = true
			}
		}
		for _, l := range c.Layers {
			if layers[l] && !covered[l] {
				errs = append(errs, c.Table.Err("layers", "レイヤー "+quote(l)+" を確かめるエビデンスがない", "layers に "+l+" を含む [[claim.evidence]] を足す"))
			}
		}
	}
	return errs
}

// partOfCycles は、part_of の循環を、循環に入る用語ごとに 1 件の誤りにする。
func (e *env) partOfCycles() []diag.Error {
	parent := map[string]string{}
	byID := map[string]int{}
	for i, t := range e.list.Terms {
		if t.PartOf != "" {
			parent[t.ID] = t.PartOf
		}
		if _, ok := byID[t.ID]; !ok {
			byID[t.ID] = i
		}
	}
	var errs []diag.Error
	for _, t := range e.list.Terms {
		seen := map[string]bool{t.ID: true}
		path := []string{t.ID}
		for cur := parent[t.ID]; cur != ""; cur = parent[cur] {
			path = append(path, cur)
			if cur == t.ID {
				errs = append(errs, t.Table.Err("part_of", "part_of が循環している（"+strings.Join(path, " → ")+"）", "どれかの part_of を消すか、別の用語にする"))
				break
			}
			if seen[cur] {
				break
			}
			seen[cur] = true
		}
	}
	return errs
}

func place(it item) string {
	if l := it.table.Line("id"); l > 0 {
		return fmt.Sprintf("%s:%d", it.table.Doc.File, l)
	}
	return it.table.Doc.File
}

func notKind(k string) string {
	if k == "" {
		return ""
	}
	return "（" + kindName[k] + "の ID）"
}

func orNone(xs []string) string {
	if len(xs) == 0 {
		return "（設定にレイヤーがない）"
	}
	return strings.Join(xs, "・")
}
