package contracttest

// Schema ⊆ proto drift guard (P03b, D-042/D-061).
//
// packages/schema (Zod) is the single source of truth for the configuration document; DesiredState is
// its 1:1 protobuf projection. This test walks the JSON Schema that `pnpm gen` writes from the Zod model
// (packages/schema/dist/json-schema/root.json, `io: 'input'`) alongside the DesiredState descriptor and
// reports, in both directions:
//
//   schema→proto  a schema leaf without a proto field (same JSON name), a container/type mismatch
//                 (record ↔ map<string,…>, array ↔ repeated, object ↔ message, string/bool/integer/number
//                 ↔ string/bool/uint32|int32|uint64|int64/double), a scalar without explicit presence
//                 (D-039), a secret-flagged leaf that has a proto field (D-040);
//   proto→schema  a proto field that has no schema leaf (a field the document can never set).
//
// Integer widths follow docs/contracts/proto.md §1: non-negative ranges ≤ 2^32−1 → uint32, larger →
// uint64; ranges with a negative minimum → int32 (int64 when they do not fit). Zod enums and literals
// are `string` (D-P03-1); discriminated unions / unions of objects are one flattened message.
//
// The JSON Schema is generated, not committed: CI (tools/ci.sh) runs `pnpm gen` before `make -C
// apps/agent test`, so under CI a missing file is a failure; outside CI the test skips with the command
// to run. VRX_DRIFT_SCHEMA=<file> points the guard at another schema (used to demonstrate a failure).

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"

	vrxv1 "ngfw/agent/gen/vrx/v1"
)

// generatedSchema is written by `pnpm gen` (packages/schema/src/gen.ts).
const generatedSchema = "../../../../packages/schema/dist/json-schema/root.json"

type jsonNode = map[string]any

// Direction of a finding.
const (
	schemaToProto = "schema→proto"
	protoToSchema = "proto→schema"
)

type finding struct {
	dir  string
	path string // JSON pointer-ish path in the document; `{}` = any record key, `[]` = any array element
	msg  string
}

func (f finding) String() string { return fmt.Sprintf("%s %s: %s", f.dir, f.path, f.msg) }

type driftChecker struct {
	findings []finding
	accepted []finding // findings matched by an acceptedDrift entry (reported in the log, not failures)
	leaves   int       // schema scalar leaves checked (coverage figure for the report)
	messages map[protoreflect.FullName]bool
}

func (c *driftChecker) add(dir, path, format string, args ...any) {
	c.findings = append(c.findings, finding{dir: dir, path: path, msg: fmt.Sprintf(format, args...)})
}

// acceptedDrift lists the known, justified differences between the schema and DesiredState, keyed by
// "<direction> <path>". Every entry must still occur (a stale entry is a finding), and only
// proto→schema supersets may be accepted — a schema leaf without a proto field never is.
var acceptedDrift = map[string]string{
	// One shared Redistribute message serves bgp/ospf/isis/rip (P02a sync, D-045). The schema's record
	// omits the protocol's own key ("a protocol cannot redistribute into itself"), so the proto is a
	// superset by exactly one field per protocol; the API's Zod validation runs before fromJSON, so the
	// field can never be set. Per-protocol messages would be four near-identical types for no gain.
	protoToSchema + " /routing/bgp/redistribute/bgp":   "shared Redistribute message; own protocol excluded by the schema",
	protoToSchema + " /routing/ospf/redistribute/ospf": "shared Redistribute message; own protocol excluded by the schema",
	protoToSchema + " /routing/isis/redistribute/isis": "shared Redistribute message; own protocol excluded by the schema",
	protoToSchema + " /routing/rip/redistribute/rip":   "shared Redistribute message; own protocol excluded by the schema",
}

// checkDrift compares a JSON Schema object (the document root) with a message descriptor. Findings
// listed in accepted are moved to c.accepted; accepted entries that no longer occur become findings.
func checkDrift(root jsonNode, md protoreflect.MessageDescriptor, accepted map[string]string) *driftChecker {
	c := &driftChecker{messages: map[protoreflect.FullName]bool{}}
	c.object("", flatten(root), md)
	used := map[string]bool{}
	kept := c.findings[:0]
	for _, f := range c.findings {
		key := f.dir + " " + f.path
		if _, ok := accepted[key]; ok && f.dir == protoToSchema {
			used[key] = true
			c.accepted = append(c.accepted, f)
			continue
		}
		kept = append(kept, f)
	}
	c.findings = kept
	for key := range accepted {
		if !used[key] {
			c.add("acceptedDrift", key, "stale entry: this difference no longer exists (or is not a proto→schema superset) — remove it")
		}
	}
	sort.Slice(c.findings, func(i, j int) bool {
		if c.findings[i].path != c.findings[j].path {
			return c.findings[i].path < c.findings[j].path
		}
		return c.findings[i].msg < c.findings[j].msg
	})
	return c
}

