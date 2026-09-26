package rsyslog

import (
	"encoding/base64"
	"fmt"
	"regexp"

	"google.golang.org/protobuf/proto"

	vrxv1 "ngfw/agent/gen/vrx/v1"
)

// The render input (F-unbound-chrony-syslog). The agent drives the export through one singleton scheduler
// descriptor (descriptor.go) whose Value is exactly this input: management.syslog, carried in a
// *vrxv1.ManagementConfig (the other management leaves are not the renderer's). Render embeds it in the export file
// as `# vrx-input: <base64 of the deterministic protobuf encoding>`; Retrieve reads it back, re-renders it and proves
// byte for byte that the file is exactly that rendering (the F-kea-dhcp-relay pattern, D-063).

// Input returns the render input of ds: its management.syslog, or nil when the list is empty.
func Input(ds *vrxv1.DesiredState) *vrxv1.ManagementConfig {
	list := ds.GetManagement().GetSyslog()
	if len(list) == 0 {
		return nil
	}
	out := &vrxv1.ManagementConfig{Syslog: make([]*vrxv1.SyslogTarget, len(list))}
	for i, t := range list {
		out.Syslog[i] = proto.Clone(t).(*vrxv1.SyslogTarget)
	}
	return out
}

// asDesired maps the descriptor's Value type onto the renderer's input (rfkit.Decode takes a DesiredState).
func asDesired(msg proto.Message) proto.Message {
	if mc, ok := msg.(*vrxv1.ManagementConfig); ok {
		return &vrxv1.DesiredState{Management: mc}
	}
	return msg
}

const inputPrefix = "# vrx-input: "

var (
	inputLineRe = regexp.MustCompile(`(?m)^# vrx-input: ([A-Za-z0-9+/]*={0,2})$`)
	base64Re    = regexp.MustCompile(`^[A-Za-z0-9+/]*={0,2}$`)
)

func encodeInput(in *vrxv1.ManagementConfig) (string, error) {
	if in == nil {
		return "", nil
	}
	b, err := proto.MarshalOptions{Deterministic: true}.Marshal(in)
	if err != nil {
		return "", fmt.Errorf("%w: encode render input: %w", ErrInput, err)
	}
	return base64.StdEncoding.EncodeToString(b), nil
}

// EmbeddedInput reads the render input back from a rendered export file. ok is false for a file without one (an
// empty export, or a file this renderer did not write).
func EmbeddedInput(conf []byte) (*vrxv1.ManagementConfig, bool, error) {
	m := inputLineRe.FindAllSubmatch(conf, 2)
	switch len(m) {
	case 0:
		return nil, false, nil
	case 1:
	default:
		return nil, false, fmt.Errorf("rsyslog: %s appears more than once", inputPrefix)
	}
	raw, err := base64.StdEncoding.DecodeString(string(m[0][1]))
	if err != nil {
		return nil, false, fmt.Errorf("rsyslog: render input: %w", err)
	}
	in := &vrxv1.ManagementConfig{}
	if err := proto.Unmarshal(raw, in); err != nil {
		return nil, false, fmt.Errorf("rsyslog: render input: %w", err)
	}
	return in, true, nil
}
