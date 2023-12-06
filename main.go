package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gcp-wli-token-fetcher/internal/log"
	"gcp-wli-token-fetcher/internal/metadata"
	"gcp-wli-token-fetcher/internal/tools"

	"github.com/robfig/cron/v3"
)

func refreshToken(ctx context.Context, tokenFile, GSAName, audience, scope string) error {
	fileExists, err := tools.FileExists(tokenFile)
	if err != nil {
		log.Logger.Error(err.Error())

		os.Exit(2)
	}

	if fileExists {
		token, err := os.ReadFile(tokenFile)
		if err != nil {
			log.Logger.Error(err.Error())
			return err
		}

		expIn, err := tools.TokenExpiresIn(string(token))
		if err != nil {
			log.Logger.Error(err.Error())

		}
		d, _ := time.ParseDuration(ctx.Value("renewthreshold").(string))
		if expIn > d {
			log.Logger.Debug(fmt.Sprintf("Token expires in %s (threshold %s), not scheduling for renewal", expIn, d))
			return nil
		} else {
			log.Logger.Info(fmt.Sprintf("Token expires in %s (threshold %s), scheduling for renewal", expIn, d))
		}
	} else {
		dir, _ := filepath.Split(tokenFile)

		err := os.MkdirAll(dir, os.ModePerm)
		if err != nil {
			log.Logger.Error(err.Error())
		}
	}

	token, err := metadata.GetGSAToken(GSAName, audience, scope)
	if err != nil {
		log.Logger.Error(err.Error())
		return err
	}

	f, err := os.OpenFile(tokenFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		log.Logger.Error(err.Error())
		return err
	}
	log.Logger.Debug(fmt.Sprintf("Writing token for service account %s into %s", GSAName, tokenFile))
	f.Write(token)
	f.Close()
	return nil
}

func main() {
	ctx := context.Background()

	var tokenFile = flag.String("file", os.Getenv("TOKEN_FILE"), "File path for a token file. Example /data/oicd/token. (Required). Envar TOKEN_FILE")
	var GSAName = flag.String("gsaname", os.Getenv("GSA_NAME"), "Google Service account email. Example name@myproject.iam.gserviceaccount.com. (Required). Envar GSA_NAME")
	var audience = flag.String("audience", os.Getenv("TOKEN_AUDIENCE"), "Service account identity audience. Example AzureADTokenExchange. (Required). Envar TOKEN_AUDIENCE")
	var scope = flag.String("scope", os.Getenv("TOKEN_SCOPE"), "Service account identity scope. Example user_impersonation. (Required). Envar TOKEN_SCOPE")
	var cronspec = flag.String("cronspec", tools.GetEnv("CRON_SPEC", "* * * * *"), "Schedule for token renewal routine. Default \"* * * * *\". Envar CRON_SPEC")
	var renewthreshold = flag.String("renewthreshold", tools.GetEnv("TOKEN_RENEW_THRESHOLD", "30m0s"), "Token TTL threshold. The token will be renewed if below this value. Default \"30m00s\". Envar TOKEN_RENEW_THRESHOLD")
	flag.Parse()

	if *tokenFile == "" || *GSAName == "" || *audience == "" || *scope == "" {
		fmt.Println("Mandatory arguments missing. Usage:")
		flag.PrintDefaults()
		os.Exit(1)
	}

	ctx = context.WithValue(ctx, "renewthreshold", *renewthreshold)

	refreshToken(ctx, *tokenFile, *GSAName, *audience, *scope)

	c := cron.New()
	c.AddFunc(*cronspec, func() {
		refreshToken(ctx, *tokenFile, *GSAName, *audience, *scope)
	})
	c.Start()
	time.Sleep(time.Duration(1<<63 - 1))
	c.Stop()
}
