package check

import (
	"os"
	"path/filepath"

	"github.com/TadahiroYamamura/vanda/internal/annotation"
	"github.com/TadahiroYamamura/vanda/internal/diag"
)

var targetKind = map[string]string{"term": "term", "feature": "feature", "claim": "claim", "evidence": "claim", "sensor": "claim"}

// annotationLinks は、アノテーションのつながりの審査。
func (e *env) annotationLinks() []diag.Error {
	errs := append([]diag.Error{}, e.annErrs...)
	if e.listErrs != nil {
		return append(errs, cannot("約束リスト", len(e.listErrs), "promise-list-shape")...)
	}
	kindOf := map[string]string{}
	for _, it := range items(e.list) {
		if _, ok := kindOf[it.id]; !ok {
			kindOf[it.id] = it.kind
		}
	}
	for _, a := range e.anns {
		want, ok := targetKind[a.Kind]
		if !ok {
			continue
		}
		got := kindOf[a.Value]
		switch {
		case got == "":
			errs = append(errs, diag.Error{File: a.File, Line: a.Line, What: "@vanda:" + a.Kind + " の " + quote(a.Value) + " が約束リストの" + kindName[want] + "にない", Fix: "約束リストにある" + kindName[want] + "の ID にするか、約束リストに足す"})
		case got != want:
			errs = append(errs, diag.Error{File: a.File, Line: a.Line, What: "@vanda:" + a.Kind + " の " + quote(a.Value) + " は" + kindName[got] + "の ID で、" + kindName[want] + "の ID でない", Fix: "@vanda:" + a.Kind + " には" + kindName[want] + "の ID を書く"})
		}
	}
	errs = append(errs, e.specTypes()...)
	return errs
}

// specTypes は、TLA+ の仕様の型に、用語か「仕様の中だけで使う」アノテーションがあり、機能のアノテーションがないことを確かめる。
func (e *env) specTypes() []diag.Error {
	byName := map[string][]annotation.Annotation{}
	for _, a := range e.anns {
		if a.Adapter == "tla" {
			byName[a.File+"\x00"+a.Name] = append(byName[a.File+"\x00"+a.Name], a)
		}
	}
	var errs []diag.Error
	for _, f := range e.files {
		if !e.isTLA(f) {
			continue
		}
		src, err := os.ReadFile(filepath.Join(e.root, filepath.FromSlash(f)))
		if err != nil {
			continue // 読めないことは、アノテーションの読み込みの誤りとして出ている
		}
		for _, ty := range annotation.TLATypes(src) {
			var term, feature bool
			for _, a := range byName[f+"\x00"+ty.Name] {
				switch a.Kind {
				case "term", "internal":
					term = true
				case "feature":
					feature = true
				}
			}
			if feature {
				errs = append(errs, diag.Error{File: f, Line: ty.Line, What: "仕様の型 " + ty.Name + "（TypeOK で使っている）に @vanda:feature が付いている", Fix: "型には @vanda:term <用語の ID> か @vanda:internal <理由> を付け、@vanda:feature は操作の定義に付ける"})
			}
			if !term {
				errs = append(errs, diag.Error{File: f, Line: ty.Line, What: "仕様の型 " + ty.Name + "（TypeOK で使っている）に @vanda:term も @vanda:internal もない", Fix: "定義のすぐ上のコメントに @vanda:term <用語の ID> か @vanda:internal <理由> を書く"})
			}
		}
	}
	return errs
}

func (e *env) isTLA(f string) bool {
	for _, r := range e.cfg.Annotations {
		if r.Adapter != "tla" {
			continue
		}
		for _, p := range r.Paths {
			if matchPath(p, f) {
				return true
			}
		}
	}
	return false
}
