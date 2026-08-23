package broker

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
)

func TestParseJWKSRejectsZeroUsableKeys(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		raw  []byte
		want string
	}{
		{
			name: "empty keys array",
			raw:  []byte(`{"keys":[]}`),
			want: "no usable RSA keys",
		},
		{
			name: "only invalid RSA material",
			raw: mustJSON(t, map[string]interface{}{
				"keys": []map[string]string{
					{"kty": "RSA", "kid": "k1", "alg": "RS256", "use": "sig", "n": "!!!", "e": "AQAB"},
				},
			}),
			want: "no usable RSA keys",
		},
		{
			name: "EC keys skipped leaving none",
			raw: mustJSON(t, map[string]interface{}{
				"keys": []map[string]string{
					{"kty": "EC", "kid": "k1", "crv": "P-256", "x": "x", "y": "y"},
				},
			}),
			want: "no usable RSA keys",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := parseJWKS(tc.raw)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("parseJWKS() err=%v want substring %q", err, tc.want)
			}
		})
	}
}

func FuzzParseJWKS(f *testing.F) {
	f.Add([]byte(`{"keys":[]}`))
	f.Add([]byte(`not json`))
	pubN := base64.RawURLEncoding.EncodeToString([]byte{0x01})
	pubE := base64.RawURLEncoding.EncodeToString([]byte{0x01, 0x00, 0x01})
	f.Add(mustJSON(f, map[string]interface{}{
		"keys": []map[string]string{
			{"kty": "RSA", "kid": "k1", "alg": "RS256", "use": "sig", "n": pubN, "e": pubE},
		},
	}))
	f.Fuzz(func(t *testing.T, data []byte) {
		_, _ = parseJWKS(data)
	})
}

func mustJSON(tb testing.TB, v interface{}) []byte {
	tb.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		tb.Fatal(err)
	}
	return raw
}
