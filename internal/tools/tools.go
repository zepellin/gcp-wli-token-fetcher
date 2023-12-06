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

func GetEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func FileExists(path string) (bool, error) {
	if _, err := os.Stat(path); err == nil {
		log.Logger.Debug(fmt.Sprintf("Found file %s", path))
		return true, nil
	} else if errors.Is(err, os.ErrNotExist) {
		log.Logger.Debug(fmt.Sprintf("File not found %s", path))
		return false, nil
	} else {
		return false, err
	}
}

func getJWTExp(token string) (time.Time, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return time.Time{}, errors.New("error decoding token: JWT must have three parts")
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}, errors.New("error decoding token")
	}

	exp := time.Unix(gjson.Get(string(payload), "exp").Int(), 0)
	log.Logger.Debug(fmt.Sprintf("Retrieved expiration time %s from a token", exp))
	return exp, nil
}

func TokenExpiresIn(token string) (time.Duration, error) {
	exp, err := getJWTExp(token)
	if err != nil {
		return 0, err
	}

	return exp.Sub(time.Now()), nil
}
