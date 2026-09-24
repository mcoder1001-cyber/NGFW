package chrony

import (
	"encoding/hex"
	"errors"
	"fmt"
	"hash/fnv"
	"net/netip"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/renderers"
)

// ErrInvalid is wrapped by every error about the desired state (a value the renderer refuses
// to put into chrony's files). The schema checks first; the renderer checks again (D-049).
var ErrInvalid = errors.New("chrony: invalid desired state")

// ErrSecret is wrapped when a key reference cannot be resolved. The message names the
// reference, never the secret.
var ErrSecret = errors.New("chrony: secret not available")

func invalid(path, format string, a ...any) error {
	return fmt.Errorf("%w: %s: %s", ErrInvalid, path, fmt.Sprintf(format, a...))
}

var (
	objectNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$`)
	hostnameRe   = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?(?:\.[A-Za-z0-9](?:[A-Za-z0-9-]{0,61}[A-Za-z0-9])?)*$`)
	allDigitsRe  = regexp.MustCompile(`^[0-9.]+$`)
	// secretRefOf('key') in packages/schema (D-051).
	keyRefRe = regexp.MustCompile(`^key/[A-Za-z0-9][A-Za-z0-9_.-]{0,122}$`)
)

// keyID derives the chrony key id of a reference.
func keyID(ref string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(ref))
	return h.Sum32()%4294967295 + 1
}

// MaxKeyBytes bounds one symmetric key.
const MaxKeyBytes = 128

// SecretResolver returns the secret value of a reference ("key/ntp-upstream"). Production:
// the agent's secret store; tests: a fixture map with VRX_TEST_PSK_<id> values.
type SecretResolver func(ref string) ([]byte, error)

type input struct {
	ntp *vrxv1.NtpService
}

func extract(desired proto.Message) (input, error) {
	switch d := desired.(type) {
	case nil:
		return input{}, nil
	case *vrxv1.DesiredState:
		return input{ntp: d.GetServices().GetNtp()}, nil
	case *vrxv1.ServicesConfig:
		return input{ntp: d.GetNtp()}, nil
	case *vrxv1.NtpService:
		return input{ntp: d}, nil
	default:
		return input{}, fmt.Errorf("%w: unsupported desired type %T", ErrInvalid, desired)
	}
}

// ----- template model -----------------------------------------------------------------------

type confData struct {
	Paths        Paths
	Enabled      bool
	BindAddrs    []string
	Port         uint32
	Makestep     string // "<threshold> <limit>"
	RTCSync      bool
	Allow, Deny  []string
	RateLimit    *rateLimit
	LocalStratum uint32
	Orphan       bool
	NTS          bool
}

type rateLimit struct{ Interval, Burst, Leak int64 }

type source struct {
	Kind    string // "server" | "pool"
	Host    string // canonical IP or lower-case host name
	Port    uint16
	IBurst  bool
	Prefer  bool
	MinPoll *int32
	MaxPoll *int32
	KeyID   uint32
	NTS     bool
}

// sourcesData is the vrx.sources template model: the embedded render input and the server/pool lines.
type sourcesData struct {
	Input   string
	Sources []source
}

type key struct {
	ID  uint32
	Hex string
}

type rendered struct {
	conf    confData
	sources []source
	keys    []key
}

func host(path, s string) (string, error) {
	if a, err := netip.ParseAddr(s); err == nil {
		if a.Zone() != "" {
			return "", invalid(path, "%q has a zone", s)
		}
		return a.Unmap().String(), nil
	}
	if len(s) > 253 || !hostnameRe.MatchString(s) || allDigitsRe.MatchString(s) {
		return "", invalid(path, "%q is neither an IP address nor a host name", s)
	}
	return strings.ToLower(s), nil
}

