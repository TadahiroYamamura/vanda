package review

// このファイルは、一覧の事後条件の確かめ（センサー）を持つ。設定の [[annotation]] の **/*_sensor.go で読む。

import (
	"fmt"
	"net/http"
)

// checkCount は、一覧を返す前に、一覧の数が違う鍵の数に合い、同じものが 2 度出ていないかを確かめる
// （漏れも重複もない一覧という事後条件）。破れていれば、一覧を返さずに 500 にする。
// 一覧の作り方が正しければ外からは破れないので、テストは、食い違う changes と es を直に渡して確かめる。
//
// @vanda:sensor review-count-verified
func checkCount(changes []rawChange, es []Entry) *Error {
	seen := map[string]bool{}
	for _, e := range es {
		if seen[e.key] {
			return &Error{Status: http.StatusInternalServerError, Message: "一覧に同じもの（" + e.key + "）が 2 度出ています。", Fix: "vanda の不具合です。開発者に知らせてください。"}
		}
		seen[e.key] = true
	}
	if want := len(changes); len(es) != want {
		return &Error{Status: http.StatusInternalServerError, Message: fmt.Sprintf("一覧の数（%d）と差分の数（%d）が合いません。", len(es), want), Fix: "vanda の不具合です。開発者に知らせてください。"}
	}
	return nil
}
