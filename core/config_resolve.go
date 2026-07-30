package core

import (
	"fmt"
	"os"
	"strings"
)

// EnvSource 标识一个配置值的来源，用于 `config show` 展示与错误定位。
type EnvSource int

const (
	SourceDefault EnvSource = iota
	SourceDotEnv
	SourceEnv
	SourceFlag
)

func (s EnvSource) String() string {
	switch s {
	case SourceDotEnv:
		return ".env 文件"
	case SourceEnv:
		return "环境变量"
	case SourceFlag:
		return "命令行"
	default:
		return "默认值"
	}
}

// Resolver 按 命令行 > 环境变量 > .env 文件 > 默认值 的优先级查找配置值。
//
// 真实环境变量压过 .env 文件是刻意的：本仓库 Dockerfile 用 COPY . .，
// 若文件优先，镜像里烤进去的开发用 .env 会静默覆盖 docker run -e 传入的生产密钥。
type Resolver struct {
	DotEnv map[string]string
	Flags  map[string]string

	// Lookup 默认为 os.LookupEnv，测试可替换以保持 hermetic。
	Lookup func(string) (string, bool)
}

// lookup 返回去除首尾空白后的值。
// 空值等同于未设置——保留改造前 getEnv 的语义，使 HTTP_PORT= 仍回落默认值。
func (r *Resolver) lookup(key string) (string, EnvSource, bool) {
	if v, ok := r.Flags[key]; ok {
		if v = strings.TrimSpace(v); v != "" {
			return v, SourceFlag, true
		}
	}
	lookupEnv := r.Lookup
	if lookupEnv == nil {
		lookupEnv = os.LookupEnv
	}
	if v, ok := lookupEnv(key); ok {
		if v = strings.TrimSpace(v); v != "" {
			return v, SourceEnv, true
		}
	}
	if v, ok := r.DotEnv[key]; ok {
		if v = strings.TrimSpace(v); v != "" {
			return v, SourceDotEnv, true
		}
	}
	return "", SourceDefault, false
}

// ResolvedValue 记录单个配置项的最终解析结果，供 `config show` 渲染。
type ResolvedValue struct {
	Spec   Spec
	Value  string // 已按 Spec.Secret 脱敏，可安全打印
	Raw    string // 未脱敏的生效值，仅供 --show-secrets 显式使用
	Source EnvSource
	Err    error // 宽松模式下保留解析错误，此时值为回落的默认值
}

// ConfigError 描述单个配置项的解析失败。
// Value 在构造时即完成脱敏，保证密钥不会随错误信息进入日志。
type ConfigError struct {
	Key    string
	Value  string
	Source EnvSource
	Msg    string
}

func (e *ConfigError) Error() string {
	return fmt.Sprintf("%s %s，当前=%s（来源：%s）", e.Key, e.Msg, e.Value, e.Source)
}

// ConfigErrors 聚合全部配置错误，一次性报出，避免运维逐个试错。
type ConfigErrors []*ConfigError

func (e ConfigErrors) Error() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "配置加载失败（%d 项）：", len(e))
	for _, err := range e {
		sb.WriteString("\n  - ")
		sb.WriteString(err.Error())
	}
	return sb.String()
}

// Unwrap 让 errors.Is / errors.As 能穿透到具体的 ConfigError。
func (e ConfigErrors) Unwrap() []error {
	out := make([]error, len(e))
	for i, err := range e {
		out[i] = err
	}
	return out
}
