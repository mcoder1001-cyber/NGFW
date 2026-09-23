package frr

import (
	"context"
	"errors"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	"ngfw/agent/internal/renderers"
)

// fakeBGP stands in for P12's `router bgp` section: it slots into the registry without
// touching framework files and receives the same input message.
type fakeBGP struct{}

func (fakeBGP) Name() string { return "bgp" }
func (fakeBGP) Order() int   { return 500 }
func (fakeBGP) Render(desired proto.Message) ([]string, error) {
	ds, _, err := Desired(desired)
	if err != nil {
		return nil, err
	}
	rid := "0.0.0.0"
	if ds.GetSystem().GetHostname() != "" {
		rid = "10.12.0.1"
	}
	return []string{"router bgp 65012", " bgp router-id " + rid, "exit"}, nil
}

// lineSection returns fixed lines (to test the framework's per-line backstop).
type lineSection struct {
	name  string
	order int
	lines []string
	err   error
}

func (s lineSection) Name() string                           { return s.name }
func (s lineSection) Order() int                             { return s.order }
func (s lineSection) Render(proto.Message) ([]string, error) { return s.lines, s.err }

func TestSectionOrderAndSeparators(t *testing.T) {
	r := testRenderer(
		lineSection{name: "zzz", order: 400, lines: []string{"route-map RM-A permit 10", "exit"}},
		fakeBGP{},
		lineSection{name: "empty", order: 450, lines: []string{"", "   "}}, // absent: no "!" emitted
	)
	out := renderConf(t, r, nil)
	want := "service integrated-vtysh-config\n!\nroute-map RM-A permit 10\nexit\n!\nrouter bgp 65012\n bgp router-id 0.0.0.0\nexit\n!\nend\n"
	if !strings.HasSuffix(out, want) {
		t.Fatalf("got\n%s\nwant suffix\n%s", out, want)
	}
}

func TestSectionLineBackstop(t *testing.T) {
	for name, lines := range map[string][]string{
		"embedded newline": {"router bgp 1\nno router bgp 1"},
		"carriage return":  {"router bgp 1\r"},
		"bare end":         {"router bgp 1", " end", "exit"},
		"escape":           {"description \x1b[2J"},
		"nul":              {"a\x00"},
		"invalid utf8":     {"a\xff"},
	} {
		r := testRenderer(lineSection{name: "evil", order: 500, lines: lines})
		if _, err := r.Render(context.Background(), nil); err == nil || !errors.Is(err, renderers.ErrUnsafe) {
			t.Errorf("%s: err = %v, want ErrUnsafe", name, err)
		}
	}
	boom := errors.New("boom")
	r := testRenderer(lineSection{name: "failing", order: 500, err: boom})
	if _, err := r.Render(context.Background(), nil); !errors.Is(err, boom) || !strings.Contains(err.Error(), `section "failing"`) {
		t.Errorf("section error not propagated with its name: %v", err)
	}
	r = testRenderer(fakeBGP{}, fakeBGP{})
	if _, err := r.Render(context.Background(), nil); err == nil {
		t.Error("duplicate section name accepted")
	}
}

func TestRegisterSection(t *testing.T) {
	t.Cleanup(func() {
		registryMu.Lock()
		delete(registry, "bgp")
		registryMu.Unlock()
	})
	RegisterSection(fakeBGP{})
	got := RegisteredSections()
	if len(got) != 1 || got[0].Name() != "bgp" {
		t.Fatalf("RegisteredSections = %v", got)
	}
	// A renderer without WithSections picks up the registry.
	out := renderConf(t, New(renderers.NewRecordingRunner()), nil)
	if !strings.Contains(out, "!\nrouter bgp 65012\n") {
		t.Fatalf("registered section not rendered:\n%s", out)
	}
	for name, s := range map[string]Section{
		"nil":            nil,
		"duplicate":      fakeBGP{},
		"framework name": lineSection{name: "static", order: 500},
		"bad name":       lineSection{name: "Bad Name", order: 500},
		"order low":      lineSection{name: "low", order: OrderStatic},
		"order high":     lineSection{name: "high", order: 900},
	} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("%s: RegisterSection did not panic", name)
				}
			}()
			RegisterSection(s)
		}()
	}
}

func TestFrameworkSectionsArePublicSections(t *testing.T) {
	for _, s := range frameworkSections(DefaultVersion, IdentityMapper) {
		if s.Order() >= OrderProtocolMin && s.Order() <= OrderProtocolMax {
			t.Errorf("framework section %s uses the protocol order range", s.Name())
		}
		if _, err := s.Render(nil); err != nil {
			t.Errorf("%s.Render(nil): %v", s.Name(), err)
		}
	}
}
