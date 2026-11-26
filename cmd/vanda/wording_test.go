package main_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// 使わなくなった言葉（docs/plans/promise-reorg-shape-impl.md の 2.7）。仕様を指す「約束」（「約束リスト」は今も使う）と、「PO」「監督」。
// PO は、POST のような英語の語の一部を除く。
var oldWord = regexp.MustCompile(`約束(?:[^リ]|$)|(?:^|[^A-Za-z])PO(?:[^A-Za-z]|$)|監督`)

// oldWordLines は、vanda が出した文 s の行のうち、使わなくなった言葉を含むものを返す。
func oldWordLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if oldWord.MatchString(line) {
			out = append(out, line)
		}
	}
	return out
}

// wantNoOldWords は、vanda が出した文 s（where は、どこに出たものか）に、使わなくなった言葉がないことを確かめる。
// CLI を動かす部品（runVandaIn）と API を呼ぶ部品（vandaServer.do）から呼び、ほかのテストが出させた文もすべて確かめる。
func wantNoOldWords(t serveTB, where, s string) {
	helper(t)
	if lines := oldWordLines(s); len(lines) > 0 {
		t.Errorf("%s に、使わなくなった言葉（仕様を指す「約束」・「PO」・「監督」）が残っている:\n  %s", where, strings.Join(lines, "\n  "))
	}
}

// シナリオは testscript が vanda を動かすので、出力を横から読めない。vanda の代わりに、本物の vanda の標準出力と標準エラーを
// ファイルに書き写してから同じものを出す入れ物を PATH に置き、シナリオをすべて動かした後に、書き写したものを読む。
func Testシナリオで出たチェックの結果の行と誤りの文に仕様を指す約束とPOと監督が残っていない(t *testing.T) {
	t.Parallel()
	bin := buildVanda(t)
	logDir := t.TempDir()
	wrapDir := t.TempDir()
	wrapper := "#!/bin/sh\n" +
		"f=$(mktemp \"$VANDA_OUTPUT_LOG/XXXXXXXX\") || exit 97\n" +
		"\"$VANDA_REAL\" \"$@\" >\"$f.out\" 2>\"$f.err\"\n" +
		"code=$?\n" +
		"cat \"$f.out\"\n" +
		"cat \"$f.err\" >&2\n" +
		"exit $code\n"
	if err := writeFileForExec(filepath.Join(wrapDir, "vanda"), []byte(wrapper), 0o755); err != nil {
		t.Fatal(err)
	}
	// 並べて動くシナリオが全部終わってから読むよう、t.Run で囲む
	t.Run("シナリオ", func(t *testing.T) {
		runScenarios(t, wrapDir, map[string]string{"VANDA_REAL": bin, "VANDA_OUTPUT_LOG": logDir})
	})
	var outs, errs int
	for _, pattern := range []string{"*.out", "*.err"} {
		files, err := filepath.Glob(filepath.Join(logDir, pattern))
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range files {
			data, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			if len(data) == 0 {
				continue
			}
			if strings.HasSuffix(f, ".out") {
				outs++
			} else {
				errs++
			}
			wantNoOldWords(t, "シナリオの vanda の出力", string(data))
		}
	}
	// 書き写せていなければ、何も確かめていない。シナリオは、通るチェックも落ちるチェックも多く動かす
	if outs < 50 || errs < 50 {
		t.Errorf("書き写した出力が少ない（標準出力 %d 件、標準エラー %d 件）。入れ物が vanda の出力を書き写せていない", outs, errs)
	}
}
