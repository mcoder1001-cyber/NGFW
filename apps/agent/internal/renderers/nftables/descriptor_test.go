package nftables

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/renderers"
	"ngfw/agent/internal/scheduler"
)

// kernelJSON is a real `nft -j list table inet vrx_w9` of testdata/<name>.golden, captured once from
// nft 1.1.6 inside a throwaway slot namespace (testdata/kernel-<name>.json).
func kernelJSON(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "kernel-"+name+".json")) //nolint:gosec // testdata
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// TestKernelRoundTrip: the kernel's own JSON of a rendering, annotated from the value, equals the value
// — the property the scheduler relies on (Retrieve == desired after Apply).
func TestKernelRoundTrip(t *testing.T) {
	basic, _ := build(t, fixture(t))
	full, _ := build(t, doc(t, fullDoc))
	for name, v := range map[string]*HostTable{"basic": basic, "full": full} {
		k, err := ParseKernel(kernelJSON(t, name), "vrx_w9")
		if err != nil {
			t.Fatal(err)
		}
		got := k.Table()
		got.Config = v.GetConfig()
		annotate(got, v)
		if !proto.Equal(got, v) {
			t.Errorf("%s: kernel view differs from the rendered value:\n got %v\nwant %v", name, got, v)
		}
	}
}

func TestKernelDriftIsVisible(t *testing.T) {
	v, _ := build(t, doc(t, fullDoc))
	raw := kernelJSON(t, "full")
	for name, edit := range map[string]func([]byte) []byte{
		"set element": func(b []byte) []byte { return bytes.Replace(b, []byte(`"192.0.2.10"`), []byte(`"192.0.2.11"`), 1) },
		"rule comment": func(b []byte) []byte {
			return bytes.Replace(b, []byte(`vrx:local-in:1000/0:54750c03`), []byte(`hand edit`), 1)
		},
		"policy": func(b []byte) []byte { return bytes.Replace(b, []byte(`"policy": "drop"`), []byte(`"policy": "accept"`), 1) },
		"prio":   func(b []byte) []byte { return bytes.Replace(b, []byte(`"prio": -100`), []byte(`"prio": -99`), 1) },
	} {
		k, err := ParseKernel(edit(slices.Clone(raw)), "vrx_w9")
		if err != nil {
			t.Fatal(err)
		}
		got := k.Table()
		got.Config = v.GetConfig()
		annotate(got, v)
		if proto.Equal(got, v) {
			t.Errorf("%s: drift not visible", name)
		}
	}
}

func TestParseKernelCountersAndElements(t *testing.T) {
	raw := bytes.Replace(kernelJSON(t, "full"), []byte(`"packets": 0,
       "bytes": 0`), []byte(`"packets": 7,
       "bytes": 420`), 1)
	k, err := ParseKernel(raw, "vrx_w9")
	if err != nil {
		t.Fatal(err)
	}
	if r := k.Chains[0].Rules[0]; r.Packets != 7 || r.Bytes != 420 || r.Verdict != "accept" || r.Comment != "vrx:@established/0:c77fde03" {
		t.Errorf("first rule %+v", r)
	}
	if k.Chains[0].Rules[len(k.Chains[0].Rules)-1].Verdict != "drop" {
		t.Error("verdict of the last rule")
	}
	// nft prints a /32 as a bare address and ranges as {"range": [a, b]}; both normalise to prefixes.
	var raws []json.RawMessage
	for _, e := range []string{`"10.0.0.1"`, `{"prefix": {"addr": "10.1.0.0", "len": 16}}`, `{"range": ["10.2.0.0", "10.2.0.5"]}`, `{"elem": {"val": "2001:db8::1", "comment": "x"}}`} {
		raws = append(raws, json.RawMessage(e))
	}
	elems, err := setElements(raws)
	if err != nil || !slices.Equal(elems, []string{"10.0.0.1/32", "10.1.0.0/16", "10.2.0.0/30", "10.2.0.4/31", "2001:db8::1/128"}) {
		t.Errorf("elements %v %v", elems, err)
	}
	if _, err := ParseKernel([]byte("{not json"), "vrx"); err == nil {
		t.Error("bad JSON accepted")
	}
}

