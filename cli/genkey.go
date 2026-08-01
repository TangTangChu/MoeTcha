package cli

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

func runGenKey(args []string) int {
	fs := flag.NewFlagSet("gen-key", flag.ContinueOnError)
	n := fs.Int("bytes", 32, "随机字节数（十六进制输出长度为其两倍）")
	format := fs.String("format", "raw", "输出格式：raw（裸十六进制）或 env（NAME=十六进制，需配合 --name）")
	name := fs.String("name", "", "配合 --format env 的变量名，如 CAPTCHA_TOKEN_SIGNING_KEY")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, "用法：moetcha gen-key [--bytes 32] [--format raw|env] [--name KEY]")
		fs.PrintDefaults()
	}
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *n < 16 {
		fmt.Fprintln(os.Stderr, "错误：--bytes 不得小于 16（签名密钥需要足够的熵）")
		return 2
	}

	key, err := randomHex(*n)
	if err != nil {
		fmt.Fprintf(os.Stderr, "生成随机密钥失败：%v\n", err)
		return 1
	}

	switch *format {
	case "raw":
		fmt.Println(key)
	case "env":
		if strings.TrimSpace(*name) == "" {
			fmt.Fprintln(os.Stderr, "错误：--format env 需要配合 --name 指定变量名")
			return 2
		}
		fmt.Printf("%s=%s\n", strings.TrimSpace(*name), key)
	default:
		fmt.Fprintf(os.Stderr, "错误：--format 必须为 raw 或 env，当前=%q\n", *format)
		return 2
	}
	return 0
}

// randomHex 返回 n 字节的密码学随机数的十六进制表示。
// 用 crypto/rand 而非依赖外部 openssl——Windows 裸机上没有 openssl。
func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// prompter 封装交互式提问，非交互环境下一律返回默认值。
type prompter struct {
	in          *bufio.Reader
	out         io.Writer
	interactive bool
}

func newPrompter() *prompter {
	return &prompter{
		in:          bufio.NewReader(os.Stdin),
		out:         os.Stderr, // 提示走 stderr，便于 stdout 被重定向成配置内容
		interactive: isInteractive(),
	}
}

// isInteractive 判断标准输入是否连着终端。用 os.Stdin.Stat() 而非
// golang.org/x/term，避免为一次判断引入新依赖。
func isInteractive() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// ask 提问并返回回答；直接回车或读到 EOF 时返回 def。
func (p *prompter) ask(label, def string) string {
	if !p.interactive {
		return def
	}
	if def != "" {
		fmt.Fprintf(p.out, "%s [%s]: ", label, def)
	} else {
		fmt.Fprintf(p.out, "%s: ", label)
	}
	line, err := p.in.ReadString('\n')
	if err != nil && line == "" {
		return def
	}
	if v := strings.TrimSpace(line); v != "" {
		return v
	}
	return def
}

// askEnum 反复提问直到答案落在 allowed 内。
func (p *prompter) askEnum(label string, allowed []string, def string) string {
	if !p.interactive {
		return def
	}
	full := fmt.Sprintf("%s (%s)", label, strings.Join(allowed, "/"))
	for {
		v := strings.ToLower(p.ask(full, def))
		for _, a := range allowed {
			if v == a {
				return v
			}
		}
		fmt.Fprintf(p.out, "  %s 请输入 %s 之一\n", glyphs.fail, strings.Join(allowed, " / "))
	}
}

func (p *prompter) askBool(label string, def bool) bool {
	if !p.interactive {
		return def
	}
	hint := "y/N"
	if def {
		hint = "Y/n"
	}
	for {
		fmt.Fprintf(p.out, "%s (%s): ", label, hint)
		line, err := p.in.ReadString('\n')
		if err != nil && line == "" {
			return def
		}
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "":
			return def
		case "y", "yes", "true", "1":
			return true
		case "n", "no", "false", "0":
			return false
		}
		fmt.Fprintf(p.out, "  %s 请输入 y 或 n\n", glyphs.fail)
	}
}

func (p *prompter) notef(format string, a ...any) {
	fmt.Fprintf(p.out, format, a...)
}
