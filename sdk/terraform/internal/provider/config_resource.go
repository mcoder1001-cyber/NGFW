package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"ngfw/sdk/terraform/internal/client"
)

// vrx_config — the generic resource: one JSON value at one JSON pointer of the configuration document. Works for
// every schema domain without provider changes (the API validates it). Each apply is its own confirmed commit.
type configResource struct{ p *providerData }

type configModel struct {
	ID                    types.String `tfsdk:"id"`
	Pointer               types.String `tfsdk:"pointer"`
	Value                 types.String `tfsdk:"value"`
	SensitiveValue        types.String `tfsdk:"sensitive_value"`
	SensitiveValueVersion types.Int64  `tfsdk:"sensitive_value_version"`
	Revision              types.Int64  `tfsdk:"revision"`
}

var (
	_ resource.ResourceWithImportState    = (*configResource)(nil)
	_ resource.ResourceWithValidateConfig = (*configResource)(nil)
	_ resource.ResourceWithConfigure      = (*configResource)(nil)
	_ resource.ResourceWithModifyPlan     = (*configResource)(nil)
)

func newConfigResource() resource.Resource { return &configResource{} }

func (r *configResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_config"
}

func (r *configResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "One JSON value at a JSON pointer of the VRX configuration (`PUT /api/v1/config/{pointer}`), " +
			"committed with confirmation on every apply. Import with the pointer: `terraform import vrx_config.x /interfaces/loop1`.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true, MarkdownDescription: "The pointer.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"pointer": schema.StringAttribute{Required: true,
				MarkdownDescription: "RFC 6901 pointer below the root, first token a configuration domain (e.g. `/interfaces/loop1`, `/system/hostname`).",
				PlanModifiers:       []planmodifier.String{samePointer{}, stringplanmodifier.RequiresReplace()}},
			"value": schema.StringAttribute{Required: true,
				MarkdownDescription: "The JSON value (use `jsonencode(...)`). Members the API fills in as schema defaults are not drift. " +
					"Write-only members (e.g. `passwordHash`) are refused here — put them into `sensitive_value`.",
				PlanModifiers: []planmodifier.String{semanticJSON{}}},
			"sensitive_value": schema.StringAttribute{Optional: true, Sensitive: true, WriteOnly: true,
				MarkdownDescription: "JSON merged over `value` when writing (objects member-wise, arrays index-wise) — for write-only " +
					"members such as `passwordHash`. Write-only: never stored in plan or state (Terraform ≥ 1.11); the API never returns it. " +
					"Change `sensitive_value_version` to send it again."},
			"sensitive_value_version": schema.Int64Attribute{Optional: true,
				MarkdownDescription: "Bump to re-send `sensitive_value` (it is not in state, so Terraform cannot see its changes)."},
			"revision": schema.Int64Attribute{Computed: true,
				MarkdownDescription: "Revision created by the last commit of this resource (0 when the commit changed nothing)."},
		},
	}
}

func (r *configResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if d, ok := req.ProviderData.(*providerData); ok {
		r.p = d
	}
}

func (r *configResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var m configModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	ptr := ""
	if !m.Pointer.IsUnknown() && !m.Pointer.IsNull() {
		p, err := client.NormalizePointer(m.Pointer.ValueString())
		if err != nil {
			resp.Diagnostics.AddAttributeError(path.Root("pointer"), "invalid pointer", err.Error())
			return
		}
		ptr = p
	}
	if !m.Value.IsUnknown() && !m.Value.IsNull() {
		v, err := decodeJSON(m.Value.ValueString())
		if err != nil {
			resp.Diagnostics.AddAttributeError(path.Root("value"), "value is not JSON", err.Error())
			return
		}
		if ptr != "" {
			if leaves := writeOnlyLeaves(v, ptr); len(leaves) > 0 {
				resp.Diagnostics.AddAttributeError(path.Root("value"), "write-only member in value",
					fmt.Sprintf("%s is write-only (a secret): move it into sensitive_value so it never appears in plans or state", strings.Join(leaves, ", ")))
			}
		}
	}
	if !m.SensitiveValue.IsUnknown() && !m.SensitiveValue.IsNull() {
		if _, err := decodeJSON(m.SensitiveValue.ValueString()); err != nil {
			// never echo the content
			resp.Diagnostics.AddAttributeError(path.Root("sensitive_value"), "sensitive_value is not JSON", "the value does not parse as JSON")
		}
	}
}

// body = value, with sensitive_value (from the configuration — write-only values are never in the plan) merged over it.
func (r *configResource) body(ctx context.Context, plan configModel, cfg configModel) (string, any, error) {
	ptr, err := client.NormalizePointer(plan.Pointer.ValueString())
	if err != nil {
		return "", nil, err
	}
	v, err := decodeJSON(plan.Value.ValueString())
	if err != nil {
		return "", nil, fmt.Errorf("value: %w", err)
	}
	if !cfg.SensitiveValue.IsNull() && !cfg.SensitiveValue.IsUnknown() {
		sv, err := decodeJSON(cfg.SensitiveValue.ValueString())
		if err != nil {
			return "", nil, fmt.Errorf("sensitive_value is not JSON")
		}
		v = deepMerge(v, sv)
	}
	_ = ctx
	return ptr, v, nil
}

