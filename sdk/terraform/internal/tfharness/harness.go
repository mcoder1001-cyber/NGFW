// Package tfharness drives a provider through the Terraform plugin protocol (v6) the way Terraform core does —
// validate → plan (with core's proposed-new-state rule) → apply → refresh → re-plan, import, destroy — without the
// terraform CLI (not installed on this host; terraform-plugin-testing needs it). It also renders a plan summary
// with Terraform's masking of sensitive and write-only attributes, used as plan-output evidence.
package tfharness

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"sort"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

// H is one provider instance.
type H struct {
	// Warnings are the warning diagnostics of the last Apply / Configure ("summary: detail").
	Warnings []string
	ctx      context.Context
	srv      tfprotov6.ProviderServer
	schema   *tfprotov6.GetProviderSchemaResponse
}

// New starts the provider server in-process and fetches its schemas.
func New(p func() provider.Provider) (*H, error) {
	srv, err := providerserver.NewProtocol6WithError(p())()
	if err != nil {
		return nil, err
	}
	h := &H{ctx: context.Background(), srv: srv}
	h.schema, err = srv.GetProviderSchema(h.ctx, &tfprotov6.GetProviderSchemaRequest{})
	if err != nil {
		return nil, err
	}
	if err := diagErr(h.schema.Diagnostics); err != nil {
		return nil, err
	}
	return h, nil
}

// Configure validates and configures the provider with Go-native values.
func (h *H) Configure(cfg map[string]any) error {
	v, err := Build(h.schema.Provider.ValueType(), cfg)
	if err != nil {
		return err
	}
	dv, err := dyn(v)
	if err != nil {
		return err
	}
	vr, err := h.srv.ValidateProviderConfig(h.ctx, &tfprotov6.ValidateProviderConfigRequest{Config: dv})
	if err != nil {
		return err
	}
	if err := diagErr(vr.Diagnostics); err != nil {
		return err
	}
	r, err := h.srv.ConfigureProvider(h.ctx, &tfprotov6.ConfigureProviderRequest{Config: dv, TerraformVersion: "1.11.0-harness"})
	if err != nil {
		return err
	}
	h.Warnings = warnings(r.Diagnostics)
	return diagErr(r.Diagnostics)
}

// ResourceType is the object type of a resource.
func (h *H) ResourceType(name string) tftypes.Type { return h.res(name).ValueType() }

func (h *H) res(name string) *tfprotov6.Schema {
	s, ok := h.schema.ResourceSchemas[name]
	if !ok {
		panic("no resource schema " + name)
	}
	return s
}

// Config builds a resource configuration from Go-native values (missing attributes are null).
func (h *H) Config(res string, attrs map[string]any) (tftypes.Value, error) {
	return Build(h.ResourceType(res), attrs)
}

// Null is the null value of a resource (no prior state / destroy).
func (h *H) Null(res string) tftypes.Value { return tftypes.NewValue(h.ResourceType(res), nil) }

// Validate runs ValidateResourceConfig.
func (h *H) Validate(res string, config tftypes.Value) error {
	dv, err := dyn(config)
	if err != nil {
		return err
	}
	r, err := h.srv.ValidateResourceConfig(h.ctx, &tfprotov6.ValidateResourceConfigRequest{
		TypeName: res, Config: dv,
		ClientCapabilities: &tfprotov6.ValidateResourceConfigClientCapabilities{WriteOnlyAttributesAllowed: true},
	})
	if err != nil {
		return err
	}
	return diagErr(r.Diagnostics)
}

// Plan is the result of PlanResourceChange.
type Plan struct {
	Prior, Planned  tftypes.Value
	RequiresReplace []*tftypes.AttributePath
	Text            string // rendered like `terraform plan` (sensitive / write-only masked)
}

// NoChanges reports a plan that changes nothing (Terraform's "No changes.").
func (p *Plan) NoChanges() bool { return p.Prior.Equal(p.Planned) }