func (r *Renderer) build(in input) (*rendered, error) {
	n := in.ntp
	out := &rendered{conf: confData{Paths: r.paths, Port: 123, Makestep: "1 3"}}
	if n == nil || !n.GetEnabled() {
		out.conf.Port = 0
		return out, nil
	}
	c := &out.conf
	c.Enabled = true
	vrf := n.GetVrf()
	if vrf != "" && !objectNameRe.MatchString(vrf) {
		return nil, invalid("vrf", "VRF %q is not an object name", vrf)
	}
	if n.Port != nil {
		c.Port = n.GetPort()
	}
	if c.Port > 65535 {
		return nil, invalid("port", "port %d outside 0..65535", c.Port)
	}
	// bind addresses: chrony takes one per family
	fams := map[bool]bool{}
	for i, l := range n.GetListen() {
		a, err := netip.ParseAddr(l)
		if err != nil || a.Zone() != "" {
			return nil, invalid(fmt.Sprintf("listen/%d", i), "%q is not an IP address", l)
		}
		a = a.Unmap()
		if fams[a.Is4()] {
			return nil, invalid(fmt.Sprintf("listen/%d", i), "chrony binds one address per family")
		}
		fams[a.Is4()] = true
		if r.paths.LoopbackOnly && !a.IsLoopback() {
			return nil, invalid(fmt.Sprintf("listen/%d", i), "%s is outside the test scope (loopback only)", a)
		}
		c.BindAddrs = append(c.BindAddrs, a.String())
	}
	slices.Sort(c.BindAddrs)
	if r.paths.LoopbackOnly && c.Port != 0 && len(c.BindAddrs) == 0 {
		return nil, invalid("listen", "a test instance serving on port %d must bind a loopback address", c.Port)
	}
	for _, f := range []struct {
		name string
		in   []string
		out  *[]string
	}{{"allow", n.GetAllow(), &c.Allow}, {"deny", n.GetDeny(), &c.Deny}} {
		for i, p := range f.in {
			net, err := renderers.Network(p)
			if err != nil {
				return nil, invalid(fmt.Sprintf("%s/%d", f.name, i), "%q is not a CIDR prefix", p)
			}
			if slices.Contains(*f.out, net) {
				return nil, invalid(fmt.Sprintf("%s/%d", f.name, i), "prefix %s twice", net)
			}
			*f.out = append(*f.out, net)
		}
	}
	serving := len(c.Allow) > 0 || n.GetNtsServer() != nil
	if serving && c.Port == 0 {
		return nil, invalid("port", "port 0 disables server mode; remove allow/ntsServer or set a port")
	}
	if n.GetNtsServer() != nil {
		// NTS-KE server certificates are F-ntp (RF-3 out of scope): refuse rather than render a
		// half-configured server.
		return nil, invalid("ntsServer", "serving NTS is not supported by this renderer yet (F-ntp)")
	}
	if rl := n.GetRateLimit(); rl != nil {
		if !serving {
			return nil, invalid("rateLimit", "rateLimit applies to server mode")
		}
		iv, burst, leak := int64(3), int64(8), int64(2)
		if rl.Interval != nil {
			iv = int64(rl.GetInterval())
		}
		if rl.Burst != nil {
			burst = int64(rl.GetBurst())
		}
		if rl.Leak != nil {
			leak = int64(rl.GetLeak())
		}
		if iv < -19 || iv > 12 || burst > 255 || leak > 4 {
			return nil, invalid("rateLimit", "interval -19..12, burst 0..255, leak 0..4")
		}
		c.RateLimit = &rateLimit{Interval: iv, Burst: burst, Leak: leak}
	}
	if n.LocalStratum != nil {
		if n.GetLocalStratum() < 1 || n.GetLocalStratum() > 15 {
			return nil, invalid("localStratum", "stratum %d outside 1..15", n.GetLocalStratum())
		}
		c.LocalStratum = n.GetLocalStratum()
	}
	c.Orphan = n.GetOrphan()
	if c.Orphan && c.LocalStratum == 0 {
		return nil, invalid("orphan", "orphan mode needs localStratum")
	}
	c.RTCSync = r.paths.ClockControl && (n.RtcSync == nil || n.GetRtcSync())
	if ms := n.GetMakestep(); ms != nil {
		th, lim := 1.0, int32(3)
		if ms.ThresholdSec != nil {
			th = ms.GetThresholdSec()
		}
		if ms.Limit != nil {
			lim = ms.GetLimit()
		}
		if th < 0.1 || th > 1000 || lim < -1 || lim > 100 {
			return nil, invalid("makestep", "threshold 0.1..1000 s, limit -1..100")
		}
		c.Makestep = strconv.FormatFloat(th, 'f', -1, 64) + " " + strconv.FormatInt(int64(lim), 10)
	}

	// sources and keys
	keyIDs := map[string]uint32{}
	var refs []string
	seen := map[string]bool{}
	for i, s := range n.GetServers() {
		path := fmt.Sprintf("servers/%d", i)
		h, err := host(path+"/address", s.GetAddress())
		if err != nil {
			return nil, err
		}
		if seen[h] {
			return nil, invalid(path+"/address", "server %s twice", h)
		}
		seen[h] = true
		src := source{Kind: "server", Host: h, Port: r.paths.SourcePort, IBurst: s.Iburst == nil || s.GetIburst(), Prefer: s.GetPrefer(), NTS: s.GetNts()}
		for _, pp := range []struct {
			v   *int32
			dst **int32
			n   string
		}{{s.MinPoll, &src.MinPoll, "minPoll"}, {s.MaxPoll, &src.MaxPoll, "maxPoll"}} {
			if pp.v != nil {
				if *pp.v < -6 || *pp.v > 24 {
					return nil, invalid(path+"/"+pp.n, "poll %d outside -6..24", *pp.v)
				}
				v := *pp.v
				*pp.dst = &v
			}
		}
		if src.MinPoll != nil && src.MaxPoll != nil && *src.MinPoll > *src.MaxPoll {
			return nil, invalid(path+"/minPoll", "minimum poll exceeds maximum poll")
		}
		if s.KeyRef != nil {
			if src.NTS {
				return nil, invalid(path+"/keyRef", "NTS and a symmetric key are exclusive")
			}
			ref := s.GetKeyRef()
			if !keyRefRe.MatchString(ref) {
				return nil, invalid(path+"/keyRef", "%q is not a key/<name> reference", ref)
			}
			if _, ok := keyIDs[ref]; !ok {
				keyIDs[ref] = 0
				refs = append(refs, ref)
			}
			src.KeyID = 0 // assigned below, after sorting the references
		}
		if src.NTS {
			c.NTS = true
		}
		out.sources = append(out.sources, src)
	}
	for i, p := range n.GetPools() {
		path := fmt.Sprintf("pools/%d", i)
		h, err := host(path, p)
		if err != nil {
			return nil, err
		}
		if _, isIP := netip.ParseAddr(h); isIP == nil {
			return nil, invalid(path, "a pool is a host name, not an address")
		}
		if seen[h] {
			return nil, invalid(path, "%s twice", h)
		}
		seen[h] = true
		out.sources = append(out.sources, source{Kind: "pool", Host: h, IBurst: true})
	}
	if len(out.sources) == 0 && c.LocalStratum == 0 {
		return nil, invalid("servers", "an enabled NTP service needs a server, a pool or a local stratum")
	}
	// Key ids are a function of the reference alone (FNV-32a, 1..2^32-1), so adding or
	// removing another key never renumbers this one (review L6); a collision is refused.
	slices.Sort(refs)
	byID := map[uint32]string{}
	for _, ref := range refs {
		id := keyID(ref)
		if other, dup := byID[id]; dup {
			return nil, invalid("servers", "key references %s and %s map to the same chrony key id %d; rename one", other, ref, id)
		}
		byID[id], keyIDs[ref] = ref, id
	}
	for i, s := range n.GetServers() {
		if s.KeyRef != nil {
			out.sources[i].KeyID = keyIDs[s.GetKeyRef()]
		}
	}
	for _, ref := range refs {
		if r.secrets == nil {
			return nil, fmt.Errorf("%w: %s (no secret resolver configured)", ErrSecret, ref)
		}
		val, err := r.secrets(ref)
		if err != nil {
			return nil, fmt.Errorf("%w: %s", ErrSecret, ref) // never wrap err: it might quote the value
		}
		if len(val) == 0 || len(val) > MaxKeyBytes {
			return nil, fmt.Errorf("%w: %s: key must be 1..%d bytes", ErrSecret, ref, MaxKeyBytes)
		}
		out.keys = append(out.keys, key{ID: keyIDs[ref], Hex: strings.ToUpper(hex.EncodeToString(val))})
	}
	return out, nil
}
