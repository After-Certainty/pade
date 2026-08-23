package broker_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/After-Certainty/pade/internal/broker"
	"github.com/golang-jwt/jwt/v5"
)

func TestVerifyRequiresExpiration(t *testing.T) {
	key := mustKey(t)
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(jwksFor(key, "test-kid"))
	}))
	t.Cleanup(jwks.Close)

	v := &broker.Verifier{
		Issuer:   testIssuer,
		Audience: testAudience,
		JWKSURL:  jwks.URL,
		HTTPDo:   jwks.Client().Do,
	}
	ctx := context.Background()

	valid := mustSign(t, key, "test-kid", jwt.MapClaims{
		"iss": testIssuer, "sub": testSubject, "aud": testAudience,
		"iat": time.Now().Unix(), "exp": time.Now().Add(2 * time.Minute).Unix(),
	})
	if _, err := v.Verify(ctx, valid); err != nil {
		t.Fatalf("valid token: %v", err)
	}

	noExp := mustSign(t, key, "test-kid", jwt.MapClaims{
		"iss": testIssuer, "sub": testSubject, "aud": testAudience,
		"iat": time.Now().Unix(),
	})
	if _, err := v.Verify(ctx, noExp); err == nil {
		t.Fatal("expected missing exp to fail")
	}

	expired := mustSign(t, key, "test-kid", jwt.MapClaims{
		"iss": testIssuer, "sub": testSubject, "aud": testAudience,
		"iat": time.Now().Add(-2 * time.Hour).Unix(),
		"exp": time.Now().Add(-time.Hour).Unix(),
	})
	if _, err := v.Verify(ctx, expired); err == nil {
		t.Fatal("expected expired token to fail")
	}

	tooLong := mustSign(t, key, "test-kid", jwt.MapClaims{
		"iss": testIssuer, "sub": testSubject, "aud": testAudience,
		"iat": time.Now().Unix(),
		"exp": time.Now().Add(48 * time.Hour).Unix(),
	})
	if _, err := v.Verify(ctx, tooLong); err == nil {
		t.Fatal("expected max lifetime rejection")
	}

	wrongIss := mustSign(t, key, "test-kid", jwt.MapClaims{
		"iss": "https://evil.example", "sub": testSubject, "aud": testAudience,
		"iat": time.Now().Unix(), "exp": time.Now().Add(time.Minute).Unix(),
	})
	if _, err := v.Verify(ctx, wrongIss); err == nil {
		t.Fatal("expected wrong issuer rejection")
	}

	wrongAud := mustSign(t, key, "test-kid", jwt.MapClaims{
		"iss": testIssuer, "sub": testSubject, "aud": "https://other.example",
		"iat": time.Now().Unix(), "exp": time.Now().Add(time.Minute).Unix(),
	})
	if _, err := v.Verify(ctx, wrongAud); err == nil {
		t.Fatal("expected wrong audience rejection")
	}

	hs := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"iss": testIssuer, "sub": testSubject, "aud": testAudience,
		"iat": time.Now().Unix(), "exp": time.Now().Add(time.Minute).Unix(),
	})
	hs.Header["kid"] = "test-kid"
	hsTok, err := hs.SignedString([]byte("secret"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := v.Verify(ctx, hsTok); err == nil {
		t.Fatal("expected unexpected algorithm rejection")
	}
}

func TestJWKSRejectsOversized(t *testing.T) {
	key := mustKey(t)
	pub := &key.PublicKey
	n := base64.RawURLEncoding.EncodeToString(pub.N.Bytes())
	e := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes())
	base, _ := json.Marshal(map[string]interface{}{
		"keys": []map[string]string{
			{"kty": "RSA", "kid": "k1", "alg": "RS256", "use": "sig", "n": n, "e": e},
		},
	})
	// Pad past 1 MiB while remaining syntactically closer to JWKS (trailing garbage is fine; size check is first).
	oversized := append(bytes.Repeat([]byte(" "), (1<<20)+16), base...)
	v := &broker.Verifier{
		Issuer:   testIssuer,
		Audience: testAudience,
		JWKSURL:  "http://127.0.0.1/keys",
		HTTPDo: func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(bytes.NewReader(oversized)),
				Header:     make(http.Header),
			}, nil
		},
	}
	_, err := v.Verify(context.Background(), "x.y.z")
	if err == nil || !strings.Contains(err.Error(), "exceeds size limit") {
		t.Fatalf("expected size limit error, got %v", err)
	}
}

