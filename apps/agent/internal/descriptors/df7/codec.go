package df7

import (
	"bytes"
	"encoding/json"
	"fmt"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
)

// Encode converts a typed spec (a struct with json tags) to the *structpb.Struct the scheduler
// carries as desired/actual Value (D-055). Desired values and Retrieve results go through the
// same function, so proto.Equal on the documents is equality of the typed specs: every spec
// field is tagged `omitempty`, so a zero value and an absent field are one form, and nil and
// empty slices encode alike. Integers must stay below 2^53 (JSON numbers are float64); the
// specs' Validate methods bound the few u64 fields.
func Encode(v any) *structpb.Struct {
	raw, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("df7: encode %T: %v", v, err)) // unsupported Go type: a programming error
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		panic(fmt.Sprintf("df7: encode %T: %v", v, err))
	}
	s, err := structpb.NewStruct(m)
	if err != nil {
		panic(fmt.Sprintf("df7: encode %T: %v", v, err))
	}
	return s
}

// Decode converts a Value back to the typed spec T. Unknown fields are an error (they would be
// silently dropped and then re-created forever), as is any other message type.
func Decode[T any](msg proto.Message) (T, error) {
	var out T
	s, ok := msg.(*structpb.Struct)
	if !ok || s == nil {
		return out, Specf("value is %T, want *structpb.Struct", msg)
	}
	raw, err := json.Marshal(s.AsMap())
	if err != nil {
		return out, Specf("%v", err)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&out); err != nil {
		return out, Specf("decode %T: %v", out, err)
	}
	return out, nil
}
