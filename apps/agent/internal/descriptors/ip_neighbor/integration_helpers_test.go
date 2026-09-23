package ipneighbor

import (
	"fmt"
	"testing"

	"ngfw/agent/internal/descriptors/df2"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
)

func sprintf(format string, a ...any) string { return fmt.Sprintf(format, a...) }

func keys(kvs []scheduler.KV) []scheduler.Key {
	out := make([]scheduler.Key, 0, len(kvs))
	for _, kv := range kvs {
		out = append(out, kv.Key)
	}
	return out
}

func swIfIndexOf(t *testing.T, c vpp.Client, owner, name string) uint32 {
	t.Helper()
	ifs, err := df2.DumpInterfaces(t.Context(), c, owner)
	if err != nil {
		t.Fatal(err)
	}
	idx, err := ifs.Index(name)
	if err != nil {
		t.Fatal(err)
	}
	return uint32(idx)
}