func TestJWKSRejectsDuplicateKid(t *testing.T) {
	key := mustKey(t)
	pub := &key.PublicKey
	n := base64.RawURLEncoding.EncodeToString(pub.N.Bytes())
	e := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes())
	raw, _ := json.Marshal(map[string]interface{}{
		"keys": []map[string]string{
			{"kty": "RSA", "kid": "dup", "alg": "RS256", "use": "sig", "n": n, "e": e},
			{"kty": "RSA", "kid": "dup", "alg": "RS256", "use": "sig", "n": n, "e": e},
		},
	})
	v := &broker.Verifier{
		Issuer:   testIssuer,
		Audience: testAudience,
		JWKSURL:  "http://127.0.0.1/keys",
		HTTPDo: func(*http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: 200,
				Body:       io.NopCloser(strings.NewReader(string(raw))),
				Header:     make(http.Header),
			}, nil
		},
	}
	_, err := v.Verify(context.Background(), "x.y.z")
	if err == nil || !strings.Contains(err.Error(), "duplicate kid") {
		t.Fatalf("expected duplicate kid error, got %v", err)
	}
}

func TestJWKSRejectsRemoteHTTP(t *testing.T) {
	v := &broker.Verifier{
		Issuer:   testIssuer,
		Audience: testAudience,
		JWKSURL:  "http://evil.example/keys",
	}
	_, err := v.Verify(context.Background(), "token")
	if err == nil || !strings.Contains(err.Error(), "insecure http") {
		t.Fatalf("expected insecure http rejection, got %v", err)
	}
}

func TestJWKSRejectsBadAlgAndUse(t *testing.T) {
	key := mustKey(t)
	pub := &key.PublicKey
	n := base64.RawURLEncoding.EncodeToString(pub.N.Bytes())
	e := base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes())

	for _, tc := range []struct {
		name string
		key  map[string]string
		want string
	}{
		{"bad alg", map[string]string{"kty": "RSA", "kid": "k1", "alg": "HS256", "use": "sig", "n": n, "e": e}, "unsupported alg"},
		{"bad use", map[string]string{"kty": "RSA", "kid": "k1", "alg": "RS256", "use": "enc", "n": n, "e": e}, "unsupported use"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, _ := json.Marshal(map[string]interface{}{"keys": []map[string]string{tc.key}})
			v := &broker.Verifier{
				Issuer: testIssuer, Audience: testAudience, JWKSURL: "http://127.0.0.1/keys",
				HTTPDo: func(*http.Request) (*http.Response, error) {
					return &http.Response{
						StatusCode: 200,
						Body:       io.NopCloser(strings.NewReader(string(raw))),
						Header:     make(http.Header),
					}, nil
				},
			}
			_, err := v.Verify(context.Background(), "x.y.z")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q in error, got %v", tc.want, err)
			}
		})
	}
}

func TestJWKSReuseWithinTTL(t *testing.T) {
	key := mustKey(t)
	fetches := 0
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetches++
		_ = json.NewEncoder(w).Encode(jwksFor(key, "test-kid"))
	}))
	t.Cleanup(jwks.Close)

	v := &broker.Verifier{
		Issuer:   testIssuer,
		Audience: testAudience,
		JWKSURL:  jwks.URL,
		HTTPDo:   jwks.Client().Do,
	}
	ctx := context.Background()
	tok := mustSign(t, key, "test-kid", jwt.MapClaims{
		"iss": testIssuer, "sub": testSubject, "aud": testAudience,
		"iat": time.Now().Unix(), "exp": time.Now().Add(2 * time.Minute).Unix(),
	})
	if _, err := v.Verify(ctx, tok); err != nil {
		t.Fatalf("first verify: %v", err)
	}
	if _, err := v.Verify(ctx, tok); err != nil {
		t.Fatalf("second verify: %v", err)
	}
	if fetches != 1 {
		t.Fatalf("fetches=%d want 1", fetches)
	}
}

