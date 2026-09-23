package dfkit_test

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"os"
	"path/filepath"
	"testing"

	"go.fd.io/govpp/adapter"
	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/dfkit/dfkittest"
	"ngfw/agent/internal/vpp/bootid"
)

type spec struct {
	Name  string `json:"name"`
	Count uint32 `json:"count"`
	On    bool   `json:"on"`
}

func TestCodec(t *testing.T) {
	a := dfkit.Encode(spec{Name: "x", Count: 4294967295, On: true})
	b := dfkit.Encode(spec{On: true, Count: 4294967295, Name: "x"})
	if !proto.Equal(a, b) || len(a.Fields) != 3 {
		t.Fatalf("encode not canonical: %v vs %v", a, b)
	}
	if !proto.Equal(dfkit.Encode(spec{}), dfkit.Encode(spec{})) || len(dfkit.Encode(spec{}).Fields) != 3 {
		t.Fatal("zero values must be encoded")
	}
	var got spec
	if err := dfkit.Decode(a, &got); err != nil || got != (spec{Name: "x", Count: 4294967295, On: true}) {
		t.Fatalf("decode %+v %v", got, err)
	}
	extra, _ := structpb.NewStruct(map[string]any{"name": "x", "bogus": 1})
	wrong, _ := structpb.NewStruct(map[string]any{"name": 5})
	for _, bad := range []proto.Message{extra, wrong, &structpb.Value{}, nil} {
		if err := dfkit.Decode(bad, &got); !errors.Is(err, dfkit.ErrSpec) {
			t.Errorf("%v: %v", bad, err)
		}
	}
}

func TestAddr(t *testing.T) {
	for in, want := range map[string]string{"10.5.0.1": "10.5.0.1", "::ffff:10.5.0.1": "10.5.0.1", "FD00::1": "fd00::1"} {
		a, err := dfkit.ParseAddr(in)
		if err != nil || a.String() != want {
			t.Errorf("%s: %v %v", in, a, err)
		}
		if back := dfkit.FromAPIAddress(dfkit.ToAPIAddress(a)); back != a {
			t.Errorf("round trip %s → %s", a, back)
		}
	}
	if _, err := dfkit.ParseAddr("fe80::1%eth0"); !errors.Is(err, dfkit.ErrSpec) {
		t.Fatal(err)
	}
	if _, err := dfkit.ParsePrefix("10.5.0.1/16"); !errors.Is(err, dfkit.ErrSpec) {
		t.Fatal(err)
	}
	if p, err := dfkit.ParsePrefix("10.5.0.0/16"); err != nil || dfkit.Family(p.Addr()) != "ip4" || dfkit.Family(netip.IPv6Loopback()) != "ip6" {
		t.Fatal(err)
	}
}

func TestInterfaces(t *testing.T) {
	f := dfkittest.NewFake(dfkittest.Iface{Index: 1, Name: "loop501", Tag: "w5:loop501"}, dfkittest.Iface{Index: 2, Name: "loop601", Tag: "w6:loop601"})
	tbl, err := dfkit.DumpInterfaces(context.Background(), f, "w5")
	if err != nil {
		t.Fatal(err)
	}
	if idx, err := tbl.Resolve("loop501"); err != nil || idx != 1 {
		t.Fatal(idx, err)
	}
	if _, err := tbl.Resolve("loop601"); !errors.Is(err, dfkit.ErrNotOwned) {
		t.Fatal(err)
	}
	if _, err := tbl.Resolve("nope"); !errors.Is(err, dfkit.ErrNoInterface) {
		t.Fatal(err)
	}
	if n, ok := tbl.Reportable(2, "x.y"); ok || n != "" {
		t.Fatal(n)
	}
	if dfkit.DefaultInterfaceKey("loop501") != "interface/loop501" || dfkit.DefaultVRFKey("5001") != "vrf/5001" {
		t.Fatal("key schemes")
	}
}

func TestErrors(t *testing.T) {
	err := dfkit.RetrieveUnsupported("x.y")
	if !errors.Is(err, dfkit.ErrRetrieveUnsupported) || err.Error() != "x.y: vpp has no dump for this object type" {
		t.Fatal(err)
	}
	if !errors.Is(dfkit.PluginError("lcp", &adapter.UnknownMsgError{MsgName: "m"}), dfkit.ErrPluginNotLoaded) {
		t.Fatal("plugin error")
	}
	if dfkit.PluginError("lcp", nil) != nil {
		t.Fatal("nil")
	}
	wrapped := fmt.Errorf("ctx: %w", api.VPPApiError(api.VALUE_EXIST))
	if !dfkit.IsVPPError(wrapped, api.NO_SUCH_ENTRY, api.VALUE_EXIST) || dfkit.IsVPPError(wrapped, api.NO_SUCH_ENTRY) || dfkit.IsVPPError(errors.New("x"), api.VALUE_EXIST) {
		t.Fatal("IsVPPError")
	}
}