// PlanChange validates config and plans prior → config (config null = destroy).
func (h *H) PlanChange(res string, prior, config tftypes.Value) (*Plan, error) {
	if !config.IsNull() {
		if err := h.Validate(res, config); err != nil {
			return nil, err
		}
	}
	s := h.res(res)
	proposed := tftypes.NewValue(s.ValueType(), nil)
	if !config.IsNull() {
		var err error
		if proposed, err = proposedNew(s.Block.Attributes, prior, config); err != nil {
			return nil, err
		}
	}
	pdv, err := dyn(prior)
	if err != nil {
		return nil, err
	}
	cdv, err := dyn(config)
	if err != nil {
		return nil, err
	}
	ndv, err := dyn(proposed)
	if err != nil {
		return nil, err
	}
	r, err := h.srv.PlanResourceChange(h.ctx, &tfprotov6.PlanResourceChangeRequest{
		TypeName: res, PriorState: pdv, ProposedNewState: ndv, Config: cdv,
		ClientCapabilities: &tfprotov6.PlanResourceChangeClientCapabilities{DeferralAllowed: false},
	})
	if err != nil {
		return nil, err
	}
	if err := diagErr(r.Diagnostics); err != nil {
		return nil, err
	}
	planned, err := r.PlannedState.Unmarshal(s.ValueType())
	if err != nil {
		return nil, err
	}
	if !config.IsNull() {
		if err := planValid(s.Block.Attributes, config, planned, tftypes.NewAttributePath()); err != nil {
			return nil, fmt.Errorf("Provider produced invalid plan for %s: %w", res, err)
		}
	}
	p := &Plan{Prior: prior, Planned: planned, RequiresReplace: r.RequiresReplace}
	p.Text = Render(res, s, prior, planned, r.RequiresReplace)
	return p, nil
}

// Apply applies a plan; config is the same configuration the plan was made from.
func (h *H) Apply(res string, p *Plan, config tftypes.Value) (tftypes.Value, error) {
	s := h.res(res)
	pdv, _ := dyn(p.Prior)
	ndv, _ := dyn(p.Planned)
	cdv, _ := dyn(config)
	r, err := h.srv.ApplyResourceChange(h.ctx, &tfprotov6.ApplyResourceChangeRequest{
		TypeName: res, PriorState: pdv, PlannedState: ndv, Config: cdv,
	})
	if err != nil {
		return tftypes.Value{}, err
	}
	h.Warnings = warnings(r.Diagnostics)
	if err := diagErr(r.Diagnostics); err != nil {
		return tftypes.Value{}, err
	}
	st, err := r.NewState.Unmarshal(s.ValueType())
	if err != nil {
		return tftypes.Value{}, err
	}
	return st, consistent(res, p.Planned, st)
}

// consistent is Terraform core's post-apply check: every value known in the plan must be in the new state unchanged
// ("Provider produced inconsistent result after apply").
func consistent(res string, planned, st tftypes.Value) error {
	if planned.IsNull() {
		if !st.IsNull() {
			return fmt.Errorf("%s: destroy returned a non-null state", res)
		}
		return nil
	}
	var bad []string
	_ = tftypes.Walk(planned, func(p *tftypes.AttributePath, pv tftypes.Value) (bool, error) {
		if !pv.IsKnown() {
			return false, nil
		}
		if !pv.IsFullyKnown() {
			return true, nil
		}
		sv, _, err := tftypes.WalkAttributePath(st, p)
		if v, ok := sv.(tftypes.Value); err != nil || !ok || !v.Equal(pv) {
			bad = append(bad, p.String())
		}
		return false, nil
	})
	if len(bad) > 0 {
		return fmt.Errorf("provider produced inconsistent result after apply for %s: %s", res, strings.Join(bad, ", "))
	}
	return nil
}

// Read refreshes a state.
func (h *H) Read(res string, state tftypes.Value) (tftypes.Value, error) {
	s := h.res(res)
	dv, _ := dyn(state)
	r, err := h.srv.ReadResource(h.ctx, &tfprotov6.ReadResourceRequest{TypeName: res, CurrentState: dv})
	if err != nil {
		return tftypes.Value{}, err
	}
	if err := diagErr(r.Diagnostics); err != nil {
		return tftypes.Value{}, err
	}
	return r.NewState.Unmarshal(s.ValueType())
}

// Import imports by id and refreshes (terraform import).
func (h *H) Import(res, id string) (tftypes.Value, error) {
	s := h.res(res)
	r, err := h.srv.ImportResourceState(h.ctx, &tfprotov6.ImportResourceStateRequest{TypeName: res, ID: id})
	if err != nil {
		return tftypes.Value{}, err
	}
	if err := diagErr(r.Diagnostics); err != nil {
		return tftypes.Value{}, err
	}
	if len(r.ImportedResources) != 1 {
		return tftypes.Value{}, fmt.Errorf("import returned %d resources", len(r.ImportedResources))
	}
	st, err := r.ImportedResources[0].State.Unmarshal(s.ValueType())
	if err != nil {
		return tftypes.Value{}, err
	}
	return h.Read(res, st)
}

