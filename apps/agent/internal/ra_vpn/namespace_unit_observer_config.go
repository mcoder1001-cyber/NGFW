package ravpn

import (
	"bytes"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"ngfw/agent/internal/vpp/bootid"
)

var observerBootUUID = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{4}-[a-f0-9]{12}$`)

// ObserverOpenFilePath derives the sole permitted runtime drop-in path from a
// canonical manager-observed PID. It never accepts a caller-chosen path.
func ObserverOpenFilePath(pid int) (string, error) {
	if pid <= 1 || pid > math.MaxInt32 {
		return "", ErrEngine
	}
	return "/run/systemd/system/ngfw-ra-observer@" + strconv.Itoa(pid) + ".service.d/10-openfile.conf", nil
}

// RenderObserverOpenFile renders literal paths because systemd OpenFile does
// not expand instance specifiers. The trusted publisher verifies live target
// identity independently; this pure renderer also supports authenticating a
// previous complete generation before a proven-dead replacement.
func RenderObserverOpenFile(instance string, target bootid.Identity) ([]byte, error) {
	if !ValidInstance(instance) || !target.Complete() || !observerBootUUID.MatchString(target.BootID) {
		return nil, ErrEngine
	}
	if _, err := ObserverOpenFilePath(target.PID); err != nil {
		return nil, ErrEngine
	}
	numeric := strconv.Itoa(target.PID)
	content := fmt.Sprintf("# ngfw-ra-observer-v1 instance=%s boot=%s pid=%s start=%d\n[Service]\nOpenFile=\nOpenFile=/proc/%s/ns/net:unit-net:read-only\nOpenFile=/proc/%s/exe:unit-exe:read-only\nOpenFile=%s:source-agent-exe:read-only\n", instance, target.BootID, numeric, target.StartTime, numeric, numeric, sourceAgentExecutableReference)
	if len(content) > 1024 {
		return nil, ErrEngine
	}
	return []byte(content), nil
}

// ValidateObserverOpenFile accepts only the exact bounded generated content
// for the requested instance and complete target identity, never generic drop-ins.
func ValidateObserverOpenFile(instance string, target bootid.Identity, data []byte) error {
	expected, err := RenderObserverOpenFile(instance, target)
	if err != nil || !bytes.Equal(expected, data) {
		return ErrEngine
	}
	return nil
}

// ParseObserverOpenFile authenticates the full ownership marker and every byte
// of the fixed configuration before a publisher may consider dead-owner refresh.
func ParseObserverOpenFile(data []byte) (string, bootid.Identity, error) {
	if len(data) == 0 || len(data) > 1024 {
		return "", bootid.Identity{}, ErrEngine
	}
	header, _, ok := strings.Cut(string(data), "\n")
	fields := strings.Fields(header)
	if !ok || len(fields) != 6 || fields[0] != "#" || fields[1] != "ngfw-ra-observer-v1" {
		return "", bootid.Identity{}, ErrEngine
	}
	values := make([]string, 4)
	for index, key := range []string{"instance=", "boot=", "pid=", "start="} {
		if !strings.HasPrefix(fields[index+2], key) {
			return "", bootid.Identity{}, ErrEngine
		}
		values[index] = strings.TrimPrefix(fields[index+2], key)
	}
	pid, err := strconv.Atoi(values[2])
	if err != nil || strconv.Itoa(pid) != values[2] {
		return "", bootid.Identity{}, ErrEngine
	}
	start, err := strconv.ParseUint(values[3], 10, 64)
	if err != nil || strconv.FormatUint(start, 10) != values[3] {
		return "", bootid.Identity{}, ErrEngine
	}
	identity := bootid.Identity{BootID: values[1], PID: pid, StartTime: start}
	if ValidateObserverOpenFile(values[0], identity, data) != nil {
		return "", bootid.Identity{}, ErrEngine
	}
	return values[0], identity, nil
}
