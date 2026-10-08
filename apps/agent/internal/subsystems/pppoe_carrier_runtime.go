package subsystems

import (
	"bytes"
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"time"

	"ngfw/agent/internal/descriptors/df6"
	desc "ngfw/agent/internal/descriptors/pppoe"
	"ngfw/agent/internal/renderers"
	"ngfw/agent/internal/renderers/pppoe"
)

func (rt *PppoeRuntime) carrierRootDir() string {
	if rt.carrierRoot != "" {
		return rt.carrierRoot
	}
	return "/var/lib/ngfw/pppoe-carrier"
}
func (rt *PppoeRuntime) carrierHooksDir() string {
	if rt.carrierHooks != "" {
		return rt.carrierHooks
	}
	return "/usr/lib/ngfw/pppoe-carrier-hooks"
}

func carrierUnit(s pppoe.Session) string {
	if s.Carrier != nil {
		return "ngfw-pppoe-carrier@" + s.Carrier.Token() + ".service"
	}
	return "ngfw-pppoe-" + s.HostIf + ".service"
}
func (rt *PppoeRuntime) sessionStateDir(s pppoe.Session) string {
	if s.Carrier != nil {
		return filepath.Join(rt.stateDir, s.Carrier.Token())
	}
	return rt.stateDir
}
func (rt *PppoeRuntime) sessionRenderer(s pppoe.Session) *pppoe.Renderer {
	if s.Carrier != nil {
		return rt.carrierRenderer(s)
	}
	return rt.renderer
}
func (rt *PppoeRuntime) carrierRenderer(s pppoe.Session) *pppoe.Renderer {
	base := filepath.Join(rt.carrierRootDir(), s.Carrier.Token(), "ppp")
	return pppoe.New(pppoe.WithPaths(pppoe.CarrierPaths(base, rt.sessionStateDir(s))))
}
func (rt *PppoeRuntime) carrierLease(ctx context.Context, s pppoe.Session) (desc.CarrierLease, error) {
	if s.Carrier == nil {
		return desc.CarrierLease{}, errors.New("kernel PPP requires an explicit carrier")
	}
	leases, err := (&pppoeCarrierHost{runner: rt.runner}).Inventory(ctx, rt.owner)
	if err != nil {
		return desc.CarrierLease{}, err
	}
	for _, lease := range leases {
		if lease.Token == s.Carrier.Token() {
			if !reflect.DeepEqual(lease.Spec, *s.Carrier) {
				return lease, errors.New("carrier specification changed")
			}
			return lease, nil
		}
	}
	return desc.CarrierLease{}, errors.New("carrier namespace lease is absent")
}

