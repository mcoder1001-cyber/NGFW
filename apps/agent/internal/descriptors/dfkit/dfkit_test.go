package dfkit_test

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"testing"

	"go.fd.io/govpp/adapter"
	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/dfkit/dfkittest"
)

type spec struct {
	Name  string `json:"name"`
	Count uint32 `json:"count"`
	On    bool   `json:"on"`
}

func TestCodec(t *testing.T) {
	a := dfkit.Encode(spec{Name: "x", Count: 4294967295, On: true})
	b := dfkit.Encode(spec{On: true, Count: 4294967295, Name: "x"})
	if !proto.Equal(a, b) || len(a.Fields) != 3 {
		t.Fatalf("encode not canonical: %v vs %v", a, b)
	}
	if !proto.Equal(dfkit.Encode(spec{}), dfkit.Encode(spec{})) || len(dfkit.Encode(spec{}).Fields) != 3 {
		t.Fatal("zero values must be encoded")
	}
	var got spec
	if err := dfkit.Decode(a, &got); err != nil || got != (spec{Name: "x", Count: 4294967295, On: true}) {
		t.Fatalf("decode %+v %v", got, err)
	}
	extra, _ := structpb.NewStruct(map[string]any{"name": "x", "bogus": 1})
	wrong, _ := structpb.NewStruct(map[string]any{"name": 5})
	for _, bad := range []proto.Message{extra, wrong, &structpb.Value{}, nil} {
		if err := dfkit.Decode(bad, &got); !errors.Is(err, dfkit.ErrSpec) {
			t.Errorf("%v: %v", bad, err)
		}
	}
}

func TestAddr(t *testing.T) {
	for in, want := range map[string]string{"10.5.0.1": "10.5.0.1", "::ffff:10.5.0.1": "10.5.0.1", "FD00::1": "fd00::1"} {
		a, err := dfkit.ParseAddr(in)
		if err != nil || a.String() != want {
			t.Errorf("%s: %v %v", in, a, err)
		}
		if back := dfkit.FromAPIAddress(dfkit.ToAPIAddress(a)); back != a {
			t.Errorf("round trip %s → %s", a, back)
		}
	}
	if _, err := dfkit.ParseAddr("fe80::1%eth0"); !errors.Is(err, dfkit.ErrSpec) {
		t.Fatal(err)
	}
	if _, err := dfkit.ParsePrefix("10.5.0.1/16"); !errors.Is(err, dfkit.ErrSpec) {
		t.Fatal(err)
	}
	if p, err := dfkit.ParsePrefix("10.5.0.0/16"); err != nil || dfkit.Family(p.Addr()) != "ip4" || dfkit.Family(netip.IPv6Loopback()) != "ip6" {
		t.Fatal(err)
	}
}

func TestInterfaces(t *testing.T) {
	f := dfkittest.NewFake(dfkittest.Iface{Index: 1, Name: "loop501", Tag: "w5:loop501"}, dfkittest.Iface{Index: 2, Name: "loop601", Tag: "w6:loop601"})
	tbl, err := dfkit.DumpInterfaces(context.Background(), f, "w5")
	if err != nil {
		t.Fatal(err)
	}
	if idx, err := tbl.Resolve("loop501"); err != nil || idx != 1 {
		t.Fatal(idx, err)
	}
	if _, err := tbl.Resolve("loop601"); !errors.Is(err, dfkit.ErrNotOwned) {
		t.Fatal(err)
	}
	if _, err := tbl.Resolve("nope"); !errors.Is(err, dfkit.ErrNoInterface) {
		t.Fatal(err)
	}
	if n, ok := tbl.Reportable(2, "x.y"); ok || n != "" {
		t.Fatal(n)
	}
	if dfkit.DefaultInterfaceKey("loop501") != "interface/loop501" || dfkit.DefaultVRFKey("5001") != "vrf/5001" {
		t.Fatal("key schemes")
	}
}

func TestErrors(t *testing.T) {
	err := dfkit.RetrieveUnsupported("x.y")
	if !errors.Is(err, dfkit.ErrRetrieveUnsupported) || err.Error() != "x.y: vpp has no dump for this object type" {
		t.Fatal(err)
	}
	if !errors.Is(dfkit.PluginError("lcp", &adapter.UnknownMsgError{MsgName: "m"}), dfkit.ErrPluginNotLoaded) {
		t.Fatal("plugin error")
	}
	if dfkit.PluginError("lcp", nil) != nil {
		t.Fatal("nil")
	}
	wrapped := fmt.Errorf("ctx: %w", api.VPPApiError(api.VALUE_EXIST))
	if !dfkit.IsVPPError(wrapped, api.NO_SUCH_ENTRY, api.VALUE_EXIST) || dfkit.IsVPPError(wrapped, api.NO_SUCH_ENTRY) || dfkit.IsVPPError(errors.New("x"), api.VALUE_EXIST) {
		t.Fatal("IsVPPError")
	}
}
