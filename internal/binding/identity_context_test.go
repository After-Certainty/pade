package binding

import (
	"context"
	"testing"
)

func TestVerifiedIdentityFrom(t *testing.T) {
	t.Parallel()
	if _, ok := VerifiedIdentityFrom(context.Background()); ok {
		t.Fatal("expected absent identity on empty context")
	}
	// VerifiedIdentityFrom treats a missing value as absent; callers should
	// still pass a non-nil Context into Resolve/Probe.
	if _, ok := VerifiedIdentityFrom(context.TODO()); ok {
		t.Fatal("expected absent identity on empty context")
	}
	if _, ok := VerifiedIdentityFrom(WithVerifiedIdentity(context.Background(), VerifiedIdentity{Subject: "user:1"})); ok {
		t.Fatal("expected absent identity when IDToken empty")
	}
	ctx := WithVerifiedIdentity(context.Background(), VerifiedIdentity{
		Subject: "user:42",
		IDToken: "eyJhbGciOiJSUzI1NiJ9.payload.sig",
	})
	got, ok := VerifiedIdentityFrom(ctx)
	if !ok {
		t.Fatal("expected identity")
	}
	if got.Subject != "user:42" || got.IDToken != "eyJhbGciOiJSUzI1NiJ9.payload.sig" {
		t.Fatalf("got %+v", got)
	}
}
