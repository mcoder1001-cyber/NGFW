package ravpn

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"golang.org/x/sys/unix"
	"math"
	"ngfw/agent/internal/vpp/bootid"
	"sync"
	"time"
)

// NumericPublisherWorkBudget bounds publication and activation separately from
// request/reply IPC. The original whole-operation deadline always clips it.
const NumericPublisherWorkBudget = 15 * time.Second

// NumericPublisherServerWholeBudget is one immutable server lifetime budget.
// Phase maxima are clipped by it and are not an additive timing promise.
const NumericPublisherServerWholeBudget = NumericOpenFilePublicationBudget - NumericPublisherCleanupBudget

// numericPublisherWorkFrame is a closed publish-only transition. A complete
// frame carries exactly one held canonical source executable; its acknowledgment
// carries zero rights. Both bind the exact already-authenticated source/server.
// A fresh unpredictable 64hex token is created only after work completes; an
// early acknowledgment cannot predict it. It is never logged. Final success
// remains unavailable until fresh proof checks and exact acknowledgment.
type numericPublisherWorkFrame struct {
	Version int             `json:"version"`
	Phase   string          `json:"phase"`
	Source  bootid.Identity `json:"source"`
	Server  bootid.Identity `json:"server"`
	Token   string          `json:"token"`
}

func validateNumericPublisherWorkFrame(frame numericPublisherWorkFrame, phase string, source, server bootid.Identity, token string) error {
	if !ValidInstance(token) || frame.Token != token || (phase != "publication-complete" && phase != "publication-ack") || source.PID <= 1 || source.PID > math.MaxInt32 || server.PID <= 1 || server.PID > math.MaxInt32 || frame.Version != 1 || frame.Phase != phase || !source.Complete() || !server.Complete() || source.Equal(server) || !frame.Source.Equal(source) || !frame.Server.Equal(server) {
		return ErrBoundary
	}
	return nil
}

func boundNumericPublisherWorkSocket(ctx context.Context, fd int) error {
	if ctx.Err() != nil {
		return ErrBoundary
	}
	duration := NumericPublisherWorkBudget
	if deadline, ok := ctx.Deadline(); ok {
		duration = min(duration, time.Until(deadline))
	}
	if duration <= 0 {
		return ErrBoundary
	}
	timeout := unix.NsecToTimeval(max(duration.Nanoseconds(), int64(1000)))
	if unix.SetsockoptTimeval(fd, unix.SOL_SOCKET, unix.SO_RCVTIMEO, &timeout) != nil || unix.SetsockoptTimeval(fd, unix.SOL_SOCKET, unix.SO_SNDTIMEO, &timeout) != nil {
		return ErrBoundary
	}
	return nil
}

func sendNumericPublisherWorkFrame(fd int, phase string, source, server bootid.Identity, token string, image int) error {
	frame := numericPublisherWorkFrame{Version: 1, Phase: phase, Source: source, Server: server, Token: token}
	if validateNumericPublisherWorkFrame(frame, phase, source, server, token) != nil {
		return ErrBoundary
	}
	var rights []byte
	if phase == "publication-complete" {
		if image < 0 {
			return ErrBoundary
		}
		rights = unix.UnixRights(image)
	} else if image >= 0 {
		return ErrBoundary
	}
	data, err := json.Marshal(frame)
	if err != nil || unix.Sendmsg(fd, data, rights, nil, 0) != nil {
		return ErrBoundary
	}
	return nil
}

// The watcher never consumes packets. POLLIN can mean a legitimate queued
// acknowledgment and is deliberately ignored. Its owner joins before fd reuse.
func watchNumericPublisherPeer(ctx context.Context, fd int, cancel context.CancelFunc) numericPublisherPeerStop {
	if fd < 0 || fd > math.MaxInt32 {
		cancel()
		return func() {}
	}
	pollFD := int32(fd)
	stop := make(chan struct{})
	exited := make(chan struct{})
	go func() {
		defer close(exited)
		for {
			select {
			case <-stop:
				return
			case <-ctx.Done():
				_ = unix.Shutdown(fd, unix.SHUT_RDWR)
				return
			default:
			}
			fds := []unix.PollFd{{Fd: pollFD, Events: unix.POLLHUP | unix.POLLRDHUP | unix.POLLERR}}
			_, err := unix.Poll(fds, 50)
			if err == unix.EINTR {
				continue
			}
			if err != nil || fds[0].Revents&(unix.POLLHUP|unix.POLLRDHUP|unix.POLLERR|unix.POLLNVAL) != 0 {
				cancel()
				_ = unix.Shutdown(fd, unix.SHUT_RDWR)
				return
			}
		}
	}()
	var stopOnce sync.Once
	return func() { stopOnce.Do(func() { close(stop) }); <-exited }
}

func newNumericPublisherWorkToken() (string, error) {
	var token [32]byte
	if _, err := rand.Read(token[:]); err != nil {
		return "", ErrBoundary
	}
	return hex.EncodeToString(token[:]), nil
}

// This boundary check does not receive or duplicate queued ancillary rights.
// Unexpected input (including a second acknowledgment) fails closed; socket
// closure releases all rights still owned by the kernel receive queue.
func noNumericPublisherQueuedInput(fd int) error {
	if fd < 0 || fd > math.MaxInt32 {
		return ErrBoundary
	}
	fds := []unix.PollFd{{Fd: int32(fd), Events: unix.POLLIN | unix.POLLHUP | unix.POLLRDHUP | unix.POLLERR}}
	if _, err := unix.Poll(fds, 0); err != nil || fds[0].Revents != 0 {
		return ErrBoundary
	}
	return nil
}
