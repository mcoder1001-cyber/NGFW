package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
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
	Value                 jsonValue    `tfsdk:"value"`
	SensitiveValue        types.String `tfsdk:"sensitive_value"`
	SensitiveValueVersion types.Int64  `tfsdk:"sensitive_value_version"`
	Revision              types.Int64  `tfsdk:"revision"`
}

// privateNode is the private-state key holding the node exactly as the API stored it after our last write (or as
// last seen on import). Read reports drift when the live node differs from it — including members added out of band.
const privateNode = "node"

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
				MarkdownDescription: "RFC 6901 pointer in canonical form (leading `/`, no trailing `/`), first token a configuration " +
					"domain, e.g. `/interfaces/loop1`, `/system/hostname`. Changing it replaces the resource.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"value": schema.StringAttribute{Required: true, CustomType: jsonType{},
				MarkdownDescription: "The JSON value (use `jsonencode(...)`); the whole node is replaced (PUT). JSON that means the " +
					"same is not drift. Members the API fills in as schema defaults are not drift; members changed or ADDED out of band " +
					"are (the next apply would remove them, and the plan shows it). Write-only members (e.g. `passwordHash`) are refused " +
					"here — put them into `sensitive_value`."},
			"sensitive_value": schema.StringAttribute{Optional: true, Sensitive: true, WriteOnly: true,
				MarkdownDescription: "JSON merged over `value` when writing — for write-only members such as `passwordHash`. Objects merge " +
					"member-wise; arrays with an item key (`management.users` by `username`) merge by that key, other arrays must mirror " +
					"`value` element by element. Write-only: never stored in plan or state (Terraform ≥ 1.11); the API never returns it. " +
					"Change `sensitive_value_version` to send it again."},
			"sensitive_value_version": schema.Int64Attribute{Optional: true,
				MarkdownDescription: "Bump to re-send `sensitive_value` (it is not in state, so Terraform cannot see its changes)."},
			"revision": schema.Int64Attribute{Computed: true,
				MarkdownDescription: "Revision created by the last commit of this resource; 0 after an import or when the last apply " +
					"committed nothing (the value was already in place)."},
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
	if !m.Pointer.IsUnknown() && !m.Pointer.IsNull() {
		p, err := client.NormalizePointer(m.Pointer.ValueString())
		if err != nil {
			resp.Diagnostics.AddAttributeError(path.Root("pointer"), "invalid pointer", err.Error())
			return
		}
		if p != m.Pointer.ValueString() {
			resp.Diagnostics.AddAttributeError(path.Root("pointer"), "non-canonical pointer",
				fmt.Sprintf("write the pointer as %q (leading slash, no trailing slash) — state and imports use that form", p))
			return
		}
	}
	checkValue(&resp.Diagnostics, m.Pointer, m.Value)
	if !m.SensitiveValue.IsUnknown() && !m.SensitiveValue.IsNull() {
		if _, err := decodeJSON(m.SensitiveValue.ValueString()); err != nil {
			// never echo the content
			resp.Diagnostics.AddAttributeError(path.Root("sensitive_value"), "sensitive_value is not JSON", "the value does not parse as JSON")
		}
	}
}

// checkValue: `value` must be JSON without write-only members. Runs at validate AND at plan (ModifyPlan) — a value
// that depends on another resource is unknown at validate time — and once more in body() before any HTTP call.
func checkValue(d *diag.Diagnostics, ptr types.String, v jsonValue) {
	if v.IsUnknown() || v.IsNull() {
		return
	}
	j, err := decodeJSON(v.ValueString())
	if err != nil {
		d.AddAttributeError(path.Root("value"), "value is not JSON", err.Error())
		return
	}
	if ptr.IsUnknown() || ptr.IsNull() {
		return
	}
	p, err := client.NormalizePointer(ptr.ValueString())
	if err != nil {
		return
	}
	if leaves := writeOnlyLeaves(j, p); len(leaves) > 0 {
		d.AddAttributeError(path.Root("value"), "write-only member in value",
			fmt.Sprintf("%s is write-only (a secret): move it into sensitive_value so it never appears in plans or state", strings.Join(leaves, ", ")))
	}
}

