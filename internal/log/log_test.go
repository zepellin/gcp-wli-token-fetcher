package log

import (
	"os"
	"testing"

	"log/slog"

	"github.com/stretchr/testify/assert"
)

func TestGetLogLevel(t *testing.T) {
	testCases := []struct {
		level    string
		expected slog.Level
	}{
		{level: "DEBUG", expected: slog.LevelDebug},
		{level: "INFO", expected: slog.LevelInfo},
		{level: "WARN", expected: slog.LevelWarn},
		{level: "ERROR", expected: slog.LevelError},
		{level: "UNKNOWN", expected: slog.LevelInfo},
		{level: "debug", expected: slog.LevelDebug},
		{level: "info", expected: slog.LevelInfo},
		{level: "warn", expected: slog.LevelWarn},
		{level: "error", expected: slog.LevelError},
		{level: "unknown", expected: slog.LevelInfo},
	}

	for _, tc := range testCases {
		t.Run(tc.level, func(t *testing.T) {
			result := getLogLevel(tc.level)
			assert.Equal(t, tc.expected, result)
		})
	}
}
func TestGetLogLevelFromEnv(t *testing.T) {
	// Set up test cases
	testCases := []struct {
		envValue string
		expected slog.Level
	}{
		{envValue: "DEBUG", expected: slog.LevelDebug},
		{envValue: "INFO", expected: slog.LevelInfo},
		{envValue: "WARN", expected: slog.LevelWarn},
		{envValue: "ERROR", expected: slog.LevelError},
		{envValue: "UNKNOWN", expected: slog.LevelInfo},
		{envValue: "debug", expected: slog.LevelDebug},
		{envValue: "info", expected: slog.LevelInfo},
		{envValue: "warn", expected: slog.LevelWarn},
		{envValue: "error", expected: slog.LevelError},
		{envValue: "unknown", expected: slog.LevelInfo},
	}

	// Run test cases
	for _, tc := range testCases {
		t.Run(tc.envValue, func(t *testing.T) {
			// Set up environment variable
			os.Setenv("LOG_LEVEL", tc.envValue)
			defer os.Unsetenv("LOG_LEVEL")

			// Call the function under test
			result := getLogLevel(os.Getenv("LOG_LEVEL"))

			// Check the result
			assert.Equal(t, tc.expected, result)
		})
	}
}
