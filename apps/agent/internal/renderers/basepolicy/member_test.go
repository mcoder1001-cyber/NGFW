package basepolicy

import (
	"context"
	"errors"
	"ngfw/agent/internal/renderers"
	"ngfw/agent/internal/scheduler"
	"slices"
	"strings"
	"testing"
)

type memberNft struct {
	state            []string
	calls            []renderers.Command
	mode             string
	reads, mutations int
	recoveryCanceled bool
	cancel           context.CancelFunc
}

func (f *memberNft) Run(ctx context.Context, c renderers.Command) (renderers.Output, error) {
	f.calls = append(f.calls, c)
	if len(f.calls) > 2 && ctx.Err() != nil {
		f.recoveryCanceled = true
	}
	if slices.Equal(c.Args, []string{"-j", "list", "set", "inet", "vrx_base", "punt_interfaces"}) {
		f.reads++
		if f.reads == 2 && (f.mode == "compensate" || f.mode == "unknown" || f.mode == "unconfirmed") {
			return renderers.Output{}, errors.New("EIO readback")
		}
		if f.reads >= 3 && (f.mode == "unknown" || f.mode == "unconfirmed") {
			return renderers.Output{}, errors.New("EIO confirmation")
		}
		return renderers.Output{Stdout: kernel(f.state)}, nil
	}
	if c.Path != NftBin || !slices.Equal(c.Args, []string{"-f", "-"}) {
		return renderers.Output{}, errors.New("invalid command")
	}
	f.mutations++
	if f.mutations == 1 && f.cancel != nil {
		f.cancel()
	}
	parts := strings.Split(string(c.Stdin), `"`)
	if len(parts) != 3 {
		return renderers.Output{}, errors.New("invalid typed input")
	}
	host := parts[1]
	if f.mutations == 2 && f.mode == "unknown" {
		return renderers.Output{}, errors.New("EIO compensation")
	}
	if f.mode == "reject" && f.mutations == 1 {
		return renderers.Output{}, errors.New("rejection")
	}
	if strings.HasPrefix(string(c.Stdin), "delete element ") {
		f.state = slices.DeleteFunc(f.state, func(s string) bool { return s == host })
	} else {
		if !slices.Contains(f.state, host) {
			f.state = append(f.state, host)
			slices.Sort(f.state)
		}
	}
	if f.mutations == 1 && f.mode != "success" {
		return renderers.Output{}, errors.New("EIO reply lost")
	}
	return renderers.Output{}, nil
}
func TestElementUnknownResolutionAndCompensation(t *testing.T) {
	for _, mode := range []string{"success", "reject", "committed", "compensate", "unknown", "unconfirmed"} {
		t.Run(mode, func(t *testing.T) {
			f := &memberNft{state: []string{"other", "tap1"}, mode: mode}
			r, _ := New(f, "mgmt0")
			err := r.Element(context.Background(), "tap1", false)
			switch mode {
			case "success", "committed":
				if err != nil || slices.Contains(f.state, "tap1") {
					t.Fatalf("%v %v", err, f.state)
				}
			case "reject", "compensate":
				if err == nil || errors.Is(err, scheduler.ErrUncertainOutcome) || !slices.Contains(f.state, "tap1") {
					t.Fatalf("%v %v", err, f.state)
				}
			default:
				if !errors.Is(err, scheduler.ErrUncertainOutcome) {
					t.Fatal("unresolved mutation lacked marker", err)
				}
			}
			if !slices.Contains(f.state, "other") {
				t.Fatal("foreign member altered")
			}
			for _, c := range f.calls {
				if strings.Contains(string(c.Stdin), "flush") {
					t.Fatal("whole-set mutation")
				}
			}
		})
	}
}
func TestElementAddNoOpAndInvalidInputs(t *testing.T) {
	f := &memberNft{state: []string{"other"}, mode: "success"}
	r, _ := New(f, "mgmt0")
	if err := r.Element(context.Background(), "tap1", true); err != nil {
		t.Fatal(err)
	}
	count := f.mutations
	if err := r.Element(context.Background(), "tap1", true); err != nil || f.mutations != count {
		t.Fatal("no-op mutated")
	}
	if err := r.Element(context.Background(), "mgmt0", true); err == nil || f.mutations != count {
		t.Fatal("management admitted")
	}
}

func TestElementCompensationUsesIndependentBoundedContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f := &memberNft{state: []string{"tap1"}, mode: "compensate", cancel: cancel}
	r, _ := New(f, "mgmt0")
	if err := r.Element(ctx, "tap1", false); err == nil {
		t.Fatal("expected compensated failure")
	}
	if ctx.Err() == nil {
		t.Fatal("fixture did not cancel transaction")
	}
	if f.recoveryCanceled || !slices.Contains(f.state, "tap1") {
		t.Fatal("compensation used canceled context or failed restore")
	}
}
