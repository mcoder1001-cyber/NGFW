package rfkit

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	vrxv1 "ngfw/agent/gen/vrx/v1"
)

// Stand-in fields (D-055). Renderer fields that the desired-state proto does not carry yet
// are read from a *structpb.Struct input (the configuration document as JSON) at the JSON
// path the schema will use. Ext navigates that document; a nil Ext (typed input) answers
// "absent" everywhere.

// ErrExt is wrapped by every stand-in type error.
var ErrExt = errors.New("rfkit: invalid stand-in field")

// Decode turns a renderer input into the typed desired state plus the raw document for the
// stand-in fields: *vrxv1.DesiredState (no stand-ins), *structpb.Struct (document), nil.
func Decode(msg proto.Message) (*vrxv1.DesiredState, *Ext, error) {
	switch m := msg.(type) {
	case nil:
		return &vrxv1.DesiredState{}, nil, nil
	case *vrxv1.DesiredState:
		if m == nil {
			return &vrxv1.DesiredState{}, nil, nil
		}
		return m, nil, nil
	case *structpb.Struct:
		if m == nil {
			return &vrxv1.DesiredState{}, nil, nil
		}
		raw, err := protojson.Marshal(m)
		if err != nil {
			return nil, nil, fmt.Errorf("%w: encode document: %v", ErrExt, err)
		}
		ds := &vrxv1.DesiredState{}
		if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(raw, ds); err != nil {
			return nil, nil, fmt.Errorf("%w: decode document: %v", ErrExt, err)
		}
		return ds, &Ext{v: structpb.NewStructValue(m), path: ""}, nil
	default:
		return nil, nil, fmt.Errorf("%w: unsupported input type %T (want *vrxv1.DesiredState or *structpb.Struct)", ErrExt, msg)
	}
}

// Ext is a position in the stand-in document.
type Ext struct {
	v    *structpb.Value
	path string
}

// Path is the JSON pointer-ish path of this position ("services.snmp.views").
func (e *Ext) Path() string {
	if e == nil {
		return ""
	}
	return e.path
}

func (e *Ext) child(name string) string {
	if e.path == "" {
		return name
	}
	return e.path + "." + name
}

// Get descends through object keys; absent keys give nil.
func (e *Ext) Get(keys ...string) *Ext {
	cur := e
	for _, k := range keys {
		if cur == nil {
			return nil
		}
		s := cur.v.GetStructValue()
		if s == nil {
			return nil
		}
		v, ok := s.GetFields()[k]
		if !ok || v == nil {
			return nil
		}
		if _, isNull := v.GetKind().(*structpb.Value_NullValue); isNull {
			return nil
		}
		cur = &Ext{v: v, path: cur.child(k)}
	}
	return cur
}

// Index returns element i of a list (nil when absent or not a list).
func (e *Ext) Index(i int) *Ext {
	if e == nil {
		return nil
	}
	l := e.v.GetListValue()
	if l == nil || i < 0 || i >= len(l.GetValues()) {
		return nil
	}
	return &Ext{v: l.GetValues()[i], path: fmt.Sprintf("%s[%d]", e.path, i)}
}

// Len is the length of a list (0 when absent).
func (e *Ext) Len() (int, error) {
	if e == nil {
		return 0, nil
	}
	l := e.v.GetListValue()
	if l == nil {
		return 0, fmt.Errorf("%w: %s must be a list", ErrExt, e.path)
	}
	return len(l.GetValues()), nil
}

// Keys returns the keys of an object in document order (sorted by protobuf; callers sort).
func (e *Ext) Keys() ([]string, error) {
	if e == nil {
		return nil, nil
	}
	s := e.v.GetStructValue()
	if s == nil {
		return nil, fmt.Errorf("%w: %s must be an object", ErrExt, e.path)
	}
	out := make([]string, 0, len(s.GetFields()))
	for k := range s.GetFields() {
		out = append(out, k)
	}
	return out, nil
}

// String returns a string field (ok=false when absent).
func (e *Ext) String() (string, bool, error) {
	if e == nil {
		return "", false, nil
	}
	s, isStr := e.v.GetKind().(*structpb.Value_StringValue)
	if !isStr {
		return "", false, fmt.Errorf("%w: %s must be a string", ErrExt, e.path)
	}
	return s.StringValue, true, nil
}

// Bool returns a boolean field (ok=false when absent).
func (e *Ext) Bool() (bool, bool, error) {
	if e == nil {
		return false, false, nil
	}
	b, isBool := e.v.GetKind().(*structpb.Value_BoolValue)
	if !isBool {
		return false, false, fmt.Errorf("%w: %s must be a boolean", ErrExt, e.path)
	}
	return b.BoolValue, true, nil
}

// Uint returns an integer field in [lo, hi] (ok=false when absent).
func (e *Ext) Uint(lo, hi uint32) (uint32, bool, error) {
	if e == nil {
		return 0, false, nil
	}
	n, isNum := e.v.GetKind().(*structpb.Value_NumberValue)
	if !isNum || math.IsNaN(n.NumberValue) || n.NumberValue != math.Trunc(n.NumberValue) ||
		n.NumberValue < float64(lo) || n.NumberValue > float64(hi) {
		return 0, false, fmt.Errorf("%w: %s must be an integer %d..%d", ErrExt, e.path, lo, hi)
	}
	return uint32(n.NumberValue), true, nil
}

// Strings returns a list of strings (nil when absent).
func (e *Ext) Strings() ([]string, error) {
	n, err := e.Len()
	if err != nil || n == 0 {
		return nil, err
	}
	out := make([]string, 0, n)
	for i := range n {
		s, _, err := e.Index(i).String()
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, nil
}

// OnlyKeys fails when the object has keys outside allowed (strict stand-ins: a typo must not
// be silently ignored).
func (e *Ext) OnlyKeys(allowed ...string) error {
	keys, err := e.Keys()
	if err != nil {
		return err
	}
	for _, k := range keys {
		ok := false
		for _, a := range allowed {
			if k == a {
				ok = true
				break
			}
		}
		if !ok {
			return fmt.Errorf("%w: %s has unknown key %q (allowed: %s)", ErrExt, e.path, k, strings.Join(allowed, ", "))
		}
	}
	return nil
}
