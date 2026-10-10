package binding_test

import (
	"github.com/After-Certainty/pade/internal/binding"
	"testing"
	"time"
)

func TestMaterialExpirationBoundary(t *testing.T) {
	now := time.Date(2035, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name  string
		delta time.Duration
		valid bool
	}{
		{"past", -time.Nanosecond, false}, {"exact", 0, false}, {"near future", time.Nanosecond, true}, {"future", time.Hour, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			expiry := now.Add(tc.delta)
			mat := &binding.Material{ExpiresAt: &expiry}
			if (mat.ValidateExpiration(now) == nil) != tc.valid {
				t.Fatal("incorrect expiration boundary")
			}
		})
	}
	if (&binding.Material{}).ValidateExpiration(now) != nil {
		t.Fatal("absent expiration rejected")
	}
	zero := time.Time{}
	if (&binding.Material{ExpiresAt: &zero}).ValidateExpiration(now) == nil {
		t.Fatal("explicit zero timestamp treated as unknown")
	}
}

func TestMergeRejectsMaterialThatExpiredAfterResolution(t *testing.T) {
	expired := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	results := []binding.ResolveResult{{Material: &binding.Material{Env: map[string]string{"TOKEN": "synthetic"}, ExpiresAt: &expired}}}
	if _, err := binding.MergeEnv(nil, results); err == nil {
		t.Fatal("expired material reached injection environment")
	}
}