// ModifyPlan (after the attribute plan modifiers): the write-only guard on the planned value, and `revision` keeps its
// prior value when nothing that is written changes. Configured attributes are never rewritten.
func (r *configResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.Plan.Raw.IsNull() {
		return
	}
	var pl configModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &pl)...)
	if resp.Diagnostics.HasError() {
		return
	}
	checkValue(&resp.Diagnostics, pl.Pointer, pl.Value)
	if resp.Diagnostics.HasError() || req.State.Raw.IsNull() {
		return
	}
	var st configModel
	resp.Diagnostics.Append(req.State.Get(ctx, &st)...)
	if pl.Pointer.Equal(st.Pointer) && pl.Value.Equal(st.Value) && pl.SensitiveValueVersion.Equal(st.SensitiveValueVersion) {
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("revision"), st.Revision)...)
	}
}

// body = value, with sensitive_value (from the configuration — write-only values are never in the plan) merged over it.
func (r *configResource) body(plan configModel, cfg configModel) (string, any, error) {
	ptr, err := client.NormalizePointer(plan.Pointer.ValueString())
	if err != nil {
		return "", nil, err
	}
	v, err := decodeJSON(plan.Value.ValueString())
	if err != nil {
		return "", nil, fmt.Errorf("value: %w", err)
	}
	if leaves := writeOnlyLeaves(v, ptr); len(leaves) > 0 {
		return "", nil, fmt.Errorf("%s is write-only (a secret) and must not be in value (it would be stored in the state): use sensitive_value", strings.Join(leaves, ", "))
	}
	if !cfg.SensitiveValue.IsNull() && !cfg.SensitiveValue.IsUnknown() {
		sv, err := decodeJSON(cfg.SensitiveValue.ValueString())
		if err != nil {
			return "", nil, fmt.Errorf("sensitive_value is not JSON")
		}
		if v, err = mergeSensitive(v, sv, ptr); err != nil {
			return "", nil, fmt.Errorf("sensitive_value: %w", err)
		}
	}
	return ptr, v, nil
}

func (r *configResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, cfg configModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	ptr, body, err := r.body(plan, cfg)
	if err != nil {
		resp.Diagnostics.AddError("invalid vrx_config", err.Error())
		return
	}
	res, err := r.p.apply(ctx, "create "+ptr, client.Edit{Pointer: ptr, Want: body, Do: func(ctx context.Context) error {
		if _, err := r.p.client.Running(ctx, ptr); err == nil {
			return fmt.Errorf("%s already exists in the running configuration — import it first: terraform import <address> %s", ptr, ptr)
		} else if !client.IsNotFound(err) {
			return err
		}
		return r.p.client.Put(ctx, ptr, body)
	}})
	if err != nil {
		addAPIError(&resp.Diagnostics, "create "+ptr, err)
		return
	}
	if res.Status == "unchanged" {
		resp.Diagnostics.AddError("VRX: nothing was committed for create "+ptr, concurrentHint)
		return
	}
	plan.ID = types.StringValue(ptr)
	plan.SensitiveValue = types.StringNull()
	plan.Revision = types.Int64Value(revisionOf(res))
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
	r.rememberNode(ctx, ptr, resp.Private.SetKey, &resp.Diagnostics)
	r.p.reportCommit(&resp.Diagnostics, "create "+ptr, res)
}

