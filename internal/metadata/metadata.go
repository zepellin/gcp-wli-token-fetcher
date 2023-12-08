package metadata

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"gcp-wli-token-fetcher/internal/log"
	"gcp-wli-token-fetcher/internal/tools"
)

type GSA struct {
	Name     string // Name of the GSA (Google Service Account)
	Audience string // Audience for the GSA token
	Scope    string // Scope for the GSA token
}

var (
	metadataRequestFail = errors.New("Metadata server request failed")
)

// Token retrieves a token for the service account.
// It takes the metadata server URL as a parameter and returns the token as a byte array and an error, if any.
// The function constructs a request URL using the service account name, audience, and scope.
// It then sends a GET request to the metadata server with the constructed URL and retrieves the token.
// The retrieved token is returned as a byte array.
// If there is an error during the request or reading the response body, an empty byte array and the error are returned.
func (g *GSA) token(metadataServerURL string) ([]byte, error) {
	log.Logger.Debug(fmt.Sprintf("Retrieving token for service account %s with audience %s and scope %s", g.Name, g.Audience, g.Scope))

	requestURL := fmt.Sprintf("%s/computeMetadata/v1/instance/service-accounts/%s/identity?audience=%s&scope=%s", metadataServerURL, g.Name, g.Audience, g.Scope)
	log.Logger.Debug(fmt.Sprintf("Making HTTP request to %s", requestURL))

	client := &http.Client{}
	req, err := http.NewRequest("GET", requestURL, nil)
	if err != nil {
		return []byte{}, err
	}
	req.Header.Set("Metadata-Flavor", "Google")
	res, err := client.Do(req)
	if err != nil || res.StatusCode != 200 {
		body, _ := io.ReadAll(res.Body)
		log.Logger.Error(fmt.Sprintf("HTTP request error, code %d, message %s", res.StatusCode, body))
		return []byte{}, metadataRequestFail
	}
	defer res.Body.Close()

	if b, err := io.ReadAll(res.Body); err == nil {
		return b, nil
	}
	log.Logger.Debug(fmt.Sprintf("Retrieved token for service account %s with audience %s and scope %s successfully", g.Name, g.Audience, g.Scope))
	log.Logger.Debug(fmt.Sprintf("Token: %s", res.Body))
	return []byte{}, err
}

// RefreshToken refreshes the token for the service account.
// It reads the token from the specified token file, checks if it is expired,
// and schedules it for renewal if necessary. If the token file does not exist,
// it creates the necessary directory structure and fetches a new token from
// the metadata server. The refreshed token is then written back to the token file.
//
// Parameters:
// - ctx: The context.Context object for the operation.
// - tokenFile: The path to the token file.
// - metadataServerURL: The URL of the metadata server.
//
// Returns:
// - error: An error if any occurred during the token refresh process.
func (g *GSA) RefreshToken(ctx context.Context, tokenFile, metadataServerURL string) error {
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
			// If the token cannot be parsed, set the expiration time to 0
			expIn = time.Duration(0)
		}

		d, err := time.ParseDuration(ctx.Value("renewthreshold").(string))
		if err != nil {
			log.Logger.Error(fmt.Sprintf("Failed to parse renewalthreshold as duration %s", ctx.Value("renewthreshold").(string)))
		}

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

	token, err := g.token(metadataServerURL)
	if err != nil {
		log.Logger.Error(err.Error())
		return err
	}

	f, err := os.OpenFile(tokenFile, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		log.Logger.Error(err.Error())
		return err
	}
	log.Logger.Debug(fmt.Sprintf("Writing token for service account %s into %s", g.Name, tokenFile))
	f.Write(token)
	f.Close()
	return nil
}
