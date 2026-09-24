package bootid_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"ngfw/agent/binapi/memclnt"
	"ngfw/agent/internal/vpp/bootid"
	"ngfw/agent/internal/vpp/fake"
)

func TestParseStat(t *testing.T) {
	tail := " S 1 2 3 0 -1 4194560 0 0 0 0 0 0 0 0 20 0 3 0 987654 1000 10 0"
	for name, line := range map[string]string{
		"plain":          "4242 (vpp_main)" + tail,
		"comm spaces":    "4242 (vpp main thread)" + tail,
		"comm paren":     "4242 (a) b)" + tail,
		"comm ') S 1 2'": "4242 (x) S 1 2 3)" + tail,
		"comm empty":     "4242 ()" + tail,
		"newline":        "4242 (vpp)" + tail + "\n",
	} {
		t.Run(name, func(t *testing.T) {
			got, err := bootid.ParseStat([]byte(line))
			if err != nil || got != 987654 {
				t.Fatalf("ParseStat(%q) = %d, %v", line, got, err)
			}
		})
	}
	for name, line := range map[string]string{
		"no comm":      "4242 vpp S 1 2",
		"short":        "4242 (vpp) S 1 2 3",
		"non numeric":  "4242 (vpp) S 1 2 3 0 -1 4194560 0 0 0 0 0 0 0 0 20 0 3 0 x 1000",
		"empty":        "",
		"only closing": "4242 vpp) S 1 2 3 0 -1 4194560 0 0 0 0 0 0 0 0 20 0 3 0 5 1000",
	} {
		t.Run("bad "+name, func(t *testing.T) {
			if v, err := bootid.ParseStat([]byte(line)); err == nil {
				t.Fatalf("ParseStat(%q) = %d, want error", line, v)
			}
		})
	}
}

func TestStringParseRoundTrip(t *testing.T) {
	for _, id := range []bootid.Identity{
		{BootID: "0f1e2d3c-4b5a-6978-8796-a5b4c3d2e1f0", PID: 4242, StartTime: 987654},
		{PID: 4242},
		{BootID: "b", PID: 0, StartTime: 1},
		{},
	} {
		s := id.String()
		got, err := bootid.Parse(s)
		if err != nil || !got.Equal(id) {
			t.Fatalf("Parse(%q) = %+v, %v; want %+v", s, got, err, id)
		}
		if !bootid.Matches(s, id) {
			t.Fatalf("Matches(%q, %+v) = false", s, id)
		}
	}
	if s := (bootid.Identity{BootID: "b", PID: 7, StartTime: 9}).String(); s != "b/7/9" {
		t.Fatalf("String = %q (the encoding persisted records rely on)", s)
	}
	if s := (bootid.Identity{PID: 7}).String(); s != "?/7/?" {
		t.Fatalf("String = %q", s)
	}
}

func TestParseLegacyAndMalformed(t *testing.T) {
	cur := bootid.Identity{BootID: "b", PID: 4242, StartTime: 9}
	// pre-D-080 records carried the PID only: never a match (re-add once / claim expired)
	if _, err := bootid.Parse("4242"); !errors.Is(err, bootid.ErrLegacy) {
		t.Fatalf("Parse(pid) err = %v, want ErrLegacy", err)
	}
	if bootid.Matches("4242", cur) {
		t.Fatal("legacy pid-only record matched the current identity")
	}
	for _, s := range []string{"", "b/4242", "b/4242/9/x", "/4242/9", "b/x/9", "b/-1/9", "b/4242/0", "b/4242/y", "fake/1000"} {
		if _, err := bootid.Parse(s); !errors.Is(err, bootid.ErrFormat) {
			t.Fatalf("Parse(%q) err = %v, want ErrFormat", s, err)
		}
		if bootid.Matches(s, cur) {
			t.Fatalf("Matches(%q) = true", s)
		}
	}
}

func TestEqual(t *testing.T) {
	a := bootid.Identity{BootID: "b1", PID: 10, StartTime: 100}
	for _, o := range []bootid.Identity{
		{BootID: "b2", PID: 10, StartTime: 100}, // host reboot, same PID
		{BootID: "b1", PID: 11, StartTime: 100},
		{BootID: "b1", PID: 10, StartTime: 101}, // PID reused after a restart
		{PID: 10},
	} {
		if a.Equal(o) {
			t.Fatalf("%v equal to %v", a, o)
		}
	}
	if !a.Equal(bootid.Identity{BootID: "b1", PID: 10, StartTime: 100}) {
		t.Fatal("identical identities differ")
	}
	if !a.Complete() || (bootid.Identity{BootID: "b", PID: 1}).Complete() || (bootid.Identity{PID: 1, StartTime: 1}).Complete() {
		t.Fatal("Complete wrong")
	}
	if a.IsZero() || !(bootid.Identity{}).IsZero() {
		t.Fatal("IsZero wrong")
	}
}

func TestCurrentWithFakeProc(t *testing.T) {
	root := t.TempDir()
	if err := bootid.WriteFakeProc(root, "boot-1", map[int]uint64{4242: 555}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(bootid.SetProcRoot(root))
	f := fake.New(fake.WithControlPingReply(&memclnt.ControlPingReply{VpePID: 4242}))
	ctx := context.Background()
	id, err := bootid.Current(ctx, f)
	if err != nil {
		t.Fatal(err)
	}
	if want := (bootid.Identity{BootID: "boot-1", PID: 4242, StartTime: 555}); !id.Equal(want) || !id.Complete() {
		t.Fatalf("Current = %+v, want %+v", id, want)
	}
	// VPP restarted: new PID without a readable stat → partial identity, still different
	f.Reply("control_ping", &memclnt.ControlPingReply{VpePID: 4243})
	id2, err := bootid.Current(ctx, f)
	if err != nil {
		t.Fatal(err)
	}
	if id2.Equal(id) || id2.Complete() || id2.String() != "boot-1/4243/?" {
		t.Fatalf("after restart = %v", id2)
	}
	// same PID after a reboot: the start time differs
	if err := os.WriteFile(filepath.Join(root, "sys/kernel/random/boot_id"), []byte("boot-2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	f.Reply("control_ping", &memclnt.ControlPingReply{VpePID: 4242})
	id3, _ := bootid.Current(ctx, f)
	if id3.Equal(id) || id3.BootID != "boot-2" {
		t.Fatalf("after reboot = %v", id3)
	}
}

func TestCurrentControlPingFails(t *testing.T) {
	f := fake.New() // no control_ping handler
	if _, err := bootid.Current(context.Background(), f); err == nil {
		t.Fatal("want error")
	}
}

func TestReaderRealProc(t *testing.T) {
	// the test process itself: boot_id and a start time must be readable on Linux
	id := bootid.Reader{}.ForPID(os.Getpid())
	if !id.Complete() {
		t.Skipf("no /proc here: %v", id)
	}
	if again := (bootid.Reader{}).ForPID(os.Getpid()); !again.Equal(id) {
		t.Fatalf("unstable: %v vs %v", id, again)
	}
}
