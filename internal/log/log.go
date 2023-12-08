package log

import (
	"log/slog"
	"os"
	"strings"
)

// getLogLevel returns the corresponding slog.Level based on the provided level string.
// If the level string is "DEBUG", it returns slog.LevelDebug.
// If the level string is "INFO", it returns slog.LevelInfo.
// If the level string is "WARN", it returns slog.LevelWarn.
// If the level string is "ERROR", it returns slog.LevelError.
// For any other level string, it returns slog.LevelInfo.
func getLogLevel(level string) slog.Level {
	switch strings.ToUpper(level) {
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

// Logger is a global logger instance that can be used for logging messages.
var Logger = slog.New(handler)
