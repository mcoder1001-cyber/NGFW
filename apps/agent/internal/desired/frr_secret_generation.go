package desired

import (
	"errors"
	"sort"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/structpb"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/vpn"
)

// FRRReferencedSecrets returns only password references in the FRR document.
// Walking reference leaves covers BGP groups/neighbors, OSPF/RIP keys and ISIS
// passwords without treating descriptions or unselected domains as secrets.
func FRRReferencedSecrets(doc *ngfwv1.DesiredState) []string {
	refs := map[string]bool{}
	var walk func(protoreflect.Message)
	walk = func(message protoreflect.Message) {
		message.Range(func(field protoreflect.FieldDescriptor, value protoreflect.Value) bool {
			inspect := func(value protoreflect.Value) {
				if field.Message() != nil {
					walk(value.Message())
				} else if field.Kind() == protoreflect.StringKind && strings.HasSuffix(field.JSONName(), "Ref") && strings.HasPrefix(value.String(), "password/") {
					refs[value.String()] = true
				}
			}
			switch {
			case field.IsMap():
				value.Map().Range(func(_ protoreflect.MapKey, item protoreflect.Value) bool {
					if field.MapValue().Message() != nil {
						walk(item.Message())
					}
					return true
				})
			case field.IsList():
				list := value.List()
				for i := 0; i < list.Len(); i++ {
					inspect(list.Get(i))
				}
			default:
				inspect(value)
			}
			return true
		})
	}
	if doc != nil {
		walk(doc.ProtoReflect())
	}
	out := make([]string, 0, len(refs))
	for ref := range refs {
		out = append(out, ref)
	}
	sort.Strings(out)
	return out
}

// FRRValueWithSecretBindings adds internal keyed generations, never plaintext,
// to a normal FRR value. The user document/reference names are left unchanged.
// Nil bindings preserve the historical value format. Non-nil bindings must
// exactly cover the document's password references, with existing keyed HMACs.
func FRRValueWithSecretBindings(doc *ngfwv1.DesiredState, status string, bindings map[string]string) (*structpb.Struct, error) {
	value := FRRValue(doc, status)
	if bindings == nil {
		return value, nil
	}
	fields := map[string]*structpb.Value{}
	for ref, fingerprint := range bindings {
		fields[ref] = structpb.NewStringValue(fingerprint)
	}
	value.Fields["secretRefs"] = structpb.NewStructValue(&structpb.Struct{Fields: fields})
	if _, err := FRRSecretBindings(value); err != nil {
		return nil, err
	}
	return value, nil
}

// FRRSecretBindings validates and clones internal generation bindings. An
// absent field is the legacy format; consumers with a historical resolver must
// refuse missing bindings for a secret-bearing document. No error prints hashes.
func FRRSecretBindings(value proto.Message) (map[string]string, error) {
	doc, _, err := ParseFRRValue(value)
	if err != nil {
		return nil, err
	}
	raw, present := value.(*structpb.Struct).Fields["secretRefs"]
	if !present {
		return nil, nil
	}
	bad := func() (map[string]string, error) {
		return nil, errors.New("desired: malformed FRR secret generation bindings")
	}
	generation := raw.GetStructValue()
	if generation == nil {
		return bad()
	}
	wanted := FRRReferencedSecrets(doc)
	if len(generation.Fields) != len(wanted) {
		return bad()
	}
	bindings := make(map[string]string, len(wanted))
	for _, ref := range wanted {
		item, ok := generation.Fields[ref]
		if !ok || item == nil {
			return bad()
		}
		fingerprint := item.GetStringValue()
		if !strings.HasPrefix(fingerprint, vpn.RefHMAC) || vpn.CheckRef(fingerprint) != nil {
			return bad()
		}
		bindings[ref] = fingerprint
	}
	return bindings, nil
}
