package ravpn

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// NamespaceFailure exposes a fixed source stage number, never command output.
type NamespaceFailure struct{ Step uint8 }

func (e *NamespaceFailure) Error() string { return "remote-access: namespace operation refused" }
func (e *NamespaceFailure) Unwrap() error { return ErrBoundary }

type boundedOutput struct{ bytes.Buffer }

func (out *boundedOutput) Write(data []byte) (int, error) {
	if out.Len()+len(data) > 1<<20 {
		return 0, io.ErrShortBuffer
	}
	return out.Buffer.Write(data)
}

// command executes only a fixed tool with generated arguments; subprocess
// diagnostics remain private because network/crypto errors can contain secrets.
func command(ctx context.Context, tool string, input []byte, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var cmd *exec.Cmd
	switch tool {
	case "/usr/sbin/ip":
		// #nosec G204 -- fixed literal executable; private callers supply only typed-plan generated arguments, never shell text.
		cmd = exec.CommandContext(ctx, "/usr/sbin/ip", args...)
	case "/usr/sbin/nft":
		// #nosec G204 -- fixed literal executable and generated --check/--file - arguments; rules arrive on stdin after plan validation.
		cmd = exec.CommandContext(ctx, "/usr/sbin/nft", args...)
	case "/usr/bin/mount":
		// #nosec G204 -- fixed literal tool; namespace helpers derive bind paths from validated instance ownership.
		cmd = exec.CommandContext(ctx, "/usr/bin/mount", args...)
	case "/usr/bin/unshare":
		// #nosec G204 -- fixed literal tool; namespace creation supplies fixed --net and mount argv, no shell.
		cmd = exec.CommandContext(ctx, "/usr/bin/unshare", args...)
	case "/usr/bin/nsenter":
		// #nosec G204 -- fixed literal tool; kernel readback supplies validated fixed namespace binding and ip argv.
		cmd = exec.CommandContext(ctx, "/usr/bin/nsenter", args...)
	default:
		return nil, ErrBoundary
	}
	cmd.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C", "LC_ALL=C"}
	cmd.Stdin = bytes.NewReader(input)
	var out boundedOutput
	cmd.Stdout = &out
	if cmd.Run() != nil || out.Len() > 1<<20 {
		return nil, ErrBoundary
	}
	return out.Bytes(), nil
}

// ConfigureNamespace operates after namespace and capability verification.
// It refuses preexisting foreign XFRM interfaces, priority rules and nft tables.
func ConfigureNamespace(ctx context.Context, instance string) error {
	plan, err := ReadPrivatePlan(instance)
	if err != nil {
		return &NamespaceFailure{Step: 1}
	}
	links, err := command(ctx, "/usr/sbin/ip", nil, "-j", "-d", "link", "show")
	if err != nil {
		return &NamespaceFailure{Step: 2}
	}
	var items []observedLink
	if json.Unmarshal(links, &items) != nil {
		return &NamespaceFailure{Step: 3}
	}
	addXfrm := true
	outer, inner := false, false
	for _, item := range items {
		switch item.Name {
		case "outer0":
			outer = true
		case "inner0":
			inner = true
		case "xfrm0":
			if item.Info.Kind != "xfrm" || item.Info.Data.ID != 1 {
				return &NamespaceFailure{Step: 4}
			}
			addXfrm = false
		case "lo":
		default:
			if !matchesKernelLink(plan, item) {
				return &NamespaceFailure{Step: 5}
			}
		}
	}
	if !outer || !inner {
		return &NamespaceFailure{Step: 6}
	}
	family := "-4"
	endpoint, _ := netip.ParseAddr(plan.LocalAddress)
	if !endpoint.Is4() {
		family = "-6"
	}
	rules, err := command(ctx, "/usr/sbin/ip", nil, family, "-j", "rule", "show")
	if err != nil {
		return &NamespaceFailure{Step: 7}
	}
	addRule, err := ownedOuterRule(rules)
	if err != nil {
		return &NamespaceFailure{Step: 8}
	}
	tables, err := command(ctx, "/usr/sbin/nft", nil, "-j", "list", "ruleset")
	if err != nil {
		return &NamespaceFailure{Step: 9}
	}
	replace, err := ownedFirewall(tables, plan.Instance)
	if err != nil {
		return &NamespaceFailure{Step: 10}
	}
	if replace {
		full, err := command(ctx, "/usr/sbin/nft", nil, "-j", "list", "table", "inet", "ngfw_ra")
		if err != nil {
			return &NamespaceFailure{Step: 11}
		}
		if _, err = ownedFirewall(full, plan.Instance); err != nil {
			return &NamespaceFailure{Step: 12}
		}
	}
	commands, err := plan.Commands(addXfrm, addRule)
	if err != nil {
		return &NamespaceFailure{Step: 13}
	}
	for _, args := range commands {
		if _, err = command(ctx, "/usr/sbin/ip", nil, args...); err != nil {
			return &NamespaceFailure{Step: 14}
		}
	}
	firewall, err := plan.Firewall(replace)
	if err != nil {
		return &NamespaceFailure{Step: 15}
	}
	if _, err = command(ctx, "/usr/sbin/nft", firewall, "--check", "--file", "-"); err != nil {
		return &NamespaceFailure{Step: 16}
	}
	if _, err = command(ctx, "/usr/sbin/nft", firewall, "--file", "-"); err != nil {
		return &NamespaceFailure{Step: 17}
	}
	values, err := plan.Sysctls()
	if err != nil {
		return &NamespaceFailure{Step: 18}
	}
	for name, value := range values {
		if !strings.HasPrefix(name, "net/") {
			return &NamespaceFailure{Step: 19}
		}
		if err = os.WriteFile(filepath.Join("/proc/sys", name), []byte(value), 0); err != nil {
			return &NamespaceFailure{Step: 20}
		}
	}
	return nil
}

func ownedOuterRule(data []byte) (bool, error) {
	var rules []struct {
		Priority uint32          `json:"priority"`
		Mark     json.RawMessage `json:"fwmark"`
		Table    json.RawMessage `json:"table"`
	}
	if json.Unmarshal(data, &rules) != nil {
		return false, ErrBoundary
	}
	add := true
	for _, rule := range rules {
		if rule.Priority == 100 {
			if !add || string(rule.Mark) != "\"0x1\"" || string(rule.Table) != "100" {
				return false, ErrBoundary
			}
			add = false
		}
	}
	return add, nil
}

func ownedFirewall(data []byte, instance string) (bool, error) {
	var list struct {
		Objects []struct {
			Table *struct {
				Family  string `json:"family"`
				Name    string `json:"name"`
				Comment string `json:"comment"`
			} `json:"table"`
		} `json:"nftables"`
	}
	if json.Unmarshal(data, &list) != nil {
		return false, ErrBoundary
	}
	found := false
	for _, object := range list.Objects {
		table := object.Table
		if table == nil {
			continue
		}
		if found || table.Family != "inet" || table.Name != "ngfw_ra" || table.Comment != "ngfw-ra:"+instance {
			return false, ErrBoundary
		}
		found = true
	}
	return found, nil
}
