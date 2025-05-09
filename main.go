package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"gcp-wli-token-fetcher/internal/metadata"
	"gcp-wli-token-fetcher/internal/tools"
	"gcp-wli-token-fetcher/internal/types"

	"github.com/robfig/cron/v3"
)

func main() {
	ctx := context.Background()

	var version = flag.Bool("version", false, "Display version information")

	var tokenFile = flag.String("file", os.Getenv("TOKEN_FILE"), "File path for a token file. Example /data/oicd/token. (Required). Envar TOKEN_FILE")
	var GSAName = flag.String("gsaname", os.Getenv("GSA_NAME"), "Google Service account email. Example name@myproject.iam.gserviceaccount.com. (Required). Envar GSA_NAME")
	var audience = flag.String("audience", os.Getenv("TOKEN_AUDIENCE"), "Service account identity audience. Example AzureADTokenExchange. (Required). Envar TOKEN_AUDIENCE")
	var scope = flag.String("scope", os.Getenv("TOKEN_SCOPE"), "Service account identity scope. Example user_impersonation. (Required). Envar TOKEN_SCOPE")
	var cronspec = flag.String("cronspec", tools.GetEnv("CRON_SPEC", "* * * * *"), "Schedule for token renewal routine. Default \"* * * * *\". Envar CRON_SPEC")
	var renewthreshold = flag.String("renewthreshold", tools.GetEnv("TOKEN_RENEW_THRESHOLD", "30m0s"), "Token TTL threshold. The token will be renewed if below this value. Default \"30m00s\". Envar TOKEN_RENEW_THRESHOLD")
	flag.Parse()
	var metadataserverurl = flag.String("metadataserverurl", tools.GetEnv("METADATA_SERVER_URL", "http://metadata.google.internal"), "Metadata server URL. Default \"http://metadata.google.internal/\". Envar METADATA_SERVER_URL")
	flag.Parse()

	if *version {
		fmt.Printf("Version: %s\nCommit: %s\nBuild Date: %s\n", types.Version, types.Commit, types.Date)
		os.Exit(0)
	}

	if *tokenFile == "" || *GSAName == "" || *audience == "" || *scope == "" {
		fmt.Println("Mandatory arguments missing. Usage:")
		flag.PrintDefaults()
		os.Exit(1)
	}

	ctx = context.WithValue(ctx, "renewthreshold", *renewthreshold)

	i := metadata.GSA{
		Name:     *GSAName,
		Audience: *audience,
		Scope:    *scope,
	}

	// Trigger renewal on startup
	i.RefreshToken(ctx, *tokenFile, *metadataserverurl)

	c := cron.New()
	c.AddFunc(*cronspec, func() {
		i.RefreshToken(ctx, *tokenFile, *metadataserverurl)
	})
	c.Start()
	time.Sleep(time.Duration(1<<63 - 1))
	c.Stop()
}
