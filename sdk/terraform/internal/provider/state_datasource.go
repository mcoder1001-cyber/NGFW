package provider

import (
	"context"
	"net/url"
	"regexp"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

// data "vrx_state" — live, read-only state (`GET /api/v1/state/<path>`) as JSON.
type stateDataSource struct{ p *providerData }

type stateModel struct {
	ID    types.String `tfsdk:"id"`
	Path  types.String `tfsdk:"path"`
	Query types.Map    `tfsdk:"query"`
	JSON  types.String `tfsdk:"json"`
}

var statePathRE = regexp.MustCompile(`^[a-z][a-z0-9-]*(?:/[A-Za-z0-9._~-]+)*$`)

func newStateDataSource() datasource.DataSource { return &stateDataSource{} }

func (d *stateDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_state"
}

func (d *stateDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Live state of the appliance (`GET /api/v1/state/<path>`): `system`, `interfaces`, `routes`, `drift`, `events`.",
		Attributes: map[string]schema.Attribute{
			"id":    schema.StringAttribute{Computed: true},
			"path":  schema.StringAttribute{Required: true, MarkdownDescription: "State path, e.g. `interfaces` or `routes`."},
			"query": schema.MapAttribute{Optional: true, ElementType: types.StringType, MarkdownDescription: "Query parameters, e.g. `{ vrf = \"default\" }`."},
			"json":  schema.StringAttribute{Computed: true, MarkdownDescription: "The answer as JSON (use `jsondecode`)."},
		},
	}
}

func (d *stateDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, _ *datasource.ConfigureResponse) {
	if pd, ok := req.ProviderData.(*providerData); ok {
		d.p = pd
	}
}

func (d *stateDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var m stateModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	p := m.Path.ValueString()
	if !statePathRE.MatchString(p) {
		resp.Diagnostics.AddAttributeError(path.Root("path"), "invalid state path", "expected e.g. `interfaces`, `system`, `routes`")
		return
	}
	q := url.Values{}
	if !m.Query.IsNull() {
		var qm map[string]string
		resp.Diagnostics.Append(m.Query.ElementsAs(ctx, &qm, false)...)
		for k, v := range qm {
			q.Set(k, v)
		}
	}
	v, err := d.p.client.State(ctx, p, q)
	if err != nil {
		addAPIError(&resp.Diagnostics, "read state "+p, err)
		return
	}
	m.ID = types.StringValue(p)
	m.JSON = types.StringValue(canonicalJSON(normalizeNumbers(v)))
	resp.Diagnostics.Append(resp.State.Set(ctx, m)...)
}