func TestBootIdentity(t *testing.T) {
	root := t.TempDir()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(filepath.Join(root, "sys/kernel/random"), 0o750))
	must(os.WriteFile(filepath.Join(root, "sys/kernel/random/boot_id"), []byte("b7712a53-c1e7\n"), 0o600))
	must(os.MkdirAll(filepath.Join(root, "1000"), 0o750))
	// comm with blanks and ')' — fields are counted after the last ')'
	stat := "1000 (vpp main) x) S 1 1000 1000 0 -1 4194560 1 0 0 0 5 6 0 0 20 0 3 0 424242 1000 10 18446744073709551615"
	must(os.WriteFile(filepath.Join(root, "1000/stat"), []byte(stat), 0o600))
	t.Cleanup(bootid.SetProcRoot(root))
	f := dfkittest.NewFake()
	id, err := dfkit.BootIdentity(context.Background(), f)
	if err != nil || id.String() != "b7712a53-c1e7/1000/424242" {
		t.Fatalf("identity %q %v", id, err)
	}
	f.RestartVPP() // PID 1001: no /proc entry → error, never a guessed identity
	if _, err := dfkit.BootIdentity(context.Background(), f); err == nil {
		t.Fatal("missing /proc/<pid>/stat must be an error")
	}
}

// TD-1: a BootRecord written in the pre-D-080 PID-only format (or any other non-triple) never
// matches the current identity: the object is re-added once, then recorded in the new format.
func TestBootRecordLegacyFormat(t *testing.T) {
	f := dfkittest.NewFake()
	ctx := context.Background()
	store := dfkit.NewMemoryBootStore()
	for _, legacy := range []string{"1000", "fake/1000", ""} {
		if err := store.Put(dfkit.BootRecord{Key: "k/1", Identity: legacy, Value: "{}"}); err != nil {
			t.Fatal(err)
		}
		applied, id, err := dfkit.AppliedThisBoot(ctx, f, store, "k/1", "{}")
		if err != nil || applied {
			t.Fatalf("legacy %q: applied=%v err=%v", legacy, applied, err)
		}
		if started, err := dfkit.StartedThisBoot(ctx, f, store, "k/1"); err != nil || started {
			t.Fatalf("legacy %q: started=%v err=%v", legacy, started, err)
		}
		if err := store.Put(dfkit.BootRecord{Key: "k/1", Identity: id, Value: "{}"}); err != nil {
			t.Fatal(err)
		}
		if applied, _, _ := dfkit.AppliedThisBoot(ctx, f, store, "k/1", "{}"); !applied {
			t.Fatalf("record %q in the new format not matched", id)
		}
	}
	f.RestartVPP()
	if applied, _, _ := dfkit.AppliedThisBoot(ctx, f, store, "k/1", "{}"); applied {
		t.Fatal("record of the previous VPP instance matched")
	}
}

func TestFileBootStore(t *testing.T) {
	p := filepath.Join(t.TempDir(), "boot.json")
	s, err := dfkit.NewFileBootStore(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Put(dfkit.BootRecord{Key: "pcap.capture/global", Identity: "a/1/2", Value: "{}"}); err != nil {
		t.Fatal(err)
	}
	s2, err := dfkit.NewFileBootStore(p) // agent restart
	if err != nil {
		t.Fatal(err)
	}
	if r, ok := s2.Get("pcap.capture/global"); !ok || r.Identity != "a/1/2" {
		t.Fatalf("not persisted: %+v", r)
	}
	if err := s2.Delete("pcap.capture/global"); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Dir(p), 0o500); err != nil { //nolint:gosec // test: make the dir read-only // write fails → memory unchanged
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(filepath.Dir(p), 0o700) }) //nolint:gosec // test cleanup
	if os.Geteuid() != 0 {
		if err := s2.Put(dfkit.BootRecord{Key: "k"}); err == nil {
			t.Fatal("write into a read-only dir succeeded")
		}
		if _, ok := s2.Get("k"); ok {
			t.Fatal("memory changed although the write failed")
		}
	}
}

func TestTargetClaims(t *testing.T) {
	f := dfkittest.NewFake(dfkittest.Iface{Index: 1, Name: "loop501", Tag: "w5c:loop501"}, dfkittest.Iface{Index: 9, Name: "ens192"})
	ctx := context.Background()
	tagged, err := dfkit.ResolveTarget(ctx, f, "loop501", "w5c", "x.y")
	if err != nil || tagged.Untagged || tagged.Adopt() != nil {
		t.Fatalf("tagged: %+v %v", tagged, err)
	}
	u, err := dfkit.ResolveTarget(ctx, f, "ens192", "w5c", "x.y")
	if err != nil || !u.Untagged {
		t.Fatal(err)
	}
	if !errors.Is(u.Adopt(), dfkit.ErrNotOurs) {
		t.Fatal("unclaimed untagged object adopted")
	}
	if err := u.Claim(); err != nil || u.Adopt() != nil {
		t.Fatalf("claimed: %v", err)
	}
	tbl, _ := dfkit.DumpInterfaces(ctx, f, "w5c")
	if n, ok := tbl.Reportable(9, "x.y"); !ok || n != "ens192" {
		t.Fatal("claimed untagged not reportable")
	}
	f.RestartVPP() // D-080: claims of an earlier VPP instance expire
	u2, _ := dfkit.ResolveTarget(ctx, f, "ens192", "w5c", "x.y")
	if u2.Claimed() {
		t.Fatal("claim survived a VPP restart")
	}
	if _, ok, err := dfkit.ResolveForDelete(ctx, f, "ens192", "w5c", "x.y"); ok || err != nil {
		t.Fatalf("unclaimed untagged must not be deleted: %v %v", ok, err)
	}
	if _, ok, err := dfkit.ResolveForDelete(ctx, f, "gone0", "w5c", "x.y"); ok || err != nil {
		t.Fatalf("gone interface: %v %v", ok, err)
	}
}
