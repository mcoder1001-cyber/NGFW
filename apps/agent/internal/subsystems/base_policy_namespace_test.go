package subsystems

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go.fd.io/govpp/api"

	binlcp "ngfw/agent/binapi/lcp"
	"ngfw/agent/binapi/vlib"
	"ngfw/agent/internal/descriptors/interface/ifacetest"
	"ngfw/agent/internal/vpp"
)

type namespaceContextClient struct {
	vpp.Client
	t    *testing.T
	want context.Context
}

func (c namespaceContextClient) Invoke(ctx context.Context, req, reply api.Message) error {
	c.t.Helper()
	if ctx != c.want {
		c.t.Fatal("caller context was not propagated")
	}
	return c.Client.Invoke(ctx, req, reply)
}

func TestBasePolicyNamespaceCorroboration(t *testing.T) {
	const unset = "lcp default netns '<unset>'\r\nlcp lcp-sync off\r\n"
	for _, tc := range []struct {
		name, namespace, status, want string
		apiError, cliError            error
		cliRetval                     int32
		wantCLI, wantError            bool
	}{
		{name: "valid root", namespace: ""},
		{name: "valid named", namespace: "other\x00\x00", want: "other"},
		{name: "API transport failure", apiError: errors.New("disconnected"), wantError: true},
		{name: "garbage confirmed unset", namespace: "\xb1\x96\x02}", status: unset, wantCLI: true},
		{name: "garbage confirmed named", namespace: "\xff", status: "lcp default netns 'other'\n", wantCLI: true, wantError: true},
		{name: "missing status", namespace: "\xff", status: "lcp lcp-sync off\n", wantCLI: true, wantError: true},
		{name: "duplicate status", namespace: "\xff", status: unset + unset, wantCLI: true, wantError: true},
		{name: "conflicting named status", namespace: "\xff", status: unset + "lcp default netns 'other'\n", wantCLI: true, wantError: true},
		{name: "conflicting malformed status", namespace: "\xff", status: unset + "lcp default broken\n", wantCLI: true, wantError: true},
		{name: "substring is not status", namespace: "\xff", status: "prefix lcp default netns '<unset>'\n", wantCLI: true, wantError: true},
		{name: "indented status is not exact", namespace: "\xff", status: " lcp default netns '<unset>'\n", wantCLI: true, wantError: true},
		{name: "oversize", namespace: "\xff", status: unset + strings.Repeat("x", maxLCPNamespaceStatus), wantCLI: true, wantError: true},
		{name: "NUL", namespace: "\xff", status: unset + "\x00", wantCLI: true, wantError: true},
		{name: "escape", namespace: "\xff", status: unset + "\x1b", wantCLI: true, wantError: true},
		{name: "C1 control", namespace: "\xff", status: unset + "\u0085", wantCLI: true, wantError: true},
		{name: "embedded CR", namespace: "\xff", status: unset + "x\ry", wantCLI: true, wantError: true},
		{name: "invalid utf8", namespace: "\xff", status: unset + "\xff", wantCLI: true, wantError: true},
		{name: "CLI transport failure", namespace: "\xff", cliError: errors.New("disconnected"), wantCLI: true, wantError: true},
		{name: "CLI negative retval", namespace: "\xff", status: unset, cliRetval: -1, wantCLI: true, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := ifacetest.New()
			client.On("lcp_default_ns_get", func(api.Message) ([]api.Message, error) {
				return []api.Message{&binlcp.LcpDefaultNsGetReply{Netns: tc.namespace}}, tc.apiError
			})
			calls := 0
			client.On("cli_inband", func(m api.Message) ([]api.Message, error) {
				calls++
				if cmd := m.(*vlib.CliInband).Cmd; cmd != "show lcp" {
					t.Fatalf("unexpected CLI command %q", cmd)
				}
				return []api.Message{&vlib.CliInbandReply{Reply: tc.status, Retval: tc.cliRetval}}, tc.cliError
			})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			got, err := basePolicyNamespace(ctx, namespaceContextClient{Client: client, t: t, want: ctx})
			if (err != nil) != tc.wantError || got != tc.want {
				t.Fatalf("namespace %q, error %v; want namespace %q, error %v", got, err, tc.want, tc.wantError)
			}
			if (calls == 1) != tc.wantCLI || calls > 1 {
				t.Fatalf("CLI calls %d, want fallback %v", calls, tc.wantCLI)
			}
			if tc.wantCLI && tc.wantError && !strings.Contains(err.Error(), "invalid spec") {
				t.Fatalf("original malformed namespace cause lost: %v", err)
			}
		})
	}
}
