package core

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

// DotEnvEntry 表示 .env 文件中的一条键值对。
type DotEnvEntry struct {
	Key   string
	Value string
	Line  int
}

// DotEnvSyntaxError 描述 .env 文件中的一处语法错误。
// 注意：Text 只保留出错行的键名部分，绝不回显值，避免把签名密钥写进日志。
type DotEnvSyntaxError struct {
	File string
	Line int
	Key  string
	Msg  string
}

func (e *DotEnvSyntaxError) Error() string {
	loc := fmt.Sprintf("第 %d 行", e.Line)
	if e.File != "" {
		loc = fmt.Sprintf("%s 第 %d 行", e.File, e.Line)
	}
	if e.Key != "" {
		return fmt.Sprintf("%s（%s）：%s", loc, e.Key, e.Msg)
	}
	return fmt.Sprintf("%s：%s", loc, e.Msg)
}

// LoadDotEnvFile 读取并解析指定的 .env 文件。
// 文件不存在时返回 (nil, nil)——这是正常情况，不是错误。
func LoadDotEnvFile(path string) ([]DotEnvEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	return ParseDotEnv(f, path)
}

// ParseDotEnv 解析 .env 格式内容。filename 仅用于错误信息。
func ParseDotEnv(r io.Reader, filename string) ([]DotEnvEntry, error) {
	var entries []DotEnvEntry
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	lineNo := 0
	for scanner.Scan() {
		lineNo++
		line := scanner.Text()
		if lineNo == 1 {
			line = strings.TrimPrefix(line, "\ufeff") // UTF-8 BOM
		}
		// bufio.Scanner 按 \n 切分，CRLF 文件会留下尾部 \r。
		line = strings.TrimRight(line, "\r")

		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		trimmed = strings.TrimPrefix(trimmed, "export ")
		trimmed = strings.TrimSpace(trimmed)

		eq := strings.IndexByte(trimmed, '=')
		if eq < 0 {
			return nil, &DotEnvSyntaxError{File: filename, Line: lineNo, Msg: "缺少 = 分隔符"}
		}

		key := strings.TrimSpace(trimmed[:eq])
		if err := validateDotEnvKey(key); err != nil {
			return nil, &DotEnvSyntaxError{File: filename, Line: lineNo, Key: key, Msg: err.Error()}
		}

		value, err := parseDotEnvValue(trimmed[eq+1:])
		if err != nil {
			return nil, &DotEnvSyntaxError{File: filename, Line: lineNo, Key: key, Msg: err.Error()}
		}

		entries = append(entries, DotEnvEntry{Key: key, Value: value, Line: lineNo})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

// DotEnvMap 把条目压平成 map，同名键以最后一次出现为准。
func DotEnvMap(entries []DotEnvEntry) map[string]string {
	if len(entries) == 0 {
		return nil
	}
	m := make(map[string]string, len(entries))
	for _, e := range entries {
		m[e.Key] = e.Value
	}
	return m
}

func validateDotEnvKey(key string) error {
	if key == "" {
		return fmt.Errorf("键名为空")
	}
	for i, r := range key {
		switch {
		case r >= 'A' && r <= 'Z', r >= 'a' && r <= 'z', r == '_':
		case r >= '0' && r <= '9' && i > 0:
		default:
			return fmt.Errorf("键名只能包含字母、数字和下划线，且不能以数字开头")
		}
	}
	return nil
}

// parseDotEnvValue 解析 = 右侧的值。
// 双引号内支持转义，单引号内为字面量，未加引号时按行尾注释截断。
func parseDotEnvValue(raw string) (string, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return "", nil
	}

	switch v[0] {
	case '"':
		return parseQuoted(v, '"', true)
	case '\'':
		return parseQuoted(v, '\'', false)
	}

	// 未加引号：空白后的 # 视为行尾注释。
	for i := 0; i < len(v); i++ {
		if v[i] != '#' {
			continue
		}
		if i > 0 && v[i-1] != ' ' && v[i-1] != '\t' {
			continue // 值内部的 #，例如密码里的字符
		}
		v = v[:i]
		break
	}
	return strings.TrimSpace(v), nil
}

// parseQuoted 解析带引号的值，返回去引号（并按需反转义）后的内容。
// 引号未闭合时报错而非当作字面量——静默误解析签名密钥属于安全故障。
func parseQuoted(v string, quote byte, unescape bool) (string, error) {
	var sb strings.Builder
	for i := 1; i < len(v); i++ {
		c := v[i]
		if unescape && c == '\\' && i+1 < len(v) {
			i++
			switch v[i] {
			case 'n':
				sb.WriteByte('\n')
			case 'r':
				sb.WriteByte('\r')
			case 't':
				sb.WriteByte('\t')
			case '\\', '"', '\'':
				sb.WriteByte(v[i])
			default:
				sb.WriteByte('\\')
				sb.WriteByte(v[i])
			}
			continue
		}
		if c == quote {
			rest := strings.TrimSpace(v[i+1:])
			if rest != "" && !strings.HasPrefix(rest, "#") {
				return "", fmt.Errorf("闭合引号后存在多余内容")
			}
			return sb.String(), nil
		}
		sb.WriteByte(c)
	}
	return "", fmt.Errorf("引号未闭合（多行值不受支持）")
}
