package vpp

import (
	"errors"
	"fmt"
	"strings"
)

// MaxTagLen is the longest interface tag VPP accepts (sw_interface_tag_add_del carries a
// 64-byte NUL-terminated string).
const MaxTagLen = 63

// ErrTagTooLong is returned by OwnerTag when owner and id do not fit into MaxTagLen bytes.
var ErrTagTooLong = errors.New("vpp: owner tag too long")

// OwnerTag builds the interface tag that marks an object as owned by this agent:
// "<owner>:<id>", where owner is the agent's VRX_OWNER (tests: their VRX_TEST_PREFIX) and id
// is the object id part of the scheduler key (Key.ID()). Retrieve filters dumps with
// ParseOwnerTag so two agents on one VPP never touch each other's objects. Neither owner nor
// id may be empty or contain ":" (owner), NUL or newlines.
func OwnerTag(owner, id string) (string, error) {
	switch {
	case owner == "" || id == "":
		return "", errors.New("vpp: owner and id must not be empty")
	case strings.ContainsAny(owner, ":\x00\r\n") || strings.ContainsAny(id, "\x00\r\n"):
		return "", fmt.Errorf("vpp: invalid characters in owner tag %q:%q", owner, id)
	}
	tag := owner + ":" + id
	if len(tag) > MaxTagLen {
		return "", fmt.Errorf("%w: %d > %d bytes", ErrTagTooLong, len(tag), MaxTagLen)
	}
	return tag, nil
}

// ParseOwnerTag splits a tag produced by OwnerTag. ok is false for tags of other owners and
// for tags not in the owner format. The tag as read back from VPP may carry trailing NULs;
// they are stripped.
func ParseOwnerTag(tag, owner string) (id string, ok bool) {
	tag = strings.TrimRight(tag, "\x00")
	prefix := owner + ":"
	if owner == "" || !strings.HasPrefix(tag, prefix) || len(tag) == len(prefix) {
		return "", false
	}
	return tag[len(prefix):], true
}
