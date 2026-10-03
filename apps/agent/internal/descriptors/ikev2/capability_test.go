package ikev2_test

import (
	"context"
	"errors"
	"go.fd.io/govpp/api"
	ikeapi "ngfw/agent/binapi/ikev2"
	"ngfw/agent/internal/descriptors/ikev2"
	"ngfw/agent/internal/vpp/fake"
	"testing"
)

func TestUnsafeNativeStateRefusedBeforeDump(t *testing.T) {
	c := fake.New()
	c.On("ikev2_plugin_get_version", func(api.Message) ([]api.Message, error) {
		return []api.Message{&ikeapi.Ikev2PluginGetVersionReply{Major: 1, Minor: 0}}, nil
	})
	if _, err := ikev2.SAs(context.Background(), c, "vrx"); !errors.Is(err, ikev2.ErrUnsafeState) {
		t.Fatalf("unsafe plugin accepted: %v", err)
	}
	if len(c.CallsNamed("ikev2_sa_v3_dump")) != 0 {
		t.Fatal("crashing dump API called on unsafe plugin")
	}
}
