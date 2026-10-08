package desired

// services.hostStack (F-host-stack) ↔ descriptors/hoststack:
//
//	hostStack.enabled = true               → hoststack.session/global        (write-only; owner sets, others require)
//	hostStack.namespaces.<id>              → hoststack.namespace/<id>        (write-only; keyed generation resolved at VPP call)
//	hostStack.sessionRules[] (tag)         → hoststack.session-rule/<tag>    (Retrieve: session_rules_v2_dump)
//	hostStack.tcpSourceAddresses           → hoststack.tcp-src/<fib>         (write-only, irreversible per fib)
//	hostStack.httpStatic (enabled)         → hoststack.http-static/global    (globals owner + NGFW_HOSTSTACK_HTTP_STATIC=1)
//
// Only session rules can be read back, so HostStackAssemble reports only them.

import (
	"context"
	"os"
	"sort"
	"strconv"
	"strings"

	"google.golang.org/protobuf/proto"

	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/dfkit"
	"ngfw/agent/internal/descriptors/hoststack"
	"ngfw/agent/internal/descriptors/vpn"
	"ngfw/agent/internal/renderers/rfkit"
	"ngfw/agent/internal/scheduler"
)

func init() { ServicesImplemented["hostStack"] = true }

// RuleWriteOnly marks a leaf the agent applies but VPP cannot report (D-147); the API's drift view
// ignores it like agent.unsupported-field.
const RuleWriteOnly = "agent.write-only-field"

// HostStack projects services.hostStack.
func HostStack(s Sink, hs *ngfwv1.HostStackService, vrfID func(string) (uint32, bool), options ...HostSecretOptions) {
	if hs == nil {
		return
	}
	base := []string{"services", "hostStack"}
	pt := func(segs ...string) string { return Ptr(append(append([]string{}, base...), segs...)...) }
	// D-147: write-only leaves (no VPP getter/dump) are applied but never retrieved; the note keeps
	// the API's running-vs-actual drift from reporting them forever (COVERAGE_RULES).
	writeOnly := func(leaf string) {
		s.Warnf(pt(leaf), RuleWriteOnly, "services.hostStack.%s is write-only in VPP: applied, but Retrieve cannot report it", leaf)
	}
	if hs.Enabled != nil {
		writeOnly("enabled")
	}
	if len(hs.GetNamespaces()) > 0 {
		writeOnly("namespaces")
	}
	if hs.GetTcpSourceAddresses() != nil {
		writeOnly("tcpSourceAddresses")
	}
	if hs.GetHttpStatic() != nil {
		writeOnly("httpStatic")
	}
	if hs.GetEnabled() {
		s.Add(hoststack.KeySession, hoststack.Session{Enabled: true}.Proto(), pt("enabled"))
	}
	for _, id := range sortedKeys(hs.GetNamespaces()) {
		n := hs.GetNamespaces()[id]
		generation := ""
		if ref := n.GetSecretRef(); ref != "" {
			if len(options) == 0 || options[0].Ref == nil || rfkit.CheckRef(ref, "key") != nil {
				s.Errorf(pt("namespaces", id, "secretRef"), "services.host-stack-secret-channel", "namespace secret generation is unavailable")
				continue
			}
			var err error
			generation, err = options[0].Ref(context.Background(), ref)
			if err != nil || !strings.HasPrefix(generation, vpn.RefHMAC) || vpn.CheckRef(generation) != nil {
				s.Errorf(pt("namespaces", id, "secretRef"), "services.host-stack-secret-channel", "namespace secret generation is unavailable")
				continue
			}
		}
		if err := hoststack.ValidID(id); err != nil {
			s.Errorf(pt("namespaces", id), "services.host-stack-id", "%v", err)
			continue
		}
		fib, ok := vrfID(n.GetVrf())
		if !ok {
			s.Errorf(pt("namespaces", id, "vrf"), "services.host-stack-vrf", "VRF %q does not exist", n.GetVrf())
			continue
		}
		s.Add(hoststack.KeyNamespace(id), hoststack.Namespace{ID: id, Interface: n.GetInterface(), Vrf: fib, SecretGeneration: generation}.Proto(), pt("namespaces", id))
	}
	for i, r := range hs.GetSessionRules() {
		v := hoststack.Rule{Tag: r.GetTag(), Scope: r.GetScope(), Transport: r.GetTransport(), Local: r.GetLocal(),
			LocalPort: r.GetLocalPort(), Remote: r.GetRemote(), RemotePort: r.GetRemotePort(), Action: r.GetAction(),
			RedirectAppIndex: r.GetRedirectAppIndex(), AppNamespace: r.GetAppNamespace()}
		if v.Scope == "" {
			v.Scope = "global"
		}
		rp := pt("sessionRules", strconv.Itoa(i))
		if err := hoststack.ValidID(v.Tag); err != nil {
			s.Errorf(rp+"/tag", "services.host-stack-id", "%v", err)
			continue
		}
		if _, err := dfkit.ParsePrefix(v.Local); err != nil {
			s.Errorf(rp+"/local", "services.host-stack-prefix", "%v", err)
			continue
		}
		if _, err := dfkit.ParsePrefix(v.Remote); err != nil {
			s.Errorf(rp+"/remote", "services.host-stack-prefix", "%v", err)
			continue
		}
		if v.AppNamespace != "" {
			if _, ok := hs.GetNamespaces()[v.AppNamespace]; !ok {
				s.Errorf(rp+"/appNamespace", "services.host-stack-namespace", "app namespace %q does not exist", v.AppNamespace)
				continue
			}
		}
		s.Add(hoststack.KeyRule(v.Tag), v.Proto(), rp)
	}
	if t := hs.GetTcpSourceAddresses(); t != nil {
		fib, ok := vrfID(t.GetVrf())
		if !ok {
			s.Errorf(pt("tcpSourceAddresses", "vrf"), "services.host-stack-vrf", "VRF %q does not exist", t.GetVrf())
		} else {
			s.Add(hoststack.KeyTCPSrc(fib), hoststack.TCPSrc{First: t.GetFirst(), Last: t.GetLast(), Vrf: fib}.Proto(), pt("tcpSourceAddresses"))
		}
	}
	if h := hs.GetHttpStatic(); h != nil && h.GetEnabled() {
		switch {
		case os.Getenv(hoststack.EnvHTTPStatic) != "1":
			s.Errorf(pt("httpStatic", "enabled"), "services.host-stack-http-static",
				"http_static is opt-in on the agent (%s=1, globals owner only): it cannot be disabled once enabled", hoststack.EnvHTTPStatic)
		case hoststack.ValidURI(h.GetUri()) != nil:
			s.Errorf(pt("httpStatic", "uri"), "services.host-stack-uri", "%v", hoststack.ValidURI(h.GetUri()))
		case hoststack.ValidWWWRoot(h.GetWwwRootPath()) != nil:
			s.Errorf(pt("httpStatic", "wwwRootPath"), "services.host-stack-www-root", "%v", hoststack.ValidWWWRoot(h.GetWwwRootPath()))
		default:
			mb := h.GetCacheSizeMb()
			if mb == 0 {
				mb = 10
			}
			s.Add(hoststack.KeyHTTPStatic, hoststack.HTTPStatic{WWWRoot: h.GetWwwRootPath(), URI: h.GetUri(), CacheSizeMB: mb}.Proto(), pt("httpStatic"))
		}
	}
}

