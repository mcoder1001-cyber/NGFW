package tfharness

import (
	"strings"
	"testing"

	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// The rule that caught review H1: a provider may not rewrite the planned value of a configured attribute.
func TestPlanValidRejectsRewrittenConfiguredValues(t *testing.T) {
	attrs := []*tfprotov6.SchemaAttribute{
		{Name: "value", Type: tftypes.String, Required: true},
		{Name: "revision", Type: tftypes.Number, Computed: true},
		{Name: "secret", Type: tftypes.String, Optional: true, WriteOnly: true},
		{Name: "vrf", Type: tftypes.String, Optional: true, Computed: true},
	}
	typ := tftypes.Object{AttributeTypes: map[string]tftypes.Type{"value": tftypes.String, "revision": tftypes.Number, "secret": tftypes.String, "vrf": tftypes.String}}
	obj := func(value, secret, vrf any) tftypes.Value {
		v, _ := Build(typ, map[string]any{"value": value, "secret": secret, "vrf": vrf})
		return v
	}
	ok := []struct{ cfg, plan tftypes.Value }{
		{obj(`{"a":1}`, "s", nil), obj(`{"a":1}`, nil, "default")},
	}
	for _, c := range ok {
		if err := planValid(attrs, c.cfg, c.plan, tftypes.NewAttributePath()); err != nil {
			t.Fatal(err)
		}
	}
	bad := map[string]struct{ cfg, plan tftypes.Value }{
		"does not match config value":  {obj(`{ "a": 1 }`, nil, nil), obj(`{"a":1}`, nil, nil)},
		"write-only attribute planned": {obj(`{"a":1}`, "s", nil), obj(`{"a":1}`, "s", nil)},
		"planned value \"x\"":          {obj(`{"a":1}`, nil, "y"), obj(`{"a":1}`, nil, "x")},
	}
	for want, c := range bad {
		if err := planValid(attrs, c.cfg, c.plan, tftypes.NewAttributePath()); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("want %q, got %v", want, err)
		}
	}
}
