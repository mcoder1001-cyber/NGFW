//go:build !vrxtestsecrets

package subsystems

import "testing"

// Review F2: in every build without the test-secrets tag (the product agent) the fixture hook is nil, so nothing but
// the (empty) store's Put can fill the WireGuard secrets.
func TestWireguardFixtureHookNilInProductBuild(t *testing.T) {
	if wireguardFixture != nil {
		t.Fatal("wireguardFixture is set in a build without the test-secrets tag: the file secret channel reached a product build")
	}
}
