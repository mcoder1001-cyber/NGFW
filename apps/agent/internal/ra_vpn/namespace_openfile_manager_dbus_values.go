package ravpn

import (
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/godbus/dbus/v5"
)

type managerDBusListen struct{ Kind, Address string }
type managerDBusExec struct {
	Path                                                       string
	Arguments                                                  []string
	IgnoreFailure                                              bool
	StartRealtime, StartMonotonic, ExitRealtime, ExitMonotonic uint64
	PID                                                        uint32
	Code, Status                                               int32
}

func managerDBusText(value string) bool {
	return len(value) <= 4096 && utf8.ValidString(value) && !strings.ContainsAny(value, "\x00\r\n")
}

// managerDBusPropertyValue keeps the existing textual consumer predicates but
// obtains their inputs from strictly typed, bounded native property replies.
func managerDBusPropertyValue(key string, variant dbus.Variant) (string, error) {
	signature := variant.Signature().String()
	switch key {
	case "Id", "FragmentPath", "ActiveState", "SubState", "ControlGroup", "User", "Group":
		v, ok := variant.Value().(string)
		if signature != "s" || !ok || !managerDBusText(v) {
			return "", ErrBoundary
		}
		return v, nil
	case "DropInPaths":
		v, ok := variant.Value().([]string)
		if signature != "as" || !ok || len(v) > 64 {
			return "", ErrBoundary
		}
		for _, s := range v {
			if !managerDBusText(s) || strings.ContainsRune(s, ' ') {
				return "", ErrBoundary
			}
		}
		value := strings.Join(v, " ")
		if !managerDBusText(value) {
			return "", ErrBoundary
		}
		return value, nil
	case "MainPID", "ControlPID":
		v, ok := variant.Value().(uint32)
		if signature != "u" || !ok || v > 2147483647 {
			return "", ErrBoundary
		}
		return strconv.FormatUint(uint64(v), 10), nil
	case "CapabilityBoundingSet":
		v, ok := variant.Value().(uint64)
		if signature != "t" || !ok {
			return "", ErrBoundary
		}
		if v == 0 {
			return "", nil
		}
		return strconv.FormatUint(v, 10), nil
	case "NoNewPrivileges":
		v, ok := variant.Value().(bool)
		if signature != "b" || !ok {
			return "", ErrBoundary
		}
		if v {
			return "yes", nil
		}
		return "no", nil
	case "Listen":
		if signature != "a(ss)" {
			return "", ErrBoundary
		}
		var v []managerDBusListen
		if variant.Store(&v) != nil || len(v) != 1 || !managerDBusText(v[0].Kind) || !managerDBusText(v[0].Address) {
			return "", ErrBoundary
		}
		return v[0].Address + " (" + v[0].Kind + ")", nil
	case "ExecStart":
		if signature != "a(sasbttttuii)" {
			return "", ErrBoundary
		}
		var v []managerDBusExec
		if variant.Store(&v) != nil || len(v) != 1 || len(v[0].Arguments) == 0 || len(v[0].Arguments) > 64 || !managerDBusText(v[0].Path) || strings.ContainsAny(v[0].Path, " ;{}") {
			return "", ErrBoundary
		}
		for _, argument := range v[0].Arguments {
			if !managerDBusText(argument) || strings.ContainsAny(argument, " ;{}") {
				return "", ErrBoundary
			}
		}
		value := "{ path=" + v[0].Path + " ; argv[]=" + strings.Join(v[0].Arguments, " ") + " ; }"
		if !managerDBusText(value) {
			return "", ErrBoundary
		}
		return value, nil
	}
	return "", ErrBoundary
}
