package log

import (
	"log/slog"
	"os"
)

func getLogLevel(level string) slog.Level {
	switch level {
	case "DEBUG":
		return slog.LevelDebug
	case "INFO":
		return slog.LevelInfo
	case "WARN":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

var opts = &slog.HandlerOptions{
	Level: getLogLevel(os.Getenv("LOG_LEVEL")),
}

var handler = slog.NewJSONHandler(os.Stdout, opts)
var Logger = slog.New(handler)
