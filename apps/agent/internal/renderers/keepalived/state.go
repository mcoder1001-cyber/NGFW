package keepalived

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"ngfw/agent/internal/renderers"
	"ngfw/agent/internal/renderers/rfkit"
)

const (
	maxConfSize  = 1 << 20
	maxDumpSize  = 4 << 20
	maxStateSize = 4 << 10
	dumpWait     = 3 * time.Second
)

// rendered is what the agent needs to know about a keepalived.conf it wrote (parsed from the
// renderer's own output format, so it works after an agent restart).
type rendered struct {
	instances []renderedInstance
	secrets   []string
}

type renderedInstance struct {
	name, iface string
	vrid, prio  int
}

func parseRendered(conf []byte) rendered {
	var out rendered
	var cur *renderedInstance
	for _, raw := range strings.Split(string(conf), "\n") {
		f := strings.Fields(raw)
		if len(f) == 0 {
			continue
		}
		switch {
		case f[0] == "vrrp_instance" && len(f) >= 2:
			out.instances = append(out.instances, renderedInstance{name: f[1]})
			cur = &out.instances[len(out.instances)-1]
		case cur != nil && f[0] == "interface" && len(f) == 2:
			cur.iface = f[1]
		case cur != nil && f[0] == "virtual_router_id" && len(f) == 2:
			cur.vrid, _ = strconv.Atoi(f[1])
		case cur != nil && f[0] == "priority" && len(f) == 2:
			cur.prio, _ = strconv.Atoi(f[1])
		case f[0] == "auth_pass" && len(f) == 2:
			out.secrets = append(out.secrets, f[1])
		case f[0] == "vrrp_sync_group" || f[0] == "vrrp_script":
			cur = nil
		}
	}
	return out
}

// matches reports ErrNotConverged unless the dump holds exactly the rendered instances.
func (w rendered) matches(d []DumpInstance) error {
	if len(d) != len(w.instances) {
		names := make([]string, 0, len(d))
		for _, i := range d {
			names = append(names, i.Name)
		}
		return fmt.Errorf("%w: keepalived runs %d instances %v, rendered %d", rfkit.ErrNotConverged, len(d), names, len(w.instances))
	}
	for _, want := range w.instances {
		i := slices.IndexFunc(d, func(x DumpInstance) bool { return x.Name == want.name })
		if i < 0 {
			return fmt.Errorf("%w: instance %s missing in keepalived", rfkit.ErrNotConverged, want.name)
		}
		got := d[i]
		if got.Interface != want.iface || got.VRID != want.vrid || got.BasePriority != want.prio {
			return fmt.Errorf("%w: instance %s runs %s/vrid %d/priority %d, rendered %s/%d/%d", rfkit.ErrNotConverged,
				want.name, got.Interface, got.VRID, got.BasePriority, want.iface, want.vrid, want.prio)
		}
	}
	return nil
}

// DumpInstance is one vrrp_instance from keepalived's JSON dump. Only these fields are
// copied: the dump also carries auth_data (the PASS key in plaintext), which never leaves.
type DumpInstance struct {
	Name              string   `json:"name"`
	Interface         string   `json:"interface"`
	VRID              int      `json:"vrid"`
	State             string   `json:"state"`
	BasePriority      int      `json:"basePriority"`
	EffectivePriority int      `json:"effectivePriority"`
	VIPsSet           bool     `json:"vipsSet"`
	VIPs              []string `json:"vips"`
	Version           int      `json:"version"`
	LastTransition    float64  `json:"lastTransition"`
	AdvertSent        int64    `json:"advertSent"`
	AdvertRcvd        int64    `json:"advertRcvd"`
	BecomeMaster      int64    `json:"becomeMaster"`
	ReleaseMaster     int64    `json:"releaseMaster"`
	AuthFailure       int64    `json:"authFailure"`
}

// keepalived's VRRP state numbers (vrrp.h: VRRP_STATE_INIT/BACK/MAST/FAULT).
var dumpStates = map[int]string{0: "INIT", 1: "BACKUP", 2: "MASTER", 3: "FAULT"}

type rawDump []struct {
	Data struct {
		IName             string   `json:"iname"`
		IfpIfname         string   `json:"ifp_ifname"`
		VRID              int      `json:"vrid"`
		BasePriority      int      `json:"base_priority"`
		EffectivePriority int      `json:"effective_priority"`
		VIPSet            bool     `json:"vipset"`
		State             int      `json:"state"`
		Version           int      `json:"version"`
		LastTransition    float64  `json:"last_transition"`
		VIPs              []string `json:"vips"`
	} `json:"data"`
	Stats struct {
		AdvertRcvd    int64 `json:"advert_rcvd"`
		AdvertSent    int64 `json:"advert_sent"`
		BecomeMaster  int64 `json:"become_master"`
		ReleaseMaster int64 `json:"release_master"`
		AuthFailure   int64 `json:"auth_failure"`
	} `json:"stats"`
}

