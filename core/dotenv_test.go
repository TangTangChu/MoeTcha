package core

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestParseDotEnv(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  map[string]string
	}{
		{
			name:  "基础键值",
			input: "HTTP_PORT=8080\nLOG_LEVEL=debug\n",
			want:  map[string]string{"HTTP_PORT": "8080", "LOG_LEVEL": "debug"},
		},
		{
			name:  "注释与空行",
			input: "# 这是注释\n\n  # 缩进注释\nHTTP_PORT=8080\n",
			want:  map[string]string{"HTTP_PORT": "8080"},
		},
		{
			name:  "export 前缀",
			input: "export HTTP_PORT=8080\n",
			want:  map[string]string{"HTTP_PORT": "8080"},
		},
		{
			name:  "CRLF 换行",
			input: "HTTP_PORT=8080\r\nLOG_LEVEL=info\r\n",
			want:  map[string]string{"HTTP_PORT": "8080", "LOG_LEVEL": "info"},
		},
		{
			name:  "UTF-8 BOM",
			input: "\ufeffHTTP_PORT=8080\n",
			want:  map[string]string{"HTTP_PORT": "8080"},
		},
		{
			name:  "双引号与转义",
			input: "A=\"hello world\"\nB=\"line\\nbreak\"\nC=\"quo\\\"te\"\n",
			want:  map[string]string{"A": "hello world", "B": "line\nbreak", "C": `quo"te`},
		},
		{
			name:  "单引号为字面量",
			input: "A='no\\nescape'\n",
			want:  map[string]string{"A": `no\nescape`},
		},
		{
			name:  "未加引号的行尾注释",
			input: "HTTP_PORT=8080 # 端口\n",
			want:  map[string]string{"HTTP_PORT": "8080"},
		},
		{
			name:  "值内部的井号不算注释",
			input: "PASS=abc#def\n",
			want:  map[string]string{"PASS": "abc#def"},
		},
		{
			name:  "引号后的注释",
			input: "A=\"v\" # 说明\n",
			want:  map[string]string{"A": "v"},
		},
		{
			name:  "空值",
			input: "API_TOKENS=\nHTTP_PORT=8080\n",
			want:  map[string]string{"API_TOKENS": "", "HTTP_PORT": "8080"},
		},
		{
			name:  "键两侧空白",
			input: "  HTTP_PORT  =  8080  \n",
			want:  map[string]string{"HTTP_PORT": "8080"},
		},
		{
			name:  "重复键以最后一次为准",
			input: "HTTP_PORT=1\nHTTP_PORT=2\n",
			want:  map[string]string{"HTTP_PORT": "2"},
		},
		{
			name:  "值中包含等号",
			input: "TOKEN=a=b=c\n",
			want:  map[string]string{"TOKEN": "a=b=c"},
		},
		{
			name:  "无末尾换行",
			input: "HTTP_PORT=8080",
			want:  map[string]string{"HTTP_PORT": "8080"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entries, err := ParseDotEnv(strings.NewReader(tt.input), "test")
			if err != nil {
				t.Fatalf("ParseDotEnv 出错：%v", err)
			}
			got := DotEnvMap(entries)
			if len(got) != len(tt.want) {
				t.Fatalf("键数量 = %d，期望 %d（got=%v）", len(got), len(tt.want), got)
			}
			for k, want := range tt.want {
				if got[k] != want {
					t.Errorf("%s = %q，期望 %q", k, got[k], want)
				}
			}
		})
	}
}

func TestParseDotEnvErrors(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"缺少等号", "HTTP_PORT 8080\n"},
		{"双引号未闭合", "A=\"unterminated\n"},
		{"单引号未闭合", "A='unterminated\n"},
		{"引号后有多余内容", "A=\"v\" junk\n"},
		{"键名为空", "=8080\n"},
		{"键名含非法字符", "HTTP-PORT=8080\n"},
		{"键名以数字开头", "1PORT=8080\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ParseDotEnv(strings.NewReader(tt.input), "test"); err == nil {
				t.Fatal("期望报错，实际通过")
			}
		})
	}
}

// 引号未闭合必须报错而非当作字面量：静默把残缺的签名密钥读进配置属于安全故障。
func TestUnclosedQuoteIsNotSilentlyAccepted(t *testing.T) {
	_, err := ParseDotEnv(strings.NewReader("CAPTCHA_TOKEN_SIGNING_KEY=\"abc\n"), "test")
	if err == nil {
		t.Fatal("引号未闭合应当报错")
	}
	if !strings.Contains(err.Error(), "引号未闭合") {
		t.Errorf("错误信息应说明引号未闭合，实际=%q", err.Error())
	}
}

// 错误信息不得回显值，否则密钥会顺着日志泄露。
func TestDotEnvErrorDoesNotLeakValue(t *testing.T) {
	const secret = "s3cr3t-signing-key"
	_, err := ParseDotEnv(strings.NewReader("CAPTCHA_TOKEN_SIGNING_KEY=\""+secret+"\n"), "test")
	if err == nil {
		t.Fatal("期望报错")
	}
	if strings.Contains(err.Error(), secret) {
		t.Errorf("错误信息泄露了值：%q", err.Error())
	}
}

func TestLoadDotEnvFileMissingIsNotAnError(t *testing.T) {
	entries, err := LoadDotEnvFile(filepath.Join(t.TempDir(), "does-not-exist.env"))
	if err != nil {
		t.Fatalf("文件不存在应视为正常情况，实际报错：%v", err)
	}
	if entries != nil {
		t.Errorf("期望 nil，实际=%v", entries)
	}
}

func TestDotEnvMapEmpty(t *testing.T) {
	if got := DotEnvMap(nil); got != nil {
		t.Errorf("空输入应返回 nil，实际=%v", got)
	}
}
