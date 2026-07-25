package core

import (
	"log/slog"
	"os"
)

var Logger *slog.Logger

func init() {
	Logger = slog.Default()
}

func InitLogger(level string) {
	var l slog.Level
	switch level {
	case "debug":
		l = slog.LevelDebug
	case "warn":
		l = slog.LevelWarn
	case "error":
		l = slog.LevelError
	default:
		l = slog.LevelInfo
	}
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level:     l,
		AddSource: l == slog.LevelDebug,
	})
	Logger = slog.New(handler)
	slog.SetDefault(Logger)
}
