package agent

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"log/slog"
	"math/big"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/ownertable"
	ravpn "ngfw/agent/internal/ra_vpn"
	"ngfw/agent/internal/renderers/strongswan"
	"ngfw/agent/internal/scheduler"
	"ngfw/agent/internal/secretchannel"
	"ngfw/agent/internal/subsystems"
	"ngfw/agent/internal/vpp"
	"ngfw/agent/internal/vpp/bootid"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// This supervisor is test-only. It launches the actual fixed helper solely in
// verified private PID/mount/network namespaces; production VPP verification,
// immutable sealed cache, descriptors and Service.Apply remain real.
type productionRAUnit struct {
	identity ravpn.UnitIdentity
	cmd      *exec.Cmd
	wait     chan error
	stop     bool
}
type productionRAUnits struct {
	t     *testing.T
	mu    sync.Mutex
	units map[string]*productionRAUnit
}

func (u *productionRAUnits) Observe(ctx context.Context, p *ravpn.NetworkPlan) (ravpn.UnitIdentity, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.observe(p)
}
func (u *productionRAUnits) observe(p *ravpn.NetworkPlan) (ravpn.UnitIdentity, error) {
	h, ok := u.units[p.Instance]
	if !ok || h.stop || h.identity.NamespaceInode != p.NamespaceInode {
		return ravpn.UnitIdentity{}, ravpn.ErrEngine
	}
	stat, e := os.Stat("/proc/" + strconv.Itoa(h.identity.PID) + "/ns/net")
	if e != nil || stat.Sys().(*syscall.Stat_t).Ino != p.NamespaceInode || (bootid.Reader{}).ForPID(h.identity.PID).StartTime != h.identity.StartTicks {
		return ravpn.UnitIdentity{}, ravpn.ErrEngine
	}
	actual, e := os.Stat("/proc/" + strconv.Itoa(h.identity.PID) + "/exe")
	expected, ee := os.Stat(filepath.Join(os.Getenv("NGFW_RA_ENGINE_ROOT"), "sbin/charon-systemd"))
	if e != nil || ee != nil || !os.SameFile(actual, expected) {
		return ravpn.UnitIdentity{}, ravpn.ErrEngine
	}
	return h.identity, nil
}
func (u *productionRAUnits) Start(ctx context.Context, p *ravpn.NetworkPlan) (ravpn.UnitIdentity, error) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if _, e := ravpn.ReadAgentPlan(p.Instance); e != nil {
		return ravpn.UnitIdentity{}, ravpn.ErrEngine
	}
	if h, ok := u.units[p.Instance]; ok && !h.stop {
		return ravpn.UnitIdentity{}, ravpn.ErrEngine
	}
	workspace := u.t.TempDir()
	log, e := os.OpenFile(filepath.Join(workspace, "private-unit.log"), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return ravpn.UnitIdentity{}, ravpn.ErrEngine
	}
	u.t.Cleanup(func() { log.Close() })
	mnt, e := os.Open("/proc/self/ns/mnt")
	if e != nil {
		return ravpn.UnitIdentity{}, ravpn.ErrEngine
	}
	defer mnt.Close()
	launcher, e := filepath.Abs("../../../../test/topology/ra-vpn/private-daemon.py")
	if e != nil {
		return ravpn.UnitIdentity{}, ravpn.ErrEngine
	}
	root := filepath.Join(ravpn.InstanceRoot, p.Instance)
	cmd := exec.Command("/usr/bin/nsenter", "--net="+filepath.Join(root, "netns"), "--", "/usr/bin/unshare", "--mount", "--pid", "--fork", "--mount-proc", "/usr/bin/python3", launcher, p.Instance, os.Getenv("NGFW_RA_ENGINE_ROOT"), os.Getenv("NGFW_RA_HELPER"), workspace)
	cmd.Env = []string{"PATH=/usr/sbin:/usr/bin:/sbin:/bin", "LANG=C"}
	cmd.ExtraFiles = []*os.File{mnt}
	cmd.Stdout = log
	cmd.Stderr = log
	if cmd.Start() != nil {
		return ravpn.UnitIdentity{}, ravpn.ErrEngine
	}
	h := &productionRAUnit{cmd: cmd, wait: make(chan error, 1)}
	u.units[p.Instance] = h
	go func() { h.wait <- cmd.Wait() }()
	socket := filepath.Join(root, "daemon/vici.sock")
	for end := time.Now().Add(15 * time.Second); time.Now().Before(end) && ctx.Err() == nil; time.Sleep(20 * time.Millisecond) {
		if h.identity.PID == 0 {
			children, _ := os.ReadFile("/proc/" + strconv.Itoa(cmd.Process.Pid) + "/task/" + strconv.Itoa(cmd.Process.Pid) + "/children")
			for _, candidate := range strings.Fields(string(children)) {
				pid, _ := strconv.Atoi(candidate)
				stat, e := os.Stat("/proc/" + candidate + "/ns/net")
				if e == nil && pid > 1 && stat.Sys().(*syscall.Stat_t).Ino == p.NamespaceInode {
					identity := (bootid.Reader{}).ForPID(pid)
					h.identity = ravpn.UnitIdentity{BootID: identity.BootID, PID: pid, StartTicks: identity.StartTime, NamespaceInode: p.NamespaceInode}
				}
			}
		}
		if h.identity.Valid() && strongswan.RestrictRAVICISocket(ctx, socket, h.identity.PID) == nil {
			return u.observe(p)
		}
	}
	u.stop(h)
	return ravpn.UnitIdentity{}, ravpn.ErrEngine
}
func (u *productionRAUnits) stop(h *productionRAUnit) error {
	if h.stop {
		return nil
	}
	if h.identity.Valid() && (bootid.Reader{}).StartTime(h.identity.PID) == h.identity.StartTicks {
		syscall.Kill(h.identity.PID, syscall.SIGTERM)
	}
	select {
	case <-h.wait:
		h.stop = true
		return nil
	case <-time.After(3 * time.Second):
	}
	if h.identity.Valid() && (bootid.Reader{}).StartTime(h.identity.PID) == h.identity.StartTicks {
		syscall.Kill(h.identity.PID, syscall.SIGKILL)
	}
	h.cmd.Process.Kill()
	select {
	case <-h.wait:
		h.stop = true
		return nil
	case <-time.After(3 * time.Second):
		return ravpn.ErrEngine
	}
}
func (u *productionRAUnits) Stop(ctx context.Context, p *ravpn.NetworkPlan, wanted ravpn.UnitIdentity) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	actual, e := u.observe(p)
	if e != nil || actual != wanted {
		return ravpn.ErrEngine
	}
	return u.stop(u.units[p.Instance])
}

