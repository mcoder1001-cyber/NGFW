package unbound

import (
	"encoding/base64"
	"fmt"
	"regexp"
	"sort"

	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/renderers"
)

// The render input (F-unbound-chrony-syslog). The agent drives Unbound through one singleton scheduler
// descriptor (descriptor.go) whose Value is exactly this input: services.dns.resolvers (every resolver, enabled or
// not, as the document has it; the VPP cache is DF-8's). Render embeds the input in unbound.conf as the comment line
// `# vrx-input: <base64 of the deterministic protobuf encoding>`, so Retrieve reads it back from the file the daemon
// loads, re-renders it and proves byte for byte that the file is exactly that rendering — the Value is derived
// from the file, never echoed from memory (D-063; the F-kea-dhcp-relay pattern).

// Input returns the render input of dns: its resolvers, or nil when there is none.
func Input(dns *vrxv1.DnsService) *vrxv1.DnsService {
	if len(dns.GetResolvers()) == 0 {
		return nil
	}
	out := &vrxv1.DnsService{Resolvers: make(map[string]*vrxv1.DnsResolver, len(dns.GetResolvers()))}
	for name, r := range dns.GetResolvers() {
		out.Resolvers[name] = proto.Clone(r).(*vrxv1.DnsResolver)
	}
	return out
}

// InputOf is Input for any accepted renderer input (*vrxv1.DesiredState, *vrxv1.ServicesConfig, *vrxv1.DnsService).
func InputOf(desired proto.Message) (*vrxv1.DnsService, error) {
	in, err := extract(desired)
	if err != nil {
		return nil, err
	}
	return Input(in.dns), nil
}

const inputPrefix = "# vrx-input: "

var (
	inputLineRe = regexp.MustCompile(`(?m)^# vrx-input: ([A-Za-z0-9+/]*={0,2})$`)
	base64Re    = regexp.MustCompile(`^[A-Za-z0-9+/]*={0,2}$`)
)

// encodeInput is the base64 of the deterministic protobuf encoding ("" for nil).
func encodeInput(in *vrxv1.DnsService) (string, error) {
	if in == nil {
		return "", nil
	}
	b, err := proto.MarshalOptions{Deterministic: true}.Marshal(in)
	if err != nil {
		return "", fmt.Errorf("%w: encode render input: %w", ErrInvalid, err)
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

// b64 is the template helper for the input line: base64 characters only (nothing else can reach the comment).
func b64(s string) (string, error) {
	if !base64Re.MatchString(s) {
		return "", fmt.Errorf("%w: render input is not base64", renderers.ErrUnsafe)
	}
	return s, nil
}

// EmbeddedInput reads the render input back from a rendered unbound.conf. ok is false for a file without one: an
// idle configuration, or a file this renderer did not write.
func EmbeddedInput(conf []byte) (*vrxv1.DnsService, bool, error) {
	m := inputLineRe.FindAllSubmatch(conf, 2)
	switch len(m) {
	case 0:
		return nil, false, nil
	case 1:
	default:
		return nil, false, fmt.Errorf("unbound: %s appears more than once", inputPrefix)
	}
	raw, err := base64.StdEncoding.DecodeString(string(m[0][1]))
	if err != nil {
		return nil, false, fmt.Errorf("unbound: render input: %w", err)
	}
	in := &vrxv1.DnsService{}
	if err := proto.Unmarshal(raw, in); err != nil {
		return nil, false, fmt.Errorf("unbound: render input: %w", err)
	}
	return in, true, nil
}

// ResolverNames lists the resolvers of an input, sorted (logs and state).
func ResolverNames(in *vrxv1.DnsService) []string {
	out := make([]string, 0, len(in.GetResolvers()))
	for n := range in.GetResolvers() {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