// ParseDump decodes keepalived.json into the whitelisted fields.
func ParseDump(b []byte) ([]DumpInstance, error) {
	var raw rawDump
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, fmt.Errorf("keepalived.json: %w", err)
	}
	out := make([]DumpInstance, 0, len(raw))
	for _, r := range raw {
		vips := make([]string, 0, len(r.Data.VIPs))
		for _, v := range r.Data.VIPs {
			vips = append(vips, strings.Fields(v + " ")[0]) // "10.8.240.1/24 dev w8-a scope global set" → prefix
		}
		st, ok := dumpStates[r.Data.State]
		if !ok {
			st = strconv.Itoa(r.Data.State)
		}
		out = append(out, DumpInstance{
			Name: r.Data.IName, Interface: r.Data.IfpIfname, VRID: r.Data.VRID, State: st,
			BasePriority: r.Data.BasePriority, EffectivePriority: r.Data.EffectivePriority,
			VIPsSet: r.Data.VIPSet, VIPs: vips, Version: r.Data.Version, LastTransition: r.Data.LastTransition,
			AdvertSent: r.Stats.AdvertSent, AdvertRcvd: r.Stats.AdvertRcvd, BecomeMaster: r.Stats.BecomeMaster,
			ReleaseMaster: r.Stats.ReleaseMaster, AuthFailure: r.Stats.AuthFailure,
		})
	}
	slices.SortFunc(out, func(a, b DumpInstance) int { return strings.Compare(a.Name, b.Name) })
	return out, nil
}

// jsonSignal returns keepalived's SIGJSON number (a realtime signal whose number depends on
// the build), asked once from `keepalived --signum=JSON`.
func (r *Renderer) jsonSignal(ctx context.Context) (syscall.Signal, error) {
	if r.jsonSig > 0 {
		return syscall.Signal(r.jsonSig), nil
	}
	out, err := r.runner.Run(ctx, renderers.Command{Path: KeepalivedBin, Args: []string{"--signum=JSON"}, Timeout: 5 * time.Second})
	if err != nil {
		return 0, fmt.Errorf("keepalived --signum=JSON: %w (built without JSON support?)", err)
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(out.Stdout)))
	if err != nil || n < 1 || n > 64 {
		return 0, fmt.Errorf("keepalived --signum=JSON answered %q", strings.TrimSpace(string(out.Stdout)))
	}
	r.jsonSig = n
	return syscall.Signal(n), nil
}

// dump asks the running keepalived for its JSON dump (SIGJSON → $TMPDIR/keepalived.json),
// reads it (bounded), removes it (it holds auth_data in plaintext) and returns the
// whitelisted view.
func (r *Renderer) dump(ctx context.Context) ([]DumpInstance, error) {
	r.dumpMu.Lock()
	defer r.dumpMu.Unlock()
	sig, err := r.jsonSignal(ctx)
	if err != nil {
		return nil, err
	}
	file := filepath.Join(r.paths.DumpDir, "keepalived.json")
	_ = os.Remove(file)
	if err := r.ctl.Signal(ctx, sig); err != nil {
		return nil, err
	}
	// The wait is not cut short by the caller's deadline: keepalived writes the file (holding
	// auth_data) whenever it gets to it, and it must be read and removed, not left behind.
	var b []byte
	err = rfkit.Poll(context.WithoutCancel(ctx), dumpWait, 25*time.Millisecond, func(context.Context) error {
		var rerr error
		b, rerr = rfkit.ReadFileLimit(file, maxDumpSize)
		if rerr == nil && !json.Valid(b) {
			rerr = errors.New("incomplete")
		}
		return rerr
	})
	_ = os.Remove(file)
	if err != nil {
		return nil, fmt.Errorf("keepalived did not write %s: %w", file, err)
	}
	return ParseDump(b)
}

// StateRecord is one <instance>.state file written by vrx-keepalived-notify.
type StateRecord struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	State string `json:"state"`
	Time  string `json:"time"`
}

