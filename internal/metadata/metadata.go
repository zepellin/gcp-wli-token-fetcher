package metadata

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// GSA identifies the Google Service Account to fetch identity tokens for.
type GSA struct {
	Name     string // Name of the GSA (Google Service Account)
	Audience string // Audience for the GSA token
	Scope    string // Optional scope for the GSA token, omitted from the request when empty
}

var client = &http.Client{Timeout: 10 * time.Second}

// RefreshToken ensures the token file contains a token that stays valid for
// at least renewThreshold. It fetches a new token from the metadata server
// when the existing one is missing, unparseable, or expiring soon, and writes
// it atomically so readers never observe a partial token.
func (g *GSA) RefreshToken(ctx context.Context, tokenFile, metadataServerURL string, renewThreshold time.Duration) error {
	if existing, err := os.ReadFile(tokenFile); err == nil {
		expIn, err := tokenExpiresIn(string(existing))
		switch {
		case err != nil:
			slog.Warn("could not parse existing token, renewing", "error", err)
		case expIn > renewThreshold:
			slog.Debug("token still valid, skipping renewal", "expiresIn", expIn.String(), "threshold", renewThreshold.String())
			return nil
		default:
			slog.Info("token expiring soon, renewing", "expiresIn", expIn.String(), "threshold", renewThreshold.String())
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("reading token file: %w", err)
	}

	token, err := g.token(ctx, metadataServerURL)
	if err != nil {
		return err
	}

	slog.Debug("writing token", "gsa", g.Name, "file", tokenFile)
	return writeFileAtomic(tokenFile, token)
}

// CheckToken returns nil if tokenFile holds a JWT that has not yet expired.
func CheckToken(tokenFile string) error {
	data, err := os.ReadFile(tokenFile)
	if err != nil {
		return fmt.Errorf("reading token file: %w", err)
	}
	expIn, err := tokenExpiresIn(string(data))
	if err != nil {
		return err
	}
	if expIn <= 0 {
		return fmt.Errorf("token expired %s ago", (-expIn).Round(time.Second))
	}
	return nil
}

// token fetches an identity token for the service account from the metadata
// server.
func (g *GSA) token(ctx context.Context, metadataServerURL string) ([]byte, error) {
	query := url.Values{"audience": {g.Audience}}
	if g.Scope != "" {
		query.Set("scope", g.Scope)
	}
	requestURL := fmt.Sprintf("%s/computeMetadata/v1/instance/service-accounts/%s/identity?%s",
		metadataServerURL, url.PathEscape(g.Name), query.Encode())
	slog.Debug("requesting token from metadata server", "gsa", g.Name, "url", requestURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Metadata-Flavor", "Google")

	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("metadata server request failed: %w", err)
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("reading metadata server response: %w", err)
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("metadata server returned status %d: %s", res.StatusCode, body)
	}
	return body, nil
}

// tokenExpiresIn returns the remaining time until the JWT expires. The token
// signature is not verified; only the exp claim is inspected.
func tokenExpiresIn(token string) (time.Duration, error) {
	claims := jwt.MapClaims{}
	if _, _, err := jwt.NewParser().ParseUnverified(token, claims); err != nil {
		return 0, fmt.Errorf("parsing token: %w", err)
	}
	exp, err := claims.GetExpirationTime()
	if err != nil {
		return 0, fmt.Errorf("reading exp claim: %w", err)
	}
	if exp == nil {
		return 0, errors.New("token has no exp claim")
	}
	return time.Until(exp.Time), nil
}

// writeFileAtomic writes data to path via a temp file and rename, creating
// parent directories as needed. The file is world-readable (0644) because
// the token is typically consumed by another container running as a
// different user.
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating token directory: %w", err)
	}

	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("creating temp token file: %w", err)
	}
	defer os.Remove(tmp.Name())

	if err := writeAndClose(tmp, data); err != nil {
		return fmt.Errorf("writing token file: %w", err)
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return fmt.Errorf("replacing token file: %w", err)
	}
	return nil
}

func writeAndClose(f *os.File, data []byte) error {
	if err := f.Chmod(0o644); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}
