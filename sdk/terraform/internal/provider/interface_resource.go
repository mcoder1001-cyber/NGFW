package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"ngfw/sdk/terraform/internal/client"
)

// vrx_interface — the typed resource of one entry of /interfaces. Its attributes are GENERATED from the
// InterfacesConfig JSON Schema (zz_interface_schema_gen.go); the conversion to and from the API document is generic.
type interfaceResource struct{ p *providerData }

var (
	_ resource.ResourceWithImportState = (*interfaceResource)(nil)
	_ resource.ResourceWithConfigure   = (*interfaceResource)(nil)
	// the interface name pattern of the schema's propertyNames (logical names, D-069)
	ifNameRE = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]*(?:/[0-9a-fA-F]+)*$`)
	// provider-owned attributes that are not members of the API object
	ifaceOwnAttrs = map[string]bool{"id": true, "name": true, "revision": true}
)

func newInterfaceResource() resource.Resource { return &interfaceResource{} }

func (r *interfaceResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_interface"
}

func (r *interfaceResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := interfaceGeneratedAttributes()
	attrs["id"] = schema.StringAttribute{Computed: true, MarkdownDescription: "The interface name.",
		PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}}
	attrs["name"] = schema.StringAttribute{Required: true,
		MarkdownDescription: "Logical interface name (the key under `/interfaces`), e.g. `loop1`, `TenGigabitEthernet0/0/0`. Changing it replaces the interface.",
		PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()}}
	attrs["revision"] = schema.Int64Attribute{Computed: true,
		MarkdownDescription: "Revision created by the last commit of this resource."}
	resp.Schema = schema.Schema{
		MarkdownDescription: "One interface of the VRX configuration (`/interfaces/<name>`); attributes generated from the " +
			"interfaces JSON Schema. Each apply is a confirmed commit. Import with the name: `terraform import vrx_interface.x loop1`.",
		Attributes: attrs,
	}
}

func (r *interfaceResource) Configure(_ context.Context, req resource.ConfigureRequest, _ *resource.ConfigureResponse) {
	if d, ok := req.ProviderData.(*providerData); ok {
		r.p = d
	}
}

func ifacePointer(name string) (string, error) {
	if !ifNameRE.MatchString(name) || len(name) > 63 {
		return "", fmt.Errorf("invalid interface name %q", name)
	}
	return "/interfaces/" + client.EscapeToken(name), nil
}

func (r *interfaceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var name types.String
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("name"), &name)...)
	if resp.Diagnostics.HasError() {
		return
	}
	ptr, err := ifacePointer(name.ValueString())
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("name"), "invalid name", err.Error())
		return
	}
	body, err := tfToJSON(req.Plan.Raw, interfaceJSONNames, ifaceOwnAttrs)
	if err != nil {
		resp.Diagnostics.AddError("cannot encode the interface", err.Error())
		return
	}
	res, err := r.p.client.Apply(ctx, r.p.confirmSec, r.p.comment+": create "+ptr, func(ctx context.Context) error {
		if _, err := r.p.client.Running(ctx, ptr); err == nil {
			return fmt.Errorf("%s already exists in the running configuration — import it: terraform import <address> %s", ptr, name.ValueString())
		} else if !client.IsNotFound(err) {
			return err
		}
		return r.p.client.Put(ctx, ptr, body)
	})
	if err != nil {
		addAPIError(&resp.Diagnostics, "create "+ptr, err)
		return
	}
	resp.State.Raw = req.Plan.Raw
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), name.ValueString())...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("revision"), revisionOf(res))...)
}

func (r *interfaceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var name types.String
	var rev types.Int64
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("name"), &name)...)
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("revision"), &rev)...)
	if resp.Diagnostics.HasError() {
		return
	}
	ptr, err := ifacePointer(name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("invalid name in state", err.Error())
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
	obj, ok := live.(map[string]any)
	if !ok {
		resp.Diagnostics.AddError("unexpected API answer", fmt.Sprintf("%s is not an object", ptr))
		return
	}
	obj["id"], obj["name"] = name.ValueString(), name.ValueString()
	obj["revision"] = json.Number("0")
	if !rev.IsNull() && !rev.IsUnknown() {
		obj["revision"] = json.Number(fmt.Sprint(rev.ValueInt64()))
	}
	typ := resp.State.Schema.Type().TerraformType(ctx)
	v, err := jsonToTF(typ, obj, interfaceJSONNames)
	if err != nil {
		resp.Diagnostics.AddError("cannot decode the interface", err.Error())
		return
	}
	resp.State.Raw = v
}

func (r *interfaceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var name types.String
	resp.Diagnostics.Append(req.Plan.GetAttribute(ctx, path.Root("name"), &name)...)
	if resp.Diagnostics.HasError() {
		return
	}
	ptr, err := ifacePointer(name.ValueString())
	if err != nil {
		resp.Diagnostics.AddAttributeError(path.Root("name"), "invalid name", err.Error())
		return
	}
	body, err := tfToJSON(req.Plan.Raw, interfaceJSONNames, ifaceOwnAttrs)
	if err != nil {
		resp.Diagnostics.AddError("cannot encode the interface", err.Error())
		return
	}
	res, err := r.p.client.Apply(ctx, r.p.confirmSec, r.p.comment+": update "+ptr, func(ctx context.Context) error {
		return r.p.client.Put(ctx, ptr, body)
	})
	if err != nil {
		addAPIError(&resp.Diagnostics, "update "+ptr, err)
		return
	}
	resp.State.Raw = req.Plan.Raw
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), name.ValueString())...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("revision"), revisionOf(res))...)
}

func (r *interfaceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var name types.String
	resp.Diagnostics.Append(req.State.GetAttribute(ctx, path.Root("name"), &name)...)
	if resp.Diagnostics.HasError() {
		return
	}
	ptr, err := ifacePointer(name.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("invalid name in state", err.Error())
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

func (r *interfaceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if _, err := ifacePointer(req.ID); err != nil {
		resp.Diagnostics.AddError("import id must be an interface name", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("name"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}
