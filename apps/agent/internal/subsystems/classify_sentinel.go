package subsystems

// S-classify-sentinel (INC-vpp-classify-crash M2, D-191): on every VPP (re)connect the globals owner
// (D-071) makes sure the classify table at index 0 exists (ifsanitize.EnsureSentinel), so an interface
// address armed by VPP's zero-filled ip classify vector drops its packets instead of crashing VPP. It
// runs first in Connected — before the boot identity and the resync — to win the race to index 0 after
// a VPP start. Test slots are never the globals owner and do nothing here. A failure is logged, never
// fatal: the agent works without the sentinel exactly as before.

import (
	"context"
	"errors"
	"time"

	"ngfw/agent/internal/vpp/ifsanitize"
)

// sentinelTimeout bounds one EnsureSentinel run (at most MaxSentinelPops creates and deletes).
var sentinelTimeout = 20 * time.Second

func (w *Wiring) classifySentinelConnected(ctx context.Context) {
	if !w.env.GlobalsOwner {
		return
	}
	sctx, cancel := context.WithTimeout(ctx, sentinelTimeout)
	defer cancel()
	rep, err := ifsanitize.EnsureSentinel(sctx, w.env.Client)
	log := w.env.Log.With("family", "classify-sentinel", "state", string(rep.State), "left", rep.Left)
	switch {
	case errors.Is(err, ifsanitize.ErrSentinelCapped):
		log.Error("classify table sentinel: index 0 did not come back; armed classify addresses can crash VPP (INC-vpp-classify-crash)",
			"report", rep.String(), "err", err)
	case err != nil:
		log.Error("classify table sentinel failed; armed classify addresses can crash VPP (INC-vpp-classify-crash)",
			"report", rep.String(), "err", err)
	case rep.State == ifsanitize.SentinelTaken:
		log.Warn("classify table sentinel: index 0 is another client's table; it protects armed addresses only while it lives",
			"report", rep.String())
	case rep.State == ifsanitize.SentinelCreated:
		log.Info("classify table sentinel created at index 0 (INC-vpp-classify-crash M2)", "report", rep.String())
	default:
		log.Debug("classify table sentinel present", "report", rep.String())
	}
}
