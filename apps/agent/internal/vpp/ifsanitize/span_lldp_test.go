package ifsanitize_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"go.fd.io/govpp/api"

	classifyapi "ngfw/agent/binapi/classify"
	spanapi "ngfw/agent/binapi/span"
	"ngfw/agent/internal/vpp/ifsanitize"
)

// TestInheritedSPANAndLLDPAreCleared: a deleted interface leaves SPAN source mirrors (device and
// L2) and an LLDP enable on its index (TD-27); a Sanitize of the reused index clears them and
// leaves another index's mirror alone.
func TestInheritedSPANAndLLDPAreCleared(t *testing.T) {
	f, m := setup()
	s := m.If(7)
	s.Span[0][9] = spanapi.SPAN_STATE_API_RX_TX
	s.Span[1][11] = spanapi.SPAN_STATE_API_RX
	s.LLDP = true
	m.If(8).Span[0][7] = spanapi.SPAN_STATE_API_TX // another source mirroring to 7: not ours
	m.DeleteInterface(7)
	rep, err := ifsanitize.Sanitize(context.Background(), f, 7, "loop201")
	if err != nil {
		t.Fatal(err)
	}
	if d := m.Dirty(7); d != "" {
		t.Fatalf("still inherited: %s (%+v)", d, rep)
	}
	for _, w := range []string{"span device to sw_if_index 9", "span l2 to sw_if_index 11"} {
		if !slices.Contains(rep.Cleared, w) {
			t.Errorf("cleared %v, want %q", rep.Cleared, w)
		}
	}
	if !slices.Contains(rep.Reset, "lldp") {
		t.Errorf("reset %v, want lldp", rep.Reset)
	}
	if m.If(8).Span[0][7] != spanapi.SPAN_STATE_API_TX {
		t.Fatal("another index's mirror was touched")
	}
	if n := len(f.CallsNamed("sw_interface_span_enable_disable")); n != 2 {
		t.Fatalf("span disables %d, want 2", n)
	}
}

// TestSPANUnclearable: a mirror the disable does not remove is Unclearable (create: error).
func TestSPANUnclearable(t *testing.T) {
	f, m := setup()
	m.If(7).Span[0][9] = spanapi.SPAN_STATE_API_RX
	f.On("sw_interface_span_enable_disable", func(api.Message) ([]api.Message, error) {
		return []api.Message{&spanapi.SwInterfaceSpanEnableDisableReply{}}, nil // VPP ignores it
	})
	_, err := ifsanitize.Sanitize(context.Background(), f, 7, "loop201")
	if !errors.Is(err, ifsanitize.ErrUnclearable) || !strings.Contains(err.Error(), "span device") {
		t.Fatalf("err %v, want ErrUnclearable naming the span row", err)
	}
}

// TestProbeKeptWhenUnbindFails (TD-25 review L1): when the unbind of the probe table P fails after
// P was bound on an output ACL slot, P is left in VPP instead of deleted under the binding.
func TestProbeKeptWhenUnbindFails(t *testing.T) {
	f, m := setup()
	model := m.Handlers()["output_acl_set_interface"]
	bound := false
	f.On("output_acl_set_interface", func(req api.Message) ([]api.Message, error) {
		r := req.(*classifyapi.OutputACLSetInterface)
		if !r.IsAdd && bound {
			return nil, api.VPPApiError(-1) // unspecified failure
		}
		out, err := model(req)
		if r.IsAdd && err == nil {
			bound = true
		}
		return out, err
	})
	rep, err := ifsanitize.Sanitize(context.Background(), f, 7, "loop201")
	if err == nil || !strings.Contains(err.Error(), "left in VPP") {
		t.Fatalf("err %v, want the probe kept", err)
	}
	p := rep.Pops[0]
	if !m.Tables[p] || m.If(7).OutACL[0] != p {
		t.Fatalf("probe table %d deleted (tables %v, out acl %v)", p, m.Tables, m.If(7).OutACL)
	}
}
