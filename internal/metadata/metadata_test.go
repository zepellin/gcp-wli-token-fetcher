package metadata

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestToken(t *testing.T) {
	g := &GSA{
		Name:     "test-account",
		Audience: "test-audience",
		Scope:    "test-scope",
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

	// Test case 1: Successful request
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Metadata-Flavor") != "Google" {
			http.Error(w, "Invalid Metadata-Flavor header", http.StatusBadRequest)
			return
		}
		if r.URL.Path != fmt.Sprintf("/computeMetadata/v1/instance/service-accounts/%s/identity", g.Name) {
			http.Error(w, "Invalid URL path", http.StatusBadRequest)
			return
		}
		if r.URL.Query().Get("audience") != g.Audience || r.URL.Query().Get("scope") != g.Scope {
			http.Error(w, "Invalid audience or scope", http.StatusBadRequest)
			return
		}
		w.Write(j)
	}))
	defer ts.Close()

	result, err := g.token("http://" + ts.Listener.Addr().String())
	if err != nil {
		t.Errorf("Testtoken failed for a successful request: %v", err)
	}
	if string(result) != string(j) {
		t.Errorf("Testtoken failed for a successful request. Expected token: %s, Got: %s", j, result)
	}

	// Test case 2: HTTP request error
	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}))
	defer ts.Close()

	result, err = g.token("http://" + ts.Listener.Addr().String())
	if err == nil {
		t.Errorf("Testtoken did not return an error for an HTTP request error")
	}
	if !errors.Is(err, metadataRequestFail) {
		t.Errorf("Testtoken did not return metadataRequestFail error for an HTTP request error: %v, %v", err, metadataRequestFail)
	}
	if string(result) != "" {
		t.Errorf("Testtoken failed for an HTTP request error. Expected empty result, Got: %s", result)
	}
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
