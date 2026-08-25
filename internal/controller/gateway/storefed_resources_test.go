package gateway

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"

	trafficpolicypkg "github.com/ngrok/ngrok-operator/internal/trafficpolicy"
	"github.com/ngrok/ngrok-operator/pkg/managerdriver"
)

// TestStoreFedResourcesCoverTrafficPolicyKinds guards the wiring miss that
// shipped with the passive TrafficPolicy migration: the canonical
// ngrok.com/v1 TrafficPolicy was added to CacheStores and to driver.Seed, but
// to no controller's store-fed watch list. The store was therefore correct at
// startup and stale forever after — resolution worked in a fresh cluster and
// silently broke on the first post-boot edit, which is exactly the shape of
// bug that survives manual testing.
//
// driver.BaseSeededTypes is the set of kinds the store is expected to hold.
// Any traffic-policy kind in it must also be registered here with a
// ControllerEventHandler, or the store's copy only ever refreshes on restart.
// Traffic-policy kinds are identified by the TrafficPolicyResource interface
// rather than a hardcoded list, so a third kind would be covered
// automatically.
//
// Both the HTTPRoute controller and its counterpart assert this independently:
// the Ingress and Gateway feature sets can each be enabled without the other,
// so neither can rely on the other's registrations.
func TestStoreFedResourcesCoverTrafficPolicyKinds(t *testing.T) {
	watched := make(map[reflect.Type]bool)
	for _, obj := range storeFedResources() {
		watched[reflect.TypeOf(obj)] = true
	}

	checked := 0
	for _, seeded := range managerdriver.BaseSeededTypes() {
		if _, isPolicy := seeded.(trafficpolicypkg.TrafficPolicyResource); !isPolicy {
			continue
		}
		checked++
		assert.True(t, watched[reflect.TypeOf(seeded)],
			"%T is seeded into the store but not registered in storeFedResources(); "+
				"the store would go stale after startup", seeded)
	}

	assert.Positive(t, checked, "expected at least one traffic-policy kind among the seeded types")
}