// flatten resolves anyOf/oneOf/allOf into the concrete alternatives (z.toJSONSchema writes no $ref for
// RootConfig; a $ref is reported by kindOf as untyped so it cannot pass silently).
func flatten(n jsonNode) []jsonNode {
	var out []jsonNode
	composed := false
	for _, kw := range []string{"anyOf", "oneOf", "allOf"} {
		list, ok := n[kw].([]any)
		if !ok {
			continue
		}
		composed = true
		for _, item := range list {
			if m, ok := item.(jsonNode); ok {
				out = append(out, flatten(m)...)
			}
		}
	}
	if !composed || n["type"] != nil || n["properties"] != nil {
		out = append(out, n)
	}
	return out
}

// Kinds of schema nodes.
const (
	kObject  = "object"
	kMap     = "record"
	kArray   = "array"
	kString  = "string"
	kBool    = "boolean"
	kInteger = "integer"
	kNumber  = "number"
	kUnknown = "untyped"
)

func kindOfNode(n jsonNode) string {
	t, _ := n["type"].(string)
	if t == "" {
		// literal without a type keyword
		switch v := n["const"].(type) {
		case string:
			return kString
		case bool:
			return kBool
		case float64:
			if v == math.Trunc(v) {
				return kInteger
			}
			return kNumber
		}
		return kUnknown
	}
	switch t {
	case "object":
		if _, ok := n["additionalProperties"].(jsonNode); ok && n["properties"] == nil {
			return kMap
		}
		return kObject
	case "array":
		return kArray
	case "string":
		return kString
	case "boolean":
		return kBool
	case "integer":
		return kInteger
	case "number":
		// z.literal(1) | z.literal(2) is emitted as {type: number, const: 1}
		if v, ok := n["const"].(float64); ok && v == math.Trunc(v) {
			return kInteger
		}
		if enum, ok := n["enum"].([]any); ok && len(enum) > 0 {
			for _, e := range enum {
				if v, ok := e.(float64); !ok || v != math.Trunc(v) {
					return kNumber
				}
			}
			return kInteger
		}
		return kNumber
	}
	return kUnknown
}

// kindOf returns the common kind of all alternatives, or "" (and a finding) when they disagree.
func (c *driftChecker) kindOf(path string, alts []jsonNode) string {
	kinds := map[string]bool{}
	for _, a := range alts {
		kinds[kindOfNode(a)] = true
	}
	if len(kinds) == 1 {
		for k := range kinds {
			if k == kUnknown {
				c.add(schemaToProto, path, "untyped schema node (no type/const) — cannot be projected")
				return ""
			}
			return k
		}
	}
	list := make([]string, 0, len(kinds))
	for k := range kinds {
		list = append(list, k)
	}
	sort.Strings(list)
	c.add(schemaToProto, path, "union of different kinds %v — no single proto type can hold it", list)
	return ""
}

func isSecretNode(alts []jsonNode) bool {
	for _, a := range alts {
		if a["writeOnly"] == true {
			return true
		}
		if ui, ok := a["x-vrx-ui"].(jsonNode); ok && ui["secret"] == true {
			return true
		}
	}
	return false
}

