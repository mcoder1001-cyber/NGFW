package ravpn

import "context"

// numericPublisherPeerStop is an idempotent stop-and-join operation. The peer
// watcher owns disconnect cancellation through the last manager/source/proof
// checks. The terminal reply phase invokes this operation before sending its
// final reply, checks the unchanged context, and rebinds socket timeouts to its
// remaining deadline. It then receives exactly the zero-rights final OK under
// the original finite IPC/whole budget, including EOF and proof checks. Normal
// peer closure after a queued valid OK must not cancel terminal completion.
// No manager subprocess may begin after this transition. Repeated calls (the
// explicit transition and deferred cleanup) must join safely before fd reuse.
type numericPublisherPeerStop func()

func enterNumericPublisherTerminal(ctx context.Context, fd int, stop numericPublisherPeerStop) error {
	if stop == nil {
		return ErrBoundary
	}
	stop()
	// The immutable original phase deadline may have elapsed while the watcher
	// joined. No context is recreated or extended at terminal ownership transfer.
	if ctx.Err() != nil || boundUnitObserverSocket(ctx, fd) != nil {
		return ErrBoundary
	}
	return nil
}

func receiveNumericPublisherTerminalACK(ctx context.Context, fd int) error {
	ack, rights, err := receiveUnitObserverPacket(fd, 0)
	closeUnitObserverFiles(rights)
	if err != nil || string(ack) != "OK" || ctx.Err() != nil {
		return ErrBoundary
	}
	return nil
}
