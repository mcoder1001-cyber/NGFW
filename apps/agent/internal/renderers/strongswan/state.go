package strongswan

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

	"github.com/strongswan/govici/vici"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"ngfw/agent/internal/renderers"
)

// State is charon's actual state as the renderer reads it through VICI. It never contains
// key material (VICI does not expose it) and passes through the redactor before it leaves.
type State struct {
	Daemon DaemonInfo `json:"daemon"`
	Stats  Stats      `json:"stats"`
	// Conns are the connections loaded over VICI (list-conns), sorted by name.
	Conns []ConnState `json:"conns"`
	// SAs are the IKE_SAs of those connections (list-sas per connection), sorted.
	SAs []IKESA `json:"sas"`
	// SharedSecrets are the unique ids of the loaded shared secrets (never their data).
	SharedSecrets []string `json:"sharedSecrets"`
	// Pools are the loaded pool names.
	Pools []string `json:"pools"`
	// UnlistedSAs counts IKE_SAs charon reports in stats but that belong to no owned connection
	// (another owner's, or SAs of a connection unloaded by someone else); 0 when in sync.
	UnlistedSAs int64 `json:"unlistedSas"`
	// Truncated names connections with more than MaxSAsPerConn IKE_SAs (only the first are
	// listed; review L3).
	Truncated []string `json:"truncated,omitempty"`
	// StaleSAs counts live CHILD_SAs whose selectors or mode contradict the loaded config and
	// IKE_SAs with a mismatching identity/version (Apply terminates them; review H1).
	StaleSAs int `json:"staleSas"`
	// DaemonStartedAt is charon's start time (VICI stats uptime.since); AckedStartedAt the one
	// last acknowledged with AckRestart (persisted in Paths.BootRecord). Restarted means charon
	// restarted since: kernel/VPP SAs of the previous charon may be orphaned and must be
	// reconciled (P11/DF-5), then AckRestart (review M3).
	DaemonStartedAt string `json:"daemonStartedAt"`
	AckedStartedAt  string `json:"ackedStartedAt,omitempty"`
	Restarted       bool   `json:"restarted"`
}

// DaemonInfo is VICI version().
type DaemonInfo struct {
	Daemon  string `json:"daemon"`
	Version string `json:"version"`
	Sysname string `json:"sysname"`
	Release string `json:"release"`
	Machine string `json:"machine"`
}

// Stats is the part of VICI stats() the agent reports.
type Stats struct {
	UptimeSince   string   `json:"uptimeSince"`
	IKESAsTotal   int64    `json:"ikeSasTotal"`
	IKESAsHalf    int64    `json:"ikeSasHalfOpen"`
	WorkersTotal  int64    `json:"workersTotal"`
	WorkersIdle   int64    `json:"workersIdle"`
	LoadedPlugins []string `json:"loadedPlugins"`
}

// ConnState is one loaded connection (list-conn).
type ConnState struct {
	Name        string       `json:"name"`
	Tunnel      string       `json:"tunnel"`
	Version     string       `json:"version"`
	LocalAddrs  []string     `json:"localAddrs"`
	RemoteAddrs []string     `json:"remoteAddrs"`
	LocalID     string       `json:"localId,omitempty"`
	RemoteID    string       `json:"remoteId,omitempty"`
	LocalAuth   string       `json:"localAuth,omitempty"`
	RemoteAuth  string       `json:"remoteAuth,omitempty"`
	RekeyTime   int64        `json:"rekeyTime"`
	ReauthTime  int64        `json:"reauthTime"`
	Children    []ChildState `json:"children"`
}

// ChildState is one loaded CHILD_SA config.
type ChildState struct {
	Name      string   `json:"name"`
	Mode      string   `json:"mode"`
	RekeyTime int64    `json:"rekeyTime"`
	LocalTS   []string `json:"localTs"`
	RemoteTS  []string `json:"remoteTs"`
}

