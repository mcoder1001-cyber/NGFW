// Package secretvalue binds scheduler configuration to keyed sealed generations.
// It never contains plaintext. Persisted metadata permits exact restart/rollback.
package secretvalue

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"sort"
	"strings"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/known/structpb"
	"ngfw/agent/internal/descriptors/vpn"
)

var ErrInvalid = errors.New("secret generation bindings unavailable or invalid")

const prefix = "# ngfw-secret-generations "

type bindingsKey struct{}

// References returns reference leaves from a consumer-trimmed protobuf configuration.
func References(value proto.Message) []string {
	refs := map[string]bool{}
	var walk func(protoreflect.Message)
	walk = func(m protoreflect.Message) {
		m.Range(func(f protoreflect.FieldDescriptor, v protoreflect.Value) bool {
			inspect := func(v protoreflect.Value) {
				if f.Message() != nil {
					walk(v.Message())
				} else if f.Kind() == protoreflect.StringKind && strings.HasSuffix(f.JSONName(), "Ref") && v.String() != "" {
					refs[v.String()] = true
				}
			}
			switch {
			case f.IsMap():
				v.Map().Range(func(_ protoreflect.MapKey, item protoreflect.Value) bool {
					if f.MapValue().Message() != nil {
						walk(item.Message())
					}
					return true
				})
			case f.IsList():
				for i := 0; i < v.List().Len(); i++ {
					inspect(v.List().Get(i))
				}
			default:
				inspect(v)
			}
			return true
		})
	}
	if value != nil {
		walk(value.ProtoReflect())
	}
	out := make([]string, 0, len(refs))
	for ref := range refs {
		out = append(out, ref)
	}
	sort.Strings(out)
	return out
}

// Validate requires exact reference coverage and only keyed HMAC generations.
func Validate(value proto.Message, bindings map[string]string) error {
	refs := References(value)
	if len(bindings) != len(refs) || len(bindings) > 256 {
		return ErrInvalid
	}
	for _, ref := range refs {
		generation, ok := bindings[ref]
		if !ok || len(ref) > 256 || !strings.HasPrefix(generation, vpn.RefHMAC) || vpn.CheckRef(generation) != nil {
			return ErrInvalid
		}
	}
	return nil
}

// Wrap preserves ordinary values or wraps configuration with validated keyed generations.
func Wrap(value proto.Message, bindings map[string]string) (proto.Message, error) {
	if bindings == nil {
		return proto.Clone(value), nil
	}
	if err := Validate(value, bindings); err != nil {
		return nil, err
	}
	raw, err := protojson.Marshal(value)
	if err != nil {
		return nil, ErrInvalid
	}
	var config map[string]any
	if json.Unmarshal(raw, &config) != nil {
		return nil, ErrInvalid
	}
	refs := map[string]any{}
	for k, v := range bindings {
		refs[k] = v
	}
	return structpb.NewStruct(map[string]any{"config": config, "secretRefs": refs})
}

// Unwrap decodes a bound value, or a legacy typed value with nil bindings.
func Unwrap(value, dst proto.Message) (map[string]string, error) {
	proto.Reset(dst)
	if value == nil {
		return nil, ErrInvalid
	}
	if value.ProtoReflect().Descriptor() == dst.ProtoReflect().Descriptor() {
		proto.Merge(dst, value)
		return nil, nil
	}
	wrapper, ok := value.(*structpb.Struct)
	if !ok || len(wrapper.Fields) != 2 {
		return nil, ErrInvalid
	}
	config, refs := wrapper.Fields["config"].GetStructValue(), wrapper.Fields["secretRefs"].GetStructValue()
	if config == nil || refs == nil {
		return nil, ErrInvalid
	}
	raw, err := protojson.Marshal(config)
	if err != nil || protojson.Unmarshal(raw, dst) != nil {
		return nil, ErrInvalid
	}
	bindings := map[string]string{}
	for ref, v := range refs.Fields {
		bindings[ref] = v.GetStringValue()
	}
	if Validate(dst, bindings) != nil {
		return nil, ErrInvalid
	}
	return bindings, nil
}

// WithBindings isolates a render from later selection changes or caller map mutations.
func WithBindings(ctx context.Context, bindings map[string]string) context.Context {
	copy := map[string]string{}
	for k, v := range bindings {
		copy[k] = v
	}
	return context.WithValue(ctx, bindingsKey{}, copy)
}

// Binding returns only the generation explicitly bound to this render context.
func Binding(ctx context.Context, ref string) (string, bool) {
	m, ok := ctx.Value(bindingsKey{}).(map[string]string)
	if !ok {
		return "", false
	}
	value, ok := m[ref]
	return value, ok
}

// AppendMetadata appends bounded non-secret generation metadata to a daemon input file.
func AppendMetadata(content []byte, bindings map[string]string) ([]byte, error) {
	if bindings == nil {
		return content, nil
	}
	raw, err := json.Marshal(bindings)
	if err != nil || len(raw) > 65536 {
		return nil, ErrInvalid
	}
	out := append([]byte(nil), content...)
	if len(out) > 0 && out[len(out)-1] != '\n' {
		out = append(out, '\n')
	}
	out = append(out, []byte(prefix+base64.RawStdEncoding.EncodeToString(raw)+"\n")...)
	return out, nil
}

// Metadata reads canonical generation metadata, refusing duplicates and malformed encoding.
func Metadata(content []byte) (map[string]string, error) {
	var result map[string]string
	found := false
	for _, line := range strings.Split(string(content), "\n") {
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		if found || len(line) > 100000 {
			return nil, ErrInvalid
		}
		found = true
		raw, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(line, prefix))
		if err != nil || len(raw) > 65536 || json.Unmarshal(raw, &result) != nil || result == nil {
			return nil, ErrInvalid
		}
		canonical, err := json.Marshal(result)
		if err != nil || !bytes.Equal(canonical, raw) {
			return nil, ErrInvalid
		}
	}
	return result, nil
}