// object compares the union of the alternatives' properties with the message's fields, both ways.
func (c *driftChecker) object(path string, alts []jsonNode, md protoreflect.MessageDescriptor) {
	c.messages[md.FullName()] = true
	props := map[string][]jsonNode{}
	for _, a := range alts {
		p, _ := a["properties"].(jsonNode)
		for name, v := range p {
			if m, ok := v.(jsonNode); ok {
				props[name] = append(props[name], flatten(m)...)
			}
		}
	}
	names := make([]string, 0, len(props))
	for n := range props {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		p := path + "/" + name
		f := md.Fields().ByJSONName(name)
		if isSecretNode(props[name]) {
			if f != nil {
				c.add(schemaToProto, p, "secret-flagged leaf has proto field %s = %d — secrets never cross the boundary (D-040)", f.Name(), f.Number())
			}
			continue
		}
		if f == nil {
			c.add(schemaToProto, p, "schema leaf has no field in %s (JSON name %q)", md.FullName(), name)
			continue
		}
		c.field(p, props[name], f)
	}
	fields := md.Fields()
	for i := 0; i < fields.Len(); i++ {
		f := fields.Get(i)
		if _, ok := props[f.JSONName()]; !ok {
			c.add(protoToSchema, path+"/"+f.JSONName(), "proto field %s.%s = %d has no schema leaf", md.FullName(), f.Name(), f.Number())
		}
	}
}

// field compares one property with one field (cardinality first, then the element).
func (c *driftChecker) field(path string, alts []jsonNode, f protoreflect.FieldDescriptor) {
	switch c.kindOf(path, alts) {
	case "":
		return
	case kMap:
		if !f.IsMap() {
			c.add(schemaToProto, path, "schema record (z.record) but proto field %s is %s", f.Name(), cardinality(f))
			return
		}
		if f.MapKey().Kind() != protoreflect.StringKind {
			c.add(schemaToProto, path, "record keys are strings, proto map key is %s", f.MapKey().Kind())
		}
		var vals []jsonNode
		for _, a := range alts {
			vals = append(vals, flatten(a["additionalProperties"].(jsonNode))...)
		}
		c.value(path+"/{}", vals, f.MapValue())
	case kArray:
		if !f.IsList() {
			c.add(schemaToProto, path, "schema array but proto field %s is %s", f.Name(), cardinality(f))
			return
		}
		var items []jsonNode
		for _, a := range alts {
			if it, ok := a["items"].(jsonNode); ok {
				items = append(items, flatten(it)...)
			}
		}
		if len(items) == 0 {
			c.add(schemaToProto, path, "array without items schema")
			return
		}
		c.value(path+"/[]", items, f)
	default:
		if f.IsList() || f.IsMap() {
			c.add(schemaToProto, path, "schema single value but proto field %s is %s", f.Name(), cardinality(f))
			return
		}
		c.value(path, alts, f)
		if f.Kind() != protoreflect.MessageKind && !f.HasPresence() {
			c.add(schemaToProto, path, "scalar %s has implicit presence — must be proto3 `optional` (D-039)", f.Name())
		}
	}
}

func cardinality(f protoreflect.FieldDescriptor) string {
	switch {
	case f.IsMap():
		return "a map"
	case f.IsList():
		return "repeated"
	default:
		return "singular " + f.Kind().String()
	}
}

// value compares an element (singular value, list element or map value) with the field's element type.
func (c *driftChecker) value(path string, alts []jsonNode, fd protoreflect.FieldDescriptor) {
	kind := c.kindOf(path, alts)
	got := fd.Kind()
	switch kind {
	case "":
		return
	case kObject:
		if got != protoreflect.MessageKind {
			c.add(schemaToProto, path, "schema object but proto element is %s", got)
			return
		}
		c.object(path, alts, fd.Message())
		return
	case kMap, kArray:
		c.add(schemaToProto, path, "nested container (%s inside a record/array) has no proto3 projection", kind)
		return
	}
	c.leaves++
	want := scalarKinds(kind, alts)
	for _, k := range want {
		if got == k {
			return
		}
	}
	c.add(schemaToProto, path, "schema %s%s but proto type is %s (want %s)", kind, rangeOf(alts), got, kindNames(want))
}

// scalarKinds: the proto kinds that may hold a schema scalar of the given kind and range.
func scalarKinds(kind string, alts []jsonNode) []protoreflect.Kind {
	switch kind {
	case kString:
		return []protoreflect.Kind{protoreflect.StringKind}
	case kBool:
		return []protoreflect.Kind{protoreflect.BoolKind}
	case kNumber:
		return []protoreflect.Kind{protoreflect.DoubleKind}
	case kInteger:
		lo, hi := integerRange(alts)
		switch {
		case lo < 0 && lo >= math.MinInt32 && hi <= math.MaxInt32:
			return []protoreflect.Kind{protoreflect.Int32Kind, protoreflect.Sint32Kind}
		case lo < 0:
			return []protoreflect.Kind{protoreflect.Int64Kind, protoreflect.Sint64Kind}
		case hi <= math.MaxUint32:
			return []protoreflect.Kind{protoreflect.Uint32Kind}
		default:
			return []protoreflect.Kind{protoreflect.Uint64Kind}
		}
	}
	return nil
}

