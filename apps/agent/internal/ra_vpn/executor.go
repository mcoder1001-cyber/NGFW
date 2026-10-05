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
	cmd := exec.CommandContext(ctx, tool, args...)
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
		return err
	}
	links, err := command(ctx, "/usr/sbin/ip", nil, "-j", "-d", "link", "show")
	if err != nil {
		return err
	}
	var items []struct {
		Name string `json:"ifname"`
		Info struct {
			Kind string `json:"info_kind"`
			Data struct {
				ID uint32 `json:"if_id"`
			} `json:"info_data"`
		} `json:"linkinfo"`
	}
	if json.Unmarshal(links, &items) != nil {
		return ErrBoundary
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
				return ErrBoundary
			}
			addXfrm = false
		case "lo":
		default:
			return ErrBoundary
		}
	}
	if !outer || !inner {
		return ErrBoundary
	}
	family := "-4"
	endpoint, _ := netip.ParseAddr(plan.LocalAddress)
	if !endpoint.Is4() {
		family = "-6"
	}
	rules, err := command(ctx, "/usr/sbin/ip", nil, family, "-j", "rule", "show")
	if err != nil {
		return err
	}
	addRule, err := ownedOuterRule(rules)
	if err != nil {
		return err
	}
	tables, err := command(ctx, "/usr/sbin/nft", nil, "-j", "list", "ruleset")
	if err != nil {
		return err
	}
	replace, err := ownedFirewall(tables, plan.Instance)
	if err != nil {
		return err
	}
	if replace {
		full, err := command(ctx, "/usr/sbin/nft", nil, "-j", "list", "table", "inet", "ngfw_ra")
		if err != nil {
			return err
		}
		if _, err = ownedFirewall(full, plan.Instance); err != nil {
			return err
		}
	}
	commands, err := plan.Commands(addXfrm, addRule)
	if err != nil {
		return err
	}
	for _, args := range commands {
		if _, err = command(ctx, "/usr/sbin/ip", nil, args...); err != nil {
			return err
		}
	}
	firewall, err := plan.Firewall(replace)
	if err != nil {
		return err
	}
	if _, err = command(ctx, "/usr/sbin/nft", firewall, "--check", "--file", "-"); err != nil {
		return err
	}
	if _, err = command(ctx, "/usr/sbin/nft", firewall, "--file", "-"); err != nil {
		return err
	}
	values, err := plan.Sysctls()
	if err != nil {
		return err
	}
	for name, value := range values {
		if !strings.HasPrefix(name, "net/") {
			return ErrBoundary
		}
		if err = os.WriteFile(filepath.Join("/proc/sys", name), []byte(value), 0); err != nil {
			return ErrBoundary
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
