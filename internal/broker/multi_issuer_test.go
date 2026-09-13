package broker_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/After-Certainty/pade/internal/broker"
	"github.com/golang-jwt/jwt/v5"
)

func TestMultiIssuerPolicyValidation(t *testing.T) {
	t.Parallel()

	t.Run("accepts multi-issuer", func(t *testing.T) {
		t.Parallel()
		p, err := broker.ParsePolicy([]byte(`
version: "0.1"
oidc:
  issuers:
    cursor:
      issuer: https://api.cursor.com
      audience: https://broker.example
      jwksURL: https://api.cursor.com/keys
    google:
      issuer: https://accounts.google.com
      audience: https://broker.example
      jwksURL: https://www.googleapis.com/oauth2/v3/certs
policies:
  - issuer: cursor
    subject: "user:1"
    requireRepoURLs: false
    capabilities: ["cap.a"]
  - issuer: google
    subject: "12345"
    requireRepoURLs: false
    capabilities: ["cap.b"]
`))
		if err != nil {
			t.Fatal(err)
		}
		if !p.MultiIssuer() {
			t.Fatal("expected multi-issuer mode")
		}
	})

	t.Run("rejects legacy and issuers together", func(t *testing.T) {
		t.Parallel()
		_, err := broker.ParsePolicy([]byte(`
version: "0.1"
oidc:
  issuer: https://api.cursor.com
  audience: https://broker.example
  issuers:
    cursor:
      issuer: https://api.cursor.com
      audience: https://broker.example
      jwksURL: https://api.cursor.com/keys
policies:
  - issuer: cursor
    subject: "user:1"
    requireRepoURLs: false
    capabilities: ["cap.a"]
`))
		if err == nil || !strings.Contains(err.Error(), "cannot set both") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("rejects duplicate issuer URLs", func(t *testing.T) {
		t.Parallel()
		_, err := broker.ParsePolicy([]byte(`
version: "0.1"
oidc:
  issuers:
    a:
      issuer: https://accounts.google.com
      audience: https://broker.example
      jwksURL: https://www.googleapis.com/oauth2/v3/certs
    b:
      issuer: https://accounts.google.com
      audience: https://other.example
      jwksURL: https://www.googleapis.com/oauth2/v3/certs
policies:
  - issuer: a
    subject: "1"
    requireRepoURLs: false
    capabilities: ["cap.a"]
`))
		if err == nil || !strings.Contains(err.Error(), "duplicate issuer URL") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("requires issuer on rules in multi mode", func(t *testing.T) {
		t.Parallel()
		_, err := broker.ParsePolicy([]byte(`
version: "0.1"
oidc:
  issuers:
    cursor:
      issuer: https://api.cursor.com
      audience: https://broker.example
      jwksURL: https://api.cursor.com/keys
policies:
  - subject: "user:1"
    requireRepoURLs: false
    capabilities: ["cap.a"]
`))
		if err == nil || !strings.Contains(err.Error(), "issuer is required") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("rejects unknown issuer alias on rule", func(t *testing.T) {
		t.Parallel()
		_, err := broker.ParsePolicy([]byte(`
version: "0.1"
oidc:
  issuers:
    cursor:
      issuer: https://api.cursor.com
      audience: https://broker.example
      jwksURL: https://api.cursor.com/keys
policies:
  - issuer: google
    subject: "user:1"
    requireRepoURLs: false
    capabilities: ["cap.a"]
`))
		if err == nil || !strings.Contains(err.Error(), "unknown issuer alias") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("rejects duplicate issuer+subject", func(t *testing.T) {
		t.Parallel()
		_, err := broker.ParsePolicy([]byte(`
version: "0.1"
oidc:
  issuers:
    cursor:
      issuer: https://api.cursor.com
      audience: https://broker.example
      jwksURL: https://api.cursor.com/keys
policies:
  - issuer: cursor
    subject: "user:1"
    requireRepoURLs: false
    capabilities: ["cap.a"]
  - issuer: cursor
    subject: "user:1"
    requireRepoURLs: false
    capabilities: ["cap.b"]
`))
		if err == nil || !strings.Contains(err.Error(), "duplicate issuer") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("same subject distinct across issuers", func(t *testing.T) {
		t.Parallel()
		p, err := broker.ParsePolicy([]byte(`
version: "0.1"
oidc:
  issuers:
    cursor:
      issuer: https://api.cursor.com
      audience: https://broker.example
      jwksURL: https://api.cursor.com/keys
    google:
      issuer: https://accounts.google.com
      audience: https://broker.example
      jwksURL: https://www.googleapis.com/oauth2/v3/certs
policies:
  - issuer: cursor
    subject: "shared-sub"
    requireRepoURLs: false
    capabilities: ["cap.cursor"]
  - issuer: google
    subject: "shared-sub"
    requireRepoURLs: false
    capabilities: ["cap.google"]
`))
		if err != nil {
			t.Fatal(err)
		}
		if d := p.Authorize(broker.Claims{Subject: "shared-sub", IssuerAlias: "cursor"}, "cap.cursor"); !d.Allowed {
			t.Fatalf("cursor: %+v", d)
		}
		if d := p.Authorize(broker.Claims{Subject: "shared-sub", IssuerAlias: "google"}, "cap.google"); !d.Allowed {
			t.Fatalf("google: %+v", d)
		}
		if d := p.Authorize(broker.Claims{Subject: "shared-sub", IssuerAlias: "google"}, "cap.cursor"); d.Allowed {
			t.Fatal("google subject must not match cursor-only capability")
		}
		if d := p.Authorize(broker.Claims{Subject: "shared-sub"}, "cap.cursor"); d.Allowed {
			t.Fatal("missing issuer alias must fail closed in multi mode")
		}
	})

	t.Run("requires jwksURL per issuer", func(t *testing.T) {
		t.Parallel()
		_, err := broker.ParsePolicy([]byte(`
version: "0.1"
oidc:
  issuers:
    google:
      issuer: https://accounts.google.com
      audience: https://broker.example
policies:
  - issuer: google
    subject: "1"
    requireRepoURLs: false
    capabilities: ["cap.a"]
`))
		if err == nil || !strings.Contains(err.Error(), "jwksURL") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("legacy still works", func(t *testing.T) {
		t.Parallel()
		p, err := broker.ParsePolicy([]byte(`
version: "0.1"
oidc:
  issuer: https://api.cursor.com
  audience: https://broker.example
policies:
  - subject: "user:1"
    requireRepoURLs: false
    capabilities: ["cap.a"]
`))
		if err != nil {
			t.Fatal(err)
		}
		if p.MultiIssuer() {
			t.Fatal("expected legacy mode")
		}
		if d := p.Authorize(broker.Claims{Subject: "user:1"}, "cap.a"); !d.Allowed {
			t.Fatalf("%+v", d)
		}
	})
}

func TestVerifierSetSelection(t *testing.T) {
	t.Parallel()
	cursorKey := mustRSAKey(t)
	googleKey := mustRSAKey(t)

	cursorJWKS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(rsaJWKS(cursorKey, "cursor-kid"))
	}))
	t.Cleanup(cursorJWKS.Close)
	googleJWKS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(rsaJWKS(googleKey, "google-kid"))
	}))
	t.Cleanup(googleJWKS.Close)

	set, err := broker.NewVerifierSet([]broker.TrustedIssuer{
		{Alias: "cursor", Issuer: "https://api.cursor.com", Audience: "https://broker.example", JWKSURL: cursorJWKS.URL},
		{Alias: "google", Issuer: "https://accounts.google.com", Audience: "https://broker.example", JWKSURL: googleJWKS.URL},
	})
	if err != nil {
		t.Fatal(err)
	}

	cursorHits := 0
	googleHits := 0
	set.VerifierForAlias("cursor").HTTPDo = func(r *http.Request) (*http.Response, error) {
		cursorHits++
		return cursorJWKS.Client().Do(r)
	}
	set.VerifierForAlias("google").HTTPDo = func(r *http.Request) (*http.Response, error) {
		googleHits++
		return googleJWKS.Client().Do(r)
	}

	cursorTok := mustSignRSA(t, cursorKey, "cursor-kid", jwt.MapClaims{
		"iss": "https://api.cursor.com", "sub": "user:1", "aud": "https://broker.example",
		"iat": time.Now().Unix(), "exp": time.Now().Add(time.Minute).Unix(),
	})
	googleTok := mustSignRSA(t, googleKey, "google-kid", jwt.MapClaims{
		"iss": "https://accounts.google.com", "sub": "12345", "aud": "https://broker.example",
		"iat": time.Now().Unix(), "exp": time.Now().Add(time.Minute).Unix(),
	})

	ctx := context.Background()
	cClaims, err := set.Verify(ctx, cursorTok)
	if err != nil {
		t.Fatal(err)
	}
	if cClaims.IssuerAlias != "cursor" || cClaims.Subject != "user:1" {
		t.Fatalf("%+v", cClaims)
	}
	gClaims, err := set.Verify(ctx, googleTok)
	if err != nil {
		t.Fatal(err)
	}
	if gClaims.IssuerAlias != "google" || gClaims.Subject != "12345" {
		t.Fatalf("%+v", gClaims)
	}
	if cursorHits == 0 || googleHits == 0 {
		t.Fatalf("expected independent JWKS caches; cursor=%d google=%d", cursorHits, googleHits)
	}

	untrusted := mustSignRSA(t, cursorKey, "cursor-kid", jwt.MapClaims{
		"iss": "https://evil.example", "sub": "user:1", "aud": "https://broker.example",
		"iat": time.Now().Unix(), "exp": time.Now().Add(time.Minute).Unix(),
	})
	beforeCursor, beforeGoogle := cursorHits, googleHits
	if _, err := set.Verify(ctx, untrusted); err == nil || !strings.Contains(err.Error(), "untrusted issuer") {
		t.Fatalf("err=%v", err)
	}
	if cursorHits != beforeCursor || googleHits != beforeGoogle {
		t.Fatalf("peek iss must not drive JWKS fetch: cursor %d→%d google %d→%d", beforeCursor, cursorHits, beforeGoogle, googleHits)
	}

	wrongAud := mustSignRSA(t, googleKey, "google-kid", jwt.MapClaims{
		"iss": "https://accounts.google.com", "sub": "12345", "aud": "https://other.example",
		"iat": time.Now().Unix(), "exp": time.Now().Add(time.Minute).Unix(),
	})
	if _, err := set.Verify(ctx, wrongAud); err == nil {
		t.Fatal("expected wrong audience rejection")
	}

	wrongSig := mustSignRSA(t, cursorKey, "google-kid", jwt.MapClaims{
		"iss": "https://accounts.google.com", "sub": "12345", "aud": "https://broker.example",
		"iat": time.Now().Unix(), "exp": time.Now().Add(time.Minute).Unix(),
	})
	if _, err := set.Verify(ctx, wrongSig); err == nil {
		t.Fatal("expected wrong signature rejection")
	}

	if _, err := set.Verify(ctx, "not-a-jwt"); err == nil {
		t.Fatal("expected malformed rejection")
	}
}

func mustRSAKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func rsaJWKS(key *rsa.PrivateKey, kid string) map[string]any {
	pub := &key.PublicKey
	return map[string]any{
		"keys": []map[string]string{{
			"kty": "RSA", "kid": kid, "alg": "RS256", "use": "sig",
			"n": base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
			"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
		}},
	}
}

func mustSignRSA(t *testing.T, key *rsa.PrivateKey, kid string, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = kid
	s, err := tok.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	return s
}