// fakeNft is a RecordingRunner answer that behaves like nft for one table: -c succeeds, -f "loads" the
// file (remembers which golden it is), -j list answers with the captured kernel JSON or "no such table".
type fakeNft struct {
	loaded  string // "" | "full" | "basic"
	failF   bool
	listErr bool
}

func (f *fakeNft) run(cmd renderers.Command) (renderers.Output, error) {
	fail := func(stderr string) (renderers.Output, error) {
		out := renderers.Output{Stderr: []byte(stderr), ExitCode: 1}
		return out, &renderers.ExitError{Command: cmd, Output: out}
	}
	switch {
	case slices.Equal(cmd.Args[:1], []string{"-c"}):
		return renderers.Output{}, nil
	case cmd.Args[0] == "-f":
		if f.failF {
			return fail("Error: Could not process rule: Operation not supported")
		}
		content, err := os.ReadFile(cmd.Args[1])
		if err != nil {
			return renderers.Output{}, err
		}
		switch {
		case bytes.Contains(content, []byte("chain in_local-in")):
			f.loaded = "full"
		case bytes.Contains(content, []byte("chain in_mgmt-in")):
			f.loaded = "basic"
		default:
			f.loaded = ""
		}
		return renderers.Output{}, nil
	case cmd.Args[0] == "-j":
		if f.listErr {
			return fail("Error: Operation not permitted")
		}
		if f.loaded == "" {
			return fail("Error: No such file or directory; did you mean table 'vrx_w9' in family inet?\nlist table inet vrx_w9\n                 ^^^^^^")
		}
		raw, err := os.ReadFile(filepath.Join("testdata", "kernel-"+f.loaded+".json"))
		return renderers.Output{Stdout: raw}, err
	}
	return fail("unexpected argv")
}

func unitPaths(t *testing.T, mode string) Paths {
	dir := t.TempDir()
	return Paths{Table: "vrx_w9", Mode: mode, RulesFile: filepath.Join(dir, "host-acl-w9.nft"), StoreFile: filepath.Join(dir, "host-acl-w9.json")}
}

