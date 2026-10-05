package ravpn

import "ngfw/agent/internal/vpp/bootid"

// numericPublisherReady is the closed validation-phase frame. Exactly one
// manager-opened canonical source executable FD accompanies it. The client must
// verify the fresh fixed server identity and that held image before starting the
// unchanged five-second request/reply phase; this frame alone grants no trust.
// Each one-shot server validates installation afresh, including both probe and
// publish activations. No installation proof is reused across processes/calls.
type numericPublisherReady struct {
	Version int             `json:"version"`
	Phase   string          `json:"phase"`
	Source  bootid.Identity `json:"source"`
	Server  bootid.Identity `json:"server"`
}

func validateNumericPublisherReady(frame numericPublisherReady, source bootid.Identity) error {
	if frame.Version != 1 || frame.Phase != "validation-ready" || !source.Complete() || !frame.Source.Equal(source) || !frame.Server.Complete() || frame.Server.Equal(source) {
		return ErrBoundary
	}
	return nil
}
