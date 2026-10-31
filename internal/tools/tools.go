//go:build tools

// Package tools は、テストで使う依存を go.mod に残すためだけにある。使う所ができる前に、テスト担当のテストを動かせるようにするため。
package tools

import (
	_ "github.com/rogpeppe/go-internal/testscript"
	_ "pgregory.net/rapid"
)