func TestDescriptorLifecycle(t *testing.T) {
	ctx := context.Background()
	fake := &fakeNft{}
	rec := renderers.NewRecordingRunner().On(NftBin, fake.run)
	p := unitPaths(t, ModeApply)
	d := NewDescriptor(New(rec, p), NewStore(p.StoreFile), nil)
	if d.Name() != DescriptorName || d.KeyOf(nil) != Key || Key != scheduler.Key("host-acl.nftables/vrx") {
		t.Fatalf("name/key %s %s", d.Name(), d.KeyOf(nil))
	}
	if kvs, err := d.Retrieve(ctx); err != nil || kvs != nil {
		t.Fatalf("empty: %v %v", kvs, err)
	}
	full, _ := build(t, doc(t, fullDoc))
	if _, err := d.Create(ctx, full); err != nil {
		t.Fatal(err)
	}
	calls := rec.Calls()
	if len(calls) != 3 || calls[1].Args[0] != "-c" || calls[1].Args[2] == p.RulesFile || !slices.Equal(calls[2].Args, []string{"-f", p.RulesFile}) {
		t.Fatalf("argv: %v", calls)
	}
	written, _ := os.ReadFile(p.RulesFile)
	want, _ := os.ReadFile("testdata/full.golden")
	if !bytes.Equal(written, want) {
		t.Error("the loaded file must be the golden rendering")
	}
	if st, _ := os.Stat(p.RulesFile); st.Mode().Perm() != 0o600 {
		t.Errorf("rules file mode %v", st.Mode())
	}
	kvs, err := d.Retrieve(ctx)
	if err != nil || len(kvs) != 1 || !proto.Equal(kvs[0].Value, full) {
		t.Fatalf("Retrieve after Create must equal desired: %v %v", kvs, err)
	}

	// A failed load leaves the old file (and the old kernel table) in place and the store unchanged.
	basic, _ := build(t, fixture(t))
	fake.failF = true
	if _, err := d.Update(ctx, full, basic, nil); err == nil {
		t.Fatal("Update must fail")
	}
	if again, _ := os.ReadFile(p.RulesFile); !bytes.Equal(again, want) {
		t.Error("the previous file must be restored")
	}
	if kvs, _ := d.Retrieve(ctx); !proto.Equal(kvs[0].Value, full) {
		t.Error("store and table must still be the old value")
	}
	fake.failF = false
	if _, err := d.Update(ctx, full, basic, nil); err != nil {
		t.Fatal(err)
	}
	if kvs, _ := d.Retrieve(ctx); !proto.Equal(kvs[0].Value, basic) {
		t.Error("Retrieve after Update")
	}

	// A lost table (restart with an empty kernel) is visible: config from the store, no chains → Update.
	fake.loaded = ""
	kvs, _ = d.Retrieve(ctx)
	if len(kvs) != 1 || proto.Equal(kvs[0].Value, basic) || len(kvs[0].Value.(*HostTable).GetChains()) != 0 {
		t.Errorf("lost table: %v", kvs)
	}
	// A fresh descriptor (agent restart) reads the same store.
	d2 := NewDescriptor(New(rec, p), NewStore(p.StoreFile), nil)
	fake.loaded = "basic"
	if kvs, _ := d2.Retrieve(ctx); !proto.Equal(kvs[0].Value, basic) {
		t.Error("restart: Retrieve must come from the store + kernel")
	}

	if err := d.Delete(ctx, basic, nil); err != nil {
		t.Fatal(err)
	}
	if kvs, err := d.Retrieve(ctx); err != nil || kvs != nil {
		t.Errorf("after Delete: %v %v", kvs, err)
	}
	if _, err := os.Stat(p.StoreFile); !os.IsNotExist(err) {
		t.Error("store must be removed")
	}
	fake.listErr = true
	if _, err := d.Retrieve(ctx); err == nil || !strings.Contains(err.Error(), "not permitted") {
		t.Errorf("list errors other than a missing table must fail: %v", err)
	}
}

func TestDescriptorCheckMode(t *testing.T) {
	ctx := context.Background()
	fake := &fakeNft{}
	rec := renderers.NewRecordingRunner().On(NftBin, fake.run)
	p := unitPaths(t, ModeCheck)
	d := NewDescriptor(New(rec, p), NewStore(p.StoreFile), nil)
	v, _ := build(t, fixture(t))
	if _, err := d.Create(ctx, v); err != nil {
		t.Fatal(err)
	}
	for _, c := range rec.Calls() {
		if c.Args[0] != "-c" {
			t.Errorf("check mode ran %v", c.Args)
		}
	}
	if kvs, _ := d.Retrieve(ctx); len(kvs) != 1 || !proto.Equal(kvs[0].Value, v) {
		t.Error("check mode: Retrieve is the stored value")
	}
	rt := &Runtime{Owner: "w9", Descriptor: d}
	st, err := rt.State(ctx, time.Unix(0, 0))
	if err != nil || st.GetMode() != ModeCheck || st.GetPresent() || !st.GetInSync() || len(st.GetChains()) != 1 {
		t.Errorf("state %v %v", st, err)
	}
}

