package core

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
)

var (
	// Logger 是全局结构化日志器，随 InitLogger 初始化。
	Logger *slog.Logger

	// logLevel 是运行期可调的日志级别：serve 控制台的 set LOG_LEVEL 直接
	// 调整它，handler 与 Logger 无需重建。
	logLevel = new(slog.LevelVar)
)

func init() {
	Logger = slog.Default()
	logLevel.Set(slog.LevelInfo)
}

// InitLogger 按 level 初始化全局日志器；level 非法时回退 info。
func InitLogger(level string) {
	l, ok := parseLogLevel(level)
	if !ok {
		l = slog.LevelInfo
	}
	logLevel.Set(l)
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level:     logLevel,
		AddSource: l == slog.LevelDebug,
	})
	Logger = slog.New(handler)
	slog.SetDefault(Logger)
}

// SetLogLevel 运行期调整日志级别（serve 控制台命令），非法值返回错误。
func SetLogLevel(level string) error {
	l, ok := parseLogLevel(level)
	if !ok {
		return fmt.Errorf("日志级别必须为 debug / info / warn / error，当前=%q", level)
	}
	logLevel.Set(l)
	return nil
}

// LogLevel 返回当前生效的日志级别名（小写，如 info）。
func LogLevel() string {
	return strings.ToLower(logLevel.Level().String())
}

func parseLogLevel(level string) (slog.Level, bool) {
	switch level {
	case "debug":
		return slog.LevelDebug, true
	case "info":
		return slog.LevelInfo, true
	case "warn":
		return slog.LevelWarn, true
	case "error":
		return slog.LevelError, true
	}
	return slog.LevelInfo, false
}