// stopCarrier withdraws readiness before any fallible operation. Reverse scheduler
// dependencies retain both transport TAPs until the daemon stage has stopped.
// Caller holds rt.mu and the agent transaction fence.
func (rt *PppoeRuntime) stopCarrier(ctx context.Context, s pppoe.Session) error {
	delete(rt.carrierReady, s.Iface)
	if old, ok := rt.mirrored[s.Iface]; ok {
		if err := rt.Mirror(ctx, old, false); err != nil {
			return err
		}
		delete(rt.mirrored, s.Iface)
	}
	lease, err := rt.carrierLease(ctx, s)
	if err != nil {
		return err
	}
	if err = (&pppoeCarrierHost{runner: rt.runner}).Withdraw(ctx, lease); err != nil {
		return err
	}
	if err = rt.carrierRenderer(s).StopIPv6(ctx, s.HostIf); err != nil {
		return err
	}
	if _, err = rt.runner.Run(ctx, renderers.Command{Path: pppoe.SystemctlBin, Args: []string{"stop", carrierUnit(s)}, Timeout: 25 * time.Second}); err != nil {
		return errors.New("carrier unit stop failed")
	}
	for _, suffix := range []string{".state", ".state6", ".pd", ".ipv6.pid"} {
		if err = os.Remove(filepath.Join(rt.sessionStateDir(s), s.HostIf+suffix)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

// CarrierFiles returns the exact private product file set for recovery readback.
func (rt *PppoeRuntime) CarrierFiles(sessions []pppoe.Session) (renderers.Files, error) {
	out := renderers.Files{}
	for _, s := range sessions {
		if s.Carrier == nil {
			return nil, errors.New("carrier file readback requires a carrier session")
		}
		files, err := rt.carrierRenderer(s).RenderCarrier(s)
		if err != nil {
			return nil, err
		}
		for _, kind := range []string{"ip-up", "ip-down", "ipv6-up", "ipv6-down"} {
			content, err := os.ReadFile(filepath.Join(rt.carrierHooksDir(), kind))
			if err != nil {
				return nil, errors.New("packaged PPP hook dispatcher is unavailable")
			}
			base := filepath.Join(rt.carrierRootDir(), s.Carrier.Token(), "ppp")
			files[filepath.Join(base, kind)] = renderers.File{Content: content, Mode: 0755}
		}
		for path, file := range files {
			out[path] = file
		}
	}
	return out, nil
}

func (rt *PppoeRuntime) applyCarriers(ctx context.Context, sessions []pppoe.Session) error {
	want := map[string]pppoe.Session{}
	rendered := map[string]renderers.Files{}
	for _, s := range sessions {
		if s.Carrier == nil {
			return errors.New("product PPP requires a distinct logical interface and explicit kernel carrier parent")
		}
		files, err := rt.CarrierFiles([]pppoe.Session{s})
		if err != nil {
			return err
		}
		rendered[s.Iface] = files
		copy := s
		copy.Password = ""
		want[s.Iface] = copy
	}
	rt.mu.Lock()
	defer rt.mu.Unlock()
	if rt.carrierReady == nil {
		rt.carrierReady = map[string]carrierForwarding{}
	}
	// Compute edits before mutating any session. Every partial transition remains
	// remembered in applied and fenced by its persistent pending marker for retry.
	changed := map[string]bool{}
	for name, next := range want {
		old, ok := rt.applied[name]
		changed[name] = !ok || !reflect.DeepEqual(old, next)
		for path, file := range rendered[name] {
			body, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(body, file.Content) {
				changed[name] = true
			}
		}
		if _, err := os.Stat(filepath.Join(rt.sessionStateDir(next), "ipv6-transitions", next.HostIf)); err == nil {
			changed[name] = true
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	for name, old := range rt.applied {
		if _, exists := want[name]; exists && !changed[name] {
			continue
		}
		if old.Carrier == nil {
			return errors.New("legacy product PPP session requires explicit stopped migration")
		}
		if err := rt.stopCarrier(ctx, old); err != nil {
			return err
		}
		if _, exists := want[name]; !exists {
			oldFiles, err := rt.carrierRenderer(old).RenderCarrier(old)
			if err != nil {
				return err
			}
			for path := range oldFiles {
				if err = os.Remove(path); err != nil && !os.IsNotExist(err) {
					return err
				}
			}
			delete(rt.applied, name)
		}
	}
	host := &pppoeCarrierHost{runner: rt.runner}
	for name, s := range want {
		if !changed[name] {
			continue
		}
		// Remember before the first start; rollback can stop a partially started unit.
		rt.applied[name] = s
		r := rt.carrierRenderer(s)
		if err := r.StopIPv6(ctx, s.HostIf); err != nil {
			return err
		}
		lease, err := rt.carrierLease(ctx, s)
		if err != nil {
			return err
		}
		ifs, err := df6.DumpInterfaces(ctx, rt.vpp, rt.owner)
		if err != nil {
			return err
		}
		index, err := ifs.Index(s.Carrier.Parent)
		if err != nil {
			return err
		}
		details, ok := ifs.Table().Details(uint32(index))
		if !ok {
			return errors.New("raw PPP parent disappeared")
		}
		if err = host.Withdraw(ctx, lease); err != nil {
			return err
		}
		if err = host.Prepare(ctx, lease, net.HardwareAddr(details.L2Address[:]).String()); err != nil {
			return err
		}
		for path := range rendered[name] {
			if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				return err
			}
		}
		if err = renderers.WriteFiles(rendered[name]); err != nil {
			return err
		}
		if err = r.ResumeIPv6(s.HostIf); err != nil {
			return err
		}
		if _, err = rt.runner.Run(ctx, renderers.Command{Path: pppoe.SystemctlBin, Args: []string{"start", carrierUnit(s)}, Timeout: 25 * time.Second}); err != nil {
			return errors.New("carrier unit start failed")
		}
		if err = r.CompleteIPv6Transition(s.HostIf); err != nil {
			return err
		}
	}
	return nil
}
