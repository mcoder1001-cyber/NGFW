package subsystems

import (
	"context"
	"errors"
	"ngfw/agent/internal/descriptors/core"
	"ngfw/agent/internal/descriptors/pppoe"
	"ngfw/agent/internal/descriptors/vpn"
	"ngfw/agent/internal/descriptors/vrrp"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/vpp"
	"path/filepath"
	"sync/atomic"
)

var pppoeActive atomic.Pointer[PppoeRuntime]
var pppoeConfigs = map[string]*pppoe.ClientConfig{}

func init() { Domains[Interfaces] = append(Domains[Interfaces], pppoe.ClientConfigName) }

// PppoeSupervised reports whether this process may supervise host units.
func PppoeSupervised() bool { rt := pppoeActive.Load(); return rt != nil && rt.globalsOwner }

// SetPppoeSecrets injects the existing socket-only sealed secret cache.
func SetPppoeSecrets(owner string, r vpn.Resolver) error {
	pppoeMu.Lock()
	d := pppoeConfigs[owner]
	pppoeMu.Unlock()
	if d == nil {
		return errors.New("PPPoE client is not registered")
	}
	d.SetResolver(r)
	return nil
}
func (w *Wiring) registerPppoeClient(reg scheduler.Registry) error {
	rt := PppoeOf(w.env.Owner)
	pppoeActive.Store(rt)
	d := pppoe.NewClientConfig(rt, rt.renderer, filepath.Join(w.env.StateDir, "pppoe-"+w.env.Owner+"-applied.pb"))
	reg.Register(d)
	pppoeMu.Lock()
	pppoeConfigs[w.env.Owner] = d
	pppoeMu.Unlock()
	if lookup, ok := reg.(interface {
		ForKey(scheduler.Key) (scheduler.Descriptor, bool)
	}); ok {
		if descriptor, ok := lookup.ForKey(core.InterfaceAddrKey("x", "192.0.2.1/32")); ok {
			if source, ok := descriptor.(interface {
				SetVirtualAddressSource(core.VirtualAddressSource)
			}); ok {
				source.SetVirtualAddressSource(func(ctx context.Context, c vpp.Client, owner string) (map[uint32]map[string]bool, error) {
					addrs := map[uint32]map[string]bool{}
					if vrrpVPPEnabled.Load() {
						var err error
						addrs, err = vrrp.OwnedVirtualAddresses(ctx, c, owner)
						if err != nil {
							return nil, err
						}
					}
					return rt.runtimeAddresses(ctx, addrs)
				})
			}
		}
	}
	reg.Register(&pppoeObservation{})
	if err := w.AddDynamicSource(DynamicSource{Name: "pppoe-watch", Descriptors: []string{"pppoe.client.observation"}, Desired: rt.watchDesired, Run: rt.watch}); err != nil {
		return err
	}
	w.OnClose(func() {
		pppoeMu.Lock()
		delete(pppoeConfigs, w.env.Owner)
		delete(pppoeReg, w.env.Owner)
		pppoeMu.Unlock()
		pppoeActive.CompareAndSwap(rt, nil)
	})
	return nil
}