func TestJWKSRefreshAfterTTL(t *testing.T) {
	key := mustKey(t)
	fetches := 0
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetches++
		_ = json.NewEncoder(w).Encode(jwksFor(key, "test-kid"))
	}))
	t.Cleanup(jwks.Close)

	now := time.Now()
	v := &broker.Verifier{
		Issuer:   testIssuer,
		Audience: testAudience,
		JWKSURL:  jwks.URL,
		HTTPDo:   jwks.Client().Do,
		Now:      func() time.Time { return now },
	}
	ctx := context.Background()
	tok := mustSign(t, key, "test-kid", jwt.MapClaims{
		"iss": testIssuer, "sub": testSubject, "aud": testAudience,
		"iat": now.Unix(), "exp": now.Add(30 * time.Minute).Unix(),
	})
	if _, err := v.Verify(ctx, tok); err != nil {
		t.Fatalf("first verify: %v", err)
	}
	now = now.Add(6 * time.Minute)
	if _, err := v.Verify(ctx, tok); err != nil {
		t.Fatalf("after TTL verify: %v", err)
	}
	if fetches != 2 {
		t.Fatalf("fetches=%d want 2", fetches)
	}
}

func TestJWKSUnknownKidRefresh(t *testing.T) {
	oldKey := mustKey(t)
	newKey := mustKey(t)
	phase := 0
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if phase == 0 {
			_ = json.NewEncoder(w).Encode(jwksFor(oldKey, "old-kid"))
			return
		}
		_ = json.NewEncoder(w).Encode(jwksFor(newKey, "new-kid"))
	}))
	t.Cleanup(jwks.Close)

	v := &broker.Verifier{
		Issuer:   testIssuer,
		Audience: testAudience,
		JWKSURL:  jwks.URL,
		HTTPDo:   jwks.Client().Do,
	}
	ctx := context.Background()
	// Prime cache with old kid only.
	if _, err := v.Verify(ctx, mustSign(t, oldKey, "old-kid", jwt.MapClaims{
		"iss": testIssuer, "sub": testSubject, "aud": testAudience,
		"iat": time.Now().Unix(), "exp": time.Now().Add(2 * time.Minute).Unix(),
	})); err != nil {
		t.Fatalf("prime cache: %v", err)
	}
	phase = 1
	tok := mustSign(t, newKey, "new-kid", jwt.MapClaims{
		"iss": testIssuer, "sub": testSubject, "aud": testAudience,
		"iat": time.Now().Unix(), "exp": time.Now().Add(2 * time.Minute).Unix(),
	})
	if _, err := v.Verify(ctx, tok); err != nil {
		t.Fatalf("verify with rotated kid: %v", err)
	}
}

func TestJWKSConcurrentRefresh(t *testing.T) {
	key := mustKey(t)
	var fetches int64
	start := make(chan struct{})
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&fetches, 1)
		time.Sleep(20 * time.Millisecond)
		_ = json.NewEncoder(w).Encode(jwksFor(key, "test-kid"))
	}))
	t.Cleanup(jwks.Close)

	now := time.Now().Add(-10 * time.Minute)
	v := &broker.Verifier{
		Issuer:   testIssuer,
		Audience: testAudience,
		JWKSURL:  jwks.URL,
		HTTPDo:   jwks.Client().Do,
		Now:      func() time.Time { return now },
	}
	ctx := context.Background()
	tok := mustSign(t, key, "test-kid", jwt.MapClaims{
		"iss": testIssuer, "sub": testSubject, "aud": testAudience,
		"iat": now.Unix(), "exp": now.Add(2 * time.Minute).Unix(),
	})

	const n = 16
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		go func() {
			<-start
			_, err := v.Verify(ctx, tok)
			errs <- err
		}()
	}
	close(start)
	for i := 0; i < n; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("verify: %v", err)
		}
	}
	if atomic.LoadInt64(&fetches) != 1 {
		t.Fatalf("fetches=%d want 1", fetches)
	}
}

