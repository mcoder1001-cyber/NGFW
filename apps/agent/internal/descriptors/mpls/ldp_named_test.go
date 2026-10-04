package mpls

import (
	"errors"
	"fmt"
	"testing"

	"ngfw/agent/internal/descriptors/df7"
	"ngfw/agent/internal/descriptors/df7/df7test"
	"ngfw/agent/internal/descriptors/dfkit"
)

func TestNamedRouteIsolationRestartCollision(t *testing.T) {
	f, tables, _, _, _ := fakeMPLS()
	ctx := t.Context()
	df7.SetBootStore(df7test.Owner, nil)
	for _, table := range []uint32{0, 100} {
		tables[table] = df7test.Owner + ":" + fmt.Sprint(table)
		named := NewNamedRoute("mpls-route.ldp", f, df7test.Owner)
		static := NewRoute(f, df7test.Owner)
		route := Route{Table: table, Label: 16000, EOS: true, EOSProto: PayloadIP4, Paths: paths(t, df7.Path{Type: df7.PathDrop})}
		obj := df7.Encode(route)
		if _, err := named.Create(ctx, obj); err != nil {
			t.Fatal(err)
		}
		if got, err := static.Retrieve(ctx); err != nil || len(got) != 0 {
			t.Fatalf("static adopted LDP: %+v %v", got, err)
		}
		if _, err := static.Create(ctx, obj); !errors.Is(err, dfkit.ErrNotOurs) {
			t.Fatalf("collision: %v", err)
		}
		if _, err := NewIPBind(f, df7test.Owner).Create(ctx, df7.Encode(IPBind{MPLSTable: table, Label: 16000, Prefix: "198.51.100.0/24"})); !errors.Is(err, dfkit.ErrNotOurs) {
			t.Fatalf("IP binding collision: %v", err)
		}
		restarted := NewNamedRoute("mpls-route.ldp", f, df7test.Owner)
		got, err := restarted.Retrieve(ctx)
		if err != nil || len(got) != 1 || got[0].Key != restarted.KeyOf(obj) {
			t.Fatalf("restart: %+v %v", got, err)
		}
		if err := restarted.Delete(ctx, obj, nil); err != nil {
			t.Fatal(err)
		}
	}
}
