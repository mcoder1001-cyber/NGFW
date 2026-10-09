package pppoe

import (
	"context"
	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"
	tapapi "ngfw/agent/binapi/tapv2"
	iface "ngfw/agent/internal/descriptors/interface"
	"ngfw/agent/internal/descriptors/interface/ifacetest"
	"ngfw/agent/internal/descriptors/tapv2"
	"ngfw/agent/internal/scheduler"
	"testing"
)

func TestCarrierTapReadbackProvidesCanonicalCreator(t *testing.T) {
	f := ifacetest.New()
	owned := f.Add("tap42", "tap", "w9:pppwan")
	ra := f.Add("tap43", "tap", "w9:ra_other")
	f.On("sw_interface_tap_v2_dump", func(api.Message) ([]api.Message, error) {
		return []api.Message{
			&tapapi.SwInterfaceTapV2Details{SwIfIndex: owned, ID: 42, HostIfName: "pt0123456789ab", HostNamespace: "ngp-0123456789ab", RxRingSz: 256, TxRingSz: 256},
			&tapapi.SwInterfaceTapV2Details{SwIfIndex: ra, ID: 43, HostIfName: "ra_other", HostNamespace: "/run/ngfw-ra/n/ra_other", RxRingSz: 256, TxRingSz: 256}}, nil
	})
	d := &CarrierTapDescriptor{Tap: tapv2.New(f, "w9"), Admit: func(context.Context, *tapv2.Tap) error { return nil }}
	rows, err := d.Retrieve(t.Context())
	if err != nil || len(rows) != 1 {
		t.Fatal(rows, err)
	}
	if rows[0].Key != scheduler.Join(CarrierTapName, "pppwan") {
		t.Fatal(rows[0].Key)
	}
	provided := d.ProvidedKeys(rows[0].Value)
	if len(provided) != 1 || provided[0] != "tapv2.tap/pppwan" {
		t.Fatal(provided)
	}
	alias := iface.NewAlias(f, "w9")
	expected := &iface.InterfaceAlias{Name: "pppwan", Creator: string(provided[0])}
	if _, err = alias.Create(t.Context(), expected); err != nil {
		t.Fatal(err)
	}
	aliases, err := alias.Retrieve(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, row := range aliases {
		if row.Key == iface.AliasKey("pppwan") {
			found = proto.Equal(row.Value, expected)
		}
	}
	if !found {
		t.Fatal("carrier alias cannot roundtrip canonical VPP TAP creator")
	}
}
