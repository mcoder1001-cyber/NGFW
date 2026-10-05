package ravpn

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"time"
)

// Credentials is a verified sealed-cache snapshot, never scheduler/RPC state.
// Caller wipes PrivateKey after writing its protected profile generation.
type Credentials struct {
	Certificate []byte `json:"-"`
	PrivateKey  []byte `json:"-"`
	ClientCA    []byte `json:"-"`
	ClientCRL   []byte `json:"-"`
}

// String keeps private keys out of generic logging.
func (Credentials) String() string { return "remote-access credentials <redacted>" }

// GoString keeps private keys out of detailed Go formatting.
func (c Credentials) GoString() string { return c.String() }

func strongPublicKey(key crypto.PublicKey) bool {
	switch value := key.(type) {
	case *rsa.PublicKey:
		return value.N.BitLen() >= 2048 && value.E >= 65537
	case *ecdsa.PublicKey:
		return value.Curve.Params().BitSize >= 256
	case ed25519.PublicKey:
		return len(value) == ed25519.PublicKeySize
	}
	return false
}
func strongSignature(algorithm x509.SignatureAlgorithm) bool {
	switch algorithm {
	case x509.SHA256WithRSA, x509.SHA384WithRSA, x509.SHA512WithRSA, x509.SHA256WithRSAPSS, x509.SHA384WithRSAPSS, x509.SHA512WithRSAPSS, x509.ECDSAWithSHA256, x509.ECDSAWithSHA384, x509.ECDSAWithSHA512, x509.PureEd25519:
		return true
	}
	return false
}
func validCertificate(cert *x509.Certificate, now time.Time) bool {
	return cert != nil && !now.Before(cert.NotBefore) && now.Before(cert.NotAfter) && strongPublicKey(cert.PublicKey) && strongSignature(cert.SignatureAlgorithm)
}
func exactPEM(data []byte, labels map[string]bool, limit int) bool {
	count := 0
	for len(bytes.TrimSpace(data)) != 0 {
		data = bytes.TrimSpace(data)
		if !bytes.HasPrefix(data, []byte("-----BEGIN ")) {
			return false
		}
		block, rest := pem.Decode(data)
		if block == nil || !labels[block.Type] || len(block.Headers) != 0 {
			return false
		}
		count++
		if count > limit {
			return false
		}
		data = rest
	}
	return count > 0
}

// VerifyCredentials checks the same snapshot that will be written. TLS/public
// key clients require a currently valid signing CA and its fresh signed CRL;
// absence never falls back to accepting an unverified client certificate.
func VerifyCredentials(value Credentials, identity string, certificateClients bool, now time.Time) error {
	if len(value.Certificate) == 0 || len(value.Certificate) > 65536 || len(value.PrivateKey) == 0 || len(value.PrivateKey) > 16384 || identity == "" {
		return ErrBoundary
	}
	if !exactPEM(value.Certificate, map[string]bool{"CERTIFICATE": true}, 16) || !exactPEM(value.PrivateKey, map[string]bool{"PRIVATE" + " KEY": true, "RSA PRIVATE" + " KEY": true, "EC PRIVATE" + " KEY": true}, 1) {
		return ErrBoundary
	}
	pair, err := tls.X509KeyPair(value.Certificate, value.PrivateKey)
	if err != nil || len(pair.Certificate) == 0 || len(pair.Certificate) > 16 {
		return ErrBoundary
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil || leaf.IsCA || !validCertificate(leaf, now) || leaf.VerifyHostname(identity) != nil || leaf.KeyUsage&x509.KeyUsageDigitalSignature == 0 {
		return ErrBoundary
	}
	for _, raw := range pair.Certificate[1:] {
		issuer, err := x509.ParseCertificate(raw)
		if err != nil || !issuer.IsCA || !validCertificate(issuer, now) || leaf.CheckSignatureFrom(issuer) != nil {
			return ErrBoundary
		}
		leaf = issuer
	}
	if !certificateClients {
		if len(value.ClientCA) != 0 || len(value.ClientCRL) != 0 {
			return ErrBoundary
		}
		return nil
	}
	if len(value.ClientCA) == 0 || len(value.ClientCA) > 65536 || len(value.ClientCRL) == 0 || len(value.ClientCRL) > 4<<20 {
		return ErrBoundary
	}
	caBlock, rest := pem.Decode(value.ClientCA)
	if caBlock == nil || caBlock.Type != "CERTIFICATE" || len(bytes.TrimSpace(rest)) != 0 {
		return ErrBoundary
	}
	ca, err := x509.ParseCertificate(caBlock.Bytes)
	if err != nil || !ca.IsCA || !validCertificate(ca, now) || ca.KeyUsage&x509.KeyUsageCertSign == 0 || ca.KeyUsage&x509.KeyUsageCRLSign == 0 {
		return ErrBoundary
	}
	crlBlock, rest := pem.Decode(value.ClientCRL)
	if crlBlock == nil || crlBlock.Type != "X509 CRL" || len(bytes.TrimSpace(rest)) != 0 {
		return ErrBoundary
	}
	crl, err := x509.ParseRevocationList(crlBlock.Bytes)
	if err != nil || !bytes.Equal(crl.RawIssuer, ca.RawSubject) || crl.CheckSignatureFrom(ca) != nil || now.Before(crl.ThisUpdate) || !now.Before(crl.NextUpdate) || !strongSignature(crl.SignatureAlgorithm) {
		return ErrBoundary
	}
	return nil
}
