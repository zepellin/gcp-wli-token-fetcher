package main

import (
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestNewConfig(t *testing.T) {
	valid := func() (config, error) {
		return newConfig("/tmp/token", "gsa@example.iam.gserviceaccount.com", "aud", "scope", "http://metadata.google.internal", "1m", "30m")
	}

	t.Run("valid", func(t *testing.T) {
		cfg, err := valid()
		if err != nil {
			t.Fatalf("newConfig() returned an error: %v", err)
		}
		if cfg.interval != time.Minute {
			t.Errorf("interval = %s, want 1m", cfg.interval)
		}
		if cfg.renewThreshold != 30*time.Minute {
			t.Errorf("renewThreshold = %s, want 30m", cfg.renewThreshold)
		}
	})

	t.Run("scope is optional", func(t *testing.T) {
		if _, err := newConfig("/tmp/token", "gsa", "aud", "", "url", "1m", "30m"); err != nil {
			t.Errorf("newConfig() returned an error for an empty scope: %v", err)
		}
	})

	t.Run("missing required arguments", func(t *testing.T) {
		_, err := newConfig("", "", "aud", "scope", "http://metadata.google.internal", "1m", "30m")
		if err == nil {
			t.Fatal("newConfig() did not return an error for missing arguments")
		}
		for _, flag := range []string{"-file", "-gsaname"} {
			if !strings.Contains(err.Error(), flag) {
				t.Errorf("error %q should mention %s", err, flag)
			}
		}
	})

	t.Run("invalid interval", func(t *testing.T) {
		if _, err := newConfig("/tmp/token", "gsa", "aud", "scope", "url", "* * * * *", "30m"); err == nil {
			t.Error("newConfig() did not return an error for a non-duration interval")
		}
		if _, err := newConfig("/tmp/token", "gsa", "aud", "scope", "url", "-1m", "30m"); err == nil {
			t.Error("newConfig() did not return an error for a negative interval")
		}
	})

	t.Run("invalid renewthreshold", func(t *testing.T) {
		if _, err := newConfig("/tmp/token", "gsa", "aud", "scope", "url", "1m", "soon"); err == nil {
			t.Error("newConfig() did not return an error for a non-duration renewthreshold")
		}
	})
}

func TestCheckTokenRequiresFile(t *testing.T) {
	err := checkToken("")
	if err == nil || !strings.Contains(err.Error(), "-file") {
		t.Errorf("checkToken(\"\") = %v, want an error mentioning -file", err)
	}
}

func TestNewLogger(t *testing.T) {
	testCases := []struct {
		level   string
		enabled slog.Level
	}{
		{"DEBUG", slog.LevelDebug},
		{"debug", slog.LevelDebug},
		{"INFO", slog.LevelInfo},
		{"WARN", slog.LevelWarn},
		{"error", slog.LevelError},
		{"", slog.LevelInfo},
		{"UNKNOWN", slog.LevelInfo},
	}

	for _, tc := range testCases {
		t.Run("level "+tc.level, func(t *testing.T) {
			logger := newLogger(tc.level)
			if !logger.Enabled(t.Context(), tc.enabled) {
				t.Errorf("newLogger(%q) should log at %s", tc.level, tc.enabled)
			}
			if logger.Enabled(t.Context(), tc.enabled-1) {
				t.Errorf("newLogger(%q) should not log below %s", tc.level, tc.enabled)
			}
		})
	}
}

func TestGetEnv(t *testing.T) {
	t.Setenv("EXISTING_KEY", "existing_value")
	if got := getEnv("EXISTING_KEY", "fallback"); got != "existing_value" {
		t.Errorf("getEnv(EXISTING_KEY) = %s, want existing_value", got)
	}
	if got := getEnv("NON_EXISTING_KEY", "fallback"); got != "fallback" {
		t.Errorf("getEnv(NON_EXISTING_KEY) = %s, want fallback", got)
	}
}
