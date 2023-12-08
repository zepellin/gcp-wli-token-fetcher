package tools

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"gcp-wli-token-fetcher/internal/log"

	"github.com/tidwall/gjson"
)

var (
	jwtParseFail  = errors.New("Failed to parse JWT")
	jwtDecodeFail = errors.New("Failed to decode JWT")
)

// GetEnv retrieves the value of the environment variable specified by the key parameter.
// If the environment variable is not found, it returns the fallback value.
func GetEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

// FileExists checks if a file exists at the specified file path.
// It returns true if the file exists, false otherwise.
// If an error occurs during the file check, it returns false and the error.
func FileExists(filePath string) (bool, error) {
	info, err := os.Stat(filePath)

	if os.IsNotExist(err) {
		return false, nil
	}

	if err != nil {
		return false, err
	}

	if info.IsDir() {
		return false, fmt.Errorf("'%s' is a directory", filePath)
	}

	return true, nil
}

// getJWTExp parses a JWT token and returns its expiration time.
// It expects a token string in the format "header.payload.signature".
// If the token is not in the expected format or if the expiration time cannot be retrieved, an error is returned.
// The expiration time is returned in UTC.
func getJWTExp(token string) (time.Time, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return time.Time{}, jwtParseFail
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])

	if err != nil {
		return time.Time{}, jwtDecodeFail
	}

	exp := time.Unix(gjson.Get(string(payload), "exp").Int(), 0)
	if exp == time.Unix(0, 0) {
		return time.Time{}, jwtDecodeFail
	}

	log.Logger.Debug(fmt.Sprintf("Retrieved expiration time %s from a token", exp))
	return exp.UTC(), nil
}

// TokenExpiresIn calculates the remaining time until the given token expires.
// It takes a token string as input and returns the duration until expiration.
// If there is an error while parsing the token or getting the expiration time, it returns an error.
func TokenExpiresIn(token string) (time.Duration, error) {
	exp, err := getJWTExp(token)
	if err != nil {
		return time.Duration(time.Duration(0).Seconds()), err
	}

	return exp.Sub(time.Now()), nil
}
