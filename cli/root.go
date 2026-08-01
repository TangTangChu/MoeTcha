// Package cli 提供 moetcha 的命令行入口。
//
// 零第三方依赖，仅用标准库 flag 做子命令分发。
// 裸跑 moetcha 等价于 moetcha serve，保证 Dockerfile 的 CMD 无需改动。
package cli

import (
	"fmt"
	"os"
	"runtime"
	"strings"
)

var (
	// Version 可在构建时通过 -ldflags "-X moetcha/cli.Version=v1.2.3" 注入。
	Version = "dev"
	// GitCommit 构建时注入（如 git rev-parse --short HEAD），默认 none。
	GitCommit = "none"
	// BuildDate 构建时注入（如 date -u +%Y-%m-%dT%H:%M:%SZ），默认 unknown。
	BuildDate = "unknown"
)

const usageText = `MoeTcha —— 验证码服务

用法：
  moetcha [serve] [选项]        启动服务（默认命令）
  moetcha config <子命令>       配置管理
  moetcha gen-key [选项]        生成随机密钥
  moetcha version               显示版本
  moetcha help                  显示本帮助

serve 选项：
  --env-file <路径>   指定 .env 文件（默认 .env，不存在则忽略）
  --port <端口>       覆盖 HTTP_PORT
  --log-level <级别>  覆盖 LOG_LEVEL（debug/info/warn/error）
  --console <模式>    运行期控制台：auto（stdin 为终端时启用）/ on / off
  --set KEY=VALUE     覆盖任意配置项，可重复

运行期命令（serve 运行中，控制台启用时输入）：
  help / status / metrics / config [KEY] / reload / set LOG_LEVEL=debug / set CAPTCHA_DIFFICULTY=hard / quit

环境变量：
  MOETCHA_GLYPHS=nerd|unicode|ascii|auto   终端字形（Nerd Fonts / Unicode / 纯 ASCII），默认 auto

config 子命令：
  init      生成 .env（交互向导，或用 --preset dev|prod 非交互生成）
  show      查看生效配置及其来源
  validate  校验配置，有错则以非零码退出
  template  输出完整 .env 模板到标准输出
  get <KEY>       查看单个配置项（密钥默认脱敏，加 --show-secrets 明文）
  set <KEY=VALUE> 写入单个配置项到 .env

配置优先级：命令行 > 真实环境变量 > .env 文件 > 默认值

示例：
  moetcha config init --preset dev
  moetcha config show --env-file .env
  moetcha config get CAPTCHA_DIFFICULTY
  moetcha config set CAPTCHA_TTL=30s
  moetcha gen-key
  moetcha gen-key --format env --name CAPTCHA_TOKEN_SIGNING_KEY
`

// Run 执行命令行，返回进程退出码：0 成功 / 1 运行期错误 / 2 用法错误。
func Run(args []string) int {
	if len(args) < 2 {
		return runServe(nil)
	}

	switch cmd := args[1]; cmd {
	case "serve":
		return runServe(args[2:])
	case "config":
		return runConfig(args[2:])
	case "gen-key":
		return runGenKey(args[2:])
	case "version", "--version", "-v":
		printVersion()
		return 0
	case "help", "--help", "-h":
		fmt.Print(usageText)
		return 0
	default:
		// 裸跑带选项（如 moetcha --port 9000）仍按 serve 处理，保持向后兼容。
		if strings.HasPrefix(cmd, "-") {
			return runServe(args[1:])
		}
		fmt.Fprintf(os.Stderr, "未知命令：%s\n", cmd)
		if hits := suggestCommands(cmd); len(hits) == 1 {
			fmt.Fprintf(os.Stderr, "你是不是想输入：%s\n", hits[0])
		} else if len(hits) > 1 {
			fmt.Fprintf(os.Stderr, "你是不是想输入其中之一：%s\n", strings.Join(hits, " / "))
		}
		fmt.Fprintf(os.Stderr, "\n%s", usageText)
		return 2
	}
}

// printVersion 打印版本与构建信息。Version/GitCommit/BuildDate 可在构建时
// 通过 -ldflags 注入；go 版本与目标平台取自运行时，无需注入。
func printVersion() {
	fmt.Printf("moetcha %s\n", Version)
	fmt.Printf("  commit: %s\n", GitCommit)
	fmt.Printf("  built:  %s\n", BuildDate)
	fmt.Printf("  go:     %s\n", runtime.Version())
	fmt.Printf("  os:     %s/%s\n", runtime.GOOS, runtime.GOARCH)
}

// suggestCommands 返回与输入最接近的已知命令：输入是其前缀，或编辑距离 ≤ 2。
// 用于「你是不是想输入 …」模糊提示，避免打错一个字母就只甩一屏帮助。
func suggestCommands(input string) []string {
	cmds := []string{"serve", "config", "gen-key", "version", "help"}
	var hits []string
	for _, c := range cmds {
		if strings.HasPrefix(c, input) || levenshtein(input, c) <= 2 {
			hits = append(hits, c)
		}
	}
	return hits
}

// levenshtein 计算两串的编辑距离（插入/删除/替换各计 1）。仅用于命令提示，
// 命令都很短，O(m*n) 足矣。
func levenshtein(a, b string) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	prev := make([]int, lb+1)
	curr := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			del := prev[j] + 1
			ins := curr[j-1] + 1
			sub := prev[j-1] + cost
			m := del
			if ins < m {
				m = ins
			}
			if sub < m {
				m = sub
			}
			curr[j] = m
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}

// multiFlag 支持可重复出现的 --set KEY=VALUE。
type multiFlag []string

func (m *multiFlag) String() string { return strings.Join(*m, ",") }

func (m *multiFlag) Set(v string) error {
	*m = append(*m, v)
	return nil
}

// parseSetFlags 把 --set KEY=VALUE 解析成覆盖表，并校验键名确实存在于注册表——
// 打错键名却静默生效，比直接报错更难排查。
func parseSetFlags(items []string) (map[string]string, error) {
	if len(items) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(items))
	for _, item := range items {
		eq := strings.IndexByte(item, '=')
		if eq <= 0 {
			return nil, fmt.Errorf("--set 需要 KEY=VALUE 形式，当前=%q", item)
		}
		key := strings.TrimSpace(item[:eq])
		if _, ok := coreSpecByKey(key); !ok {
			return nil, fmt.Errorf("--set 中的 %s 不是已知配置项（用 moetcha config template 查看全部）", key)
		}
		out[key] = item[eq+1:]
	}
	return out, nil
}
