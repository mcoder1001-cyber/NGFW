package autosdl

import (
	"context"
	"errors"
	"testing"

	"go.fd.io/govpp/api"

	autosdlapi "ngfw/agent/binapi/auto_sdl"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/dfkit/dfkittest"
	"ngfw/agent/internal/scheduler"
)

// sdlModel is VPP 26.06's auto_sdl state (plugins/auto_sdl): FEATURE_DISABLED without the session
// SDL backend; an enable while enabled keeps the old values (duplicate-add behaviour, D-076);
// disable flushes the automatic entries.
type sdlModel struct {
	sessionSDL           bool
	enabled              bool
	threshold, remove    uint32
	entries, flushes     int
	enables, noopEnables int
}

func newFake() (*dfkittest.FakeVPP, *sdlModel) {
	m := &sdlModel{sessionSDL: true}
	f := dfkittest.NewFake()
	f.On("auto_sdl_config", func(req api.Message) ([]api.Message, error) {
		r := req.(*autosdlapi.AutoSdlConfig)
		if !m.sessionSDL {
			return []api.Message{&autosdlapi.AutoSdlConfigReply{Retval: int32(api.FEATURE_DISABLED)}}, nil
		}
		switch {
		case r.Enable && m.enabled:
			m.noopEnables++ // session_sdl_register_callbacks fails: old values stay
		case r.Enable:
			m.enabled, m.threshold, m.remove = true, r.Threshold, r.RemoveTimeout
			m.enables++
		default:
			m.enabled = false
			m.entries = 0
			m.flushes++
		}
		return []api.Message{&autosdlapi.AutoSdlConfigReply{}}, nil
	})
	return f, m
}

func TestAutoSdl(t *testing.T) {
	ctx := context.Background()
	f, m := newFake()
	boot := dfkit.NewMemoryBootStore()
	d := New(f, boot)
	want := Config{Enable: true, Threshold: 10, RemoveTimeout: 600}
	if d.KeyOf(want.Proto()) != "auto-sdl.config/global" || !scheduler.ValidName(d.Name()) || d.Dependencies(want.Proto()) != nil {
		t.Fatal("key / name / dependencies")
	}
	// VPP enabled by someone before (old values): Create starts from disabled, so the values apply
	m.enabled, m.threshold, m.remove = true, 5, 300
	if _, err := d.Create(ctx, want.Proto()); err != nil {
		t.Fatal(err)
	}
	if !m.enabled || m.threshold != 10 || m.remove != 600 || m.noopEnables != 0 {
		t.Fatalf("model after Create = %+v", m)
	}
	// resync re-apply (D-063) and an agent restart on the persisted store: nothing sent (no flush)
	m.entries = 3
	calls := len(f.CallsNamed("auto_sdl_config"))
	if _, err := d.Create(ctx, want.Proto()); err != nil {
		t.Fatal(err)
	}
	if _, err := New(f, boot).Create(ctx, want.Proto()); err != nil {
		t.Fatal(err)
	}
	if got := len(f.CallsNamed("auto_sdl_config")); got != calls || m.entries != 3 {
		t.Fatalf("re-apply sent %d calls, entries %d (want 0 calls, entries kept)", got-calls, m.entries)
	}
	// VPP restart: applied once more
	f.RestartVPP()
	m.enabled, m.threshold, m.remove = false, 0, 0
	if _, err := d.Create(ctx, want.Proto()); err != nil || !m.enabled || m.threshold != 10 {
		t.Fatalf("after a VPP restart: %v %+v", err, m)
	}
	if _, err := d.Update(ctx, want.Proto(), want.Proto(), nil); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatalf("Update: %v", err)
	}
	if _, err := d.Retrieve(ctx); !errors.Is(err, dfkit.ErrRetrieveUnsupported) {
		t.Fatalf("Retrieve: %v", err)
	}
	// Delete: disables what we enabled on this VPP instance, and forgets the record
	if err := d.Delete(ctx, want.Proto(), nil); err != nil || m.enabled {
		t.Fatalf("Delete: %v %+v", err, m)
	}
	if _, ok := boot.Get(string(Key)); ok {
		t.Fatal("record kept")
	}
	// Delete without a record of this instance: nothing sent (VPP default is off; not ours)
	m.enabled = true
	if err := d.Delete(ctx, want.Proto(), nil); err != nil || !m.enabled {
		t.Fatalf("Delete of a foreign enable: %v %+v", err, m)
	}
}

func TestAutoSdlValidationAndErrors(t *testing.T) {
	ctx := context.Background()
	f, m := newFake()
	d := New(f, dfkit.NewMemoryBootStore())
	for _, bad := range []Config{{}, {Enable: true, RemoveTimeout: 1}, {Enable: true, Threshold: 1}} {
		if _, err := d.Create(ctx, bad.Proto()); !errors.Is(err, dfkit.ErrSpec) {
			t.Fatalf("%+v accepted: %v", bad, err)
		}
	}
	m.sessionSDL = false
	if _, err := d.Create(ctx, Config{Enable: true, Threshold: DefaultThreshold, RemoveTimeout: DefaultRemoveTimeout}.Proto()); !errors.Is(err, ErrSessionSDLDisabled) {
		t.Fatalf("session SDL off: %v", err)
	}
	defer func() {
		if recover() == nil {
			t.Fatal("New without a BootStore must panic")
		}
	}()
	New(f, nil)
}

func TestRegister(t *testing.T) {
	reg := scheduler.NewRegistry()
	f, _ := newFake()
	Register(reg, f, dfkit.NewMemoryBootStore())
	if _, ok := reg.Get(Name); !ok {
		t.Fatal("not registered")
	}
}
