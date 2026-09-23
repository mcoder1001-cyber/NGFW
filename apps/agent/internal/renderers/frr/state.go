package frr

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"ngfw/agent/internal/renderers"
)

// ShowCommand is a vtysh show command. Only constants may be used (never user input): the
// runner passes it as one argv element, and ShowJSON / RegisterStateReader additionally
// check it against showCommandRe (no newline — vtysh would split -c at line breaks).
type ShowCommand string

// Framework show commands (FRR 10.7).
const (
	ShowRunningConfig ShowCommand = "show running-config"
	ShowVersion       ShowCommand = "show version"
	ShowVRF           ShowCommand = "show vrf" // text only: FRR 10.7 has no `show vrf json`
	ShowIPRoute       ShowCommand = "show ip route json"
	ShowIPRouteAll    ShowCommand = "show ip route vrf all json"
	ShowIPv6Route     ShowCommand = "show ipv6 route json"
	ShowIPv6RouteAll  ShowCommand = "show ipv6 route vrf all json"
	ShowInterface     ShowCommand = "show interface json"
	ShowInterfaceAll  ShowCommand = "show interface vrf all json"
)

const showTimeout = 15 * time.Second

var showCommandRe = regexp.MustCompile(`^show( [A-Za-z0-9_.:/-]+)+$`)

func (c ShowCommand) valid() bool { return showCommandRe.MatchString(string(c)) }

// Show runs one show command and returns its raw output.
func (r *Renderer) Show(ctx context.Context, cmd ShowCommand) ([]byte, error) {
	if err := r.check(); err != nil {
		return nil, err
	}
	if !cmd.valid() {
		return nil, fmt.Errorf("frr: show command %q must match %s", cmd, showCommandRe)
	}
	args := append(r.paths.vtyshArgs(r.paths.ConfDir), "-c", string(cmd))
	out, err := r.runner.Run(ctx, renderers.Command{Path: VtyshBin, Args: args, Timeout: showTimeout})
	if err != nil {
		return nil, fmt.Errorf("%w: vtysh -c %q: %s", ErrDaemon, cmd, toolMessage(out, err, ""))
	}
	return out.Stdout, nil
}

// ShowJSON runs a `show … json` command and returns the JSON document. Output that is not
// JSON (FRR prints "% Unknown command" with exit status 0 in some versions) is an error.
func (r *Renderer) ShowJSON(ctx context.Context, cmd ShowCommand) (json.RawMessage, error) {
	if !strings.HasSuffix(string(cmd), " json") {
		return nil, fmt.Errorf("frr: ShowJSON(%q): not a json command", cmd)
	}
	out, err := r.Show(ctx, cmd)
	if err != nil {
		return nil, err
	}
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		trimmed = "{}" // some commands print nothing when there is nothing to show
	}
	if !json.Valid([]byte(trimmed)) {
		return nil, fmt.Errorf("%w: vtysh -c %q returned non-JSON output: %.200s", ErrDaemon, cmd, trimmed)
	}
	return json.RawMessage(trimmed), nil
}

// StateReader adds one JSON show command to Retrieve under Key (P12: {"bgpSummary",
// "show bgp summary json"}).
type StateReader struct {
	Key     string
	Command ShowCommand
}

var (
	readersMu sync.Mutex
	readers   = map[string]StateReader{}
	keyRe     = regexp.MustCompile(`^[a-z][A-Za-z0-9]{0,63}$`)
)

// builtinKeys are the framework's Retrieve keys.
var builtinKeys = []string{"version", "runningConfig", "vrfs", "ipv4Routes", "ipv6Routes", "interfaces"}

// RegisterStateReader adds a reader to the global registry used by every Renderer created
// without WithStateReaders. It panics on an invalid or duplicate key or a command that is
// not a constant-shaped `show … json` (called from init(): programming errors).
func RegisterStateReader(sr StateReader) {
	if err := sr.check(); err != nil {
		panic(err.Error())
	}
	readersMu.Lock()
	defer readersMu.Unlock()
	if _, dup := readers[sr.Key]; dup || slices.Contains(builtinKeys, sr.Key) {
		panic(fmt.Sprintf("frr: state reader %q registered twice", sr.Key))
	}
	readers[sr.Key] = sr
}

func (sr StateReader) check() error {
	if !keyRe.MatchString(sr.Key) {
		return fmt.Errorf("frr: state reader key %q must match %s", sr.Key, keyRe)
	}
	if !sr.Command.valid() || !strings.HasSuffix(string(sr.Command), " json") {
		return fmt.Errorf("frr: state reader %q command %q must be a constant `show … json`", sr.Key, sr.Command)
	}
	return nil
}

