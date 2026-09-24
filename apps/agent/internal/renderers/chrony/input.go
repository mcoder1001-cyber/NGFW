package chrony

import (
	"encoding/base64"
	"fmt"
	"regexp"

	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/renderers"
)

// The render input (F-unbound-chrony-syslog). The agent drives chronyd through one singleton scheduler descriptor
// (descriptor.go) whose Value is exactly this input: services.ntp while it is enabled. Render embeds the input in
// sources.d/vrx.sources — the file chrony reloads at run time (`reload sources`), so an input change alone never
// costs a restart — as `# vrx-input: <base64 of the deterministic protobuf encoding>`. Retrieve reads it back,
// re-renders it and proves byte for byte that chrony.conf, vrx.sources and chrony.keys are exactly that rendering
// (the F-kea-dhcp-relay pattern, D-063).

// Input returns the render input of ntp: ntp itself while it is enabled, else nil.
func Input(ntp *vrxv1.NtpService) *vrxv1.NtpService {
	if !ntp.GetEnabled() {
		return nil
	}
	return proto.Clone(ntp).(*vrxv1.NtpService)
}

// InputOf is Input for any accepted renderer input (*vrxv1.DesiredState, *vrxv1.ServicesConfig, *vrxv1.NtpService).
func InputOf(desired proto.Message) (*vrxv1.NtpService, error) {
	in, err := extract(desired)
	if err != nil {
		return nil, err
	}
	return Input(in.ntp), nil
}

const inputPrefix = "# vrx-input: "

var (
	inputLineRe = regexp.MustCompile(`(?m)^# vrx-input: ([A-Za-z0-9+/]*={0,2})$`)
	base64Re    = regexp.MustCompile(`^[A-Za-z0-9+/]*={0,2}$`)
)

func encodeInput(in *vrxv1.NtpService) (string, error) {
	if in == nil {
		return "", nil
	}
	b, err := proto.MarshalOptions{Deterministic: true}.Marshal(in)
	if err != nil {
		return "", fmt.Errorf("%w: encode render input: %w", ErrInvalid, err)
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

// b64 is the template helper for the input line: base64 characters only.
func b64(s string) (string, error) {
	if !base64Re.MatchString(s) {
		return "", fmt.Errorf("%w: render input is not base64", renderers.ErrUnsafe)
	}
	return s, nil
}

// EmbeddedInput reads the render input back from a rendered vrx.sources. ok is false for a file without one (the
// disabled rendering, or a file this renderer did not write).
func EmbeddedInput(sources []byte) (*vrxv1.NtpService, bool, error) {
	m := inputLineRe.FindAllSubmatch(sources, 2)
	switch len(m) {
	case 0:
		return nil, false, nil
	case 1:
	default:
		return nil, false, fmt.Errorf("chrony: %s appears more than once", inputPrefix)
	}
	raw, err := base64.StdEncoding.DecodeString(string(m[0][1]))
	if err != nil {
		return nil, false, fmt.Errorf("chrony: render input: %w", err)
	}
	in := &vrxv1.NtpService{}
	if err := proto.Unmarshal(raw, in); err != nil {
		return nil, false, fmt.Errorf("chrony: render input: %w", err)
	}
	return in, true, nil
}
