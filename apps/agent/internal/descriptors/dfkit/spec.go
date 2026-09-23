package dfkit

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

// ErrSpec is wrapped by every decoding and validation error of a desired-state spec.
var ErrSpec = errors.New("invalid spec")

// Encode returns the canonical structpb document of a typed spec (a struct with json tags and
// no omitempty, so every field is always present). The desired value and the value Retrieve
// decodes both go through Encode, which is what makes proto.Equal a correct diff. The P03
// domain messages replace this stand-in later (D-055); only the Proto/FromProto functions of
// each spec change then.
//
// Numbers become JSON numbers (float64): specs must not carry integers above 2^53.
func Encode(spec any) *structpb.Struct {
	raw, err := json.Marshal(spec)
	if err != nil {
		panic(fmt.Sprintf("dfkit: encode %T: %v", spec, err)) // only unsupported Go types: a programming error
	}
	s := &structpb.Struct{}
	if err := protojson.Unmarshal(raw, s); err != nil {
		panic(fmt.Sprintf("dfkit: encode %T: %v", spec, err))
	}
	return s
}

// Decode fills spec (a pointer to a typed spec) from a structpb document built by Encode.
// Unknown fields and fields of the wrong JSON type are errors wrapping ErrSpec.
func Decode(msg proto.Message, spec any) error {
	s, ok := msg.(*structpb.Struct)
	if !ok || s == nil {
		return fmt.Errorf("%w: value is %T, want *structpb.Struct", ErrSpec, msg)
	}
	raw, err := protojson.Marshal(s)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrSpec, err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(spec); err != nil {
		return fmt.Errorf("%w: %T: %v", ErrSpec, spec, err)
	}
	return nil
}

// Specf returns an error wrapping ErrSpec.
func Specf(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrSpec, fmt.Sprintf(format, args...))
}