// RegisteredStateReaders returns the registered readers sorted by key.
func RegisteredStateReaders() []StateReader {
	readersMu.Lock()
	out := make([]StateReader, 0, len(readers))
	for _, sr := range readers {
		out = append(out, sr)
	}
	readersMu.Unlock()
	slices.SortFunc(out, func(a, b StateReader) int { return strings.Compare(a.Key, b.Key) })
	return out
}

// VRFState is one line of `show vrf`.
type VRFState struct {
	Name   string `json:"name"`
	Active bool   `json:"active"`
	// ID is the kernel VRF id (-1 when inactive).
	ID int `json:"id"`
	// Table is the kernel table id (0 when unknown).
	Table int `json:"table"`
}

// State is FRR's actual state as structured data.
type State struct {
	// Version is the FRR version from `show version` ("10.7.1").
	Version string `json:"version"`
	// RunningConfig is `show running-config`, normalised (see NormalizeConfig) — for drift.
	RunningConfig []string `json:"runningConfig"`
	// VRFs is `show vrf`.
	VRFs []VRFState `json:"vrfs"`
	// IPv4Routes / IPv6Routes are `show ip[v6] route vrf all json` (VRF name → prefix → entries).
	IPv4Routes json.RawMessage `json:"ipv4Routes"`
	IPv6Routes json.RawMessage `json:"ipv6Routes"`
	// Interfaces is `show interface vrf all json`.
	Interfaces json.RawMessage `json:"interfaces"`
	// Extra holds the registered readers' JSON by key.
	Extra map[string]json.RawMessage `json:"extra,omitempty"`
}

// State reads FRR's actual state through the fixed show commands and the registered readers.
func (r *Renderer) State(ctx context.Context) (*State, error) {
	st := &State{Extra: map[string]json.RawMessage{}}
	ver, err := r.Show(ctx, ShowVersion)
	if err != nil {
		return nil, err
	}
	st.Version = parseVersion(string(ver))
	rc, err := r.Show(ctx, ShowRunningConfig)
	if err != nil {
		return nil, err
	}
	st.RunningConfig = NormalizeConfig(string(rc))
	vrfOut, err := r.Show(ctx, ShowVRF)
	if err != nil {
		return nil, err
	}
	st.VRFs = parseVRFs(string(vrfOut))
	for _, x := range []struct {
		dst *json.RawMessage
		cmd ShowCommand
	}{{&st.IPv4Routes, ShowIPRouteAll}, {&st.IPv6Routes, ShowIPv6RouteAll}, {&st.Interfaces, ShowInterfaceAll}} {
		if *x.dst, err = r.ShowJSON(ctx, x.cmd); err != nil {
			return nil, err
		}
	}
	rs := r.readers
	if rs == nil {
		rs = RegisteredStateReaders()
	}
	for _, sr := range rs {
		if err := sr.check(); err != nil {
			return nil, err
		}
		if st.Extra[sr.Key], err = r.ShowJSON(ctx, sr.Command); err != nil {
			return nil, err
		}
	}
	return st, nil
}

// Retrieve implements renderers.Renderer: State as a *structpb.Struct (keys as in State's
// JSON tags; registered readers under their own keys). There is no FRR state message in
// packages/proto yet (RF-1-questions.md Q2), so this is the D-055 structpb stand-in.
func (r *Renderer) Retrieve(ctx context.Context) (proto.Message, error) {
	st, err := r.State(ctx)
	if err != nil {
		return nil, err
	}
	return st.Struct()
}

// Struct converts the state to a structpb.Struct.
func (st *State) Struct() (*structpb.Struct, error) {
	m := map[string]any{
		"version":       st.Version,
		"runningConfig": toAnySlice(st.RunningConfig),
	}
	vrfs := make([]any, 0, len(st.VRFs))
	for _, v := range st.VRFs {
		vrfs = append(vrfs, map[string]any{"name": v.Name, "active": v.Active, "id": v.ID, "table": v.Table})
	}
	m["vrfs"] = vrfs
	for k, raw := range map[string]json.RawMessage{"ipv4Routes": st.IPv4Routes, "ipv6Routes": st.IPv6Routes, "interfaces": st.Interfaces} {
		v, err := decodeAny(raw)
		if err != nil {
			return nil, fmt.Errorf("frr: state %s: %w", k, err)
		}
		m[k] = v
	}
	for k, raw := range st.Extra {
		v, err := decodeAny(raw)
		if err != nil {
			return nil, fmt.Errorf("frr: state %s: %w", k, err)
		}
		m[k] = v
	}
	return structpb.NewStruct(m)
}