func TestJWKSConcurrentUnknownKidRefresh(t *testing.T) {
	oldKey := mustKey(t)
	newKey := mustKey(t)
	var fetches int64
	release := make(chan struct{})
	inFlight := make(chan struct{})
	var inFlightOnce sync.Once

	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt64(&fetches, 1)
		if n == 1 {
			_ = json.NewEncoder(w).Encode(jwksFor(oldKey, "old-kid"))
			return
		}
		inFlightOnce.Do(func() { close(inFlight) })
		<-release
		_ = json.NewEncoder(w).Encode(jwksFor(newKey, "new-kid"))
	}))
	t.Cleanup(jwks.Close)

	v := &broker.Verifier{
		Issuer:   testIssuer,
		Audience: testAudience,
		JWKSURL:  jwks.URL,
		HTTPDo:   jwks.Client().Do,
	}
	ctx := context.Background()
	if _, err := v.Verify(ctx, mustSign(t, oldKey, "old-kid", jwt.MapClaims{
		"iss": testIssuer, "sub": testSubject, "aud": testAudience,
		"iat": time.Now().Unix(), "exp": time.Now().Add(2 * time.Minute).Unix(),
	})); err != nil {
		t.Fatalf("prime cache: %v", err)
	}
	if atomic.LoadInt64(&fetches) != 1 {
		t.Fatalf("prime fetches=%d want 1", fetches)
	}

	tok := mustSign(t, newKey, "new-kid", jwt.MapClaims{
		"iss": testIssuer, "sub": testSubject, "aud": testAudience,
		"iat": time.Now().Unix(), "exp": time.Now().Add(2 * time.Minute).Unix(),
	})

	const n = 16
	start := make(chan struct{})
	var ready sync.WaitGroup
	ready.Add(n)
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		go func() {
			ready.Done()
			<-start
			_, err := v.Verify(ctx, tok)
			errs <- err
		}()
	}
	ready.Wait()
	close(start)
	<-inFlight
	close(release)
	for i := 0; i < n; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("verify: %v", err)
		}
	}
	if got := atomic.LoadInt64(&fetches); got != 2 {
		t.Fatalf("fetches=%d want 2 (prime + one coalesced unknown-kid refresh)", got)
	}
}

func validClaimsAt(now time.Time) jwt.MapClaims {
	return jwt.MapClaims{
		"iss": testIssuer, "sub": testSubject, "aud": testAudience,
		"iat": now.Unix(), "exp": now.Add(2 * time.Minute).Unix(),
	}
}

func TestJWKSUnknownKidRefreshSuppressed(t *testing.T) {
	key := mustKey(t)
	var fetches int64
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&fetches, 1)
		_ = json.NewEncoder(w).Encode(jwksFor(key, "test-kid"))
	}))
	t.Cleanup(jwks.Close)

	now := time.Now()
	v := &broker.Verifier{
		Issuer:   testIssuer,
		Audience: testAudience,
		JWKSURL:  jwks.URL,
		HTTPDo:   jwks.Client().Do,
		Now:      func() time.Time { return now },
	}
	ctx := context.Background()
	if _, err := v.Verify(ctx, mustSign(t, key, "test-kid", validClaimsAt(now))); err != nil {
		t.Fatalf("prime cache: %v", err)
	}
	if atomic.LoadInt64(&fetches) != 1 {
		t.Fatalf("prime fetches=%d want 1", fetches)
	}

	bogus := mustSign(t, key, "bogus", validClaimsAt(now))
	if _, err := v.Verify(ctx, bogus); err == nil || !strings.Contains(err.Error(), "unknown signing key") {
		t.Fatalf("first bogus kid: %v", err)
	}
	if atomic.LoadInt64(&fetches) != 2 {
		t.Fatalf("after first bogus fetches=%d want 2", fetches)
	}
	if _, err := v.Verify(ctx, bogus); err == nil || !strings.Contains(err.Error(), "unknown signing key") {
		t.Fatalf("repeated bogus kid: %v", err)
	}
	if atomic.LoadInt64(&fetches) != 2 {
		t.Fatalf("repeated bogus fetches=%d want 2", fetches)
	}
}

