package ravpn

import (
	"ngfw/agent/internal/vpp/bootid"
)

const numericPublisherPacketLimit = 2048
const numericPublisherSocketPath = "/run/ngfw/ra/openfile.sock"
const numericPublisherListenerRole = "openfile-listener"

type numericPublisherRequest struct {
	Phase          string              `json:"phase"`
	Source         bootid.Identity     `json:"source"`
	Kind           NumericOpenFileKind `json:"kind"`
	Instance       string              `json:"instance"`
	Target         bootid.Identity     `json:"target"`
	PreviousServer bootid.Identity     `json:"previousServer"`
}

type numericPublisherResponse struct {
	Phase     string          `json:"phase"`
	Source    bootid.Identity `json:"source"`
	Server    bootid.Identity `json:"server"`
	Published bool            `json:"published"`
}

func validateNumericPublisherRequest(request numericPublisherRequest) error {
	if !request.Source.Complete() || request.Source.PID <= 1 {
		return ErrBoundary
	}
	switch request.Phase {
	case "probe":
		if request.Kind != 0 || request.Instance != "" || request.Target != (bootid.Identity{}) || request.PreviousServer != (bootid.Identity{}) {
			return ErrBoundary
		}
	case "publish":
		if !request.PreviousServer.Complete() || request.PreviousServer.PID <= 1 || !validNumericOpenFileTarget(request.Target) {
			return ErrBoundary
		}
		switch request.Kind {
		case NumericOpenFileTargets:
			if request.Instance != "" {
				return ErrBoundary
			}
		case NumericOpenFileObserver:
			if !ValidInstance(request.Instance) {
				return ErrBoundary
			}
		default:
			return ErrBoundary
		}
	default:
		return ErrBoundary
	}
	return nil
}