func toAnySlice(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

func decodeAny(raw json.RawMessage) (any, error) {
	if len(raw) == 0 {
		return map[string]any{}, nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, err
	}
	return v, nil
}

var versionLineRe = regexp.MustCompile(`FRRouting ([0-9][0-9A-Za-z.~+-]*)`)

func parseVersion(out string) string {
	if m := versionLineRe.FindStringSubmatch(out); m != nil {
		return m[1]
	}
	return ""
}

// parseVRFs parses `show vrf` (FRR 10.7):
//
//	vrf red id 5 table 1001
//	vrf blue inactive (configured)
func parseVRFs(out string) []VRFState {
	var vrfs []VRFState
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		if len(f) < 2 || f[0] != "vrf" {
			continue
		}
		v := VRFState{Name: f[1], ID: -1}
		for i := 2; i < len(f); i++ {
			switch f[i] {
			case "id":
				if i+1 < len(f) {
					v.Active = true
					_, _ = fmt.Sscanf(f[i+1], "%d", &v.ID)
				}
			case "table":
				if i+1 < len(f) {
					_, _ = fmt.Sscanf(f[i+1], "%d", &v.Table)
				}
			}
		}
		vrfs = append(vrfs, v)
	}
	slices.SortFunc(vrfs, func(a, b VRFState) int { return strings.Compare(a.Name, b.Name) })
	return vrfs
}

// NormalizeConfig turns a config text (rendered frr.conf or `show running-config`) into
// comparable lines: the vtysh banner ("Building configuration...", "Current configuration:"),
// comments ("!"), blank lines, the trailing "end" and the `frr version` line are dropped;
// indentation is kept (it carries the block structure).
func NormalizeConfig(text string) []string {
	var out []string
	for _, l := range strings.Split(strings.ReplaceAll(text, "\r", ""), "\n") {
		t := strings.TrimSpace(l)
		switch {
		case t == "", strings.HasPrefix(t, "!"), t == "end",
			t == "Building configuration...", t == "Current configuration:",
			strings.HasPrefix(t, "frr version "):
			continue
		}
		out = append(out, strings.TrimRight(l, " \t"))
	}
	return out
}

// RIBRoute is one entry of `show ip[v6] route … json` (the fields the framework needs).
type RIBRoute struct {
	Prefix    string       `json:"prefix"`
	Protocol  string       `json:"protocol"`
	VRFName   string       `json:"vrfName"`
	Distance  int          `json:"distance"`
	Tag       uint32       `json:"tag"`
	Selected  bool         `json:"selected"`
	Installed bool         `json:"installed"`
	Nexthops  []RIBNexthop `json:"nexthops"`
}

// RIBNexthop is one next hop of a RIBRoute.
type RIBNexthop struct {
	IP            string `json:"ip"`
	InterfaceName string `json:"interfaceName"`
	Active        bool   `json:"active"`
	FIB           bool   `json:"fib"`
	Unreachable   bool   `json:"unreachable"`
	Blackhole     bool   `json:"blackhole"`
}

// DecodeRIB decodes `show ip[v6] route json` (prefix → entries) or `… vrf all json`
// (vrf → prefix → entries) into a flat list sorted by (VRF, prefix, protocol).
func DecodeRIB(raw json.RawMessage) ([]RIBRoute, error) {
	var flat map[string][]RIBRoute
	if err := json.Unmarshal(raw, &flat); err == nil {
		return sortRIB(collect(flat)), nil
	}
	var byVRF map[string]map[string][]RIBRoute
	if err := json.Unmarshal(raw, &byVRF); err != nil {
		return nil, fmt.Errorf("frr: decode RIB: %w", err)
	}
	var out []RIBRoute
	for vrf, m := range byVRF {
		for _, r := range collect(m) {
			if r.VRFName == "" {
				r.VRFName = vrf
			}
			out = append(out, r)
		}
	}
	return sortRIB(out), nil
}

func collect(m map[string][]RIBRoute) []RIBRoute {
	var out []RIBRoute
	for p, entries := range m {
		for _, e := range entries {
			if e.Prefix == "" {
				e.Prefix = p
			}
			out = append(out, e)
		}
	}
	return out
}

func sortRIB(rs []RIBRoute) []RIBRoute {
	slices.SortFunc(rs, func(a, b RIBRoute) int {
		if c := strings.Compare(a.VRFName, b.VRFName); c != 0 {
			return c
		}
		if c := strings.Compare(a.Prefix, b.Prefix); c != 0 {
			return c
		}
		return strings.Compare(a.Protocol, b.Protocol)
	})
	return rs
}

// StaticRoutes returns the protocol "static" routes of both address families.
func (st *State) StaticRoutes() ([]RIBRoute, error) {
	var out []RIBRoute
	for _, raw := range []json.RawMessage{st.IPv4Routes, st.IPv6Routes} {
		if len(raw) == 0 {
			continue
		}
		rs, err := DecodeRIB(raw)
		if err != nil {
			return nil, err
		}
		for _, r := range rs {
			if r.Protocol == "static" {
				out = append(out, r)
			}
		}
	}
	return sortRIB(out), nil
}
