package metadata

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var testGSA = GSA{
	Name:     "test-account",
	Audience: "test-audience",
	Scope:    "test-scope",
}

// generateJWT returns a signed JWT expiring after expiresIn.
func generateJWT(t *testing.T, expiresIn time.Duration) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"aud": testGSA.Audience,
		"sub": testGSA.Name,
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(expiresIn).Unix(),
	})
	signed, err := token.SignedString([]byte("test-secret"))
	if err != nil {
		t.Fatalf("failed to sign test token: %v", err)
	}
	return signed
}

// newMetadataServer starts a test server that mimics the GCE metadata
// endpoint, serving token and counting requests via the returned counter.
func newMetadataServer(t *testing.T, token string, requests *int) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*requests++
		if r.Header.Get("Metadata-Flavor") != "Google" {
			http.Error(w, "Invalid Metadata-Flavor header", http.StatusBadRequest)
			return
		}
		if r.URL.Path != fmt.Sprintf("/computeMetadata/v1/instance/service-accounts/%s/identity", testGSA.Name) {
			http.Error(w, "Invalid URL path", http.StatusNotFound)
			return
		}
		if r.URL.Query().Get("audience") != testGSA.Audience || r.URL.Query().Get("scope") != testGSA.Scope {
			http.Error(w, "Invalid audience or scope", http.StatusBadRequest)
			return
		}
		w.Write([]byte(token))
	}))
	t.Cleanup(ts.Close)
	return ts
}

func TestToken(t *testing.T) {
	ctx := context.Background()
	want := generateJWT(t, time.Hour)

	t.Run("success", func(t *testing.T) {
		var requests int
		ts := newMetadataServer(t, want, &requests)

		got, err := testGSA.token(ctx, ts.URL)
		if err != nil {
			t.Fatalf("token() returned an error: %v", err)
		}
		if string(got) != want {
			t.Errorf("token() = %s, want %s", got, want)
		}
	})

	t.Run("omits empty scope", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Has("scope") {
				http.Error(w, "unexpected scope parameter", http.StatusBadRequest)
				return
			}
			w.Write([]byte(want))
		}))
		t.Cleanup(ts.Close)

		gsa := testGSA
		gsa.Scope = ""
		if _, err := gsa.token(ctx, ts.URL); err != nil {
			t.Fatalf("token() returned an error: %v", err)
		}
	})

	t.Run("http error status", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}))
		t.Cleanup(ts.Close)

		got, err := testGSA.token(ctx, ts.URL)
		if err == nil {
			t.Fatal("token() did not return an error for a 500 response")
		}
		if !strings.Contains(err.Error(), "500") {
			t.Errorf("token() error should mention status code, got: %v", err)
		}
		if got != nil {
			t.Errorf("token() = %s, want nil on error", got)
		}
	})

	t.Run("unreachable server", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		ts.Close() // shut down immediately so the address refuses connections

		_, err := testGSA.token(ctx, ts.URL)
		if err == nil {
			t.Fatal("token() did not return an error for an unreachable server")
		}
	})
}

