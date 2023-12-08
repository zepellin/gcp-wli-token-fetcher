package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"gcp-wli-token-fetcher/internal/metadata"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestMain(m *testing.M) {
	// Set up any test environment here
	os.Setenv("TOKEN_FILE", "/path/to/token/file")
	os.Setenv("GSA_NAME", "test-account")
	os.Setenv("TOKEN_AUDIENCE", "test-audience")
	os.Setenv("TOKEN_SCOPE", "test-scope")
	os.Setenv("CRON_SPEC", "* * * * *")
	os.Setenv("TOKEN_RENEW_THRESHOLD", "30m0s")
	os.Setenv("METADATA_SERVER_URL", "http://metadata.google.internal")

	// Run tests
	code := m.Run()

	// Clean up any test environment here

	os.Exit(code)
}

func TestMainFunc(t *testing.T) {
	// Set up test data and environment
	tokenFile := "/tmp/folder/token-test"
	GSAName := "test-account"
	audience := "test-audience"
	scope := "test-scope"
	ctx := context.Background()
	ctx = context.WithValue(ctx, "renewthreshold", "30m0s")

	g := &metadata.GSA{
		Name:     GSAName,
		Audience: audience,
		Scope:    scope,
	}

	// Generate a mock token
	k, err := GeneratePEMPrivateKey()
	if err != nil {
		t.Errorf("Failed to generate mock private key: %v", err)
	}
	j, err := GenerateJWT(k, jwt.MapClaims{"aud": g.Audience, "scope": g.Scope, "sub": g.Name})
	if err != nil {
		t.Errorf("Failed to generate mock token: %v", err)
	}

	// Set up mock server
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Handle requests to the metadata server
		if r.Header.Get("Metadata-Flavor") != "Google" {
			http.Error(w, "Invalid Metadata-Flavor header", http.StatusBadRequest)
			return
		}
		if r.URL.Path == fmt.Sprintf("/computeMetadata/v1/instance/service-accounts/%s/identity", GSAName) {
			w.Write(j)
		} else {
			http.NotFound(w, r)
		}
	}))
	defer ts.Close()

	// Call the main function
	g.RefreshToken(ctx, tokenFile, "http://"+ts.Listener.Addr().String())

	// Wait for the main function to start the token renewal routine
	time.Sleep(time.Second * 2)

	// Verify that the token is refreshed on startup
	token, err := os.ReadFile(tokenFile)
	if err != nil {
		t.Errorf("Failed to read token file: %v", err)
	}
	if string(token) != string(j) {
		t.Errorf("Token not refreshed on startup. Expected: %s, Got: %s", token, j)
	}

	// Verify that the token is refreshed according to the cron schedule
	time.Sleep(time.Second) // Wait for the first token renewal
	token, err = os.ReadFile(tokenFile)
	if err != nil {
		t.Errorf("Failed to read token file: %v", err)
	}
	if string(token) != string(j) {
		t.Errorf("Saved token doesn't equal token retrieved from metadata server, Got: %s, expected %s", token, j)
	}

	os.Remove(tokenFile)
}

// GeneratePEMPrivateKey generates a PEM private key.
func GeneratePEMPrivateKey() ([]byte, error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("failed to generate private key: %v", err)
	}

	privateKeyBytes := x509.MarshalPKCS1PrivateKey(privateKey)
	pemPrivateKey := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privateKeyBytes,
	}

	privateKeyPEM := pem.EncodeToMemory(pemPrivateKey)
	return privateKeyPEM, nil
}

// GenerateJWT generates a JSON Web Token (JWT) using the provided private key and claims.
func GenerateJWT(privateKey []byte, claims jwt.MapClaims) ([]byte, error) {
	block, _ := pem.Decode(privateKey)
	if block == nil {
		return []byte{}, fmt.Errorf("failed to decode private key")
	}

	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return []byte{}, fmt.Errorf("failed to parse private key: %v", err)
	}

	claims["iat"] = time.Now().Unix()
	claims["exp"] = time.Now().Add(time.Hour).Unix()

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = "key-id"
	token.Header["alg"] = "RS256"

	signedToken, err := token.SignedString(key)
	if err != nil {
		return []byte{}, fmt.Errorf("failed to sign token: %v", err)
	}

	return []byte(signedToken), nil
}