func TestJWKSUnknownKidRefreshSuppressedAcrossKids(t *testing.T) {
	key := mustKey(t)
	var fetches int64
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&fetches, 1)
		_ = json.NewEncoder(w).Encode(jwksFor(key, "test-kid"))
	}))
	t.Cleanup(jwks.Close)

	now := time.Now()
	v := &broker.Verifier{
		Issuer:   testIssuer,
		Audience: testAudience,
		JWKSURL:  jwks.URL,
		HTTPDo:   jwks.Client().Do,
		Now:      func() time.Time { return now },
	}
	ctx := context.Background()
	if _, err := v.Verify(ctx, mustSign(t, key, "test-kid", validClaimsAt(now))); err != nil {
		t.Fatalf("prime cache: %v", err)
	}
	for _, kid := range []string{"bogus-1", "bogus-2", "bogus-3"} {
		tok := mustSign(t, key, kid, validClaimsAt(now))
		if _, err := v.Verify(ctx, tok); err == nil || !strings.Contains(err.Error(), "unknown signing key") {
			t.Fatalf("kid %q: %v", kid, err)
		}
	}
	if got := atomic.LoadInt64(&fetches); got != 2 {
		t.Fatalf("fetches=%d want 2 (prime + one forced refresh for all bogus kids)", got)
	}
}

func TestJWKSUnknownKidRefreshAfterInterval(t *testing.T) {
	key := mustKey(t)
	var fetches int64
	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&fetches, 1)
		_ = json.NewEncoder(w).Encode(jwksFor(key, "test-kid"))
	}))
	t.Cleanup(jwks.Close)

	now := time.Now()
	v := &broker.Verifier{
		Issuer:   testIssuer,
		Audience: testAudience,
		JWKSURL:  jwks.URL,
		HTTPDo:   jwks.Client().Do,
		Now:      func() time.Time { return now },
	}
	ctx := context.Background()
	if _, err := v.Verify(ctx, mustSign(t, key, "test-kid", validClaimsAt(now))); err != nil {
		t.Fatalf("prime cache: %v", err)
	}
	bogus := mustSign(t, key, "bogus", validClaimsAt(now))
	if _, err := v.Verify(ctx, bogus); err == nil {
		t.Fatal("expected first bogus kid to fail")
	}
	if atomic.LoadInt64(&fetches) != 2 {
		t.Fatalf("after first bogus fetches=%d want 2", fetches)
	}

	now = now.Add(31 * time.Second)
	later := mustSign(t, key, "bogus-later", validClaimsAt(now))
	if _, err := v.Verify(ctx, later); err == nil || !strings.Contains(err.Error(), "unknown signing key") {
		t.Fatalf("after interval: %v", err)
	}
	if atomic.LoadInt64(&fetches) != 3 {
		t.Fatalf("after interval fetches=%d want 3", fetches)
	}
}

func TestJWKSConcurrentBogusKidRefresh(t *testing.T) {
	key := mustKey(t)
	var fetches int64
	release := make(chan struct{})
	inFlight := make(chan struct{})
	var inFlightOnce sync.Once

	jwks := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt64(&fetches, 1)
		if n == 1 {
			_ = json.NewEncoder(w).Encode(jwksFor(key, "test-kid"))
			return
		}
		inFlightOnce.Do(func() { close(inFlight) })
		<-release
		_ = json.NewEncoder(w).Encode(jwksFor(key, "test-kid"))
	}))
	t.Cleanup(jwks.Close)

	now := time.Now()
	v := &broker.Verifier{
		Issuer:   testIssuer,
		Audience: testAudience,
		JWKSURL:  jwks.URL,
		HTTPDo:   jwks.Client().Do,
		Now:      func() time.Time { return now },
	}
	ctx := context.Background()
	if _, err := v.Verify(ctx, mustSign(t, key, "test-kid", validClaimsAt(now))); err != nil {
		t.Fatalf("prime cache: %v", err)
	}
	if atomic.LoadInt64(&fetches) != 1 {
		t.Fatalf("prime fetches=%d want 1", fetches)
	}

	const n = 16
	start := make(chan struct{})
	var ready sync.WaitGroup
	ready.Add(n)
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		kid := "bogus"
		if i%2 == 1 {
			kid = "bogus-other"
		}
		tok := mustSign(t, key, kid, validClaimsAt(now))
		go func() {
			ready.Done()
			<-start
			_, err := v.Verify(ctx, tok)
			errs <- err
		}()
	}
	ready.Wait()
	close(start)
	<-inFlight
	close(release)
	for i := 0; i < n; i++ {
		err := <-errs
		if err == nil || !strings.Contains(err.Error(), "unknown signing key") {
			t.Fatalf("verify: %v", err)
		}
	}
	if got := atomic.LoadInt64(&fetches); got != 2 {
		t.Fatalf("fetches=%d want 2 (prime + one forced refresh for concurrent bogus kids)", got)
	}
}