// HostStackAssemble adds the retrievable part of services.hostStack (this owner's session rules)
// to ds. Write-only objects (session, namespaces, TCP pool, http_static) are never echoed (D-063).
func HostStackAssemble(ds *ngfwv1.DesiredState, kvs []scheduler.KV) {
	var rules []hoststack.Rule
	for _, kv := range kvs {
		if kv.Key.Descriptor() != hoststack.NameSessionRule {
			continue
		}
		var r hoststack.Rule
		if err := dfkit.Decode(kv.Value, &r); err == nil {
			rules = append(rules, r)
		}
	}
	if len(rules) == 0 {
		return
	}
	sort.Slice(rules, func(i, j int) bool { return rules[i].Tag < rules[j].Tag })
	if ds.Services == nil {
		ds.Services = &ngfwv1.ServicesConfig{}
	}
	hs := &ngfwv1.HostStackService{}
	for _, r := range rules {
		m := &ngfwv1.HostStackSessionRule{Tag: proto.String(r.Tag), Scope: proto.String(r.Scope), Transport: proto.String(r.Transport),
			Local: proto.String(r.Local), Remote: proto.String(r.Remote), Action: proto.String(r.Action)}
		if r.LocalPort != 0 {
			m.LocalPort = proto.Uint32(r.LocalPort)
		}
		if r.RemotePort != 0 {
			m.RemotePort = proto.Uint32(r.RemotePort)
		}
		if r.Action == "redirect" {
			m.RedirectAppIndex = proto.Uint32(r.RedirectAppIndex)
		}
		if r.AppNamespace != "" {
			m.AppNamespace = proto.String(r.AppNamespace)
		}
		hs.SessionRules = append(hs.SessionRules, m)
	}
	ds.Services.HostStack = hs
}