func (r *configResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, cfg configModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	ptr, body, err := r.body(ctx, plan, cfg)
	if err != nil {
		resp.Diagnostics.AddError("invalid vrx_config", err.Error())
		return
	}
	res, err := r.p.client.Apply(ctx, r.p.confirmSec, r.p.comment+": create "+ptr, func(ctx context.Context) error {
		if _, err := r.p.client.Running(ctx, ptr); err == nil {
			return fmt.Errorf("%s already exists in the running configuration — import it: terraform import <address> %s", ptr, ptr)
		} else if !client.IsNotFound(err) {
			return err
		}
		return r.p.client.Put(ctx, ptr, body)
	})
	if err != nil {
		addAPIError(&resp.Diagnostics, "create "+ptr, err)
		return
	}
	plan.ID = types.StringValue(ptr)
	plan.SensitiveValue = types.StringNull()
	plan.Revision = types.Int64Value(revisionOf(res))
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *configResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var st configModel
	resp.Diagnostics.Append(req.State.Get(ctx, &st)...)
	if resp.Diagnostics.HasError() {
		return
	}
	ptr, err := client.NormalizePointer(st.Pointer.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("invalid pointer in state", err.Error())
		return
	}
	live, err := r.p.client.Running(ctx, ptr)
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		addAPIError(&resp.Diagnostics, "read "+ptr, err)
		return
	}
	live = normalizeNumbers(live)
	keep := false
	if !st.Value.IsNull() && !st.Value.IsUnknown() {
		if want, err := decodeJSON(st.Value.ValueString()); err == nil && jsonSubset(want, live) {
			keep = true // only schema defaults added by the API — not drift
		}
	}
	if !keep {
		st.Value = types.StringValue(canonicalJSON(live))
	}
	st.ID = types.StringValue(ptr)
	st.SensitiveValue = types.StringNull()
	if st.Revision.IsNull() || st.Revision.IsUnknown() {
		st.Revision = types.Int64Value(0)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, st)...)
}

func (r *configResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, cfg configModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	ptr, body, err := r.body(ctx, plan, cfg)
	if err != nil {
		resp.Diagnostics.AddError("invalid vrx_config", err.Error())
		return
	}
	res, err := r.p.client.Apply(ctx, r.p.confirmSec, r.p.comment+": update "+ptr, func(ctx context.Context) error {
		return r.p.client.Put(ctx, ptr, body)
	})
	if err != nil {
		addAPIError(&resp.Diagnostics, "update "+ptr, err)
		return
	}
	plan.ID = types.StringValue(ptr)
	plan.SensitiveValue = types.StringNull()
	plan.Revision = types.Int64Value(revisionOf(res))
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *configResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var st configModel
	resp.Diagnostics.Append(req.State.Get(ctx, &st)...)
	if resp.Diagnostics.HasError() {
		return
	}
	ptr, err := client.NormalizePointer(st.Pointer.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("invalid pointer in state", err.Error())
		return
	}
	_, err = r.p.client.Apply(ctx, r.p.confirmSec, r.p.comment+": delete "+ptr, func(ctx context.Context) error {
		if err := r.p.client.Delete(ctx, ptr); err != nil && !client.IsNotFound(err) {
			return err
		}
		return nil
	})
	if err != nil {
		addAPIError(&resp.Diagnostics, "delete "+ptr, err)
	}
}

func (r *configResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	ptr, err := client.NormalizePointer(req.ID)
	if err != nil {
		resp.Diagnostics.AddError("import id must be a JSON pointer", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("pointer"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), ptr)...)
}

// ModifyPlan runs after the attribute plan modifiers: when pointer, value and sensitive_value_version are
// (semantically) unchanged there is nothing to commit, so `revision` keeps its prior value instead of becoming
// "known after apply" — otherwise a reformatted JSON string would still show as an in-place update.
func (r *configResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}
	var st, pl configModel
	resp.Diagnostics.Append(req.State.Get(ctx, &st)...)
	resp.Diagnostics.Append(req.Plan.Get(ctx, &pl)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if pl.Pointer.Equal(st.Pointer) && pl.Value.Equal(st.Value) && pl.SensitiveValueVersion.Equal(st.SensitiveValueVersion) {
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("revision"), st.Revision)...)
	}
}

// semanticJSON keeps the prior string when the configured JSON means the same (key order, whitespace, 1.0 vs 1).
type semanticJSON struct{}

func (semanticJSON) Description(context.Context) string {
	return "JSON values that are semantically equal are not a change"
}
func (m semanticJSON) MarkdownDescription(ctx context.Context) string { return m.Description(ctx) }
func (semanticJSON) PlanModifyString(_ context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if req.StateValue.IsNull() || req.StateValue.IsUnknown() || req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	a, err1 := decodeJSON(req.StateValue.ValueString())
	b, err2 := decodeJSON(req.ConfigValue.ValueString())
	if err1 == nil && err2 == nil && jsonEqual(a, b) {
		resp.PlanValue = req.StateValue
	}
}

// samePointer: `interfaces/loop1/` and `/interfaces/loop1` address the same node — not a replacement.
type samePointer struct{}

func (samePointer) Description(context.Context) string {
	return "equivalent JSON pointers are not a change"
}
func (m samePointer) MarkdownDescription(ctx context.Context) string { return m.Description(ctx) }
func (samePointer) PlanModifyString(_ context.Context, req planmodifier.StringRequest, resp *planmodifier.StringResponse) {
	if req.StateValue.IsNull() || req.StateValue.IsUnknown() || req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	a, err1 := client.NormalizePointer(req.StateValue.ValueString())
	b, err2 := client.NormalizePointer(req.ConfigValue.ValueString())
	if err1 == nil && err2 == nil && a == b {
		resp.PlanValue = req.StateValue
	}
}

func revisionOf(r *client.CommitResult) int64 {
	if r == nil || r.Revision == nil {
		return 0
	}
	return r.Revision.ID
}
