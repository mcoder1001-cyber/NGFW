package snmpd

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/gosnmp/gosnmp"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"ngfw/agent/internal/renderers/rfkit"
)

// SNMPv2-MIB system group objects read by Retrieve.
const (
	OIDSysDescr    = ".1.3.6.1.2.1.1.1.0"
	OIDSysUpTime   = ".1.3.6.1.2.1.1.3.0"
	OIDSysContact  = ".1.3.6.1.2.1.1.4.0"
	OIDSysName     = ".1.3.6.1.2.1.1.5.0"
	OIDSysLocation = ".1.3.6.1.2.1.1.6.0"
)

var systemOIDs = []string{OIDSysName, OIDSysDescr, OIDSysUpTime, OIDSysLocation, OIDSysContact}

// Target is one local SNMP endpoint plus the credential the agent uses to read it. It is
// taken from the rendered file itself (never passed on any argv).
type Target struct {
	Addr      netip.Addr
	Port      uint16
	Community string // v2c
	User      *User  // v3 (takes precedence)
}

// Describe names the credential without its secret ("v3 user u1", "v2c community").
func (t Target) Describe() string {
	if t.User != nil {
		return "v3 user " + t.User.Name
	}
	return "v2c community"
}

// Querier reads OIDs from an SNMP agent.
type Querier interface {
	// Get returns OID → value (strings; TimeTicks as a decimal count of 1/100 s).
	Get(ctx context.Context, t Target, oids []string) (map[string]string, error)
}

// GoSNMP is the production Querier (github.com/gosnmp/gosnmp, BSD-2): an in-process SNMP
// client, so communities and passphrases never appear in a process argv (snmpget would need
// them on its command line).
type GoSNMP struct{}

var (
	gsAuth = map[string]gosnmp.SnmpV3AuthProtocol{"MD5": gosnmp.MD5, "SHA": gosnmp.SHA, "SHA-256": gosnmp.SHA256, "SHA-512": gosnmp.SHA512}
	gsPriv = map[string]gosnmp.SnmpV3PrivProtocol{"AES": gosnmp.AES, "DES": gosnmp.DES}
)

// Get implements Querier.
func (GoSNMP) Get(ctx context.Context, t Target, oids []string) (map[string]string, error) {
	g := &gosnmp.GoSNMP{
		Context: ctx, Target: t.Addr.String(), Port: t.Port, Transport: "udp",
		Timeout: 700 * time.Millisecond, Retries: 1, MaxOids: len(oids),
		Version: gosnmp.Version2c, Community: t.Community,
	}
	if u := t.User; u != nil {
		sp := &gosnmp.UsmSecurityParameters{UserName: u.Name}
		g.Version, g.SecurityModel, g.MsgFlags = gosnmp.Version3, gosnmp.UserSecurityModel, gosnmp.NoAuthNoPriv
		if u.Level != "noauth" {
			g.MsgFlags = gosnmp.AuthNoPriv
			sp.AuthenticationProtocol, sp.AuthenticationPassphrase = gsAuth[u.AuthProto], u.Auth
		}
		if u.Level == "priv" {
			g.MsgFlags = gosnmp.AuthPriv
			sp.PrivacyProtocol, sp.PrivacyPassphrase = gsPriv[u.PrivProto], u.Priv
		}
		g.SecurityParameters = sp
	}
	if err := g.Connect(); err != nil {
		return nil, err
	}
	defer func() { _ = g.Conn.Close() }()
	pkt, err := g.Get(oids)
	if err != nil {
		return nil, err
	}
	if pkt.Error != gosnmp.NoError {
		return nil, fmt.Errorf("agent answered %v", pkt.Error)
	}
	out := make(map[string]string, len(pkt.Variables))
	for _, v := range pkt.Variables {
		switch v.Type {
		case gosnmp.OctetString:
			b, _ := v.Value.([]byte)
			out[v.Name] = string(b)
		case gosnmp.TimeTicks:
			out[v.Name] = strconv.FormatUint(uint64(gosnmp.ToBigInt(v.Value).Uint64()), 10)
		case gosnmp.NoSuchObject, gosnmp.NoSuchInstance, gosnmp.Null:
		default:
			out[v.Name] = fmt.Sprint(v.Value)
		}
	}
	return out, nil
}

