// Package backuprestore exposes fixed appliance operations without broadening the agent sandbox.
package backuprestore

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/renderers"
)

const upgradeBin = "/usr/sbin/ngfw-upgrade"
const supportBin = "/usr/lib/ngfw/ngfw-support-collect"
const systemctlBin = "/usr/bin/systemctl"

var binaries = renderers.NewAllowlist(upgradeBin, supportBin, systemctlBin)
var upgradeLock sync.Mutex
var bundleName = regexp.MustCompile(`^ngfw-update-[a-zA-Z0-9.-]+\.tar$`)

// Runner returns the allow-listed production runner.
func Runner() *renderers.SystemRunner {
	r := renderers.NewSystemRunner(binaries)
	r.MaxOutput = 1 << 20
	return r
}

// Bundle validates a direct regular update file, rejecting symlink parents and files.
func Bundle(path string) error {
	if filepath.Dir(path) != "/data/updates" || !bundleName.MatchString(filepath.Base(path)) {
		return errors.New("invalid bundle path")
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil || resolved != path {
		return errors.New("bundle symlink or missing file")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("bundle is not regular")
	}
	return nil
}

// Upgrade runs status in the existing read-only namespace and mutations through dedicated root systemd instances.
func Upgrade(ctx context.Context, req *ngfwv1.UpgradeAction, runner renderers.Runner) ([]string, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "missing upgrade request")
	}
	op := ""
	switch req.GetOp() {
	case ngfwv1.UpgradeOp_UPGRADE_OP_STATUS:
		op = "status"
	case ngfwv1.UpgradeOp_UPGRADE_OP_STAGE:
		op = "stage"
	case ngfwv1.UpgradeOp_UPGRADE_OP_ACTIVATE:
		op = "activate"
	case ngfwv1.UpgradeOp_UPGRADE_OP_CONFIRM:
		op = "confirm"
	case ngfwv1.UpgradeOp_UPGRADE_OP_ROLLBACK:
		op = "rollback"
	default:
		return nil, status.Error(codes.InvalidArgument, "unknown upgrade operation")
	}
	if op != "stage" && req.GetBundle() != "" {
		return nil, status.Error(codes.InvalidArgument, "bundle only valid for stage")
	}
	if op == "stage" {
		if err := Bundle(req.GetBundle()); err != nil {
			return nil, status.Error(codes.InvalidArgument, "bundle must be a regular file directly under /data/updates")
		}
	}
	if !upgradeLock.TryLock() {
		return nil, status.Error(codes.ResourceExhausted, "upgrade busy")
	}
	defer upgradeLock.Unlock()
	cmd := renderers.Command{Path: upgradeBin, Args: []string{"status", "--json"}, Timeout: 30 * time.Second}
	if op != "status" {
		state, stateErr := runner.Run(ctx, renderers.Command{Path: systemctlBin, Args: []string{"show", "--property=ActiveState", "--value", "ngfw-upgrade@stage.service", "ngfw-upgrade@activate.service", "ngfw-upgrade@confirm.service", "ngfw-upgrade@rollback.service"}, Timeout: 10 * time.Second})
		if stateErr != nil || state.ExitCode != 0 {
			return nil, status.Error(codes.Unavailable, "cannot inspect upgrade executor")
		}
		for _, line := range strings.Fields(string(state.Stdout)) {
			if line != "inactive" && line != "failed" {
				return nil, status.Error(codes.ResourceExhausted, "appliance upgrade executor busy")
			}
		}
		if op == "stage" {
			raw, _ := json.Marshal(map[string]string{"bundle": req.GetBundle()})
			_ = os.Remove("/run/ngfw/upgrade-request.json")
			file, err := os.OpenFile("/run/ngfw/upgrade-request.json", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if err != nil {
				return nil, status.Error(codes.Internal, "cannot stage upgrade request")
			}
			_, err = file.Write(raw)
			closeErr := file.Close()
			if err != nil || closeErr != nil {
				return nil, status.Error(codes.Internal, "cannot stage upgrade request")
			}
			// The root helper consumes/removes this handoff. Preserve it on RPC cancellation until the executor reads it.
		}
		cmd = renderers.Command{Path: systemctlBin, Args: []string{"start", "ngfw-upgrade@" + op + ".service"}, Timeout: 15 * time.Minute}
	}
	out, err := runner.Run(ctx, cmd)
	if err != nil || out.ExitCode != 0 {
		return nil, status.Error(codes.FailedPrecondition, "appliance upgrade operation refused; inspect host service status")
	}
	if op == "status" {
		if !json.Valid(out.Stdout) {
			return nil, status.Error(codes.Internal, "invalid upgrade status")
		}
		return []string{strings.TrimSpace(string(out.Stdout))}, nil
	}
	return []string{"upgrade " + op + " completed"}, nil
}

// Support collects fixed read-only facts; request fields cannot alter argv.
func Support(ctx context.Context, req *ngfwv1.SupportBundleAction, runner renderers.Runner) ([]string, error) {
	if req == nil || req.GetSinceSec() > 30*86400 || req.GetAuditRows() > 1000 {
		return nil, status.Error(codes.InvalidArgument, "support bounds exceeded")
	}
	out, err := runner.Run(ctx, renderers.Command{Path: supportBin, Args: []string{}, Timeout: 30 * time.Second})
	if err != nil || out.ExitCode != 0 || !json.Valid(out.Stdout) {
		return nil, status.Error(codes.Unavailable, "support collector unavailable")
	}
	text := string(out.Stdout)
	if strings.Contains(text, "PRIVATE KEY") || strings.Contains(text, "$argon2") || strings.Contains(text, "$2b$") {
		return nil, status.Error(codes.Internal, "support collector returned sensitive material")
	}
	return []string{strings.TrimSpace(text)}, nil
}