// integerRange: the union of the alternatives' ranges (const/enum count as their values). A missing
// bound is unbounded on that side.
func integerRange(alts []jsonNode) (lo, hi float64) {
	lo, hi = math.Inf(1), math.Inf(-1)
	widen := func(v float64) {
		lo, hi = math.Min(lo, v), math.Max(hi, v)
	}
	for _, a := range alts {
		if v, ok := a["const"].(float64); ok {
			widen(v)
			continue
		}
		if enum, ok := a["enum"].([]any); ok {
			for _, e := range enum {
				if v, ok := e.(float64); ok {
					widen(v)
				}
			}
			continue
		}
		mn, okMin := a["minimum"].(float64)
		if !okMin {
			mn = math.Inf(-1)
		}
		if v, ok := a["exclusiveMinimum"].(float64); ok {
			mn = v + 1
		}
		mx, okMax := a["maximum"].(float64)
		if !okMax {
			mx = math.Inf(1)
		}
		if v, ok := a["exclusiveMaximum"].(float64); ok {
			mx = v - 1
		}
		widen(mn)
		widen(mx)
	}
	return lo, hi
}

func rangeOf(alts []jsonNode) string {
	if kindOfNode(alts[0]) != kInteger {
		return ""
	}
	lo, hi := integerRange(alts)
	return fmt.Sprintf(" [%v, %v]", lo, hi)
}

func kindNames(ks []protoreflect.Kind) string {
	s := make([]string, len(ks))
	for i, k := range ks {
		s[i] = k.String()
	}
	return strings.Join(s, "|")
}

func loadGeneratedSchema(t *testing.T) jsonNode {
	t.Helper()
	path := generatedSchema
	if p := os.Getenv("VRX_DRIFT_SCHEMA"); p != "" {
		path = p
	}
	b, err := os.ReadFile(path) //nolint:gosec // fixed repository path or an explicit test override
	if err != nil {
		if os.IsNotExist(err) && os.Getenv("CI") == "" {
			t.Skipf("%s not generated — run `pnpm --filter @ngfw/schema gen` (tools/ci.sh does; under CI this is a failure)", path)
		}
		t.Fatalf("read generated JSON Schema: %v (run `pnpm gen` first)", err)
	}
	var root jsonNode
	if err := json.Unmarshal(b, &root); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return root
}

func reportFindings(t *testing.T, c *driftChecker) {
	t.Helper()
	byDir := map[string]int{}
	for _, f := range c.findings {
		byDir[f.dir]++
		t.Errorf("%s", f)
	}
	if len(c.findings) > 0 {
		t.Errorf("schema/proto drift: %d finding(s) — %d %s, %d %s. Fix packages/proto/vrx/v1/dataplane.proto "+
			"(contract(proto): commit, pnpm gen) or, for a schema gap, the schema owner.",
			len(c.findings), byDir[schemaToProto], schemaToProto, byDir[protoToSchema], protoToSchema)
	}
}

// TestSchemaProtoDrift is the guard itself: the generated JSON Schema of RootConfig and DesiredState
// must describe the same tree.
func TestSchemaProtoDrift(t *testing.T) {
	root := loadGeneratedSchema(t)
	c := checkDrift(root, (&vrxv1.DesiredState{}).ProtoReflect().Descriptor(), acceptedDrift)
	reportFindings(t, c)
	for _, f := range c.accepted {
		t.Logf("accepted: %s (%s)", f, acceptedDrift[f.dir+" "+f.path])
	}
	t.Logf("drift guard: %d scalar leaves and %d messages compared, %d accepted difference(s), %d finding(s)",
		c.leaves, len(c.messages), len(c.accepted), len(c.findings))
	if c.leaves < 500 || len(c.messages) < 150 {
		t.Errorf("walk covered only %d leaves / %d messages — the schema or the walker is broken", c.leaves, len(c.messages))
	}
}

