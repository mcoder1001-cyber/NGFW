// Package natcommon holds what every NAT-family descriptor (DF-3: nat44-ed, nat44-ei, nat64,
// nat66, npt66, det44, map, dslite, cnat, pnat) shares:
//
//   - the desired-state carrier (Encode/Decode): typed Go specs carried as *structpb.Struct
//     until the NAT proto messages land in packages/proto (contract task of F-nat44-ed-sessions);
//   - the generic Descriptor[T] adapter that turns a spec type plus five closures into a
//     scheduler.Descriptor;
//   - the ownership Scope for NAT objects that carry no tag (address pools, prefixes, maps):
//     derived from the owner id — "w<N>" (a test slot) owns 10.<N>.0.0/16, fd00:<N>::/32 and
//     tables N000–N999; every other owner (the production agent, "vrx") owns everything;
//   - the interface table (name <-> sw_if_index <-> owner tag) and address helpers;
//   - VPP error classification (already enabled / already disabled / no such entry).
package natcommon

import (
	"bytes"
	"encoding/json"
	"fmt"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

// Normalizer is implemented by specs that canonicalise themselves (IP addresses through
// net/netip, sorted repeated fields) so that Encode(desired) and Retrieve() produce
// proto.Equal values.
type Normalizer interface {
	Normalize()
}

// Encode converts a typed spec (a struct with json tags, no omitempty) into the
// *structpb.Struct carrier the scheduler diffs with proto.Equal. Every field is always
// present, so two encodings of equal specs are proto.Equal. spec is normalised first when
// it implements Normalizer (pass a pointer for that to take effect).
func Encode(spec any) (*structpb.Struct, error) {
	if n, ok := spec.(Normalizer); ok {
		n.Normalize()
	}
	raw, err := json.Marshal(spec)
	if err != nil {
		return nil, fmt.Errorf("natcommon: encode %T: %w", spec, err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("natcommon: encode %T: %w", spec, err)
	}
	s, err := structpb.NewStruct(m)
	if err != nil {
		return nil, fmt.Errorf("natcommon: encode %T: %w", spec, err)
	}
	return s, nil
}

// MustEncode is Encode for specs built from constants; it panics on error.
func MustEncode(spec any) *structpb.Struct {
	s, err := Encode(spec)
	if err != nil {
		panic(err)
	}
	return s
}

// Decode converts the carrier back into the typed spec T and normalises it. It rejects any
// proto message other than *structpb.Struct and unknown fields, so a mis-routed object
// fails loudly instead of being applied half-empty.
func Decode[T any](msg proto.Message) (T, error) {
	var out T
	s, ok := msg.(*structpb.Struct)
	if !ok || s == nil {
		return out, fmt.Errorf("natcommon: want *structpb.Struct, got %T", msg)
	}
	raw, err := json.Marshal(s.AsMap())
	if err != nil {
		return out, fmt.Errorf("natcommon: decode into %T: %w", out, err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&out); err != nil {
		return out, fmt.Errorf("natcommon: decode into %T: %w", out, err)
	}
	if n, ok := any(&out).(Normalizer); ok {
		n.Normalize()
	}
	return out, nil
}
