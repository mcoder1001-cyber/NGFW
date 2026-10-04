package desired

import (
	"context"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"strings"
	"time"

	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/descriptors/ikev2"
	"ngfw/agent/internal/descriptors/vpn"
)

type nativeCertificate struct{ keyFile, peerFile string }

// nativeCertificates performs the complete set's certificate preflight without
// filesystem or VPP mutations. Errors never include PEM or secret material.
func nativeCertificates(ds *ngfwv1.DesiredState, env IKEv2Env) (map[string]nativeCertificate, error) {
	out := map[string]nativeCertificate{}
	var sharedKey, sharedIdentity, sharedCertificate string
	for _, name := range sortedKeys(ds.GetVpn().GetIpsec().GetTunnels()) {
		t := ds.GetVpn().GetIpsec().GetTunnels()[name]
		if !tunnelEnabled(t) || t.GetAuth().GetMethod() != "cert" {
			continue
		}
		fail := func(message string) error { return fmt.Errorf("tunnel %q: %s", name, message) }
		if !env.GlobalsOwner || env.Resolve == nil || env.SecretRef == nil || env.NativeRoot == "" {
			return nil, fail("native certificate authentication requires the globals owner and sealed material resolver")
		}
		if t.GetAuth().GetRemoteCa() != "" {
			return nil, fail("remoteCa chain trust is unavailable; configure an explicit peerCertificate public-key pin")
		}
		if env.CertificateReady != nil {
			if err := env.CertificateReady(context.Background()); err != nil {
				return nil, fail(err.Error())
			}
		}
		cfg := ds.GetVpn().GetPki().GetCertificates()
		local, peer := cfg[t.GetAuth().GetCertificate()], cfg[t.GetAuth().GetPeerCertificate()]
		if local == nil || peer == nil || t.GetAuth().GetPeerCertificate() == "" {
			return nil, fail("local and pinned peer certificate objects must exist")
		}
		if local.GetPrivateKeyRef() == "" || local.GetCertificateRef() == "" || peer.GetCertificateRef() == "" || local.GetAcme() != nil || peer.GetAcme() != nil {
			return nil, fail("imported operational local certificate/key and peer leaf certificate are required")
		}
		resolve := func(ref string) ([]byte, string, error) {
			fingerprint, err := env.SecretRef(context.Background(), ref)
			if err != nil || vpn.CheckRef(fingerprint) != nil {
				return nil, "", errors.New("certificate material reference is unavailable")
			}
			material, err := env.Resolve(context.Background(), fingerprint)
			if err != nil {
				clear(material)
				return nil, "", errors.New("certificate material is unavailable")
			}
			return material, fingerprint, nil
		}
		lc, localCertRef, err := resolve(local.GetCertificateRef())
		if err != nil {
			return nil, fail(err.Error())
		}
		localCert, err := nativeLeaf(lc)
		clear(lc)
		if err != nil {
			return nil, fail("local " + err.Error())
		}
		pc, peerRef, err := resolve(peer.GetCertificateRef())
		if err != nil {
			return nil, fail(err.Error())
		}
		peerCert, err := nativeLeaf(pc)
		clear(pc)
		if err != nil {
			return nil, fail("peer " + err.Error())
		}
		kb, keyRef, err := resolve(local.GetPrivateKeyRef())
		if err != nil {
			return nil, fail(err.Error())
		}
		key, err := nativeRSAKey(kb)
		clear(kb)
		if err != nil {
			return nil, fail(err.Error())
		}
		pub := localCert.PublicKey.(*rsa.PublicKey)
		matches := key.N.Cmp(pub.N) == 0 && key.E == pub.E
		// Private RSA integers are transient; never persist them in projection values.
		key.D.SetInt64(0)
		for _, p := range key.Primes {
			p.SetInt64(0)
		}
		if !matches {
			return nil, fail("local private key does not match the local certificate")
		}
		localIdentity, remoteIdentity := t.GetLocalId(), t.GetRemoteId()
		if localIdentity == "" {
			localIdentity = t.GetLocalAddr()
		}
		if remoteIdentity == "" {
			remoteIdentity = t.GetRemoteAddr()
		}
		localIdentity = strings.TrimPrefix(localIdentity, "@")
		remoteIdentity = strings.TrimPrefix(remoteIdentity, "@")
		if nativeCertificateIdentity(localCert, localIdentity) != nil {
			return nil, fail("local IKE identity must match a SAN in the local certificate")
		}
		if nativeCertificateIdentity(peerCert, remoteIdentity) != nil {
			return nil, fail("remote IKE identity must match a SAN in the configured peer leaf")
		}
		if sharedKey != "" && (sharedKey != keyRef || sharedIdentity != localIdentity || sharedCertificate != localCertRef) {
			return nil, fail("all native certificate tunnels must share one local private key and IKE identity")
		}
		sharedKey, sharedIdentity, sharedCertificate = keyRef, localIdentity, localCertRef
		keyPath, peerPath, err := ikev2.NativePaths(env.NativeRoot, keyRef, peerRef)
		if err != nil {
			return nil, fail(err.Error())
		}
		out[name] = nativeCertificate{keyFile: keyPath, peerFile: peerPath}
	}
	return out, nil
}

