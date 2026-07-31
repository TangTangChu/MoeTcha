//go:build windows

package cli

import (
	"os"
	"syscall"
	"unsafe"
)

var (
	kernel32           = syscall.NewLazyDLL("kernel32.dll")
	procGetConsoleMode = kernel32.NewProc("GetConsoleMode")
	procSetConsoleMode = kernel32.NewProc("SetConsoleMode")
)

const enableVirtualTerminalProcessing = 0x0004

// enableConsoleVT 打开 Windows 控制台的 VT 处理，让 ANSI 颜色码能被解释。
// Win10 1607+ 才支持；句柄不是控制台（如被重定向到管道/文件）时 GetConsoleMode
// 失败，直接返回。此时 styler 的 isATTY 判定也会给出 false，不会留下乱码。
func enableConsoleVT(f *os.File) {
	h := syscall.Handle(f.Fd())
	var mode uint32
	r, _, _ := procGetConsoleMode.Call(uintptr(h), uintptr(unsafe.Pointer(&mode)))
	if r == 0 {
		return
	}
	mode |= enableVirtualTerminalProcessing
	procSetConsoleMode.Call(uintptr(h), uintptr(mode))
}