// ReadData reads a data source.
func (h *H) ReadData(ds string, attrs map[string]any) (tftypes.Value, error) {
	s := h.schema.DataSourceSchemas[ds]
	cfg, err := Build(s.ValueType(), attrs)
	if err != nil {
		return tftypes.Value{}, err
	}
	dv, _ := dyn(cfg)
	vr, err := h.srv.ValidateDataResourceConfig(h.ctx, &tfprotov6.ValidateDataResourceConfigRequest{TypeName: ds, Config: dv})
	if err != nil {
		return tftypes.Value{}, err
	}
	if err := diagErr(vr.Diagnostics); err != nil {
		return tftypes.Value{}, err
	}
	r, err := h.srv.ReadDataSource(h.ctx, &tfprotov6.ReadDataSourceRequest{TypeName: ds, Config: dv})
	if err != nil {
		return tftypes.Value{}, err
	}
	if err := diagErr(r.Diagnostics); err != nil {
		return tftypes.Value{}, err
	}
	return r.State.Unmarshal(s.ValueType())
}

// Attr returns a top-level attribute of an object value as a Go native (string, bool, *big.Float, []any, map, nil).
func Attr(v tftypes.Value, name string) any {
	var m map[string]tftypes.Value
	if v.IsNull() || v.As(&m) != nil {
		return nil
	}
	return Native(m[name])
}

// Native converts a tftypes value to Go natives (numbers as float64; unknown as the string "<unknown>").
func Native(v tftypes.Value) any {
	if !v.IsKnown() {
		return "<unknown>"
	}
	if v.IsNull() {
		return nil
	}
	t := v.Type()
	switch {
	case t.Is(tftypes.String):
		var s string
		_ = v.As(&s)
		return s
	case t.Is(tftypes.Bool):
		var b bool
		_ = v.As(&b)
		return b
	case t.Is(tftypes.Number):
		var f big.Float
		_ = v.As(&f)
		x, _ := f.Float64()
		return x
	case t.Is(tftypes.List{}), t.Is(tftypes.Set{}), t.Is(tftypes.Tuple{}):
		var l []tftypes.Value
		_ = v.As(&l)
		out := []any{}
		for _, e := range l {
			out = append(out, Native(e))
		}
		return out
	default:
		var m map[string]tftypes.Value
		_ = v.As(&m)
		out := map[string]any{}
		for k, e := range m {
			out[k] = Native(e)
		}
		return out
	}
}

// Build converts Go natives into a value of type t (nil / missing members → null).
func Build(t tftypes.Type, v any) (tftypes.Value, error) {
	if v == nil {
		return tftypes.NewValue(t, nil), nil
	}
	if tv, ok := v.(tftypes.Value); ok {
		return tv, nil
	}
	switch {
	case t.Is(tftypes.Object{}):
		m, ok := v.(map[string]any)
		if !ok {
			return tftypes.Value{}, fmt.Errorf("want map for object, got %T", v)
		}
		ot := t.(tftypes.Object)
		for k := range m {
			if _, ok := ot.AttributeTypes[k]; !ok {
				return tftypes.Value{}, fmt.Errorf("unknown attribute %q", k)
			}
		}
		vals := map[string]tftypes.Value{}
		for k, at := range ot.AttributeTypes {
			e, err := Build(at, m[k])
			if err != nil {
				return tftypes.Value{}, fmt.Errorf("%s: %w", k, err)
			}
			vals[k] = e
		}
		return tftypes.NewValue(t, vals), nil
	case t.Is(tftypes.Map{}):
		m, ok := v.(map[string]any)
		if !ok {
			return tftypes.Value{}, fmt.Errorf("want map, got %T", v)
		}
		vals := map[string]tftypes.Value{}
		for k, e := range m {
			ev, err := Build(t.(tftypes.Map).ElementType, e)
			if err != nil {
				return tftypes.Value{}, err
			}
			vals[k] = ev
		}
		return tftypes.NewValue(t, vals), nil
	case t.Is(tftypes.List{}):
		l, ok := v.([]any)
		if !ok {
			return tftypes.Value{}, fmt.Errorf("want []any, got %T", v)
		}
		vals := []tftypes.Value{}
		for _, e := range l {
			ev, err := Build(t.(tftypes.List).ElementType, e)
			if err != nil {
				return tftypes.Value{}, err
			}
			vals = append(vals, ev)
		}
		return tftypes.NewValue(t, vals), nil
	case t.Is(tftypes.Number):
		switch n := v.(type) {
		case int:
			return tftypes.NewValue(t, new(big.Float).SetInt64(int64(n))), nil
		case int64:
			return tftypes.NewValue(t, new(big.Float).SetInt64(n)), nil
		case float64:
			return tftypes.NewValue(t, big.NewFloat(n)), nil
		}
		return tftypes.Value{}, fmt.Errorf("want number, got %T", v)
	default:
		return tftypes.NewValue(t, v), nil
	}
}