func nativeLeaf(data []byte) (*x509.Certificate, error) {
	if len(data) == 0 || len(data) > 64<<10 {
		return nil, errors.New("leaf certificate must be a bounded PEM certificate")
	}
	b, rest := pem.Decode(data)
	if b == nil || b.Type != "CERTIFICATE" || len(strings.TrimSpace(string(rest))) != 0 {
		return nil, errors.New("exactly one PEM leaf certificate is required")
	}
	cert, err := x509.ParseCertificate(b.Bytes)
	if err != nil || cert.IsCA {
		return nil, errors.New("a valid non-CA leaf certificate is required")
	}
	pub, ok := cert.PublicKey.(*rsa.PublicKey)
	if !ok || pub.N.BitLen() < 2048 {
		return nil, errors.New("RSA leaf certificate with at least 2048 bits is required")
	}
	now := time.Now()
	if now.Before(cert.NotBefore) || !now.Before(cert.NotAfter) {
		return nil, errors.New("leaf certificate is not currently valid")
	}
	if cert.KeyUsage != 0 && cert.KeyUsage&x509.KeyUsageDigitalSignature == 0 {
		return nil, errors.New("leaf certificate must permit digital signatures")
	}
	return cert, nil
}
func nativeRSAKey(data []byte) (*rsa.PrivateKey, error) {
	if len(data) == 0 || len(data) > 16<<10 {
		return nil, errors.New("RSA private key must be a bounded PEM key")
	}
	b, rest := pem.Decode(data)
	if b == nil || len(strings.TrimSpace(string(rest))) != 0 {
		return nil, errors.New("one unencrypted RSA private key is required")
	}
	defer clear(b.Bytes)
	var key *rsa.PrivateKey
	var err error
	switch b.Type {
	case "RSA PRIVATE KEY":
		key, err = x509.ParsePKCS1PrivateKey(b.Bytes)
	case "PRIVATE KEY":
		var parsed any
		parsed, err = x509.ParsePKCS8PrivateKey(b.Bytes)
		key, _ = parsed.(*rsa.PrivateKey)
	default:
		err = errors.New("unsupported private key format")
	}
	if err != nil || key == nil || key.N.BitLen() < 2048 || key.Validate() != nil {
		return nil, errors.New("valid unencrypted RSA private key of at least 2048 bits is required")
	}
	return key, nil
}
func nativeCertificateIdentity(cert *x509.Certificate, id string) error {
	if strings.Contains(id, "@") {
		for _, email := range cert.EmailAddresses {
			if email == id {
				return nil
			}
		}
		return errors.New("identity absent from SAN")
	}
	return cert.VerifyHostname(id)
}