// TestSchemaProtoDriftDetectsBreakage feeds the guard deliberately broken inputs and requires each
// kind of drift to be reported — so a guard that silently passes everything cannot go unnoticed.
func TestSchemaProtoDriftDetectsBreakage(t *testing.T) {
	root := loadGeneratedSchema(t)
	props := func(n jsonNode, keys ...string) jsonNode {
		for _, k := range keys {
			n = n[k].(jsonNode)
		}
		return n
	}
	system := props(root, "properties", "system", "properties")
	system["bogusLeaf"] = jsonNode{"type": "string"}                                       // schema-only leaf
	delete(system, "hostname")                                                             // proto-only field
	system["timezone"] = jsonNode{"type": "string", "x-vrx-ui": jsonNode{"secret": true}}  // secret with a field
	props(root, "properties", "interfaces", "additionalProperties", "properties")["mtu"] = // retyped leaf
		jsonNode{"type": "string"}
	props(root, "properties", "vrfs", "additionalProperties", "properties")["id"] = // width: needs uint64
		jsonNode{"type": "integer", "minimum": float64(0), "maximum": float64(1 << 40)}
	props(root, "properties", "routing", "properties")["static"] = jsonNode{"type": "object"} // array → object

	c := checkDrift(root, (&vrxv1.DesiredState{}).ProtoReflect().Descriptor(), acceptedDrift)
	want := []string{
		"schema→proto /system/bogusLeaf: schema leaf has no field in vrx.v1.SystemConfig",
		"proto→schema /system/hostname: proto field vrx.v1.SystemConfig.hostname = 1 has no schema leaf",
		"schema→proto /system/timezone: secret-flagged leaf has proto field timezone",
		"schema→proto /interfaces/{}/mtu: schema string but proto type is uint32",
		"schema→proto /vrfs/{}/id: schema integer [0, 1.099511627776e+12] but proto type is uint32 (want uint64)",
		"schema→proto /routing/static: schema single value but proto field static is repeated",
	}
	got := make([]string, len(c.findings))
	for i, f := range c.findings {
		got[i] = f.String()
	}
	for _, w := range want {
		found := false
		for _, g := range got {
			if strings.HasPrefix(g, w) {
				found = true
			}
		}
		if !found {
			t.Errorf("guard missed: %s", w)
		}
	}
	if len(got) != len(want) {
		t.Errorf("want exactly %d findings, got %d:\n%s", len(want), len(got), strings.Join(got, "\n"))
	}
}

// TestSchemaProtoDriftDetectsImplicitPresence: a proto3 scalar without `optional` is reported (D-039).
// DesiredState itself has none (TestExplicitPresenceEverywhere), so a throw-away descriptor is built.
func TestSchemaProtoDriftDetectsImplicitPresence(t *testing.T) {
	fdp := &descriptorpb.FileDescriptorProto{
		Name:    proto.String("drift_probe.proto"),
		Package: proto.String("probe"),
		Syntax:  proto.String("proto3"),
		MessageType: []*descriptorpb.DescriptorProto{{
			Name: proto.String("Probe"),
			Field: []*descriptorpb.FieldDescriptorProto{{
				Name: proto.String("host_name"), JsonName: proto.String("hostName"), Number: proto.Int32(1),
				Label: descriptorpb.FieldDescriptorProto_LABEL_OPTIONAL.Enum(), Type: descriptorpb.FieldDescriptorProto_TYPE_STRING.Enum(),
			}},
		}},
	}
	fd, err := protodesc.NewFile(fdp, nil)
	if err != nil {
		t.Fatal(err)
	}
	schema := jsonNode{"type": "object", "properties": jsonNode{"hostName": jsonNode{"type": "string"}}}
	c := checkDrift(schema, fd.Messages().Get(0), nil)
	if len(c.findings) != 1 || !strings.Contains(c.findings[0].msg, "implicit presence") {
		t.Fatalf("want one implicit-presence finding, got %v", c.findings)
	}
}

// TestModelStaysAgentInternal: the reconciler object model (packages/proto/vrx/model, D-055) is
// agent-internal — the API↔agent contract file must not import it, so it can never leak onto the
// wire or into the TS stubs.
func TestModelStaysAgentInternal(t *testing.T) {
	imports := vrxv1.File_vrx_v1_dataplane_proto.Imports()
	for i := 0; i < imports.Len(); i++ {
		if p := imports.Get(i).Path(); strings.HasPrefix(p, "vrx/model/") {
			t.Errorf("vrx/v1/dataplane.proto imports %s — the object model must stay agent-internal", p)
		}
	}
}
