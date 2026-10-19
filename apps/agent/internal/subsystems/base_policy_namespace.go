package subsystems

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	binlcp "ngfw/agent/binapi/lcp"
	"ngfw/agent/binapi/vlib"
	"ngfw/agent/internal/descriptors/lcp"
	"ngfw/agent/internal/vpp"
)

const maxLCPNamespaceStatus = 64 * 1024

// VPP 26.06 can leave the binary reply's namespace bytes uninitialized when
// unset. Malformed bytes are not proof of root: require independent, explicit
// unset readback from the fixed read-only CLI command on the same connection.
func basePolicyNamespace(ctx context.Context, client vpp.Client) (string, error) {
	reply, err := binlcp.NewServiceClient(client).LcpDefaultNsGet(ctx, &binlcp.LcpDefaultNsGet{})
	if err != nil {
		return "", err
	}
	namespace := strings.TrimRight(reply.Netns, "\x00")
	invalid := (lcp.DefaultNetns{Netns: namespace}).Validate()
	if invalid == nil {
		return namespace, nil
	}
	status, err := vlib.NewServiceClient(client).CliInband(ctx, &vlib.CliInband{Cmd: "show lcp"})
	if err == nil {
		err = confirmUnsetLCPNamespace(status.Reply)
	}
	if err != nil {
		return "", errors.Join(invalid, fmt.Errorf("confirm unset LCP namespace: %w", err))
	}
	return "", nil
}

func confirmUnsetLCPNamespace(output string) error {
	if len(output) > maxLCPNamespaceStatus || !utf8.ValidString(output) {
		return errors.New("invalid or oversized LCP status")
	}
	for _, b := range []byte(output) {
		if (b < 0x20 && b != '\n' && b != '\r' && b != '\t') || b == 0x7f {
			return errors.New("control character in LCP status")
		}
	}
	count := 0
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSuffix(line, "\r")
		if strings.ContainsRune(line, '\r') {
			return errors.New("embedded carriage return in LCP status")
		}
		if !strings.HasPrefix(strings.TrimSpace(line), "lcp default") {
			continue
		}
		count++
		if line != "lcp default netns '<unset>'" || count != 1 {
			return errors.New("ambiguous or non-root LCP namespace status")
		}
	}
	if count != 1 {
		return errors.New("missing LCP namespace status")
	}
	return nil
}
