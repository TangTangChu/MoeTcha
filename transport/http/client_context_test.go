package http

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"moetcha/core"
)

func newIPCtx(remote string, headers map[string]string) *gin.Context {
	gin.SetMode(gin.TestMode)
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = remote
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req
	return c
}

func TestPickXFF(t *testing.T) {
	xff := "10.0.0.1, 10.0.0.2, 10.0.0.3"
	tests := []struct {
		idx  int
		want string
	}{
		{0, "10.0.0.1"}, {1, "10.0.0.2"}, {2, "10.0.0.3"},
		{-1, "10.0.0.3"}, {-3, "10.0.0.1"},
		{3, ""}, {-4, ""}, // 越界
	}
	for _, tt := range tests {
		if got := pickXFF(xff, tt.idx); got != tt.want {
			t.Errorf("pickXFF(%q, %d) = %q, want %q", xff, tt.idx, got, tt.want)
		}
	}
	if got := pickXFF("", 0); got != "" {
		t.Errorf("空 XFF 应返回空，得到 %q", got)
	}
	if got := pickXFF("  ,  , ", 0); got != "" {
		t.Errorf("全空白 XFF 应返回空，得到 %q", got)
	}
}

func TestResolveClientIP(t *testing.T) {
	tests := []struct {
		name    string
		cfg     core.IPResolveConfig
		remote  string
		headers map[string]string
		want    string
	}{
		{
			name:    "direct 忽略所有头",
			cfg:     core.IPResolveConfig{Source: "direct"},
			remote:  "5.5.5.5:1234",
			headers: map[string]string{"X-Forwarded-For": "1.1.1.1", "X-Real-IP": "2.2.2.2"},
			want:    "5.5.5.5",
		},
		{
			name:    "xff 默认取最左（原始客户端）",
			cfg:     core.IPResolveConfig{Source: "x-forwarded-for", XFFIndex: 0},
			remote:  "5.5.5.5:1234",
			headers: map[string]string{"X-Forwarded-For": "1.1.1.1, 5.5.5.5"},
			want:    "1.1.1.1",
		},
		{
			name:    "xff -1 取最右（上一跳）",
			cfg:     core.IPResolveConfig{Source: "x-forwarded-for", XFFIndex: -1},
			remote:  "5.5.5.5:1234",
			headers: map[string]string{"X-Forwarded-For": "1.1.1.1, 5.5.5.5"},
			want:    "5.5.5.5",
		},
		{
			name:    "xff 索引越界回落直连",
			cfg:     core.IPResolveConfig{Source: "x-forwarded-for", XFFIndex: 9},
			remote:  "5.5.5.5:1234",
			headers: map[string]string{"X-Forwarded-For": "1.1.1.1"},
			want:    "5.5.5.5",
		},
		{
			name:   "xff 缺头回落直连",
			cfg:    core.IPResolveConfig{Source: "x-forwarded-for"},
			remote: "5.5.5.5:1234",
			want:   "5.5.5.5",
		},
		{
			name:    "xff 值非法回落直连",
			cfg:     core.IPResolveConfig{Source: "x-forwarded-for"},
			remote:  "5.5.5.5:1234",
			headers: map[string]string{"X-Forwarded-For": "not-an-ip"},
			want:    "5.5.5.5",
		},
		{
			name:    "x-real-ip",
			cfg:     core.IPResolveConfig{Source: "x-real-ip"},
			remote:  "5.5.5.5:1234",
			headers: map[string]string{"X-Real-IP": "9.9.9.9"},
			want:    "9.9.9.9",
		},
		{
			name:    "x-real-ip 非法回落直连",
			cfg:     core.IPResolveConfig{Source: "x-real-ip"},
			remote:  "5.5.5.5:1234",
			headers: map[string]string{"X-Real-IP": "garbage"},
			want:    "5.5.5.5",
		},
		{
			name:   "x-real-ip 缺头回落直连",
			cfg:    core.IPResolveConfig{Source: "x-real-ip"},
			remote: "5.5.5.5:1234",
			want:   "5.5.5.5",
		},
		{
			name:    "未知 source 回落直连",
			cfg:     core.IPResolveConfig{Source: "weird"},
			remote:  "5.5.5.5:1234",
			headers: map[string]string{"X-Forwarded-For": "1.1.1.1"},
			want:    "5.5.5.5",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newIPCtx(tt.remote, tt.headers)
			if got := resolveClientIP(c, tt.cfg); got != tt.want {
				t.Errorf("resolveClientIP() = %q, want %q", got, tt.want)
			}
		})
	}
}
