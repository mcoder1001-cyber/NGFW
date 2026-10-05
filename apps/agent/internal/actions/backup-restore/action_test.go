package backuprestore

import (
	"context"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/renderers"
	"strings"
	"testing"
)

type fakeRunner struct {
	commands []renderers.Command
	output   renderers.Output
}

func (f *fakeRunner) Run(_ context.Context, c renderers.Command) (renderers.Output, error) {
	f.commands = append(f.commands, c)
	if c.Path == systemctlBin && len(c.Args) > 0 && c.Args[0] == "show" {
		return renderers.Output{Stdout: []byte("inactive\ninactive\ninactive\ninactive\n")}, nil
	}
	return f.output, nil
}
func TestUpgradeFixedArguments(t *testing.T) {
	r := &fakeRunner{output: renderers.Output{Stdout: []byte(`{"active_slot":"A"}`)}}
	for _, op := range []ngfwv1.UpgradeOp{ngfwv1.UpgradeOp_UPGRADE_OP_STATUS, ngfwv1.UpgradeOp_UPGRADE_OP_ACTIVATE, ngfwv1.UpgradeOp_UPGRADE_OP_CONFIRM, ngfwv1.UpgradeOp_UPGRADE_OP_ROLLBACK} {
		if _, err := Upgrade(context.Background(), &ngfwv1.UpgradeAction{Op: op}, r); err != nil {
			t.Fatal(err)
		}
	}
	if strings.Join(r.commands[0].Args, " ") != "status --json" {
		t.Fatal(r.commands)
	}
	for _, cmd := range r.commands[1:] {
		if cmd.Args[0] == "show" {
			continue
		}
		if cmd.Path != systemctlBin || len(cmd.Args) != 2 || cmd.Args[0] != "start" || !strings.HasPrefix(cmd.Args[1], "ngfw-upgrade@") {
			t.Fatal(cmd)
		}
	}
	count := len(r.commands)
	for _, req := range []*ngfwv1.UpgradeAction{{Op: 999}, {Op: ngfwv1.UpgradeOp_UPGRADE_OP_STAGE, Bundle: "/etc/passwd"}, {Op: ngfwv1.UpgradeOp_UPGRADE_OP_ACTIVATE, Bundle: "/data/updates/a.tar"}} {
		if _, err := Upgrade(context.Background(), req, r); err == nil {
			t.Fatal("unsafe request allowed")
		}
	}
	if len(r.commands) != count {
		t.Fatal("unsafe request executed")
	}
}
func TestSupportFixedBoundsAndSensitiveOutput(t *testing.T) {
	r := &fakeRunner{output: renderers.Output{Stdout: []byte(`{"services":{"agent":"active"}}`)}}
	if _, err := Support(context.Background(), &ngfwv1.SupportBundleAction{SinceSec: 86400, AuditRows: 1000}, r); err != nil {
		t.Fatal(err)
	}
	if len(r.commands[0].Args) != 0 || r.commands[0].Path != supportBin {
		t.Fatal(r.commands)
	}
	if _, err := Support(context.Background(), &ngfwv1.SupportBundleAction{AuditRows: 1001}, r); err == nil {
		t.Fatal("excessive support bound")
	}
	r.output.Stdout = []byte(`{"key":"-----BEGIN PRIVATE KEY"}`)
	if _, err := Support(context.Background(), &ngfwv1.SupportBundleAction{}, r); err == nil {
		t.Fatal("sensitive output allowed")
	}
}