func TestRefreshToken(t *testing.T) {
	ctx := context.Background()
	threshold := 30 * time.Minute

	t.Run("creates file and parent directories", func(t *testing.T) {
		want := generateJWT(t, time.Hour)
		var requests int
		ts := newMetadataServer(t, want, &requests)
		tokenFile := filepath.Join(t.TempDir(), "nested", "dir", "token")

		if err := testGSA.RefreshToken(ctx, tokenFile, ts.URL, threshold); err != nil {
			t.Fatalf("RefreshToken() returned an error: %v", err)
		}
		got, err := os.ReadFile(tokenFile)
		if err != nil {
			t.Fatalf("failed to read token file: %v", err)
		}
		if string(got) != want {
			t.Errorf("token file = %s, want %s", got, want)
		}
	})

	t.Run("skips renewal above threshold", func(t *testing.T) {
		existing := generateJWT(t, time.Hour)
		var requests int
		ts := newMetadataServer(t, generateJWT(t, time.Hour), &requests)
		tokenFile := filepath.Join(t.TempDir(), "token")
		if err := os.WriteFile(tokenFile, []byte(existing), 0o644); err != nil {
			t.Fatal(err)
		}

		if err := testGSA.RefreshToken(ctx, tokenFile, ts.URL, threshold); err != nil {
			t.Fatalf("RefreshToken() returned an error: %v", err)
		}
		if requests != 0 {
			t.Errorf("RefreshToken() made %d metadata requests for a valid token, want 0", requests)
		}
		got, _ := os.ReadFile(tokenFile)
		if string(got) != existing {
			t.Error("RefreshToken() replaced a token that was still valid")
		}
	})

	t.Run("renews below threshold", func(t *testing.T) {
		want := generateJWT(t, time.Hour)
		var requests int
		ts := newMetadataServer(t, want, &requests)
		tokenFile := filepath.Join(t.TempDir(), "token")
		if err := os.WriteFile(tokenFile, []byte(generateJWT(t, time.Minute)), 0o644); err != nil {
			t.Fatal(err)
		}

		if err := testGSA.RefreshToken(ctx, tokenFile, ts.URL, threshold); err != nil {
			t.Fatalf("RefreshToken() returned an error: %v", err)
		}
		if requests != 1 {
			t.Errorf("RefreshToken() made %d metadata requests, want 1", requests)
		}
		got, _ := os.ReadFile(tokenFile)
		if string(got) != want {
			t.Errorf("token file = %s, want %s", got, want)
		}
	})

	t.Run("renews unparseable token", func(t *testing.T) {
		want := generateJWT(t, time.Hour)
		var requests int
		ts := newMetadataServer(t, want, &requests)
		tokenFile := filepath.Join(t.TempDir(), "token")
		if err := os.WriteFile(tokenFile, []byte("not-a-jwt"), 0o644); err != nil {
			t.Fatal(err)
		}

		if err := testGSA.RefreshToken(ctx, tokenFile, ts.URL, threshold); err != nil {
			t.Fatalf("RefreshToken() returned an error: %v", err)
		}
		got, _ := os.ReadFile(tokenFile)
		if string(got) != want {
			t.Errorf("token file = %s, want %s", got, want)
		}
	})

	t.Run("keeps existing token when server unreachable", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		ts.Close()
		existing := generateJWT(t, time.Minute) // below threshold, triggers renewal
		tokenFile := filepath.Join(t.TempDir(), "token")
		if err := os.WriteFile(tokenFile, []byte(existing), 0o644); err != nil {
			t.Fatal(err)
		}

		if err := testGSA.RefreshToken(ctx, tokenFile, ts.URL, threshold); err == nil {
			t.Fatal("RefreshToken() did not return an error for an unreachable server")
		}
		got, _ := os.ReadFile(tokenFile)
		if string(got) != existing {
			t.Error("RefreshToken() modified the token file despite the fetch failing")
		}
	})
}

func TestCheckToken(t *testing.T) {
	writeToken := func(t *testing.T, token string) string {
		t.Helper()
		tokenFile := filepath.Join(t.TempDir(), "token")
		if err := os.WriteFile(tokenFile, []byte(token), 0o644); err != nil {
			t.Fatal(err)
		}
		return tokenFile
	}

	t.Run("valid token", func(t *testing.T) {
		if err := CheckToken(writeToken(t, generateJWT(t, time.Hour))); err != nil {
			t.Errorf("CheckToken() returned an error for a valid token: %v", err)
		}
	})

	t.Run("expired token", func(t *testing.T) {
		if err := CheckToken(writeToken(t, generateJWT(t, -time.Minute))); err == nil {
			t.Error("CheckToken() did not return an error for an expired token")
		}
	})

	t.Run("malformed token", func(t *testing.T) {
		if err := CheckToken(writeToken(t, "not-a-jwt")); err == nil {
			t.Error("CheckToken() did not return an error for a malformed token")
		}
	})

	t.Run("missing file", func(t *testing.T) {
		if err := CheckToken(filepath.Join(t.TempDir(), "token")); err == nil {
			t.Error("CheckToken() did not return an error for a missing file")
		}
	})
}

func TestTokenExpiresIn(t *testing.T) {
	t.Run("valid token", func(t *testing.T) {
		expIn, err := tokenExpiresIn(generateJWT(t, time.Hour))
		if err != nil {
			t.Fatalf("tokenExpiresIn() returned an error: %v", err)
		}
		if expIn < 59*time.Minute || expIn > time.Hour {
			t.Errorf("tokenExpiresIn() = %s, want ~1h", expIn)
		}
	})

	t.Run("expired token", func(t *testing.T) {
		expIn, err := tokenExpiresIn(generateJWT(t, -time.Hour))
		if err != nil {
			t.Fatalf("tokenExpiresIn() returned an error: %v", err)
		}
		if expIn >= 0 {
			t.Errorf("tokenExpiresIn() = %s, want negative for an expired token", expIn)
		}
	})

	t.Run("malformed token", func(t *testing.T) {
		if _, err := tokenExpiresIn("not-a-jwt"); err == nil {
			t.Error("tokenExpiresIn() did not return an error for a malformed token")
		}
	})

	t.Run("missing exp claim", func(t *testing.T) {
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"sub": "x"})
		signed, err := token.SignedString([]byte("test-secret"))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tokenExpiresIn(signed); err == nil {
			t.Error("tokenExpiresIn() did not return an error for a token without exp")
		}
	})
}