// probe is what the agent needs to know about a rendered snmpd.conf: expected system values,
// a local target and every secret in it. parseRendered reads the renderer's own output format
// (so Retrieve works after an agent restart without the desired state).
type probe struct {
	enabled                     bool
	sysName, sysLoc, sysContact string
	target                      *Target
	secrets                     []string
}

func (p probe) queryable() bool { return p.enabled && p.target != nil }

// matches returns an ErrNotConverged error when st does not show the rendered values.
func (p probe) matches(st *State) error {
	for _, c := range []struct{ name, want, got string }{
		{"sysName", p.sysName, st.SysName}, {"sysLocation", p.sysLoc, st.SysLocation}, {"sysContact", p.sysContact, st.SysContact},
	} {
		if c.want != "" && c.want != c.got {
			return fmt.Errorf("%w: snmpd reports %s %q, rendered %q", rfkit.ErrNotConverged, c.name, c.got, c.want)
		}
	}
	return nil
}

func parseRendered(conf []byte) probe {
	p := probe{}
	users := map[string]*User{}
	var listen []string
	var community *Target
	for _, raw := range strings.Split(string(conf), "\n") {
		word, rest, _ := strings.Cut(raw, " ")
		f := strings.Fields(rest)
		switch word {
		case "agentaddress":
			if !strings.HasPrefix(rest, "unix:") {
				p.enabled = true
				listen = strings.Split(rest, ",")
			}
		case "sysName":
			p.sysName = rest
		case "sysLocation":
			p.sysLoc = rest
		case "sysContact":
			p.sysContact = rest
		case "rocommunity", "rwcommunity":
			if len(f) >= 2 {
				p.secrets = append(p.secrets, f[0])
				if community == nil && (f[1] == "default" || prefixHasLoopback(f[1])) {
					community = &Target{Community: f[0]}
				}
			}
		case "rocommunity6", "rwcommunity6":
			if len(f) >= 1 {
				p.secrets = append(p.secrets, f[0])
			}
		case "trap2sink", "informsink":
			if len(f) >= 2 {
				p.secrets = append(p.secrets, f[1])
			}
		case "createUser":
			if len(f) >= 1 {
				u := &User{Name: f[0], Level: "noauth"}
				if len(f) >= 3 {
					u.Level, u.AuthProto, u.Auth = "auth", f[1], strings.Trim(f[2], `"`)
					p.secrets = append(p.secrets, u.Auth)
				}
				if len(f) >= 5 {
					u.Level, u.PrivProto, u.Priv = "priv", f[3], strings.Trim(f[4], `"`)
					p.secrets = append(p.secrets, u.Priv)
				}
				users[u.Name] = u
			}
		case "rouser", "rwuser":
			if len(f) >= 1 && p.target == nil {
				if u, ok := users[f[0]]; ok && u.Level != "noauth" {
					p.target = &Target{User: u}
				}
			}
		}
	}
	if p.target == nil {
		p.target = community
	}
	if p.target != nil {
		addr, port, ok := localEndpoint(listen)
		if !ok {
			p.target = nil
		} else {
			p.target.Addr, p.target.Port = addr, port
		}
	}
	return p
}

func prefixHasLoopback(s string) bool {
	pf, err := netip.ParsePrefix(s)
	return err == nil && pf.Contains(netip.MustParseAddr("127.0.0.1"))
}

// localEndpoint picks the address the agent queries: 127.0.0.1 (explicit or through the
// wildcard) first, then the first IPv4 listen address.
func localEndpoint(listen []string) (netip.Addr, uint16, bool) {
	var fallback netip.AddrPort
	for _, t := range listen {
		if !strings.HasPrefix(t, "udp:") {
			continue
		}
		ap, err := netip.ParseAddrPort(strings.TrimPrefix(t, "udp:"))
		if err != nil {
			continue
		}
		switch {
		case ap.Addr().IsLoopback():
			return ap.Addr(), ap.Port(), true
		case ap.Addr().IsUnspecified():
			return netip.MustParseAddr("127.0.0.1"), ap.Port(), true
		case !fallback.IsValid():
			fallback = ap
		}
	}
	return fallback.Addr(), fallback.Port(), fallback.IsValid()
}

