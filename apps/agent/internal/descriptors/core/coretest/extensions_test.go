package coretest

// TD-23/D-134: unit tests of the extension registry itself (RegisterExtension, and the On collision
// guard installExtensions relies on) — no feature model logic here; see the README comment above
// RegisterExtension in fakevpp.go. TestOnExtensionCollisionPanics and TestRegisterExtensionDuplicatePanics
// drive the package's unexported state directly (white-box) so a deliberately-colliding pair of
// extensions never actually lands in the shared, package-level registry, where it would break every
// later New() call in this test binary.

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"go.fd.io/govpp/api"

	"ngfw/agent/internal/vpp/fake"
)

// probeMsg is a minimal api.Message double, so these registry tests need no real VPP binapi message.
type probeMsg struct{ name string }

func (m *probeMsg) GetMessageName() string          { return m.name }
func (m *probeMsg) GetCrcString() string            { return "0" }
func (m *probeMsg) GetMessageType() api.MessageType { return api.RequestMessage }

// TestRegisterExtensionCompose: two extensions hooking different VPP messages both take effect on
// every New() model, in registration order — neither silently drops the other's installer.
func TestRegisterExtensionCompose(t *testing.T) {
	RegisterExtension("td23-test-compose-a", func(v *VPP) {
		v.On("td23_probe_a", func(api.Message) ([]api.Message, error) {
			return []api.Message{&probeMsg{name: "td23_probe_a"}}, nil
		})
	})
	RegisterExtension("td23-test-compose-b", func(v *VPP) {
		v.On("td23_probe_b", func(api.Message) ([]api.Message, error) {
			return []api.Message{&probeMsg{name: "td23_probe_b"}}, nil
		})
	})

	v := New()
	if !v.Handles("td23_probe_a") {
		t.Error("extension a's handler is missing after New() — extension b silently replaced it")
	}
	if !v.Handles("td23_probe_b") {
		t.Error("extension b's handler is missing after New()")
	}

	req, reply := &probeMsg{name: "td23_probe_a"}, &probeMsg{}
	if err := v.Invoke(context.Background(), req, reply); err != nil {
		t.Fatalf("Invoke(td23_probe_a) = %v", err)
	}
	if reply.name != "td23_probe_a" {
		t.Errorf("reply.name = %q, want %q", reply.name, "td23_probe_a")
	}
}

// TestOnExtensionCollisionPanics is the F-rpf-adl-pbr × F-bridge-l2 mactime case: both would-be
// extensions hook "feature_is_enabled". Plain fake.Client.On would let the second one silently
// replace the first's model; VPP.On instead panics, naming both extensions and the message, so the
// collision is a build-time/test-time failure for whoever rebases second, not a silently broken
// model for whoever's extension installed first.
func TestOnExtensionCollisionPanics(t *testing.T) {
	v := &VPP{Client: fake.New()}
	v.installingExt = "rpf-adl-pbr"
	v.On("feature_is_enabled", func(api.Message) ([]api.Message, error) { return nil, nil })

	v.installingExt = "bridge-l2"
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("On did not panic when a second extension claimed an already-modelled message")
		}
		msg := fmt.Sprint(r)
		for _, want := range []string{"rpf-adl-pbr", "bridge-l2", "feature_is_enabled"} {
			if !strings.Contains(msg, want) {
				t.Errorf("panic message %q does not mention %q", msg, want)
			}
		}
	}()
	v.On("feature_is_enabled", func(api.Message) ([]api.Message, error) { return nil, nil })
}

// TestOnSameExtensionReplacesItsOwnRegistration: an extension augmenting its own earlier hook (or
// core's, installed before any extension) is normal and must not panic — only two *different*
// extension names colliding is a bug.
func TestOnSameExtensionReplacesItsOwnRegistration(t *testing.T) {
	v := &VPP{Client: fake.New()}
	// Unclaimed (core-phase) registration: installingExt is empty, as it is for install/installIfExt.
	v.On("sw_interface_dump", func(api.Message) ([]api.Message, error) { return nil, nil })

	v.installingExt = "bridge-l2"
	v.On("sw_interface_dump", func(api.Message) ([]api.Message, error) { return nil, nil }) // layer on core: fine
	v.On("sw_interface_dump", func(api.Message) ([]api.Message, error) { return nil, nil }) // replace own: fine
}

// TestRegisterExtensionDuplicatePanics: registering the same extension name twice (a copy-paste of
// the one-liner, or two features reusing a slug) is a bug caught immediately, not a silent
// last-registration-wins that drops the first extension's model.
func TestRegisterExtensionDuplicatePanics(t *testing.T) {
	RegisterExtension("td23-test-dup", func(*VPP) {})
	defer func() {
		if recover() == nil {
			t.Fatal("RegisterExtension did not panic on a duplicate name")
		}
	}()
	RegisterExtension("td23-test-dup", func(*VPP) {})
}
