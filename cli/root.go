// Package cli 提供 moetcha 的命令行入口。
//
// 零第三方依赖，仅用标准库 flag 做子命令分发。
// 裸跑 moetcha 等价于 moetcha serve，保证 Dockerfile 的 CMD 无需改动。
package cli

import (
	"fmt"
	"os"
	"strings"
)

// Version 可在构建时通过 -ldflags "-X moetcha/cli.Version=v1.2.3" 注入。
var Version = "dev"

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
  --set KEY=VALUE     覆盖任意配置项，可重复

config 子命令：
  init      生成 .env（交互向导，或用 --preset dev|prod 非交互生成）
  show      查看生效配置及其来源
  validate  校验配置，有错则以非零码退出
  template  输出完整 .env 模板到标准输出

配置优先级：命令行 > 真实环境变量 > .env 文件 > 默认值

示例：
  moetcha config init --preset dev
  moetcha config show --env-file .env
  moetcha gen-key
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
		fmt.Println("moetcha " + Version)
		return 0
	case "help", "--help", "-h":
		fmt.Print(usageText)
		return 0
	default:
		// 裸跑带选项（如 moetcha --port 9000）仍按 serve 处理，保持向后兼容。
		if strings.HasPrefix(cmd, "-") {
			return runServe(args[1:])
		}
		fmt.Fprintf(os.Stderr, "未知命令：%s\n\n%s", cmd, usageText)
		return 2
	}
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
