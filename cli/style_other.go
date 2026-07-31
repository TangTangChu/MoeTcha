//go:build !windows

package cli

import "os"

// enableConsoleVT 在 POSIX 系统上是空操作：终端原生支持 ANSI 颜色码。
func enableConsoleVT(_ *os.File) {}
