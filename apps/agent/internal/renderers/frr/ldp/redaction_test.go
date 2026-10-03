package ldp

import (
	"context"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/renderers"
	"ngfw/agent/internal/renderers/frr"
)

func TestFrameworkMasksPasswordInDryRun(t *testing.T) {
	const fixtureValue = "NGFW_TEST_PSK_F_mpls_ldp_mask"
	dir := t.TempDir()
	paths := frr.Paths{ConfDir: dir, RunDir: dir, BinDir: "/usr/bin", ReloadLog: dir + "/reload.log", FileMode: 0600}
	runner := renderers.NewRecordingRunner().On(frr.ReloadBin, func(renderers.Command) (renderers.Output, error) {
		return renderers.Output{Stdout: []byte("neighbor 192.0.2.3 password " + fixtureValue + "\n")}, nil
	})
	renderer := frr.New(runner, frr.WithPaths(paths), frr.WithSections(Section{}),
		frr.WithInterfaceMapper(func(string) (string, bool) { return "wan", true }),
		frr.WithSecretResolver(frr.SecretResolverFunc(func(context.Context, string) (string, error) { return fixtureValue, nil })))
	c := config()
	c.Neighbors = map[string]*ngfwv1.LdpNeighbor{"192.0.2.3": {PasswordRef: proto.String("password/peer")}}
	desired := &ngfwv1.DesiredState{Routing: &ngfwv1.RoutingConfig{Mpls: &ngfwv1.MplsConfig{Ldp: c}}}
	files, err := renderer.Render(context.Background(), desired)
	if err != nil {
		t.Fatal(err)
	}
	if !files[paths.ConfFile()].Secret {
		t.Fatal("password config not marked secret")
	}
	diff, err := renderer.DryRun(context.Background(), files)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(diff, fixtureValue) {
		t.Fatal("password escaped into DryRun")
	}
}
