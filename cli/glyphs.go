// 终端字形（图标）分层。
//
// 输出不使用 emoji（emoji 在不同平台呈现各异、且不适合日志文件），而是按
// 终端能力三档回落：
//
//	nerd    Nerd Fonts 私有区字形（Font Awesome 子集），更精致，需要终端字体
//	        打过 Nerd Fonts 补丁（如 CaskaydiaCove Nerd Font）。
//	unicode 普通 Unicode 符号（✓ / ✗ / • / →），绝大多数终端字体都覆盖。
//	ascii   纯 ASCII（[OK] / [X] / [!] / ->），重定向到文件/管道时使用，
//	        保证日志文件里不出现不可打印字符。
//
// 选择逻辑：MOETCHA_GLYPHS=nerd|unicode|ascii|auto 可显式指定（默认 auto）。
// auto 下非终端一律 ascii；终端上仅当环境变量表明「大概率装了 Nerd Fonts」
// 才用 nerd，否则 unicode——拿不准时宁可用安全符号，也不要在屏幕上打出豆腐块。
package cli

import (
	"os"
	"strings"
)

type glyphTier int

const (
	glyphASCII glyphTier = iota
	glyphUnicode
	glyphNerd
)

// glyphSet 一组语义化的状态字形；渲染时再叠加颜色（见 style.go 的 styler）。
type glyphSet struct {
	ok     string // 成功
	fail   string // 失败
	warn   string // 警告 / 进行中
	arrow  string // 箭头 / 下一步
	prompt string // 控制台提示符
}

var (
	glyphsASCII   = glyphSet{ok: "[OK]", fail: "[X]", warn: "[!]", arrow: "->", prompt: ">"}
	glyphsUnicode = glyphSet{ok: "✓", fail: "✗", warn: "•", arrow: "→", prompt: ">"}
	// Nerd Fonts 私有区（Font Awesome 子集）：check / times / exclamation-triangle /
	// arrow-right / terminal。
	glyphsNerd = glyphSet{ok: "\uf00c", fail: "\uf00d", warn: "\uf071", arrow: "\uf061", prompt: "\uf120"}

	// glyphs 是当前进程生效的字形集，启动时探测一次（与 outStyle/errStyle 同理）。
	glyphs = resolveGlyphs()
)

func resolveGlyphs() glyphSet {
	return resolveGlyphsFor(strings.ToLower(strings.TrimSpace(os.Getenv("MOETCHA_GLYPHS"))), isATTY(os.Stderr))
}

// resolveGlyphsFor 纯函数，便于测试：env 是 MOETCHA_GLYPHS 的值（已小写化），
// tty 表示 stderr 是否连着终端（控制台、提示、警告都走 stderr）。
func resolveGlyphsFor(env string, tty bool) glyphSet {
	switch env {
	case "nerd":
		return glyphsNerd
	case "unicode":
		return glyphsUnicode
	case "ascii":
		return glyphsASCII
	}
	if !tty {
		return glyphsASCII
	}
	if nerdFontLikely() {
		return glyphsNerd
	}
	return glyphsUnicode
}

// nerdFontLikely 用环境变量做「终端大概率装了 Nerd Fonts」的保守探测。
// 误判的代价是豆腐块，因此只认少数以 Nerd Fonts 为主流的终端，其余一律退回
// Unicode 符号；拿不准时用户可用 MOETCHA_GLYPHS=nerd 强制开启。
func nerdFontLikely() bool {
	if os.Getenv("KITTY_WINDOW_ID") != "" { // kitty
		return true
	}
	if os.Getenv("WEZTERM_EXECUTABLE") != "" { // WezTerm
		return true
	}
	if os.Getenv("GHOSTTY_RESOURCES_DIR") != "" { // Ghostty
		return true
	}
	switch os.Getenv("TERM_PROGRAM") {
	case "iTerm.app", "WezTerm", "Ghostty", "Hyper", "Tabby", "Warp", "Rio":
		return true
	}
	return false
}