// proposedNew follows Terraform core's objchange.ProposedNew for attribute-only schemas: configured values win;
// null configuration of a computed attribute takes the prior value; nested objects merge recursively; write-only
// attributes are never proposed (they travel in Config only).
func proposedNew(attrs []*tfprotov6.SchemaAttribute, prior, config tftypes.Value) (tftypes.Value, error) {
	var cm, pm map[string]tftypes.Value
	if err := config.As(&cm); err != nil {
		return tftypes.Value{}, err
	}
	if !prior.IsNull() {
		if err := prior.As(&pm); err != nil {
			return tftypes.Value{}, err
		}
	}
	out := map[string]tftypes.Value{}
	for _, a := range attrs {
		cv := cm[a.Name]
		pv, hasPrior := pm[a.Name]
		switch {
		case a.WriteOnly:
			out[a.Name] = tftypes.NewValue(cv.Type(), nil)
		case cv.IsNull() && a.Computed && hasPrior:
			out[a.Name] = pv
		case a.NestedType != nil && a.NestedType.Nesting == tfprotov6.SchemaObjectNestingModeSingle && hasPrior && !cv.IsNull() && !pv.IsNull():
			v, err := proposedNew(a.NestedType.Attributes, pv, cv)
			if err != nil {
				return tftypes.Value{}, err
			}
			out[a.Name] = v
		case a.NestedType != nil && a.NestedType.Nesting == tfprotov6.SchemaObjectNestingModeMap && hasPrior && !cv.IsNull() && !pv.IsNull() && cv.IsKnown():
			var ce, pe map[string]tftypes.Value
			_ = cv.As(&ce)
			_ = pv.As(&pe)
			elems := map[string]tftypes.Value{}
			for k, e := range ce {
				if p, ok := pe[k]; ok && !e.IsNull() {
					v, err := proposedNew(a.NestedType.Attributes, p, e)
					if err != nil {
						return tftypes.Value{}, err
					}
					elems[k] = v
				} else {
					elems[k] = e
				}
			}
			out[a.Name] = tftypes.NewValue(cv.Type(), elems)
		default:
			out[a.Name] = cv
		}
	}
	return tftypes.NewValue(config.Type(), out), nil
}

// planValid is Terraform core's objchange.AssertPlanValid for attribute-only schemas: a write-only attribute is planned
// null; a configured (non-null) value of a non-computed OR optional+computed attribute must be planned exactly as
// configured (nested attributes recurse with their own flags); a non-computed attribute that is not configured must be
// planned null. Only computed attributes the configuration leaves null may take any planned value.
func planValid(attrs []*tfprotov6.SchemaAttribute, config, planned tftypes.Value, at *tftypes.AttributePath) error {
	var cm, pm map[string]tftypes.Value
	if err := config.As(&cm); err != nil {
		return err
	}
	if planned.IsNull() || !planned.IsKnown() {
		return fmt.Errorf("%s: planned object is null/unknown for a configured object", at)
	}
	if err := planned.As(&pm); err != nil {
		return err
	}
	for _, a := range attrs {
		cv, pv := cm[a.Name], pm[a.Name]
		here := at.WithAttributeName(a.Name)
		switch {
		case a.WriteOnly:
			if !pv.IsNull() {
				return fmt.Errorf("%s: write-only attribute planned non-null", here)
			}
		case cv.IsNull():
			if !a.Computed && !pv.IsNull() {
				return fmt.Errorf("%s: planned %s for a non-computed attribute that is not configured", here, format(pv))
			}
		case !cv.IsKnown():
			if pv.IsKnown() && !a.Computed {
				return fmt.Errorf("%s: planned a known value for an unknown configuration value", here)
			}
		case a.NestedType != nil && a.NestedType.Nesting == tfprotov6.SchemaObjectNestingModeSingle:
			if err := planValid(a.NestedType.Attributes, cv, pv, here); err != nil {
				return err
			}
		case a.NestedType != nil && (a.NestedType.Nesting == tfprotov6.SchemaObjectNestingModeMap || a.NestedType.Nesting == tfprotov6.SchemaObjectNestingModeList):
			if !pv.IsKnown() || pv.IsNull() {
				return fmt.Errorf("%s: configured collection planned null/unknown", here)
			}
			var ce, pe map[string]tftypes.Value
			if a.NestedType.Nesting == tfprotov6.SchemaObjectNestingModeMap {
				_ = cv.As(&ce)
				_ = pv.As(&pe)
			} else {
				var cl, pl []tftypes.Value
				_ = cv.As(&cl)
				_ = pv.As(&pl)
				ce, pe = map[string]tftypes.Value{}, map[string]tftypes.Value{}
				for i, v := range cl {
					ce[fmt.Sprint(i)] = v
				}
				for i, v := range pl {
					pe[fmt.Sprint(i)] = v
				}
			}
			if len(ce) != len(pe) {
				return fmt.Errorf("%s: planned %d elements, configured %d", here, len(pe), len(ce))
			}
			for k, e := range ce {
				p, ok := pe[k]
				if !ok {
					return fmt.Errorf("%s[%s]: configured element missing from the plan", here, k)
				}
				if err := planValid(a.NestedType.Attributes, e, p, here.WithElementKeyString(k)); err != nil {
					return err
				}
			}
		default:
			if !pv.Equal(cv) {
				return fmt.Errorf("%s: planned value %s does not match config value %s", here, format(pv), format(cv))
			}
		}
	}
	return nil
}

