package annotation

import "github.com/TadahiroYamamura/vanda/internal/config"

// Part は、エビデンスの予定を揃えるのに要るものの 1 つと、それに当たったアノテーション。
type Part struct {
	// Want は、要るもの（誤りの文に使う。例: 「types に unit-test を含む [[annotation]] で読んだファイルの @vanda:evidence a」）
	Want  string
	Found []Annotation
}

// Evidence は、約束 id のタイプ typ の予定を揃えるのに要るものを返す。どの Part にも当たったアノテーションがあれば揃っている。
// エビデンスが揃っている審査と画面で、同じ判定にするためにある。guards は、ガードレールの定義のファイル（[[guardrail]] の paths に当たるもの）。
//
// guardrail と sensor は、2 つがどちらもあるときだけ揃う。ガードレールは定義があることと本当に拒むことを、センサーは
// 実装のコードの中にあることと本当に見つけることを、どちらも確かめるため
func Evidence(anns []Annotation, guards []string, id, typ string) []Part {
	kind := config.EvidenceKind(typ)
	pick := func(ok func(Annotation) bool) []Annotation {
		var out []Annotation
		for _, a := range anns {
			if a.Value == id && ok(a) {
				out = append(out, a)
			}
		}
		return out
	}
	isGuard := map[string]bool{}
	for _, g := range guards {
		isGuard[g] = true
	}
	switch typ {
	case "guardrail":
		return []Part{
			{
				Want:  "ガードレールの定義（[[guardrail]] の paths に当たるファイル）の @vanda:evidence " + id,
				Found: pick(func(a Annotation) bool { return a.Kind == "evidence" && isGuard[a.File] }),
			},
			{
				// ガードレールの定義のアノテーションも types に guardrail を含む [[annotation]] で読むことがあるので、
				// 定義のファイルのものを数えない（定義だけで揃ったとみなさないため）
				Want:  "types に guardrail を含む [[annotation]] で読んだ、ガードレールの定義でないファイルの @vanda:evidence " + id + "（ガードレールが拒むことを確かめるもの）",
				Found: pick(func(a Annotation) bool { return a.Kind == "evidence" && contains(a.Types, typ) && !isGuard[a.File] }),
			},
		}
	case "sensor":
		return []Part{
			{
				Want:  "types に sensor を含む [[annotation]] で読んだ @vanda:sensor " + id + "（センサーの場所）",
				Found: pick(func(a Annotation) bool { return a.Kind == "sensor" && contains(a.Types, typ) }),
			},
			{
				Want:  "types に sensor を含む [[annotation]] で読んだ @vanda:evidence " + id + "（センサーが、約束が破れた時に見つけることを確かめるもの）",
				Found: pick(func(a Annotation) bool { return a.Kind == "evidence" && contains(a.Types, typ) }),
			},
		}
	}
	return []Part{{
		Want:  "types に " + typ + " を含む [[annotation]] で読んだファイルの @vanda:" + kind + " " + id,
		Found: pick(func(a Annotation) bool { return a.Kind == kind && contains(a.Types, typ) }),
	}}
}