func TestIntegrationPrivateProductionRAControllerLifecycle(t *testing.T) {
	if os.Getenv("NGFW_RA_PRIVATE_VPP") != "1" {
		t.Skip("requires verified exclusively owned VPP runner")
	}
	for _, kind := range []string{"net", "mnt"} {
		self, e := os.Stat("/proc/self/ns/" + kind)
		host, ee := os.Stat("/proc/1/ns/" + kind)
		if e != nil || ee != nil || os.SameFile(self, host) {
			t.Fatal("private coordinator boundary absent")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	connection := vpp.Dial("/run/vpp/api.sock", vpp.ConnOptions{Logger: quiet, ReplyTimeout: 5 * time.Second})
	t.Cleanup(connection.Close)
	if connection.WaitConnected(ctx) != nil {
		t.Fatal("private VPP connection")
	}
	boot, e := bootid.Current(ctx, connection)
	if e != nil || !boot.Complete() || boot.PID <= 1 {
		t.Fatal("private VPP boot proof")
	}
	self, e := os.Stat("/proc/self/ns/net")
	peer, ee := os.Stat("/proc/" + strconv.Itoa(boot.PID) + "/ns/net")
	if e != nil || ee != nil || !os.SameFile(self, peer) {
		t.Fatal("connected VPP outside owned namespace")
	}
	owner := "w19prod-" + strconv.Itoa(os.Getpid())
	state := filepath.Join("/run/ngfw", owner)
	if os.Mkdir(state, 0700) != nil {
		t.Fatal("exclusive state root")
	}
	marker := []byte("F-ra-vpn private production fixture " + owner)
	if os.WriteFile(filepath.Join(state, ".owner"), marker, 0600) != nil {
		t.Fatal("state owner marker")
	}
	t.Cleanup(func() {
		data, e := os.ReadFile(filepath.Join(state, ".owner"))
		if e == nil && string(data) == string(marker) {
			os.RemoveAll(state)
		} else {
			t.Error("state cleanup ownership mismatch")
		}
	})
	cache, e := secretchannel.Open(state, owner)
	if e != nil {
		t.Fatal("sealed cache")
	}
	unitPath, e := filepath.Abs("../../../../deploy/systemd/ngfw-ra@.service")
	if e != nil {
		t.Fatal(e)
	}
	installation := ravpn.EngineInstallation{Prefix: os.Getenv("NGFW_RA_ENGINE_ROOT"), Helper: os.Getenv("NGFW_RA_HELPER"), Unit: unitPath, OSRelease: "/etc/os-release", PackageStatus: "/var/lib/dpkg/status"}
	preparation := &ravpn.SealedPreparation{Resolver: cache, Installation: &installation, Readiness: func(context.Context) error { _, e := cache.ID(nil); return e }}
	units := &productionRAUnits{t: t, units: map[string]*productionRAUnit{}}
	t.Cleanup(func() {
		units.mu.Lock()
		for _, h := range units.units {
			if !h.stop {
				if units.stop(h) != nil {
					t.Error("own private daemon cleanup failed")
				}
			}
		}
		units.mu.Unlock()
		instance := ravpn.InstanceID(owner, "road")
		if plan, e := ravpn.ReadAgentPlan(instance); e == nil {
			if _, e := os.Stat(filepath.Join(ravpn.InstanceRoot, instance, "snapshot.json")); e == nil {
				if ravpn.CleanupSnapshot(instance) != nil {
					t.Error("owned snapshot residue refused")
					return
				}
			}
			if ravpn.RemoveNamespace(instance, plan.NamespaceInode) != nil {
				t.Error("owned namespace cleanup refused")
				return
			}
			if os.Remove(filepath.Join(ravpn.InstanceRoot, instance, "network.json")) != nil || os.Remove(filepath.Join(ravpn.InstanceRoot, instance)) != nil {
				t.Error("owned namespace directory residue")
			}
		}
	})
	t.Setenv(subsystems.EnvHostServicesDir, filepath.Join(state, "host-services"))
	owned, e := ownertable.Open(state, owner)
	if e != nil {
		t.Fatal("owned route store")
	}
	reg := scheduler.NewRegistry()
	wiring, e := subsystems.Register(reg, subsystems.Env{Client: connection, Owner: owner, StateDir: state, Owned: owned, Log: quiet, IDs: subsystems.IDScope{Range: &subsystems.IDRange{Lo: 2432, Hi: 19999}}, RA: &subsystems.RAControllerOptions{Preparation: preparation, Units: units, SecretRef: cache.Ref}})
	if e != nil {
		t.Fatal("real wiring", e)
	}
	t.Cleanup(wiring.Close)
	wiring.Connected(ctx)
	if subsystems.SetPKISecrets(owner, cache.Text) != nil {
		t.Fatal("PKI sealed source")
	}
	sched := scheduler.New(reg, quiet)
	service, e := NewService(ServiceConfig{Owner: owner, VPP: connection, Scheduler: sched, StateDir: state, BeforeTxn: wiring.BeforeTxn, NetdevKind: wiring.NetdevKind(), SecretCache: cache, Logger: quiet, TxnTimeout: 60 * time.Second})
	if e != nil {
		t.Fatal("real service", e)
	}
	service.claimsTxn = wiring.ClaimsTxn
	t.Cleanup(service.Close)
	credentials, _ := productionRACredentials(t)
	desired := doc(t, productionRADoc)
	response, e := service.Apply(ctx, &ngfwv1.ApplyRequest{Owner: owner, TxnId: "normal-foundation", Subsystems: []string{"vrfs", "interfaces", "acl"}, DesiredState: desired, SecretBundle: &ngfwv1.SecretBundle{Values: map[string][]byte{"cert/server": append([]byte(nil), credentials.Certificate...), "key/server": append([]byte(nil), credentials.PrivateKey...), "password/client": []byte("NGFW_TEST_PASSWORD_RA19")}}})
	if e != nil || response.GetStatus() != ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED {
		t.Fatal("normal foundation Apply", e, response.GetStatus(), response.GetMessage(), response.GetValidation())
	}
	capability, e := service.RemoteAccessCapabilities(ctx, &ngfwv1.RemoteAccessCapabilitiesRequest{Owner: owner})
	if e != nil || !capability.GetOperational() {
		t.Fatal("actual ready-to-activate capability", e, capability.GetReason())
	}
	response, e = service.Apply(ctx, &ngfwv1.ApplyRequest{Owner: owner, TxnId: "ra-enable", Subsystems: []string{"vpn"}, DesiredState: desired})
	if e != nil || response.GetStatus() != ngfwv1.ApplyStatus_APPLY_STATUS_APPLIED {
		t.Fatal("production RA Apply", e, response.GetStatus(), response.GetMessage(), response.GetResults(), response.GetValidation())
	}
	sessions, e := service.RemoteAccessSessions(ctx, &ngfwv1.RemoteAccessSessionsRequest{Owner: owner, Profile: "road", Limit: 100})
	if e != nil || len(sessions.GetSessions()) != 0 {
		t.Fatal("actual fresh daemon session readback", e)
	}
	t.Log("actual production desired/Wiring/Service.Apply private engine activation and VICI empty readback PASS; full packet/restart/rollback campaign remains")
}

const productionRADoc = `{"vrfs":{"outer":{"id":19000},"inner":{"id":19001}},"interfaces":{"loop2436":{"enabled":true,"vrf":"inner","ipv4":["10.19.0.53/32"]},"loop2437":{"enabled":true,"vrf":"inner","ipv4":["10.19.0.54/32"]}},"acl":{"lists":{"outer-in":{"rules":[{"sequence":10,"enabled":true,"action":"permit","ipVersion":"ipv4"}]},"outer-out":{"rules":[{"sequence":10,"enabled":true,"action":"permit","ipVersion":"ipv4"}]},"inner-in":{"rules":[{"sequence":10,"enabled":true,"action":"permit","ipVersion":"ipv4","source":{"kind":"prefix","prefix":"10.19.200.0/24"},"destination":{"kind":"prefix","prefix":"10.19.0.53/32"}}]},"inner-out":{"rules":[{"sequence":10,"enabled":true,"action":"permit","ipVersion":"ipv4","source":{"kind":"prefix","prefix":"10.19.0.53/32"},"destination":{"kind":"prefix","prefix":"10.19.200.0/24"}}]}}},"vpn":{"ipsec":{"proposals":{"ra-proposal":{"ike":{"encr":"aes256","integ":"sha256","prf":"prfsha256","dh":"ecp256"},"esp":{"encr":"aes256gcm16","dh":"ecp256"}}}},"pki":{"certificates":{"server":{"certificateRef":"cert/server","privateKeyRef":"key/server"}}},"remoteAccess":{"road":{"enabled":true,"localAddr":"192.0.2.19","localId":"vpn.example.test","vrf":"inner","underlayVrf":"outer","auth":"eap-mschapv2","certificate":"server","proposal":"ra-proposal","transport":{"outer":{"vpp":"198.18.19.0/31","namespace":"198.18.19.1/31"},"inner":{"vpp":"198.18.19.2/31","namespace":"198.18.19.3/31"}},"pools":[{"name":"clients","prefix":"10.19.200.0/24","dns":["10.19.0.53"]}],"splitTunnel":["10.19.0.0/16"],"users":[{"username":"client","passwordRef":"password/client"}],"outerPolicy":{"ingress":["outer-in"],"egress":["outer-out"]},"accessPolicy":{"ingress":["inner-in"],"egress":["inner-out"]}}}}}`

func productionRACredentials(t *testing.T) (ravpn.Credentials, time.Time) {
	t.Helper()
	now := time.Now().UTC()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	caTemplate := &x509.Certificate{SerialNumber: big.NewInt(19), Subject: pkix.Name{CommonName: "RA disposable CA"}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign | x509.KeyUsageCRLSign, SubjectKeyId: []byte{19, 1, 2, 3}}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	ca, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatal(err)
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	leaf := &x509.Certificate{SerialNumber: big.NewInt(20), Subject: pkix.Name{CommonName: "vpn.example.test"}, DNSNames: []string{"vpn.example.test"}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
	leafDER, err := x509.CreateCertificate(rand.Reader, leaf, ca, &key.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	crlDER, err := x509.CreateRevocationList(rand.Reader, &x509.RevocationList{Number: big.NewInt(1), ThisUpdate: now.Add(-time.Minute), NextUpdate: now.Add(time.Hour)}, ca, caKey)
	if err != nil {
		t.Fatal(err)
	}
	encode := func(label string, data []byte) []byte {
		return pem.EncodeToMemory(&pem.Block{Type: label, Bytes: data})
	}
	caPEM := encode("CERTIFICATE", caDER)
	return ravpn.Credentials{Certificate: append(encode("CERTIFICATE", leafDER), caPEM...), PrivateKey: encode("PRIVATE"+" KEY", keyDER), ClientCA: caPEM, ClientCRL: encode("X509 CRL", crlDER)}, now
}