// State is snmpd's actual state as read over SNMP.
type State struct {
	// Configured is false when the live file carries no usable local endpoint/credential
	// (disabled agent, v3-only noAuth, remote-only listen): nothing is queried.
	Configured bool
	// Reachable reports whether the agent answered.
	Reachable bool
	// Endpoint and Credential describe what was queried ("127.0.0.1:3861", "v3 user u1").
	Endpoint, Credential string
	SysName, SysDescr    string
	SysLocation          string
	SysContact           string
	// SysUpTime is in hundredths of a second.
	SysUpTime uint64
	// Error is the (redacted) reason Reachable is false.
	Error string
}

// query asks the agent for the system group.
func (r *Renderer) query(ctx context.Context, p probe) (*State, error) {
	t := *p.target
	vals, err := r.querier.Get(ctx, t, systemOIDs)
	if err != nil {
		return nil, err
	}
	up, _ := strconv.ParseUint(vals[OIDSysUpTime], 10, 64)
	return &State{
		Configured: true, Reachable: true,
		Endpoint:   net.JoinHostPort(t.Addr.String(), strconv.Itoa(int(t.Port))),
		Credential: t.Describe(),
		SysName:    vals[OIDSysName], SysDescr: vals[OIDSysDescr], SysUpTime: up,
		SysLocation: vals[OIDSysLocation], SysContact: vals[OIDSysContact],
	}, nil
}

// State reads the live snmpd.conf (bounded) for a local endpoint and credential and queries
// the system group. An unreachable agent is a State with Reachable=false and the redacted
// reason, not an error; errors are I/O problems with the file.
func (r *Renderer) State(ctx context.Context) (*State, error) {
	if err := r.check(); err != nil {
		return nil, err
	}
	conf, err := rfkit.ReadFileLimit(r.paths.ConfFile, maxConfSize)
	if err != nil {
		return nil, fmt.Errorf("snmpd: read %s: %w", r.paths.ConfFile, err)
	}
	p := parseRendered(conf)
	r.red.Add(p.secrets...)
	if !p.queryable() {
		return &State{}, nil
	}
	st, err := r.query(ctx, p)
	if err != nil {
		return &State{
			Configured: true,
			Endpoint:   net.JoinHostPort(p.target.Addr.String(), strconv.Itoa(int(p.target.Port))),
			Credential: p.target.Describe(),
			Error:      r.red.Redact(err.Error()),
		}, nil
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

// Struct converts the state to a structpb.Struct (no secrets: the credential is described by
// user name / kind only).
func (st *State) Struct() (*structpb.Struct, error) {
	return structpb.NewStruct(map[string]any{
		"configured": st.Configured, "reachable": st.Reachable,
		"endpoint": st.Endpoint, "credential": st.Credential,
		"sysName": st.SysName, "sysDescr": st.SysDescr, "sysLocation": st.SysLocation,
		"sysContact": st.SysContact, "sysUpTime": float64(st.SysUpTime), "error": st.Error,
	})
}

// Poller returns the 1 Hz event source: reachability, restarts (sysUpTime going backwards)
// and changes of sysName/sysLocation/sysContact.
func (r *Renderer) Poller() *rfkit.Poller {
	var lastUp uint64
	boots := 0
	last := map[string]string{}
	return &rfkit.Poller{
		Source: "snmpd",
		Redact: r.red.Redact,
		Snap: func(ctx context.Context) (map[string]string, error) {
			st, err := r.State(ctx)
			if err != nil {
				return nil, err
			}
			if st.Reachable && st.SysUpTime < lastUp {
				boots++
			}
			cur := map[string]string{"reachable": strconv.FormatBool(st.Reachable), "restarts": strconv.Itoa(boots)}
			if st.Reachable {
				lastUp = st.SysUpTime
				cur["sysName"], cur["sysLocation"], cur["sysContact"] = st.SysName, st.SysLocation, st.SysContact
			} else {
				// Unknown while unreachable: keep the last values so only "reachable" changes.
				for _, k := range []string{"sysName", "sysLocation", "sysContact"} {
					if v, ok := last[k]; ok {
						cur[k] = v
					}
				}
			}
			last = cur
			return cur, nil
		},
	}
}
