package secretvalue

import (
	"context"
	"google.golang.org/protobuf/proto"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"strings"
	"testing"
)

func TestBoundValueExactCoverageAndRestartMetadata(t *testing.T) {
	input := &ngfwv1.NtpService{Servers: []*ngfwv1.NtpService_Server{{KeyRef: proto.String("key/one")}}}
	old := "hmac:" + strings.Repeat("a", 64)
	newer := "hmac:" + strings.Repeat("b", 64)
	bindings := map[string]string{"key/one": old}
	value, err := Wrap(input, bindings)
	if err != nil {
		t.Fatal(err)
	}
	rotated, err := Wrap(input, map[string]string{"key/one": newer})
	if err != nil || proto.Equal(value, rotated) {
		t.Fatal("rotation not represented")
	}
	decoded := new(ngfwv1.NtpService)
	got, err := Unwrap(value, decoded)
	if err != nil || !proto.Equal(input, decoded) || got["key/one"] != old {
		t.Fatal("bound roundtrip")
	}
	for _, bad := range []map[string]string{{}, {"key/one": "plaintext"}, {"key/one": old, "key/extra": newer}} {
		if _, err := Wrap(input, bad); err == nil {
			t.Fatal("invalid binding accepted")
		}
	}
	raw, err := AppendMetadata([]byte("# input\n"), bindings)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := Metadata(raw)
	if err != nil || Validate(input, restored) != nil {
		t.Fatal("restart metadata")
	}
	if _, err := Metadata(append(raw, raw...)); err == nil {
		t.Fatal("duplicate metadata accepted")
	}
	ctx := WithBindings(context.Background(), bindings)
	bindings["key/one"] = newer
	if generation, _ := Binding(ctx, "key/one"); generation != old {
		t.Fatal("mutable caller changed binding")
	}
}