// IKESA is one IKE_SA (list-sa).
type IKESA struct {
	Name           string    `json:"name"`
	Tunnel         string    `json:"tunnel"`
	UniqueID       string    `json:"uniqueId"`
	Version        string    `json:"version"`
	State          string    `json:"state"`
	LocalHost      string    `json:"localHost"`
	LocalPort      string    `json:"localPort"`
	LocalID        string    `json:"localId"`
	RemoteHost     string    `json:"remoteHost"`
	RemotePort     string    `json:"remotePort"`
	RemoteID       string    `json:"remoteId"`
	Initiator      bool      `json:"initiator"`
	NATAny         bool      `json:"natAny"`
	EncrAlg        string    `json:"encrAlg,omitempty"`
	EncrKeysize    string    `json:"encrKeysize,omitempty"`
	IntegAlg       string    `json:"integAlg,omitempty"`
	PRFAlg         string    `json:"prfAlg,omitempty"`
	DHGroup        string    `json:"dhGroup,omitempty"`
	EstablishedSec int64     `json:"establishedSec"`
	RekeySec       int64     `json:"rekeySec"`
	ReauthSec      int64     `json:"reauthSec"`
	Children       []ChildSA `json:"children"`
}

// ChildSA is one CHILD_SA of an IKE_SA.
type ChildSA struct {
	Name        string   `json:"name"`
	UniqueID    string   `json:"uniqueId"`
	ReqID       string   `json:"reqId"`
	State       string   `json:"state"`
	Mode        string   `json:"mode"`
	Protocol    string   `json:"protocol"`
	Encap       bool     `json:"encap"`
	SPIIn       string   `json:"spiIn"`
	SPIOut      string   `json:"spiOut"`
	EncrAlg     string   `json:"encrAlg,omitempty"`
	EncrKeysize string   `json:"encrKeysize,omitempty"`
	IntegAlg    string   `json:"integAlg,omitempty"`
	DHGroup     string   `json:"dhGroup,omitempty"`
	ESN         bool     `json:"esn"`
	BytesIn     int64    `json:"bytesIn"`
	PacketsIn   int64    `json:"packetsIn"`
	BytesOut    int64    `json:"bytesOut"`
	PacketsOut  int64    `json:"packetsOut"`
	RekeySec    int64    `json:"rekeySec"`
	LifeSec     int64    `json:"lifeSec"`
	InstallSec  int64    `json:"installSec"`
	LocalTS     []string `json:"localTs"`
	RemoteTS    []string `json:"remoteTs"`
	IfIDIn      string   `json:"ifIdIn,omitempty"`
	IfIDOut     string   `json:"ifIdOut,omitempty"`
}

