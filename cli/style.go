// 终端着色辅助。
//
// 仅在输出确实是终端时才注入 ANSI 颜色码，重定向到文件/管道时自动退回原文，
// 避免日志文件里出现 \x1b[31m 这种乱码。stdout 与 stderr 分别判定，因为
// 它们可能被分别重定向（例如把标准输出存成配置、把错误写进日志）。
package cli

import "os"

var (
	outStyle = styler{tty: isATTY(os.Stdout)}
	errStyle = styler{tty: isATTY(os.Stderr)}
)

func init() {
	// 在 Windows 上需要显式打开控制台的 VT 处理才能解释 ANSI 颜色码；
	// 非 Windows 上是空操作。句柄被重定向时该调用静默失败，无副作用。
	enableConsoleVT(os.Stdout)
	enableConsoleVT(os.Stderr)
}

// styler 绑定一个输出流的可着色性。tty 为假时所有颜色方法都返回原文。
type styler struct{ tty bool }

func (s styler) wrap(code, text string) string {
	if !s.tty {
		return text
	}
	return code + text + "\x1b[0m"
}

func (s styler) green(t string) string  { return s.wrap("\x1b[32m", t) }
func (s styler) red(t string) string    { return s.wrap("\x1b[31m", t) }
func (s styler) yellow(t string) string { return s.wrap("\x1b[33m", t) }
func (s styler) bold(t string) string   { return s.wrap("\x1b[1m", t) }

// isATTY 判断文件描述符是否连着终端，沿用项目「零第三方依赖」的约定，
// 用 os.File.Stat() 的 ModeCharDevice 位而非 golang.org/x/term。
func isATTY(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
