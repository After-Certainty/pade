package securehttp_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/After-Certainty/pade/internal/securehttp"
)

func TestValidateURL(t *testing.T) {
	t.Parallel()
	cases := []struct {
		url     string
		wantErr bool
	}{
		{"https://broker.example.com/v1", false},
		{"http://127.0.0.1:8787", false},
		{"http://localhost:8787", false},
		{"http://[::1]:8787", false},
		{"http://evil.example.com", true},
		{"http://192.168.1.1", true},
		{"ftp://localhost", true},
		{"", true},
	}
	for _, tc := range cases {
		err := securehttp.ValidateURL(tc.url)
		if tc.wantErr && err == nil {
			t.Fatalf("%s: expected error", tc.url)
		}
		if !tc.wantErr && err != nil {
			t.Fatalf("%s: unexpected error: %v", tc.url, err)
		}
	}
}

func TestClientRejectsHTTPSToHTTPRedirect(t *testing.T) {
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`ok`))
	}))
	t.Cleanup(plain.Close)

	httpsRedirect := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, plain.URL, http.StatusFound)
	}))
	t.Cleanup(httpsRedirect.Close)

	client := securehttp.Client(0)
	client.Transport = httpsRedirect.Client().Transport
	_, err := client.Get(httpsRedirect.URL)
	if err == nil {
		t.Fatal("expected redirect downgrade to be rejected")
	}
}

func TestClientAllowsLoopbackHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`ok`))
	}))
	t.Cleanup(srv.Close)

	if err := securehttp.ValidateURL(srv.URL); err != nil {
		t.Fatal(err)
	}
	resp, err := securehttp.Client(0).Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
}