// State reads charon's state. Listing is per connection (bounded streams, see vici.go).
func (r *Renderer) State(ctx context.Context) (*State, error) {
	if err := r.check(); err != nil {
		return nil, err
	}
	s, err := r.open(ctx)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	st := &State{Conns: []ConnState{}, SAs: []IKESA{}}
	v, err := s.call(ctx, "version", nil)
	if err != nil {
		return nil, err
	}
	st.Daemon = DaemonInfo{Daemon: str(v, "daemon"), Version: str(v, "version"), Sysname: str(v, "sysname"), Release: str(v, "release"), Machine: str(v, "machine")}
	stats, err := s.call(ctx, "stats", nil)
	if err != nil {
		return nil, err
	}
	st.Stats = Stats{
		UptimeSince: str(sub(stats, "uptime"), "since"),
		IKESAsTotal: atoi64(str(sub(stats, "ikesas"), "total")), IKESAsHalf: atoi64(str(sub(stats, "ikesas"), "half-open")),
		WorkersTotal: atoi64(str(sub(stats, "workers"), "total")), WorkersIdle: atoi64(str(sub(stats, "workers"), "idle")),
		LoadedPlugins: strs(stats, "plugins"),
	}
	st.DaemonStartedAt = st.Stats.UptimeSince
	st.AckedStartedAt, st.Restarted = r.restartState(st.DaemonStartedAt)
	all, err := s.getConns(ctx)
	if err != nil {
		return nil, err
	}
	var conns []string
	for _, c := range all {
		if r.owns(c) {
			conns = append(conns, c)
		}
	}
	slices.Sort(conns)
	for _, name := range conns {
		m, err := s.listConn(ctx, name)
		if err != nil {
			return nil, err
		}
		var cfg connCfg
		if m != nil {
			st.Conns = append(st.Conns, connState(name, m))
			cfg = cfgFromVICI(m)
		}
		sas, truncated, err := s.listSAs(ctx, name)
		if err != nil {
			return nil, err
		}
		if truncated {
			st.Truncated = append(st.Truncated, name)
		}
		for _, sa := range sas {
			st.SAs = append(st.SAs, ikeSA(name, sa))
			if m == nil || slices.Contains(deadIKEStates, str(sa, "state")) {
				continue
			}
			if cfg.mismatchIKE(sa) != "" {
				st.StaleSAs++
				continue
			}
			for _, ch := range childSections(sa) {
				if !slices.Contains(deadChildStates, str(ch, "state")) && cfg.mismatchChild(ch) != "" {
					st.StaleSAs++
				}
			}
		}
	}
	slices.SortFunc(st.SAs, func(a, b IKESA) int {
		if a.Name != b.Name {
			return compareStrings(a.Name, b.Name)
		}
		return int(atoi64(a.UniqueID) - atoi64(b.UniqueID))
	})
	if st.SharedSecrets, err = s.getShared(ctx); err != nil {
		return nil, err
	}
	st.SharedSecrets = slices.DeleteFunc(st.SharedSecrets, func(id string) bool { return !r.ownsShared(id) })
	slices.Sort(st.SharedSecrets)
	if st.Pools, err = s.getPools(ctx); err != nil {
		return nil, err
	}
	st.Pools = slices.DeleteFunc(st.Pools, func(p string) bool { return !r.owns(p) })
	slices.Sort(st.Pools)
	if d := st.Stats.IKESAsTotal - int64(len(st.SAs)); d > 0 {
		st.UnlistedSAs = d
	}
	return st, nil
}

// cfgFromVICI builds the SA check config from a list-conn entry (the loaded config).
func cfgFromVICI(m *vici.Message) connCfg {
	c := connCfg{children: map[string]childCfg{}, changedChild: map[string]bool{}}
	switch str(m, "version") {
	case "IKEv1":
		c.version = "1"
	case "IKEv2":
		c.version = "2"
	}
	for _, k := range m.Keys() {
		if a := sub(m, k); a != nil && strings.HasPrefix(k, "local") && c.localID == "" {
			c.localID = str(a, "id")
		} else if a != nil && strings.HasPrefix(k, "remote") && c.remoteID == "" {
			c.remoteID = str(a, "id")
		}
	}
	if c.remoteID == "%any" {
		c.remoteID = ""
	}
	if ch := sub(m, "children"); ch != nil {
		for _, n := range ch.Keys() {
			cm := sub(ch, n)
			cc := childCfg{mode: strings.ToLower(str(cm, "mode"))}
			if l := strs(cm, "local-ts"); !slices.Contains(l, "dynamic") {
				cc.local = prefixes(strings.Join(l, ","))
			}
			if l := strs(cm, "remote-ts"); !slices.Contains(l, "dynamic") {
				cc.remote = prefixes(strings.Join(l, ","))
			}
			c.children[n] = cc
		}
	}
	return c
}

// restartState compares charon's start time with the acknowledged one. Without a record
// (first observation) the current start time becomes the baseline: nothing is known to be
// orphaned yet.
func (r *Renderer) restartState(since string) (acked string, restarted bool) {
	if r.paths.BootRecord == "" || since == "" {
		return "", false
	}
	b, err := readBoundedSmall(r.paths.BootRecord)
	if err != nil {
		_ = r.writeBootRecord(since)
		return since, false
	}
	acked = strings.TrimSpace(string(b))
	return acked, acked != since
}

