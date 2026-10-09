package desired

import (
	"context"
	"encoding/json"
	"errors"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/vpn"
	"sort"
	"strings"
)

var snmpFingerprints = map[string]func(context.Context, string) (string, error){}

// SetSnmpSecretGenerations selects sealed secret generations during projection.
func SetSnmpSecretGenerations(owner string, fingerprint func(context.Context, string) (string, error)) {
	snmpMu.Lock()
	defer snmpMu.Unlock()
	if fingerprint == nil {
		delete(snmpFingerprints, owner)
	} else {
		snmpFingerprints[owner] = fingerprint
	}
}

// SnmpSecretRefs enumerates selected community and USM credentials.
func SnmpSecretRefs(v *ngfwv1.SnmpService) []string {
	refs := map[string]bool{}
	for _, c := range v.GetCommunities() {
		if c.GetSecretRef() != "" {
			refs[c.GetSecretRef()] = true
		}
	}
	for _, u := range v.GetV3Users() {
		if u.GetAuthRef() != "" {
			refs[u.GetAuthRef()] = true
		}
		if u.GetPrivRef() != "" {
			refs[u.GetPrivRef()] = true
		}
	}
	out := make([]string, 0, len(refs))
	for ref := range refs {
		out = append(out, ref)
	}
	sort.Strings(out)
	return out
}

// SnmpBoundValue stores configuration plus keyed generations, never credentials.
func SnmpBoundValue(v *ngfwv1.SnmpService, bindings map[string]string) (proto.Message, error) {
	if bindings == nil {
		return proto.Clone(v), nil
	}
	raw, err := protojson.Marshal(v)
	if err != nil {
		return nil, err
	}
	var cfg map[string]any
	if err = json.Unmarshal(raw, &cfg); err != nil {
		return nil, err
	}
	refs := map[string]any{}
	for ref, generation := range bindings {
		refs[ref] = generation
	}
	value, err := structpb.NewStruct(map[string]any{"config": cfg, "secretRefs": refs})
	if err != nil {
		return nil, err
	}
	_, _, err = ParseSnmpValue(value)
	return value, err
}

// ParseSnmpValue accepts legacy records; sealed production consumers reject unbound references.
func ParseSnmpValue(value proto.Message) (*ngfwv1.SnmpService, map[string]string, error) {
	if v, ok := value.(*ngfwv1.SnmpService); ok {
		return v, nil, nil
	}
	bad := errors.New("snmpd.config: malformed secret generation bindings")
	st, ok := value.(*structpb.Struct)
	if !ok || len(st.Fields) != 2 || st.Fields["config"].GetStructValue() == nil || st.Fields["secretRefs"].GetStructValue() == nil {
		return nil, nil, bad
	}
	raw, err := protojson.Marshal(st.Fields["config"].GetStructValue())
	if err != nil {
		return nil, nil, bad
	}
	v := &ngfwv1.SnmpService{}
	if protojson.Unmarshal(raw, v) != nil {
		return nil, nil, bad
	}
	fields := st.Fields["secretRefs"].GetStructValue().Fields
	refs := SnmpSecretRefs(v)
	if len(refs) != len(fields) {
		return nil, nil, bad
	}
	bindings := map[string]string{}
	for _, ref := range refs {
		generation := fields[ref].GetStringValue()
		if !strings.HasPrefix(generation, vpn.RefHMAC) || vpn.CheckRef(generation) != nil {
			return nil, nil, bad
		}
		bindings[ref] = generation
	}
	return v, bindings, nil
}

func snmpValue(v *ngfwv1.SnmpService) (proto.Message, error) {
	snmpMu.RLock()
	defer snmpMu.RUnlock()
	if len(snmpFingerprints) == 0 {
		return proto.Clone(v), nil
	}
	if len(snmpFingerprints) != 1 {
		return nil, errors.New("snmpd.config: ambiguous secret owner")
	}
	bindings := map[string]string{}
	for _, fingerprint := range snmpFingerprints {
		for _, ref := range SnmpSecretRefs(v) {
			generation, err := fingerprint(context.Background(), ref)
			if err != nil {
				return nil, errors.New("snmpd.config: selected secret unavailable")
			}
			bindings[ref] = generation
		}
	}
	return SnmpBoundValue(v, bindings)
}
