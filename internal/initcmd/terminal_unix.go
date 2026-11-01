//go:build darwin || netbsd || freebsd || openbsd || linux || dragonfly || solaris

package initcmd

import "github.com/charmbracelet/x/termios"

// keepOutputProcessing は、raw にした端末で、出力の処理（改行を CR LF に直す）だけを戻す。
func keepOutputProcessing(fd uintptr) error {
	cur, err := termios.GetTermios(int(fd))
	if err != nil {
		return err
	}
	// SetTermios は速さも書き戻すので、今の速さをそのまま渡す。速さの型は OS ごとに違う（Linux は uint32、macOS は uint64）が、
	// 値は端末の速さの決まった値で uint32 に収まる
	return termios.SetTermios(int(fd), uint32(cur.Ispeed), uint32(cur.Ospeed), nil, nil, map[termios.O]bool{termios.OPOST: true, termios.ONLCR: true}, nil, nil)
}
