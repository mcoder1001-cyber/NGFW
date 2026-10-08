package desired

import (
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"strings"
	"testing"
)

func TestSnmpGenerationEnvelopePersistenceAndValidation(t *testing.T) {
	cfg := &ngfwv1.SnmpService{Enabled: proto.Bool(true), Communities: map[string]*ngfwv1.SnmpService_Community{"ro": {SecretRef: proto.String("password/ro")}}}
	fingerprint := "hmac:" + strings.Repeat("a", 64)
	value, err := SnmpBoundValue(cfg, map[string]string{"password/ro": fingerprint})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := proto.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	restored := &structpb.Struct{}
	if err = proto.Unmarshal(raw, restored); err != nil {
		t.Fatal(err)
	}
	parsed, bindings, err := ParseSnmpValue(restored)
	if err != nil || !proto.Equal(parsed, cfg) || bindings["password/ro"] != fingerprint {
		t.Fatal("scheduler protobuf round trip", err)
	}
	for name, mutate := range map[string]func(*structpb.Struct){
		"missing": func(v *structpb.Struct) { delete(v.Fields["secretRefs"].GetStructValue().Fields, "password/ro") },
		"extra": func(v *structpb.Struct) {
			v.Fields["secretRefs"].GetStructValue().Fields["password/extra"] = structpb.NewStringValue(fingerprint)
		},
		"plaintext": func(v *structpb.Struct) {
			v.Fields["secretRefs"].GetStructValue().Fields["password/ro"] = structpb.NewStringValue("NGFW_TEST_NOT_A_GENERATION")
		},
		"unknown envelope": func(v *structpb.Struct) { v.Fields["unexpected"] = structpb.NewBoolValue(true) },
	} {
		t.Run(name, func(t *testing.T) {
			bad := proto.Clone(restored).(*structpb.Struct)
			mutate(bad)
			if _, _, err := ParseSnmpValue(bad); err == nil {
				t.Fatal("malformed binding accepted")
			}
		})
	}
	if !proto.Equal(cfg, &ngfwv1.SnmpService{Enabled: proto.Bool(true), Communities: map[string]*ngfwv1.SnmpService_Community{"ro": {SecretRef: proto.String("password/ro")}}}) {
		t.Fatal("user document mutated")
	}
}
