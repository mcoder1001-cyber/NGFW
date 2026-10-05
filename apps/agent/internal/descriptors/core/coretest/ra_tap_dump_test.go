package coretest

import (
	"context"
	"io"
	"ngfw/agent/binapi/interface_types"
	tapapi "ngfw/agent/binapi/tapv2"
	"testing"
)

func TestRawTAPDumpIncludesForeignMetadataAndFiltersIndex(t *testing.T) {
	v := New()
	first := v.AddTAP("tap7", "foreign:tap7", tapapi.SwInterfaceTapV2Details{ID: 7, HostNamespace: "/foreign/ns", HostIfName: "foreign0"})
	second := v.AddTAP("tap8", "owned:tap8", tapapi.SwInterfaceTapV2Details{ID: 8, HostNamespace: "/owned/ns", HostIfName: "owned0"})
	for _, tc := range []struct {
		index uint32
		want  int
	}{{^uint32(0), 2}, {first, 1}, {second, 1}, {999, 0}} {
		stream, err := tapapi.NewServiceClient(v).SwInterfaceTapV2Dump(context.Background(), &tapapi.SwInterfaceTapV2Dump{SwIfIndex: interface_types.InterfaceIndex(tc.index)})
		if err != nil {
			t.Fatal(err)
		}
		count := 0
		for {
			row, err := stream.Recv()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			count++
			if row.SwIfIndex == first && (row.HostNamespace != "/foreign/ns" || row.HostIfName != "foreign0" || row.ID != 7) {
				t.Fatal("foreign ownership metadata lost")
			}
		}
		if count != tc.want {
			t.Fatal("raw dump invented or hid interfaces", count, tc.want)
		}
	}
}