func TestState(t *testing.T) {
	ctx := context.Background()
	fake := &fakeNft{}
	rec := renderers.NewRecordingRunner().On(NftBin, fake.run)
	p := unitPaths(t, ModeApply)
	d := NewDescriptor(New(rec, p), NewStore(p.StoreFile), nil)
	rt := &Runtime{Owner: "w9", Descriptor: d}
	Register("/tmp/x", rt)
	if RuntimeFor("/tmp/x/", "w9") != rt || RuntimeFor("/tmp/x", "w8") != nil {
		t.Error("registry")
	}
	st, err := rt.State(ctx, time.Unix(0, 0))
	if err != nil || st.GetPresent() || !st.GetInSync() || st.GetTable() != "vrx_w9" {
		t.Fatalf("nothing applied: %v %v", st, err)
	}
	v, _ := build(t, doc(t, fullDoc))
	if _, err := d.Create(ctx, v); err != nil {
		t.Fatal(err)
	}
	st, err = rt.State(ctx, time.Unix(0, 0))
	if err != nil || !st.GetPresent() || !st.GetInSync() || len(st.GetChains()) != 3 || len(st.GetSets()) != 5 {
		t.Fatalf("state %v %v", st, err)
	}
	c := st.GetChains()[0]
	if c.GetName() != "in_local-in" || c.GetList() != "local-in" || c.GetHook() != "input" || c.GetPolicy() != "drop" || c.GetPriority() != 10 {
		t.Errorf("chain %v", c)
	}
	last := c.GetRules()[len(c.GetRules())-1]
	if last.GetKind() != KindRule || last.GetList() != "local-in" || last.GetSequence() != 1000 || last.GetPointer() != "/acl/host/local-in/rules/7" || last.GetVerdict() != "drop" || !strings.HasPrefix(last.GetText(), "counter log prefix") {
		t.Errorf("rule %v", last)
	}
	if st.GetSets()[0].GetObject() != "backup" {
		t.Errorf("set %v", st.GetSets()[0])
	}
	fake.loaded = "basic" // somebody replaced the table
	if st, _ := rt.State(ctx, time.Unix(0, 0)); st.GetInSync() {
		t.Error("drift must show in_sync=false")
	}
}

func TestPathsFromEnv(t *testing.T) {
	dir := "/var/lib/vrx/agent"
	cases := []struct {
		owner, mode, ns string
		want            Paths
		err             bool
	}{
		{owner: "vrx", want: Paths{Table: "vrx", Mode: ModeApply}},
		{owner: "w9", want: Paths{Table: "vrx_w9", Mode: ModeCheck}},
		{owner: "w9", ns: "ns-w9-hacl", want: Paths{Table: "vrx_w9", Mode: ModeNetns, Netns: "ns-w9-hacl"}},
		{owner: "vrx", mode: "check", want: Paths{Table: "vrx", Mode: ModeCheck}},
		{owner: "w9", mode: "apply", err: true},
		{owner: "vrx", mode: "apply", ns: "ns-w9-hacl", err: true},
		{owner: "w9", mode: "netns", err: true},
		{owner: "w9", mode: "bogus", err: true},
		{owner: "w9", ns: "../../proc/1/ns/net", err: true},
		{owner: "w9", ns: "ns-w9-x;y", err: true},
	}
	for _, c := range cases {
		t.Setenv(EnvMode, c.mode)
		t.Setenv(EnvNetns, c.ns)
		p, err := PathsFromEnv(dir, c.owner)
		if c.err {
			if err == nil {
				t.Errorf("%+v: accepted %+v", c, p)
			}
			continue
		}
		if err != nil || p.Table != c.want.Table || p.Mode != c.want.Mode || p.Netns != c.want.Netns ||
			p.RulesFile != filepath.Join(dir, "host-acl-"+c.owner+".nft") || p.StoreFile != filepath.Join(dir, "host-acl-"+c.owner+".json") {
			t.Errorf("%+v: got %+v %v", c, p, err)
		}
	}
	// Only the product owner's table ever goes into the root netns, whatever builds the paths.
	if err := (Paths{Table: "vrx_w9", Mode: ModeApply, RulesFile: "/a", StoreFile: "/b"}).Validate("w9"); err == nil {
		t.Error("apply of a slot table in the root netns accepted")
	}
	if !slices.Equal(Binaries().Paths(), []string{NftBin}) {
		t.Errorf("product allowlist %v", Binaries().Paths())
	}
}