// Render prints a plan like Terraform does, one top-level attribute per line; sensitive values are
// `(sensitive value)`, write-only ones `(write-only attribute)`.
func Render(res string, s *tfprotov6.Schema, prior, planned tftypes.Value, replace []*tftypes.AttributePath) string {
	var pm, nm map[string]tftypes.Value
	if !prior.IsNull() {
		_ = prior.As(&pm)
	}
	if !planned.IsNull() {
		_ = planned.As(&nm)
	}
	action := "~ update in-place"
	sym := "~"
	switch {
	case prior.IsNull():
		action, sym = "+ create", "+"
	case planned.IsNull():
		action, sym = "- destroy", "-"
	case len(replace) > 0:
		action, sym = "-/+ replace", "~"
	case prior.Equal(planned):
		return "No changes. Your infrastructure matches the configuration."
	}
	names := make([]string, 0, len(s.Block.Attributes))
	meta := map[string]*tfprotov6.SchemaAttribute{}
	for _, a := range s.Block.Attributes {
		names = append(names, a.Name)
		meta[a.Name] = a
	}
	sort.Strings(names)
	var b strings.Builder
	fmt.Fprintf(&b, "  # %s will be %s\n", res, strings.TrimLeft(action, "~+-/ "))
	fmt.Fprintf(&b, "  %s resource %q {\n", sym, res)
	for _, n := range names {
		a := meta[n]
		pv, nv := pm[n], nm[n]
		mask := func(v tftypes.Value) string {
			switch {
			case a.WriteOnly:
				return "(write-only attribute)"
			case a.Sensitive && !v.IsNull():
				return "(sensitive value)"
			}
			return format(v)
		}
		switch {
		case prior.IsNull():
			if nv.IsNull() && !a.WriteOnly {
				continue
			}
			fmt.Fprintf(&b, "      + %-24s = %s\n", n, mask(nv))
		case planned.IsNull():
			if pv.IsNull() {
				continue
			}
			fmt.Fprintf(&b, "      - %-24s = %s\n", n, mask(pv))
		case !pv.Equal(nv):
			fmt.Fprintf(&b, "      ~ %-24s = %s -> %s\n", n, mask(pv), mask(nv))
		}
	}
	b.WriteString("    }")
	return b.String()
}

func format(v tftypes.Value) string {
	if !v.IsKnown() {
		return "(known after apply)"
	}
	if v.IsNull() {
		return "null"
	}
	if n, ok := Native(v).(string); ok {
		return fmt.Sprintf("%q", n)
	}
	b, err := json.Marshal(Native(v))
	if err != nil {
		return fmt.Sprintf("%v", Native(v))
	}
	return string(b)
}

func dyn(v tftypes.Value) (*tfprotov6.DynamicValue, error) {
	dv, err := tfprotov6.NewDynamicValue(v.Type(), v)
	return &dv, err
}

func warnings(ds []*tfprotov6.Diagnostic) []string {
	var out []string
	for _, d := range ds {
		if d.Severity == tfprotov6.DiagnosticSeverityWarning {
			out = append(out, d.Summary+": "+d.Detail)
		}
	}
	return out
}

func diagErr(ds []*tfprotov6.Diagnostic) error {
	var msgs []string
	for _, d := range ds {
		if d.Severity == tfprotov6.DiagnosticSeverityError {
			m := d.Summary
			if d.Detail != "" {
				m += ": " + d.Detail
			}
			msgs = append(msgs, m)
		}
	}
	if len(msgs) == 0 {
		return nil
	}
	return fmt.Errorf("%s", strings.Join(msgs, "\n"))
}
