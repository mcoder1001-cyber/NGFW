package ravpn

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"ngfw/agent/internal/renderers/strongswan"
)

// SealedPreparation resolves immutable keyed cache references. Literal names
// never select plaintext after projection, so concurrent cache activation and
// rollback cannot substitute another credential generation.
type SealedPreparation struct {
	Resolver     strongswan.SecretResolver
	Now          func() time.Time
	Readiness    func(context.Context) error
	Installation *EngineInstallation
}

func (*SealedPreparation) String() string { return "remote-access sealed preparation <redacted>" }
func (p *SealedPreparation) Prepare(ctx context.Context, s EngineSpec) (*PreparedEngine, error) {
	return p.prepare(ctx, s, false)
}
func (p *SealedPreparation) Recover(ctx context.Context, s EngineSpec) (*PreparedEngine, error) {
	return p.prepare(ctx, s, true)
}
func (p *SealedPreparation) prepare(ctx context.Context, s EngineSpec, recoverGeneration bool) (*PreparedEngine, error) {
	if p == nil || p.Resolver == nil || s.Validate() != nil || s.ServerCertificate == nil {
		return nil, ErrEngine
	}
	now := time.Now()
	if p.Now != nil {
		now = p.Now()
	}
	material := map[string][]byte{}
	defer func() {
		for _, data := range material {
			clear(data)
		}
	}()
	resolve := func(ctx context.Context, ref string) ([]byte, error) {
		if data, ok := material[ref]; ok {
			return append([]byte(nil), data...), nil
		}
		fingerprint, ok := s.Fingerprints[ref]
		if !ok || !strings.HasPrefix(fingerprint, "hmac:") || !ValidInstance(strings.TrimPrefix(fingerprint, "hmac:")) {
			return nil, ErrEngine
		}
		data, e := p.Resolver.Resolve(ctx, fingerprint)
		if e != nil || len(data) == 0 || len(data) > 4<<20 {
			clear(data)
			return nil, ErrEngine
		}
		material[ref] = data
		return append([]byte(nil), data...), nil
	}
	certificate, e := resolve(ctx, s.ServerCertificate.GetCertificateRef())
	if e != nil {
		return nil, ErrEngine
	}
	defer clear(certificate)
	key, e := resolve(ctx, s.ServerCertificate.GetPrivateKeyRef())
	if e != nil {
		return nil, ErrEngine
	}
	defer clear(key)
	credentials := Credentials{Certificate: certificate, PrivateKey: key}
	clients := s.Configuration.GetAuth() == "eap-tls" || s.Configuration.GetAuth() == "pubkey"
	if clients {
		if s.ClientCA == nil {
			return nil, ErrEngine
		}
		credentials.ClientCA, e = resolve(ctx, s.ClientCA.GetCertificateRef())
		if e != nil {
			return nil, ErrEngine
		}
		defer clear(credentials.ClientCA)
		credentials.ClientCRL, e = resolve(ctx, "cert/"+s.Configuration.GetClientCa()+".crl")
		if e != nil {
			return nil, ErrEngine
		}
		defer clear(credentials.ClientCRL)
	}
	identity := s.Configuration.GetLocalId()
	if identity == "" {
		identity = s.Configuration.GetLocalAddr()
	}
	if VerifyCredentials(credentials, identity, clients, now) != nil {
		return nil, ErrEngine
	}
	root := filepath.Join(InstanceRoot, s.Instance)
	files, e := strongswan.BuildRAFiles(ctx, s.Profile, s.Configuration, s.Proposal, root, strongswan.SecretResolverFunc(resolve))
	if e != nil {
		return nil, ErrEngine
	}
	snapshot := PrivateSnapshot{Daemon: files.Daemon, Connection: files.Connection, Secrets: files.Secrets, Credentials: credentials, CertificateName: s.Configuration.GetCertificate(), Identity: identity, CertificateClients: clients}
	if clients {
		snapshot.ClientCAName = s.Configuration.GetClientCa()
	}
	var stageError error
	if recoverGeneration {
		expected := map[string][]byte{"strongswan.conf": files.Daemon, "swanctl.conf": append(append([]byte{}, files.Connection...), files.Secrets...), "x509/" + snapshot.CertificateName + ".pem": certificate, "private/" + snapshot.CertificateName + ".pem": key}
		defer clear(expected["swanctl.conf"])
		if clients {
			expected["x509ca/"+snapshot.ClientCAName+".pem"] = credentials.ClientCA
			expected["x509crl/"+snapshot.ClientCAName+".pem"] = credentials.ClientCRL
		}
		stageError = verifySnapshotContents(s.Instance, expected)
	} else {
		stageError = WriteSnapshot(s.Instance, snapshot, now)
	}
	if stageError != nil {
		clear(files.Secrets)
		clear(files.Daemon)
		return nil, ErrEngine
	}
	// Load's private closure retains only verified bytes until one load or cleanup.
	loadMaterial := strongswan.RAMaterial{Certificates: map[string][]byte{filepath.Join(root, "x509", snapshot.CertificateName+".pem"): append([]byte(nil), certificate...)}, PrivateKey: append([]byte(nil), key...), CRL: append([]byte(nil), credentials.ClientCRL...)}
	if clients {
		loadMaterial.Certificates[filepath.Join(root, "x509ca", snapshot.ClientCAName+".pem")] = append([]byte(nil), credentials.ClientCA...)
	}
	name, e := strongswan.ConnName(s.Profile)
	if e != nil {
		CleanupSnapshot(s.Instance)
		return nil, ErrEngine
	}
	var mu sync.Mutex
	consumed := false
	erase := func() {
		clear(files.Secrets)
		clear(files.Daemon)
		clear(loadMaterial.PrivateKey)
		clear(loadMaterial.CRL)
		for _, data := range loadMaterial.Certificates {
			clear(data)
		}
	}
	return &PreparedEngine{ConnectionName: "ra-" + name, Load: func(ctx context.Context, c strongswan.ViciConn) error {
		mu.Lock()
		defer mu.Unlock()
		if consumed {
			return ErrEngine
		}
		consumed = true
		defer erase()
		loaded, e := strongswan.LoadRA(ctx, c, files, loadMaterial)
		if e != nil || loaded != "ra-"+name {
			return ErrEngine
		}
		return nil
	}, Cleanup: func(ctx context.Context) error {
		mu.Lock()
		defer mu.Unlock()
		consumed = true
		erase()
		if ctx.Err() != nil {
			return ErrEngine
		}
		return CleanupSnapshot(s.Instance)
	}}, nil
}
