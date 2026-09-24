package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// jsonType is a string attribute type holding a JSON document with SEMANTIC EQUALITY: when a Read or Apply
// returns a value that means the same JSON as the prior one (key order, whitespace, 1 vs 1.0), the framework keeps
// the prior string. Plans are never rewritten — the planned value of a configured attribute is always the configured
// string (Terraform core's plan-validity rule).
type jsonType struct{ basetypes.StringType }

var (
	_ basetypes.StringTypable                    = jsonType{}
	_ basetypes.StringValuableWithSemanticEquals = jsonValue{}
)

func (t jsonType) Equal(o attr.Type) bool { _, ok := o.(jsonType); return ok }
func (t jsonType) String() string         { return "provider.jsonType" }
func (t jsonType) ValueType(context.Context) attr.Value {
	return jsonValue{}
}

func (t jsonType) ValueFromString(_ context.Context, in basetypes.StringValue) (basetypes.StringValuable, diag.Diagnostics) {
	return jsonValue{StringValue: in}, nil
}

func (t jsonType) ValueFromTerraform(ctx context.Context, in tftypes.Value) (attr.Value, error) {
	v, err := t.StringType.ValueFromTerraform(ctx, in)
	if err != nil {
		return nil, err
	}
	sv, ok := v.(basetypes.StringValue)
	if !ok {
		return nil, fmt.Errorf("unexpected value type %T", v)
	}
	return jsonValue{StringValue: sv}, nil
}

type jsonValue struct{ basetypes.StringValue }

func jsonNull() jsonValue                          { return jsonValue{StringValue: basetypes.NewStringNull()} }
func jsonOf(s string) jsonValue                    { return jsonValue{StringValue: basetypes.NewStringValue(s)} }
func (v jsonValue) Type(context.Context) attr.Type { return jsonType{} }
func (v jsonValue) Equal(o attr.Value) bool {
	ov, ok := o.(jsonValue)
	return ok && v.StringValue.Equal(ov.StringValue)
}

func (v jsonValue) StringSemanticEquals(_ context.Context, nv basetypes.StringValuable) (bool, diag.Diagnostics) {
	var d diag.Diagnostics
	other, ok := nv.(jsonValue)
	if !ok {
		return false, d
	}
	if v.IsNull() || v.IsUnknown() || other.IsNull() || other.IsUnknown() {
		return false, d
	}
	a, err1 := decodeJSON(v.ValueString())
	b, err2 := decodeJSON(other.ValueString())
	return err1 == nil && err2 == nil && jsonEqual(a, b), d
}
