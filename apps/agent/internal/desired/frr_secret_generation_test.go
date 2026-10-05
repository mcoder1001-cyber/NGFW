package desired

import (
	"context"
	"errors"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"reflect"
	"strings"
	"testing"
)

func generationDoc(t *testing.T) *ngfwv1.DesiredState {
	t.Helper()
	doc := &ngfwv1.DesiredState{}
	err := protojson.Unmarshal([]byte(`{"interfaces":{"wan0":{"description":"password/description"}},"routing":{"bgp":{"asn":1,"neighbors":{"192.0.2.1":{"remoteAs":2,"passwordRef":"password/bgp"}},"peerGroups":{"g":{"remoteAs":2,"passwordRef":"password/group"}}},"ospf":{"interfaces":{"wan0":{"auth":{"type":"md5","keyId":1,"keyRef":"password/ospf"}}}},"rip":{"interfaces":{"wan0":{"auth":{"type":"md5","keyId":2,"keyRef":"password/rip"}}}},"isis":{"areaPasswordRef":"password/area","domainPasswordRef":"password/domain"}}}`), doc)
	if err != nil {
		t.Fatal(err)
	}
	return doc
}
func TestFRRSecretReferenceCoverageAndGenerationIdentity(t *testing.T) {
	doc := generationDoc(t)
	before := proto.Clone(doc)
	wanted := []string{"password/area", "password/bgp", "password/domain", "password/group", "password/ospf", "password/rip"}
	if got := FRRReferencedSecrets(doc); !reflect.DeepEqual(got, wanted) {
		t.Fatal("missing or unrelated reference", got)
	}
	refs := map[string]string{}
	for _, ref := range wanted {
		refs[ref] = "hmac:" + strings.Repeat("a", 64)
	}
	old, err := FRRValueWithSecretBindings(doc, FRRApplied, refs)
	if err != nil {
		t.Fatal(err)
	}
	refs["password/ospf"] = "hmac:" + strings.Repeat("b", 64)
	next, err := FRRValueWithSecretBindings(doc, FRRApplied, refs)
	if err != nil || proto.Equal(old, next) {
		t.Fatal("same-ref rotation lost generation", err)
	}
	got, err := FRRSecretBindings(old)
	if err != nil || got["password/ospf"] == refs["password/ospf"] {
		t.Fatal("generation did not snapshot", err)
	}
	got["password/bgp"] = "mutated"
	again, _ := FRRSecretBindings(old)
	if again["password/bgp"] == "mutated" {
		t.Fatal("generation map not cloned")
	}
	if !proto.Equal(doc, before) {
		t.Fatal("user desired state mutated")
	}
	legacy, err := FRRValueWithSecretBindings(doc, FRRApplied, nil)
	if err != nil || !proto.Equal(legacy, FRRValue(doc, FRRApplied)) {
		t.Fatal("legacy format changed")
	}
}
func TestFRRSecretBindingsRejectMissingExtraAndNonHMAC(t *testing.T) {
	doc := generationDoc(t)
	complete := map[string]string{}
	for _, ref := range FRRReferencedSecrets(doc) {
		complete[ref] = "hmac:" + strings.Repeat("a", 64)
	}
	for _, mode := range []string{"missing", "extra", "plaintext", "unkeyed", "malformed"} {
		t.Run(mode, func(t *testing.T) {
			refs := map[string]string{}
			for key, value := range complete {
				refs[key] = value
			}
			switch mode {
			case "missing":
				delete(refs, "password/ospf")
			case "extra":
				refs["password/unrelated"] = refs["password/ospf"]
			case "plaintext":
				refs["password/ospf"] = "NOT_A_REAL_SECRET"
			case "unkeyed":
				refs["password/ospf"] = "sha256:" + strings.Repeat("a", 64)
			case "malformed":
				refs["password/ospf"] = "hmac:bad"
			}
			if _, err := FRRValueWithSecretBindings(doc, FRRApplied, refs); err == nil || strings.Contains(err.Error(), "NOT_A_REAL_SECRET") || strings.Contains(err.Error(), strings.Repeat("a", 64)) {
				t.Fatal("unsafe bindings accepted or exposed", err)
			}
		})
	}
	value := FRRValue(doc, FRRApplied)
	value.Fields["secretRefs"] = structpb.NewStringValue("wrong-type")
	if _, err := FRRSecretBindings(value); err == nil {
		t.Fatal("malformed field accepted")
	}
}

func TestFRRSecretProjectionOnlyBindsSelectedReferences(t *testing.T) {
	doc := generationDoc(t)
	requested := []string{}
	current := "hmac:" + strings.Repeat("a", 64)
	options := FRROptions{Secrets: true, SecretRef: func(_ context.Context, ref string) (string, error) {
		requested = append(requested, ref)
		return current, nil
	}}
	project := func() proto.Message {
		sink := newLbgsSink()
		FRR(sink, doc, map[string]bool{"routing": true}, options)
		if len(sink.errors) != 0 {
			t.Fatal(sink.errors)
		}
		return sink.kvs[FRRConfigKey]
	}
	old := project()
	if len(requested) != 6 {
		t.Fatal("projection missed or included unrelated references", requested)
	}
	same := project()
	if !proto.Equal(old, same) {
		t.Fatal("unchanged referenced generations changed desired object")
	}
	current = "hmac:" + strings.Repeat("b", 64)
	if proto.Equal(old, project()) {
		t.Fatal("rotation absent from real projection")
	}
	options.SecretRef = func(context.Context, string) (string, error) { return "", errors.New("private resolver detail") }
	sink := newLbgsSink()
	FRR(sink, doc, map[string]bool{"routing": true}, options)
	if len(sink.kvs) != 0 || sink.errors[Ptr("routing")] != "routing.secret-generation-unavailable" {
		t.Fatal("unresolvable generation was projected")
	}
}
