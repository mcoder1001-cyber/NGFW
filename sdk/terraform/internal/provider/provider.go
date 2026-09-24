// Package provider implements the VRX Terraform provider (terraform-plugin-framework, protocol 6).
package provider

import (
	"context"
	"os"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"ngfw/sdk/terraform/internal/client"
)

// DefaultConfirmSeconds is the confirmed-commit window of every apply unless confirm_timeout says otherwise.
const DefaultConfirmSeconds = 60

type vrxProvider struct{ version string }

// New returns the provider factory used by main and the tests.
func New(version string) func() provider.Provider {
	return func() provider.Provider { return &vrxProvider{version: version} }
}

type providerModel struct {
	URL            types.String `tfsdk:"url"`
	APIKey         types.String `tfsdk:"api_key"`
	Insecure       types.Bool   `tfsdk:"insecure"`
	CAFile         types.String `tfsdk:"ca_file"`
	ConfirmTimeout types.Int64  `tfsdk:"confirm_timeout"`
	CommitComment  types.String `tfsdk:"commit_comment"`
}

// providerData is handed to every resource / data source.
type providerData struct {
	client     *client.Client
	confirmSec int64
	comment    string
}

func (p *vrxProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "vrx"
	resp.Version = p.version
}

func (p *vrxProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a VRX appliance through its REST API. Every apply edits the candidate, commits it " +
			"with `?confirm=<confirm_timeout>`, checks that the API still answers and confirms — if the appliance becomes " +
			"unreachable the agent reverts the change on its own.",
		Attributes: map[string]schema.Attribute{
			"url": schema.StringAttribute{Optional: true,
				MarkdownDescription: "Base URL, e.g. `https://vrx-a.example`. Env: `VRX_URL`."},
			"api_key": schema.StringAttribute{Optional: true, Sensitive: true,
				MarkdownDescription: "API key (`Authorization: ApiKey …`). Env: `VRX_API_KEY` (preferred: keeps it out of the configuration)."},
			"insecure": schema.BoolAttribute{Optional: true,
				MarkdownDescription: "Skip TLS certificate verification (self-signed lab boxes only). Default `false`."},
			"ca_file": schema.StringAttribute{Optional: true,
				MarkdownDescription: "PEM bundle of additional CAs to trust."},
			"confirm_timeout": schema.Int64Attribute{Optional: true,
				MarkdownDescription: "Seconds of the confirmed-commit window (1–3600; default 60). `0` commits without confirmation."},
			"commit_comment": schema.StringAttribute{Optional: true,
				MarkdownDescription: "Comment stored with every revision this provider creates (default `terraform`)."},
		},
	}
}

func (p *vrxProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var m providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	url := stringOr(m.URL, os.Getenv("VRX_URL"))
	key := stringOr(m.APIKey, os.Getenv("VRX_API_KEY"))
	if m.URL.IsUnknown() || m.APIKey.IsUnknown() {
		resp.Diagnostics.AddError("provider configuration unknown", "url and api_key must be known at plan time")
		return
	}
	confirm := int64(DefaultConfirmSeconds)
	if !m.ConfirmTimeout.IsNull() && !m.ConfirmTimeout.IsUnknown() {
		confirm = m.ConfirmTimeout.ValueInt64()
		if confirm < 0 || confirm > 3600 {
			resp.Diagnostics.AddError("invalid confirm_timeout", "confirm_timeout must be 0 (no confirmation) or 1–3600 seconds")
			return
		}
	}
	c, err := client.New(client.Options{
		URL:       url,
		APIKey:    key,
		Insecure:  m.Insecure.ValueBool(),
		CAFile:    m.CAFile.ValueString(),
		Timeout:   time.Duration(max(confirm, 60)+60) * time.Second,
		UserAgent: "terraform-provider-vrx/" + p.version,
	})
	if err != nil {
		resp.Diagnostics.AddError("cannot configure the VRX provider", err.Error())
		return
	}
	if m.Insecure.ValueBool() {
		resp.Diagnostics.AddWarning("TLS verification disabled", "insecure = true: the appliance certificate is not verified")
	}
	data := &providerData{client: c, confirmSec: confirm, comment: stringOr(m.CommitComment, "terraform")}
	resp.ResourceData = data
	resp.DataSourceData = data
}

func (p *vrxProvider) Resources(context.Context) []func() resource.Resource {
	return []func() resource.Resource{newConfigResource, newInterfaceResource}
}

func (p *vrxProvider) DataSources(context.Context) []func() datasource.DataSource {
	return []func() datasource.DataSource{newStateDataSource}
}

func stringOr(v types.String, fallback string) string {
	if v.IsNull() || v.IsUnknown() || v.ValueString() == "" {
		return fallback
	}
	return v.ValueString()
}
