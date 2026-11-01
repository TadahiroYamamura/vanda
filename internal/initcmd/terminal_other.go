//go:build !(darwin || netbsd || freebsd || openbsd || linux || dragonfly || solaris)

package initcmd

// keepOutputProcessing は、termios のない OS（Windows）では何もしない。Windows の端末を raw にしても、変わるのは入力の扱いだけのため。
func keepOutputProcessing(uintptr) error { return nil }