// AckRestart records charon's current start time as reconciled: call it after the orphaned
// kernel/VPP state of a previous charon has been cleaned up (P11/DF-5). Until then State
// reports Restarted and Watch reports it on every (re-)subscription.
func (r *Renderer) AckRestart(ctx context.Context) error {
	if err := r.check(); err != nil {
		return err
	}
	if r.paths.BootRecord == "" {
		return errors.New("strongswan: no Paths.BootRecord configured")
	}
	s, err := r.open(ctx)
	if err != nil {
		return err
	}
	defer s.Close()
	stats, err := s.call(ctx, "stats", nil)
	if err != nil {
		return err
	}
	return r.writeBootRecord(str(sub(stats, "uptime"), "since"))
}

func (r *Renderer) writeBootRecord(since string) error {
	if since == "" || strings.ContainsAny(since, "\n\r") {
		return fmt.Errorf("strongswan: bad charon start time %q", since)
	}
	if err := os.MkdirAll(filepath.Dir(r.paths.BootRecord), 0o700); err != nil {
		return err
	}
	return renderers.WriteFileAtomic(r.paths.BootRecord, renderers.File{Mode: 0o600, Content: []byte(since + "\n")})
}

func readBoundedSmall(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > 256 {
		return nil, fmt.Errorf("strongswan: %s is not a small regular file", path)
	}
	return os.ReadFile(path) //nolint:gosec // Paths.BootRecord
}

