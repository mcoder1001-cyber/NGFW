package ravpn

import (
	"context"
	"os/exec"
	"strings"
	"time"
	"unicode/utf8"
)

// numericPublisherManagerSnapshot binds two fresh property blocks by their exact
// Id, never by position. Only each role's required properties are mandatory;
// additional properties must belong to the fixed query union. Empty values are
// preserved, and duplicate keys or missing, duplicate or foreign units fail closed.
// A single multi-unit query is sequential manager readback, not an atomic snapshot.
// Every existing held-proof and live process check still surrounds its consumption.
type numericPublisherManagerSnapshot struct {
	socket  map[string]string
	service map[string]string
}

const numericPublisherManagerProperties = "Id,FragmentPath,DropInPaths,ActiveState,SubState,Listen,MainPID,ControlPID,ControlGroup,User,Group,CapabilityBoundingSet,NoNewPrivileges,ExecStart"

// numericPublisherManagerCommand accepts no caller-selected unit or properties.
func numericPublisherManagerCommand(ctx context.Context) *exec.Cmd {
	command := exec.CommandContext(ctx, "/usr/bin/systemctl", "show", "--all", "--property="+numericPublisherManagerProperties, "ngfw-ra-openfile.socket", "ngfw-ra-openfile.service")
	command.WaitDelay = time.Second
	return command
}

func numericPublisherManagerPair(ctx context.Context) (numericPublisherManagerSnapshot, error) {
	bounded, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	output, err := numericPublisherManagerCommand(bounded).Output()
	if err != nil || bounded.Err() != nil {
		return numericPublisherManagerSnapshot{}, ErrBoundary
	}
	return parseNumericPublisherManagerPair(output)
}

func parseNumericPublisherManagerPair(output []byte) (numericPublisherManagerSnapshot, error) {
	var snapshot numericPublisherManagerSnapshot
	if len(output) == 0 || len(output) > 16384 || !utf8.Valid(output) || strings.ContainsAny(string(output), "\x00\r") {
		return snapshot, ErrBoundary
	}
	allowed := make(map[string]bool)
	for _, key := range strings.Split(numericPublisherManagerProperties, ",") {
		allowed[key] = true
	}
	blocks := strings.Split(strings.TrimSuffix(string(output), "\n"), "\n\n")
	if len(blocks) != 2 {
		return snapshot, ErrBoundary
	}
	for _, block := range blocks {
		fields := make(map[string]string)
		for _, line := range strings.Split(block, "\n") {
			key, value, ok := strings.Cut(line, "=")
			if !ok || !allowed[key] {
				return numericPublisherManagerSnapshot{}, ErrBoundary
			}
			if _, exists := fields[key]; exists {
				return numericPublisherManagerSnapshot{}, ErrBoundary
			}
			fields[key] = value
		}
		var required string
		switch fields["Id"] {
		case "ngfw-ra-openfile.socket":
			if snapshot.socket != nil {
				return numericPublisherManagerSnapshot{}, ErrBoundary
			}
			snapshot.socket = fields
			required = "Id,FragmentPath,DropInPaths,ActiveState,SubState,Listen"
		case "ngfw-ra-openfile.service":
			if snapshot.service != nil {
				return numericPublisherManagerSnapshot{}, ErrBoundary
			}
			snapshot.service = fields
			required = "Id,MainPID,ControlPID,ActiveState,SubState,ControlGroup,FragmentPath,DropInPaths,User,Group,CapabilityBoundingSet,NoNewPrivileges,ExecStart"
		default:
			return numericPublisherManagerSnapshot{}, ErrBoundary
		}
		for _, key := range strings.Split(required, ",") {
			if _, exists := fields[key]; !exists {
				return numericPublisherManagerSnapshot{}, ErrBoundary
			}
		}
	}
	if snapshot.socket == nil || snapshot.service == nil {
		return numericPublisherManagerSnapshot{}, ErrBoundary
	}
	return snapshot, nil
}
