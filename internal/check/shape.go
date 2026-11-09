package check

import (
	"fmt"
	"sort"
	"strings"

	"github.com/TadahiroYamamura/vanda/internal/config"
	"github.com/TadahiroYamamura/vanda/internal/diag"
	"github.com/TadahiroYamamura/vanda/internal/promises"
)

var Stages = []string{"plan", "implementation", "release", "operation"}

var Types = []string{"model-check", "proof", "cross-check", "random-check", "scenario-test", "unit-test", "interface-test", "concurrency-test", "guardrail", "sensor", "human-review"}

var Flags = []string{"ai-proposal"}

// promiseListShape は、約束リストの形のチェック。読み込みの誤りに加えて、値・ID・指す先を確かめる。
func (e *env) promiseListShape() []diag.Error {
	errs := append([]diag.Error{}, e.listErrs...)
	if e.list == nil {
		return errs
	}
	// 重複は、出てきた順（ファイルの場所の順、同じファイルなら行の順）で見て、後に出てきたほうを誤りにする
	all := items(e.list)
	sort.SliceStable(all, func(i, j int) bool {
		a, b := all[i].table, all[j].table
		if a.Doc.File != b.Doc.File {
			return a.Doc.File < b.Doc.File
		}
		return a.Line("") < b.Line("")
	})
	where := map[string]item{}
	for _, it := range all {
		// id を書いていないことは、読み込みの誤りとして出ている。空の ID（id = ""）は、形の誤りにする
		if !it.table.Has("id") {
			continue
		}
		if !config.IsID(it.id) {
			errs = append(errs, it.table.Err("id", kindName[it.kind]+"の ID "+quote(it.id)+" が決まった形ではありません", "英小文字で始め、英小文字・数字をハイフンでつないだ形（例: task-id）にしてください"))
			continue
		}
		if prev, dup := where[it.id]; dup {
			errs = append(errs, it.table.Err("id", fmt.Sprintf("ID %s が、%s の%s（%s）と重複しています", quote(it.id), place(prev), kindName[prev.kind], prev.id), "用語・機能・約束のどれとも重複しない ID にしてください"))
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
			errs = append(errs, t.Table.Err("part_of", "part_of の先 "+quote(t.PartOf)+" が用語にありません"+notKind(k), "part_of に、約束リストにある用語の ID を書いてください"))
		}
	}
	errs = append(errs, e.partOfCycles()...)

	for _, f := range e.list.Features {
		waiver := f.Table.Has("formal_waiver")
		switch {
		case f.Core && !f.Formal && !waiver:
			errs = append(errs, f.Table.Err("formal", "コア機能で formal = false なのに、形式手法を使わない理由（formal_waiver）がありません", `formal_waiver = "…" に理由を書くか、formal = true にしてください`))
		case waiver && !(f.Core && !f.Formal):
			errs = append(errs, f.Table.Err("formal_waiver", "formal_waiver を書けるのは、core = true かつ formal = false のときだけです", "formal_waiver を消してください"))
		}
	}

	formal := map[string]bool{}
	for _, f := range e.list.Features {
		if it, ok := where[f.ID]; ok && it.table == f.Table {
			formal[f.ID] = f.Formal
		}
	}

	for _, c := range e.list.Claims {
		errs = append(errs, formalClaim(c, formal)...)
		for _, s := range c.Subjects {
			if k := kindOf(s); k != "term" && k != "feature" {
				errs = append(errs, c.Table.Err("subjects", "subjects の "+quote(s)+" が用語にも機能にもありません"+notKind(k), "subjects に、約束リストにある用語か機能の ID を書いてください"))
			}
		}
		for _, l := range c.Layers {
			if !layers[l] {
				errs = append(errs, c.Table.Err("layers", "レイヤー "+quote(l)+" が設定にありません", "layers を "+orNone(layerIDs)+" から選ぶか、.vanda/config.toml の [[layer]] に足してください"))
			}
		}
		for _, fl := range c.Flags {
			if !contains(Flags, fl) {
				errs = append(errs, c.Table.Err("flags", "フラグ "+quote(fl)+" は決まった値ではありません", "flags には ai-proposal だけを書いてください"))
			}
		}
		covered := map[string]bool{}
		for _, ev := range c.Evidence {
			if ev.Table.Has("stage") && !contains(Stages, ev.Stage) {
				errs = append(errs, ev.Table.Err("stage", "ステージ "+quote(ev.Stage)+" は決まった値ではありません", "stage を "+strings.Join(Stages, "・")+" のどれかにしてください"))
			}
			if ev.Table.Has("type") && !contains(Types, ev.Type) {
				errs = append(errs, ev.Table.Err("type", "タイプ "+quote(ev.Type)+" は決まった値ではありません", "type を "+strings.Join(Types, "・")+" のどれかにしてください"))
			}
			for _, l := range ev.Layers {
				if !contains(c.Layers, l) {
					errs = append(errs, ev.Table.Err("layers", "エビデンスのレイヤー "+quote(l)+" が、約束の layers にありません", "エビデンスの layers を、約束の layers（"+strings.Join(c.Layers, "・")+"）から選んでください"))
					continue
				}
				covered[l] = true
			}
		}
		for _, l := range c.Layers {
			if layers[l] && !covered[l] {
				errs = append(errs, c.Table.Err("layers", "レイヤー "+quote(l)+" を確かめるエビデンスがありません", "layers に "+l+" を含む [[claim.evidence]] を足してください"))
			}
		}
	}
	errs = append(errs, refErrs(e.list, all)...)
	return errs
}

