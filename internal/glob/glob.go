// Package glob は、設定に書くファイルのパターンを扱う。
// path.Match の書き方に加えて、何段のディレクトリにもマッチする `**` を扱う（標準のライブラリにないが、依存を足さないため自分で持つ）。
package glob

import (
	"io/fs"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// Validate は、パターンが書き方に合っているかを確かめる。
// ファイルの場所は / で始まらず、空・.・.. の部分を含まないので、そうしたパターンはどのファイルにもマッチしない。書き間違いに気づけるよう、誤りにする
func Validate(pattern string) error {
	if pattern == "" || strings.HasPrefix(pattern, "/") {
		return path.ErrBadPattern
	}
	for _, seg := range strings.Split(pattern, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return path.ErrBadPattern
		}
		if seg == "**" {
			continue
		}
		if _, err := path.Match(seg, ""); err != nil {
			return err
		}
	}
	return nil
}

// HasMeta は、パターンに `*`・`?`・`[` が含まれるかを返す。
func HasMeta(pattern string) bool {
	return strings.ContainsAny(pattern, `*?[\`)
}

// Match は、リポジトリの直下からの `/` 区切りの場所 name がパターンにマッチするかを返す。
func Match(pattern, name string) bool {
	return matchSegs(strings.Split(pattern, "/"), strings.Split(name, "/"))
}

func matchSegs(pat, name []string) bool {
	for len(pat) > 0 {
		if pat[0] == "**" {
			for i := 0; i <= len(name); i++ {
				if matchSegs(pat[1:], name[i:]) {
					return true
				}
			}
			return false
		}
		if len(name) == 0 {
			return false
		}
		ok, err := path.Match(pat[0], name[0])
		if err != nil || !ok {
			return false
		}
		pat, name = pat[1:], name[1:]
	}
	return len(name) == 0
}

// Files は、root の下のファイルを、リポジトリの直下からの `/` 区切りの場所で、名前の順に返す。.git の中は見ない。
func Files(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	sort.Strings(files)
	return files, err
}

// Select は、files のうちパターンのどれかにマッチするものを、files の順で返す。
// ワイルドカードのないパターンで、files にないものは missing に返す。
func Select(files, patterns []string) (matched, missing []string) {
	have := map[string]bool{}
	for _, f := range files {
		have[f] = true
		for _, p := range patterns {
			if Match(p, f) {
				matched = append(matched, f)
				break
			}
		}
	}
	for _, p := range patterns {
		if !HasMeta(p) && !have[p] {
			missing = append(missing, p)
		}
	}
	return matched, missing
}