// concurrentHint explains an edit that produced no commit although the plan changed something.
const concurrentHint = "the candidate showed no change after the edit although the plan changes this node — another run " +
	"with the same user may have discarded it (the candidate is per user: use one service user per pipeline). Re-run plan."

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
	stored, d := req.Private.GetKey(ctx, privateNode)
	resp.Diagnostics.Append(d...)
	var prev any
	if len(stored) > 0 {
		prev, _ = decodeJSON(string(stored))
	}
	// drift = the live node differs from what the API stored after our last write (schema defaults included there),
	// so members changed OR added out of band are reported; without a record (import) the live node is the value
	if len(stored) == 0 || !jsonEqual(prev, live) || st.Value.IsNull() || st.Value.IsUnknown() {
		st.Value = jsonOf(canonicalJSON(live))
		resp.Diagnostics.Append(resp.Private.SetKey(ctx, privateNode, []byte(canonicalJSON(live)))...)
	}
	st.ID = types.StringValue(ptr)
	st.SensitiveValue = types.StringNull()
	if st.Revision.IsNull() || st.Revision.IsUnknown() {
		st.Revision = types.Int64Value(0)
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, st)...)
}

func (r *configResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, cfg, prior configModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &prior)...)
	if resp.Diagnostics.HasError() {
		return
	}
	ptr, body, err := r.body(plan, cfg)
	if err != nil {
		resp.Diagnostics.AddError("invalid vrx_config", err.Error())
		return
	}
	res, err := r.p.apply(ctx, "update "+ptr, client.Edit{Pointer: ptr, Want: body, Do: func(ctx context.Context) error {
		return r.p.client.Put(ctx, ptr, body)
	}})
	if err != nil {
		addAPIError(&resp.Diagnostics, "update "+ptr, err)
		return
	}
	if res.Status == "unchanged" && r.changesDocument(ctx, ptr, prior, plan, cfg) {
		resp.Diagnostics.AddError("VRX: nothing was committed for update "+ptr, concurrentHint)
		return
	}
	plan.ID = types.StringValue(ptr)
	plan.SensitiveValue = types.StringNull()
	plan.Revision = types.Int64Value(revisionOf(res))
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
	r.rememberNode(ctx, ptr, resp.Private.SetKey, &resp.Diagnostics)
	r.p.reportCommit(&resp.Diagnostics, "update "+ptr, res)
}

// changesDocument: should this update have changed the running node? A reformatted `value`, or one that only
// restates what is already stored, legitimately commits nothing; a new sensitive_value version always writes.
func (r *configResource) changesDocument(ctx context.Context, ptr string, prior, plan, cfg configModel) bool {
	if !cfg.SensitiveValue.IsNull() && !plan.SensitiveValueVersion.Equal(prior.SensitiveValueVersion) {
		return true
	}
	live, err := r.p.client.Running(ctx, ptr)
	if err != nil {
		return true
	}
	want, _ := decodeJSON(plan.Value.ValueString())
	return !jsonSubset(want, normalizeNumbers(live))
}

func (r *configResource) rememberNode(ctx context.Context, ptr string, set func(context.Context, string, []byte) diag.Diagnostics, d *diag.Diagnostics) {
	live, err := r.p.client.Running(ctx, ptr)
	if err != nil {
		d.AddWarning("VRX: could not read back "+ptr, err.Error())
		return
	}
	d.Append(set(ctx, privateNode, []byte(canonicalJSON(normalizeNumbers(live))))...)
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
	res, err := r.p.apply(ctx, "delete "+ptr, client.Edit{Pointer: ptr, Absent: true, Do: func(ctx context.Context) error {
		if err := r.p.client.Delete(ctx, ptr); err != nil && !client.IsNotFound(err) {
			return err
		}
		return nil
	}})
	if err != nil {
		addAPIError(&resp.Diagnostics, "delete "+ptr, err)
		return
	}
	r.p.reportCommit(&resp.Diagnostics, "delete "+ptr, res)
}

func (r *configResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	ptr, err := client.NormalizePointer(req.ID)
	if err != nil {
		resp.Diagnostics.AddError("import id must be a JSON pointer", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("pointer"), ptr)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), ptr)...)
}

func revisionOf(r *client.CommitResult) int64 {
	if r == nil || r.Revision == nil {
		return 0
	}
	return r.Revision.ID
}
