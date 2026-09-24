package wireguard

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"ngfw/agent/binapi/interface_types"
	"ngfw/agent/binapi/wireguard"
	"ngfw/agent/internal/scheduler"
)

// PeerEvent is one WireGuard peer status change, as StreamEvents publishes it:
//
//	{ "key": "wireguard.peer/wg4001/<base64 public key>", "interface": "wg4001",
//	  "public_key": "<base64>", "peer_index": 3, "established": true, "dead": false }
//
// It is built from wireguard_peer_event (peer_index + wireguard_peer_flags: STATUS_DEAD=1,
// ESTABLISHED=2) and carries no key material. Only peers of interfaces owned by this agent are
// reported.
type PeerEvent struct {
	Key         scheduler.Key `json:"key"`
	Interface   string        `json:"interface"`
	PublicKey   string        `json:"public_key"`
	PeerIndex   uint32        `json:"peer_index"`
	Established bool          `json:"established"`
	Dead        bool          `json:"dead"`
}

// ErrEventsActive is returned when Events is called while a subscription is running.
var ErrEventsActive = errors.New("wireguard: peer event subscription already active")

// Events subscribes to peer up/down events (want_wireguard_peer_events + wireguard_peer_event) and
// delivers them until ctx is cancelled; the channel is then closed and the subscription disabled.
//
// VPP registers the client only on peers that exist when want_wireguard_peer_events is sent, so
// while a subscription is active Create registers every new peer as well. One subscription per
// descriptor at a time.
func (d *Peer) Events(ctx context.Context) (<-chan PeerEvent, error) {
	d.mu.Lock()
	if d.events {
		d.mu.Unlock()
		return nil, ErrEventsActive
	}
	d.events, d.pid = true, uint32(os.Getpid()) //nolint:gosec // pids fit
	pid := d.pid
	d.mu.Unlock()
	stop := func() {
		d.mu.Lock()
		d.events = false
		d.mu.Unlock()
	}

	w, err := d.cfg.Client.WatchEvent(ctx, &wireguard.WireguardPeerEvent{})
	if err != nil {
		stop()
		return nil, fmt.Errorf("watch wireguard_peer_event: %w", err)
	}
	svc := wireguard.NewServiceClient(d.cfg.Client)
	want := func(ctx context.Context, on uint32) error {
		_, err := svc.WantWireguardPeerEvents(ctx, &wireguard.WantWireguardPeerEvents{
			SwIfIndex: interface_types.InterfaceIndex(noInterface), PeerIndex: noInterface, EnableDisable: on, PID: pid,
		})
		return err
	}
	if err := want(ctx, 1); err != nil {
		w.Close()
		stop()
		return nil, fmt.Errorf("want_wireguard_peer_events: %w", err)
	}

	out := make(chan PeerEvent, 64)
	go func() {
		defer close(out)
		defer func() {
			stop()
			w.Close()
			cctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_ = want(cctx, 0) // best effort: VPP also drops the registration with the client
		}()
		for {
			select {
			case <-ctx.Done():
				return
			case m, ok := <-w.Events():
				if !ok {
					return
				}
				ev, ok := m.(*wireguard.WireguardPeerEvent)
				if !ok {
					continue
				}
				ref, owned := d.lookup(ctx, ev.PeerIndex)
				if !owned {
					continue
				}
				pe := PeerEvent{
					Key: ref.key, Interface: ref.iface, PublicKey: ref.publicKey, PeerIndex: ev.PeerIndex,
					Established: ev.Flags&wireguard.WIREGUARD_PEER_ESTABLISHED != 0,
					Dead:        ev.Flags&wireguard.WIREGUARD_PEER_STATUS_DEAD != 0,
				}
				select {
				case out <- pe:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out, nil
}

// lookup maps a peer index to its key by reading the peer at that index (indexes are reused, so
// nothing is cached) and checking that its interface is ours.
func (d *Peer) lookup(ctx context.Context, idx uint32) (peerRef, bool) {
	ref, found, err := d.peerAt(ctx, idx)
	if err != nil || !found || !ref.owned {
		return peerRef{}, false
	}
	return ref, true
}
