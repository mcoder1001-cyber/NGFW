package rip_test

import (
	"context"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/renderers"
	"ngfw/agent/internal/renderers/frr"
	"ngfw/agent/internal/renderers/frr/rip"
	"ngfw/agent/internal/renderers/frr/ripng"
)

func authRenderer(value string, resolve bool) *frr.Renderer {
	options := []frr.Option{frr.WithPaths(frr.TestPaths("w8")), frr.WithSections(rip.Section{}), frr.WithInterfaceMapper(mapIf), frr.WithInterfaceLines(frr.NamedInterfaceLines{Name: "rip", Fn: rip.InterfaceLines})}
	if resolve {
		options = append(options, frr.WithSecretResolver(frr.SecretResolverFunc(func(context.Context, string) (string, error) { return value, nil })))
	}
	return frr.New(renderers.NewRecordingRunner(), options...)
}
func authDoc(t *testing.T, auth *ngfwv1.OspfAuth) *ngfwv1.DesiredState {
	ds := parse(t, `{"interfaces":{"loop0":{"lcp":{"hostIfName":"w8-lo"}}},"routing":{"rip":{"interfaces":{"loop0":{}}}}}`)
	ds.Routing.Rip.Interfaces["loop0"].Auth = auth
	return ds
}

func md5() *ngfwv1.OspfAuth {
	return &ngfwv1.OspfAuth{Type: proto.String("md5"), KeyId: proto.Uint32(7), KeyRef: proto.String("password/rip")}
}
func TestMD5RenderRedactionAndRemoval(t *testing.T) {
	r := authRenderer("fixture-rip-key", true)
	files, err := r.Render(context.Background(), authDoc(t, md5()))
	if err != nil {
		t.Fatal(err)
	}
	conf := string(files[r.Paths().ConfFile()].Content)
	for _, want := range []string{"key chain ngfw-rip-", " key 7", "  key-string fixture-rip-key", "ip rip authentication mode md5", "ip rip authentication key-chain ngfw-rip-", "version 2"} {
		if !strings.Contains(conf, want) {
			t.Fatalf("missing %q", want)
		}
	}
	if !files[r.Paths().ConfFile()].Secret || strings.Contains(string(files.Redacted()[r.Paths().ConfFile()].Content), "fixture-rip-key") {
		t.Fatal("secret exposed")
	}
	again, err := r.Render(context.Background(), authDoc(t, md5()))
	if err != nil || string(again[r.Paths().ConfFile()].Content) != conf {
		t.Fatal("render is not deterministic", err)
	}
	for _, auth := range []*ngfwv1.OspfAuth{nil, {Type: proto.String("none")}} {
		files, err := r.Render(context.Background(), authDoc(t, auth))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(files[r.Paths().ConfFile()].Content), "key chain") || strings.Contains(string(files[r.Paths().ConfFile()].Content), "authentication") {
			t.Fatal("auth removal leaves configuration")
		}
	}
}
func TestMD5FailClosed(t *testing.T) {
	for _, tc := range []struct {
		name, value string
		resolve     bool
	}{
		{"missing resolver", "", false}, {"empty key", "", true}, {"CLI injection", "key\nrouter bgp 1", true}, {"too long", "01234567890123456", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := authRenderer(tc.value, tc.resolve).Render(context.Background(), authDoc(t, md5()))
			if err == nil {
				t.Fatal("invalid secret accepted")
			}
			if tc.value != "" && strings.Contains(err.Error(), tc.value) {
				t.Fatal("error leaks secret")
			}
		})
	}
	for _, auth := range []*ngfwv1.OspfAuth{
		{Type: proto.String("text")}, {Type: proto.String("md5"), KeyId: proto.Uint32(256), KeyRef: proto.String("password/rip")}, {Type: proto.String("md5"), KeyId: proto.Uint32(1), KeyRef: proto.String("psk/rip")}, {Type: proto.String("none"), KeyRef: proto.String("password/rip")},
	} {
		if _, err := authRenderer("fixture-rip-key", true).Render(context.Background(), authDoc(t, auth)); err == nil {
			t.Fatal("invalid auth accepted")
		}
	}
}
func TestRIPngRejectsV2Authentication(t *testing.T) {
	cfg := &ngfwv1.RipngConfig{Interfaces: map[string]*ngfwv1.RipInterface{"loop0": {Auth: md5()}}}
	if _, err := ripng.Render(cfg, mapIf); err == nil {
		t.Fatal("RIPng silently dropped auth")
	}
}