func compareStrings(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func connState(name string, m *vici.Message) ConnState {
	c := ConnState{
		Name: name, Tunnel: TunnelName(name), Version: str(m, "version"),
		LocalAddrs: orEmpty(strs(m, "local_addrs")), RemoteAddrs: orEmpty(strs(m, "remote_addrs")),
		RekeyTime: atoi64(str(m, "rekey_time")), ReauthTime: atoi64(str(m, "reauth_time")), Children: []ChildState{},
	}
	for _, k := range m.Keys() {
		a := sub(m, k)
		switch {
		case a == nil:
		case len(k) >= 5 && k[:5] == "local" && c.LocalAuth == "":
			c.LocalAuth, c.LocalID = str(a, "class"), str(a, "id")
		case len(k) >= 6 && k[:6] == "remote" && c.RemoteAuth == "":
			c.RemoteAuth, c.RemoteID = str(a, "class"), str(a, "id")
		}
	}
	if ch := sub(m, "children"); ch != nil {
		for _, n := range ch.Keys() {
			cm := sub(ch, n)
			c.Children = append(c.Children, ChildState{
				Name: n, Mode: str(cm, "mode"), RekeyTime: atoi64(str(cm, "rekey_time")),
				LocalTS: orEmpty(strs(cm, "local-ts")), RemoteTS: orEmpty(strs(cm, "remote-ts")),
			})
		}
	}
	return c
}

func ikeSA(name string, m *vici.Message) IKESA {
	sa := IKESA{
		Name: name, Tunnel: TunnelName(name), UniqueID: str(m, "uniqueid"), Version: str(m, "version"), State: str(m, "state"),
		LocalHost: str(m, "local-host"), LocalPort: str(m, "local-port"), LocalID: str(m, "local-id"),
		RemoteHost: str(m, "remote-host"), RemotePort: str(m, "remote-port"), RemoteID: str(m, "remote-id"),
		Initiator: str(m, "initiator") == "yes", NATAny: str(m, "nat-any") == "yes",
		EncrAlg: str(m, "encr-alg"), EncrKeysize: str(m, "encr-keysize"), IntegAlg: str(m, "integ-alg"),
		PRFAlg: str(m, "prf-alg"), DHGroup: str(m, "dh-group"),
		EstablishedSec: atoi64(str(m, "established")), RekeySec: atoi64(str(m, "rekey-time")), ReauthSec: atoi64(str(m, "reauth-time")),
		Children: []ChildSA{},
	}
	if cs := sub(m, "child-sas"); cs != nil {
		for _, k := range cs.Keys() {
			sa.Children = append(sa.Children, childSA(sub(cs, k)))
		}
	}
	slices.SortFunc(sa.Children, func(a, b ChildSA) int { return int(atoi64(a.UniqueID) - atoi64(b.UniqueID)) })
	return sa
}

func childSA(m *vici.Message) ChildSA {
	return ChildSA{
		Name: str(m, "name"), UniqueID: str(m, "uniqueid"), ReqID: str(m, "reqid"), State: str(m, "state"),
		Mode: str(m, "mode"), Protocol: str(m, "protocol"), Encap: str(m, "encap") == "yes",
		SPIIn: str(m, "spi-in"), SPIOut: str(m, "spi-out"), EncrAlg: str(m, "encr-alg"), EncrKeysize: str(m, "encr-keysize"),
		IntegAlg: str(m, "integ-alg"), DHGroup: str(m, "dh-group"), ESN: str(m, "esn") == "1",
		BytesIn: atoi64(str(m, "bytes-in")), PacketsIn: atoi64(str(m, "packets-in")),
		BytesOut: atoi64(str(m, "bytes-out")), PacketsOut: atoi64(str(m, "packets-out")),
		RekeySec: atoi64(str(m, "rekey-time")), LifeSec: atoi64(str(m, "life-time")), InstallSec: atoi64(str(m, "install-time")),
		LocalTS: orEmpty(strs(m, "local-ts")), RemoteTS: orEmpty(strs(m, "remote-ts")),
		IfIDIn: str(m, "if-id-in"), IfIDOut: str(m, "if-id-out"),
	}
}

func orEmpty(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// Retrieve implements renderers.Renderer: State as a *structpb.Struct (keys as in State's
// JSON tags; D-055 stand-in until P11 adds an IPsec state message). VICI never returns key
// material, so no text is rewritten (RF-2 review L2).
func (r *Renderer) Retrieve(ctx context.Context) (proto.Message, error) {
	st, err := r.State(ctx)
	if err != nil {
		return nil, err
	}
	return r.stateStruct(st)
}

func (r *Renderer) stateStruct(st *State) (*structpb.Struct, error) {
	raw, err := json.Marshal(st)
	if err != nil {
		return nil, fmt.Errorf("strongswan: encode state: %w", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("strongswan: redacted state is not JSON: %w", err)
	}
	return structpb.NewStruct(m)
}

// Initiate starts the CHILD_SA child of connection conn (the Actions API and tests; start
// actions do this on load). It waits up to timeoutMs for the result (-1: do not wait).
func (r *Renderer) Initiate(ctx context.Context, conn, child string, timeoutMs int) error {
	return r.control(ctx, "initiate", msg("child", child, "ike", conn, "timeout", strconv.Itoa(timeoutMs), "init-limits", "no"))
}

// Terminate closes the IKE_SAs of connection conn.
func (r *Renderer) Terminate(ctx context.Context, conn string, timeoutMs int) error {
	return r.control(ctx, "terminate", msg("ike", conn, "timeout", strconv.Itoa(timeoutMs)))
}

func (r *Renderer) control(ctx context.Context, cmd string, in *vici.Message) error {
	if _, err := SectionName(str(in, "ike")); err != nil {
		return fmt.Errorf("%w: %s: %v", ErrInput, cmd, err)
	}
	s, err := r.open(ctx)
	if err != nil {
		return err
	}
	defer s.Close()
	// initiate/terminate stream control-log events while they run; the log is not needed.
	return s.stream(ctx, cmd, "control-log", in, 10000, func(*vici.Message) error { return nil })
}
