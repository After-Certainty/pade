package providerset_test

import (
	"testing"

	"github.com/After-Certainty/pade/internal/providerset"
)

func TestConsumerRegistryOmitsExec(t *testing.T) {
	t.Parallel()
	reg := providerset.Consumer()
	if _, ok := reg.Get("exec"); ok {
		t.Fatal("Consumer registry must not register provider: exec")
	}
	for _, name := range []string{"env", "vault", "onepassword", "keeper", "keeper-secrets-manager", "broker"} {
		if _, ok := reg.Get(name); !ok {
			t.Fatalf("Consumer registry missing %q", name)
		}
	}
}

func TestBrokerRegistryIncludesExecOmitsBroker(t *testing.T) {
	t.Parallel()
	reg := providerset.Broker()
	if _, ok := reg.Get("exec"); !ok {
		t.Fatal("Broker registry must register provider: exec")
	}
	if _, ok := reg.Get("broker"); ok {
		t.Fatal("Broker registry must not nest provider: broker")
	}
	for _, name := range []string{"env", "vault", "onepassword", "keeper", "keeper-secrets-manager"} {
		if _, ok := reg.Get(name); !ok {
			t.Fatalf("Broker registry missing %q", name)
		}
	}
}
