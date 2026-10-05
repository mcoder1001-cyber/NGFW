package ravpn

import (
	"context"
	"ngfw/agent/internal/vpp/bootid"
)

// NumericOpenFileKind selects one fixed manager supplier configuration family.
// Neither kind permits a caller-supplied path, unit name, descriptor, or content.
type NumericOpenFileKind uint8

const (
	// NumericOpenFileTargets configures the canonical VPP mount-role supplier.
	NumericOpenFileTargets NumericOpenFileKind = iota + 1
	// NumericOpenFileObserver configures an owned RA unit observation supplier.
	NumericOpenFileObserver
)

// NumericOpenFilePublisher publishes a deterministic protected configuration
// derived from a complete canonical process identity. Implementations must
// authenticate that identity and refuse unknown existing configuration.
type NumericOpenFilePublisher interface {
	PublishNumericOpenFile(context.Context, NumericOpenFileKind, string, bootid.Identity) error
}