// refErrs は、自然言語で書く項目の中の壊れた参照を、その項目の行の誤りにする。
func refErrs(l *promises.List, all []item) []diag.Error {
	ids := map[string]bool{}
	for _, it := range all {
		ids[it.id] = true
	}
	var errs []diag.Error
	const literal = "文字の {@ なら {@@ と書いてください"
	for _, p := range l.Proses() {
		key := p.Key + " "
		if p.Index >= 0 {
			key = fmt.Sprintf("%s の %d 番目", p.Key, p.Index+1)
		}
		for _, part := range promises.SplitRefs(p.Text) {
			switch {
			case part.Broken != "":
				errs = append(errs, p.Table.Err(p.Key, key+"の参照 "+quote(part.Text)+" が {@id:<ID>} の形ではありません（"+part.Broken+"）", "{@id:<ID>} の形に直すか、"+literal))
			case part.Ref != "" && !ids[part.Ref]:
				errs = append(errs, p.Table.Err(p.Key, key+"の参照 "+quote(part.Text)+" が指す用語・機能・約束がありません", "約束リストにある用語・機能・約束の ID に直すか、"+literal))
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
				errs = append(errs, t.Table.Err("part_of", "part_of が循環しています（"+strings.Join(path, " → ")+"）", "どれかの part_of を消すか、別の用語にしてください"))
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
		return "（設定にレイヤーがありません）"
	}
	return strings.Join(xs, "・")
}

// formalClaim は、形式手法を使う機能を subjects に持つ約束に、model-check の予定と model_check_waiver のどちらか一方だけがあるかを確かめる。
func formalClaim(c *promises.Claim, formal map[string]bool) []diag.Error {
	var features []string
	for _, s := range c.Subjects {
		if formal[s] {
			features = append(features, s)
		}
	}
	if len(features) == 0 {
		return nil
	}
	planned := false
	for _, ev := range c.Evidence {
		if ev.Type == "model-check" {
			planned = true
		}
	}
	waiver := c.Table.Has("model_check_waiver")
	which := "形式手法を使う機能（" + strings.Join(features, "・") + "）"
	switch {
	case !planned && !waiver:
		return []diag.Error{c.Table.Err("subjects", "約束 "+c.ID+" は"+which+"を subjects に持ちますが、model-check の予定も、TLA+ で確かめない理由（model_check_waiver）もありません", "type = \"model-check\" の [[claim.evidence]] を足すか、model_check_waiver に理由を書いてください")}
	case planned && waiver:
		return []diag.Error{c.Table.Err("model_check_waiver", "約束 "+c.ID+" に、model-check の予定と、TLA+ で確かめない理由（model_check_waiver）の両方があります", "TLA+ で確かめるなら model_check_waiver を消し、確かめないなら model-check の予定を消してください")}
	}
	return nil
}
