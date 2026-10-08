package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"gcp-wli-token-fetcher/internal/metadata"
)

// Version information set by ldflags during build.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

type config struct {
	tokenFile         string
	gsaName           string
	audience          string
	scope             string
	metadataServerURL string
	interval          time.Duration
	renewThreshold    time.Duration
}

func main() {
	showVersion := flag.Bool("version", false, "Display version information")
	check := flag.Bool("check", false, "Exit 0 if the token file holds an unexpired token, 1 otherwise. Only -file is required. Intended for use as an exec startupProbe")
	tokenFile := flag.String("file", os.Getenv("TOKEN_FILE"), "File path for a token file. Example /data/oidc/token. (Required). Envar TOKEN_FILE")
	gsaName := flag.String("gsaname", os.Getenv("GSA_NAME"), "Google Service account email. Example name@myproject.iam.gserviceaccount.com. (Required). Envar GSA_NAME")
	audience := flag.String("audience", os.Getenv("TOKEN_AUDIENCE"), "Service account identity audience. Example AzureADTokenExchange. (Required). Envar TOKEN_AUDIENCE")
	scope := flag.String("scope", os.Getenv("TOKEN_SCOPE"), "Service account identity scope. Example user_impersonation. Omitted from the request when empty. Envar TOKEN_SCOPE")
	interval := flag.String("interval", getEnv("RENEW_INTERVAL", "1m"), "How often to check the token for renewal. Default \"1m\". Envar RENEW_INTERVAL")
	renewThreshold := flag.String("renewthreshold", getEnv("TOKEN_RENEW_THRESHOLD", "30m"), "Token TTL threshold. The token will be renewed if below this value. Default \"30m\". Envar TOKEN_RENEW_THRESHOLD")
	metadataServerURL := flag.String("metadataserverurl", getEnv("METADATA_SERVER_URL", "http://metadata.google.internal"), "Metadata server URL. Default \"http://metadata.google.internal\". Envar METADATA_SERVER_URL")
	flag.Parse()

	if *showVersion {
		fmt.Printf("Version: %s\nCommit: %s\nBuild Date: %s\n", version, commit, date)
		return
	}

	if *check {
		if err := checkToken(*tokenFile); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	slog.SetDefault(newLogger(os.Getenv("LOG_LEVEL")))

	cfg, err := newConfig(*tokenFile, *gsaName, *audience, *scope, *metadataServerURL, *interval, *renewThreshold)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n\nUsage:\n", err)
		flag.PrintDefaults()
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, cfg); err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
}

// newConfig validates the raw flag values and parses durations.
func newConfig(tokenFile, gsaName, audience, scope, metadataServerURL, interval, renewThreshold string) (config, error) {
	var missing []string
	for name, value := range map[string]string{
		"-file":     tokenFile,
		"-gsaname":  gsaName,
		"-audience": audience,
	} {
		if value == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return config{}, fmt.Errorf("mandatory arguments missing: %s", strings.Join(missing, ", "))
	}

	parsedInterval, err := time.ParseDuration(interval)
	if err != nil {
		return config{}, fmt.Errorf("invalid interval %q: %w", interval, err)
	}
	if parsedInterval <= 0 {
		return config{}, fmt.Errorf("interval must be positive, got %q", interval)
	}

	parsedThreshold, err := time.ParseDuration(renewThreshold)
	if err != nil {
		return config{}, fmt.Errorf("invalid renewthreshold %q: %w", renewThreshold, err)
	}

	return config{
		tokenFile:         tokenFile,
		gsaName:           gsaName,
		audience:          audience,
		scope:             scope,
		metadataServerURL: metadataServerURL,
		interval:          parsedInterval,
		renewThreshold:    parsedThreshold,
	}, nil
}

// checkToken reports whether tokenFile holds an unexpired token.
func checkToken(tokenFile string) error {
	if tokenFile == "" {
		return errors.New("mandatory arguments missing: -file")
	}
	return metadata.CheckToken(tokenFile)
}

// run refreshes the token once at startup and then on every interval tick
// until ctx is cancelled.
func run(ctx context.Context, cfg config) error {
	gsa := metadata.GSA{
		Name:     cfg.gsaName,
		Audience: cfg.audience,
		Scope:    cfg.scope,
	}

	refresh := func() {
		if err := gsa.RefreshToken(ctx, cfg.tokenFile, cfg.metadataServerURL, cfg.renewThreshold); err != nil {
			slog.Error("token refresh failed", "error", err)
		}
	}
	refresh()

	ticker := time.NewTicker(cfg.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			slog.Info("shutting down")
			return nil
		case <-ticker.C:
			refresh()
		}
	}
}

// newLogger builds a JSON logger at the given level ("DEBUG", "INFO", "WARN",
// "ERROR", case-insensitive). Unknown or empty values fall back to INFO.
func newLogger(level string) *slog.Logger {
	var l slog.Level
	if err := l.UnmarshalText([]byte(level)); err != nil {
		l = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: l}))
}

// getEnv returns the value of the environment variable key, or fallback if it
// is unset.
func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
