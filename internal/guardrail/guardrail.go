// Package guardrail は、ガードレールの定義を取り出す。段 1 のアダプタは「ファイルを読むだけ」（adapter = "file"）。
package guardrail

import (
	"github.com/TadahiroYamamura/vanda/internal/config"
	"github.com/TadahiroYamamura/vanda/internal/diag"
	"github.com/TadahiroYamamura/vanda/internal/glob"
)

// Files は、files（リポジトリの直下からの場所の一覧）のうち、ガードレールの定義のファイルを返す。
// ワイルドカードのないパターンのファイルがなければ、誤りにする。
func Files(files []string, rules []config.GuardrailRule) ([]string, []diag.Error) {
	var all []string
	var errs []diag.Error
	seen := map[string]bool{}
	for _, r := range rules {
		matched, missing := glob.Select(files, r.Paths)
		for _, m := range missing {
			errs = append(errs, diag.Error{File: m, What: "ガードレールの定義のファイルがありません", Fix: "ファイルを作るか、.vanda/config.toml の [[guardrail]] の paths を直してください"})
		}
		for _, f := range matched {
			if !seen[f] {
				seen[f] = true
				all = append(all, f)
			}
		}
	}
	return all, errs
}
