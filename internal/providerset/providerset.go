// Package providerset wires reference Consumer and Broker provider registries.
// Keeping presets outside package binding avoids import cycles with adapters.
package providerset

import (
	"github.com/After-Certainty/pade/internal/binding"
	brokerprovider "github.com/After-Certainty/pade/internal/binding/broker"
	envprovider "github.com/After-Certainty/pade/internal/binding/env"
	execprovider "github.com/After-Certainty/pade/internal/binding/exec"
	keeperprovider "github.com/After-Certainty/pade/internal/binding/keeper"
	keepersmprovider "github.com/After-Certainty/pade/internal/binding/keepersm"
	onepasswordprovider "github.com/After-Certainty/pade/internal/binding/onepassword"
	vaultprovider "github.com/After-Certainty/pade/internal/binding/vault"
)

// Consumer returns providers available to the reference Consumer.
// It intentionally omits provider: exec — arbitrary executable fulfillment is
// broker/operator-side only. The Consumer may still use provider: broker.
func Consumer() *binding.Registry {
	return binding.NewRegistry(
		envprovider.New(),
		vaultprovider.New(),
		onepasswordprovider.New(),
		keeperprovider.New(),
		keepersmprovider.New(),
		brokerprovider.New(),
	)
}

// Broker returns providers available to the reference Broker for server-side
// materialization, including provider: exec for operator-installed processes.
// It omits provider: broker (no nested broker calls).
func Broker() *binding.Registry {
	return binding.NewRegistry(
		envprovider.New(),
		vaultprovider.New(),
		onepasswordprovider.New(),
		keeperprovider.New(),
		keepersmprovider.New(),
		execprovider.New(),
	)
}