// ReadStateFile reads <dir>/<name>.state (bounded; absent = zero record, no error).
func ReadStateFile(dir, name string) (StateRecord, error) {
	b, err := rfkit.ReadFileLimit(filepath.Join(dir, name+".state"), maxStateSize)
	if errors.Is(err, os.ErrNotExist) {
		return StateRecord{}, nil
	}
	if err != nil {
		return StateRecord{}, err
	}
	var rec StateRecord
	if err := json.Unmarshal(b, &rec); err != nil || rec.Name != name {
		return StateRecord{}, fmt.Errorf("state file for %s is malformed", name)
	}
	return rec, nil
}

// InstanceState is the actual state of one rendered instance.
type InstanceState struct {
	Name string
	// State is the notify helper's last transition (MASTER/BACKUP/FAULT/STOP, "" = none yet)
	// and Since its time.
	State, Since string
	// Dump is keepalived's own view (nil when the dump was not available).
	Dump *DumpInstance
}

// State is keepalived's actual state.
type State struct {
	Instances []InstanceState
	// DumpError is the (redacted) reason the JSON dump was unavailable ("" = read).
	DumpError string
}

// State reads the live keepalived.conf for the instance list, the notify helper's state files
// and keepalived's JSON dump. An unavailable dump (daemon down) is reported in DumpError, not
// as an error.
func (r *Renderer) State(ctx context.Context) (*State, error) {
	if err := r.check(); err != nil {
		return nil, err
	}
	conf, err := rfkit.ReadFileLimit(r.paths.ConfFile, maxConfSize)
	if err != nil {
		return nil, fmt.Errorf("keepalived: read %s: %w", r.paths.ConfFile, err)
	}
	want := parseRendered(conf)
	r.red.Add(want.secrets...)
	st := &State{}
	var dump []DumpInstance
	if len(want.instances) > 0 {
		if dump, err = r.dump(ctx); err != nil {
			st.DumpError = r.red.Redact(err.Error())
		}
	}
	for _, in := range want.instances {
		rec, err := ReadStateFile(r.paths.StateDir, in.name)
		if err != nil {
			return nil, err
		}
		is := InstanceState{Name: in.name, State: rec.State, Since: rec.Time}
		if i := slices.IndexFunc(dump, func(d DumpInstance) bool { return d.Name == in.name }); i >= 0 {
			d := dump[i]
			is.Dump = &d
		}
		st.Instances = append(st.Instances, is)
	}
	return st, nil
}

// Retrieve implements renderers.Renderer: State as a *structpb.Struct.
func (r *Renderer) Retrieve(ctx context.Context) (proto.Message, error) {
	st, err := r.State(ctx)
	if err != nil {
		return nil, r.red.Error(err)
	}
	return st.Struct()
}

// Struct converts the state to a structpb.Struct (whitelisted fields only).
func (st *State) Struct() (*structpb.Struct, error) {
	list := make([]any, 0, len(st.Instances))
	for _, in := range st.Instances {
		m := map[string]any{"name": in.Name, "state": in.State, "since": in.Since}
		if d := in.Dump; d != nil {
			vips := make([]any, 0, len(d.VIPs))
			for _, v := range d.VIPs {
				vips = append(vips, v)
			}
			m["daemon"] = map[string]any{
				"state": d.State, "interface": d.Interface, "vrid": float64(d.VRID), "version": float64(d.Version),
				"basePriority": float64(d.BasePriority), "effectivePriority": float64(d.EffectivePriority),
				"vipsSet": d.VIPsSet, "vips": vips, "lastTransition": d.LastTransition,
				"advertSent": float64(d.AdvertSent), "advertRcvd": float64(d.AdvertRcvd),
				"becomeMaster": float64(d.BecomeMaster), "releaseMaster": float64(d.ReleaseMaster), "authFailure": float64(d.AuthFailure),
			}
		}
		list = append(list, m)
	}
	return structpb.NewStruct(map[string]any{"instances": list, "dumpError": st.DumpError})
}

// Poller returns the 1 Hz event source: the notify helper's state files (one key per
// rendered instance, value = MASTER/BACKUP/FAULT/STOP). It never signals keepalived.
func (r *Renderer) Poller() *rfkit.Poller {
	return &rfkit.Poller{
		Source: "keepalived",
		Redact: r.red.Redact,
		Snap: func(context.Context) (map[string]string, error) {
			conf, err := rfkit.ReadFileLimit(r.paths.ConfFile, maxConfSize)
			if err != nil {
				return nil, err
			}
			out := map[string]string{}
			for _, in := range parseRendered(conf).instances {
				rec, err := ReadStateFile(r.paths.StateDir, in.name)
				if err != nil {
					return nil, err
				}
				out[in.name] = rec.State
			}
			return out, nil
		},
	}
}
