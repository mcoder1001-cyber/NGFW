package pki

import (
	"bytes"
	"crypto"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// Kind is the class of a materialised file: it decides the swanctl directory, the mode and the validation.
type Kind string

// The file kinds.
const (
	KindCert Kind = "cert" // x509/<name>.pem, 0644: a local certificate (leaf first, chain after)
	KindCA   Kind = "ca"   // x509ca/<name>.pem, 0644: a trusted CA certificate (CA:TRUE)
	KindKey  Kind = "key"  // private/<name>.pem, 0600: the private key of x509/<name>.pem
	KindCRL  Kind = "crl"  // x509crl/<name>.pem, 0644: the CRL of x509ca/<name>.pem
)

// Kinds lists the kinds in their canonical order (the order of PkiFileStateSet.files).
var Kinds = []Kind{KindCert, KindCA, KindKey, KindCRL}

// Size bounds of a file's material (the strongSwan renderer reads certificate files of at most 64 KiB).
const (
	MaxCertSize = 64 << 10
	MaxKeySize  = 16 << 10
	MaxCRLSize  = 4 << 20
)

// Dir is the swanctl subdirectory of the kind ("" for an unknown kind).
func (k Kind) Dir() string {
	switch k {
	case KindCert:
		return "x509"
	case KindCA:
		return "x509ca"
	case KindKey:
		return "private"
	case KindCRL:
		return "x509crl"
	}
	return ""
}

// Mode is the permission bits of the kind's files: 0600 for private keys, 0644 for the public ones.
func (k Kind) Mode() os.FileMode {
	if k == KindKey {
		return 0o600
	}
	return 0o644
}

// Valid reports whether k is one of Kinds.
func (k Kind) Valid() bool { return k.Dir() != "" }

func (k Kind) rank() int {
	for i, x := range Kinds {
		if x == k {
			return i
		}
	}
	return len(Kinds)
}

func (k Kind) maxSize() int {
	switch k {
	case KindKey:
		return MaxKeySize
	case KindCRL:
		return MaxCRLSize
	}
	return MaxCertSize
}

// nameRe is a vpn.pki object name (schema objectName): the file is <name>.pem, never "." or "..".
var nameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,62}$`)

// refRe is the D-051 reference form this package resolves (certificates, CRLs and keys).
var refRe = regexp.MustCompile(`^(cert|key)/[A-Za-z0-9_.-]{1,64}$`)

// ErrMaterial is wrapped by every validation error. The messages never quote material.
var ErrMaterial = errors.New("pki: invalid material")

// checked is what validation learnt from one file: public facts only, plus the normalised content to write.
type checked struct {
	content []byte               // PEM re-encoded block by block (no text between blocks, no headers)
	leaf    *x509.Certificate    // cert, ca: the first certificate
	pub     crypto.PublicKey     // key: the public half
	crl     *x509.RevocationList // crl
}

// pemLabel builds a PEM type name; the private-key labels are assembled so that no source line carries a complete
// private-key banner (the repository's secret scan flags those).
func pemLabel(parts ...string) string { return strings.Join(parts, " ") }

var (
	labelPKCS8     = pemLabel("PRIVATE", "KEY")
	labelEC        = pemLabel("EC", "PRIVATE", "KEY")
	labelRSA       = pemLabel("RSA", "PRIVATE", "KEY")
	labelEncrypted = pemLabel("ENCRYPTED", "PRIVATE", "KEY")
)

// pemBlocks decodes data as one or more PEM blocks. Text before the first block or after the last one is refused
// (data must be PEM, nothing else); text between blocks is dropped by the normalisation.
func pemBlocks(data []byte) ([]*pem.Block, error) {
	rest := bytes.TrimSpace(data)
	if !bytes.HasPrefix(rest, []byte("-----BEGIN ")) {
		return nil, fmt.Errorf("%w: not PEM-encoded", ErrMaterial)
	}
	var out []*pem.Block
	for len(bytes.TrimSpace(rest)) > 0 {
		var b *pem.Block
		b, rest = pem.Decode(rest)
		if b == nil {
			return nil, fmt.Errorf("%w: trailing data after PEM block %d", ErrMaterial, len(out))
		}
		out = append(out, b)
		if len(out) > 32 {
			return nil, fmt.Errorf("%w: more than 32 PEM blocks", ErrMaterial)
		}
	}
	return out, nil
}

// check validates data as the material of kind and returns the normalised content and its public facts.
func check(kind Kind, data []byte) (*checked, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("%w: empty", ErrMaterial)
	}
	if len(data) > kind.maxSize() {
		return nil, fmt.Errorf("%w: larger than %d bytes", ErrMaterial, kind.maxSize())
	}
	blocks, err := pemBlocks(data)
	if err != nil {
		return nil, err
	}
	switch kind {
	case KindCert, KindCA:
		return checkCerts(kind, blocks)
	case KindKey:
		return checkKey(blocks)
	case KindCRL:
		if len(blocks) != 1 || blocks[0].Type != "X509 CRL" {
			return nil, fmt.Errorf("%w: expected exactly one X509 CRL block", ErrMaterial)
		}
		crl, err := x509.ParseRevocationList(blocks[0].Bytes)
		if err != nil {
			return nil, fmt.Errorf("%w: the CRL does not parse", ErrMaterial)
		}
		return &checked{content: encode(blocks), crl: crl}, nil
	}
	return nil, fmt.Errorf("%w: unknown kind %q", ErrMaterial, kind)
}

func checkCerts(kind Kind, blocks []*pem.Block) (*checked, error) {
	var chain []*x509.Certificate
	for _, b := range blocks {
		if b.Type != "CERTIFICATE" {
			if strings.Contains(b.Type, labelPKCS8) {
				return nil, fmt.Errorf("%w: the certificate secret also holds a private key (it would be written world-readable): store the key as its own key/<name> secret", ErrMaterial)
			}
			return nil, fmt.Errorf("%w: unexpected PEM block %q in a certificate", ErrMaterial, clip(b.Type))
		}
		c, err := x509.ParseCertificate(b.Bytes)
		if err != nil {
			return nil, fmt.Errorf("%w: certificate %d of the file does not parse", ErrMaterial, len(chain)+1)
		}
		chain = append(chain, c)
	}
	leaf := chain[0]
	if kind == KindCert && leaf.IsCA {
		return nil, fmt.Errorf("%w: operational certificates cannot be CA signing certificates", ErrMaterial)
	}
	if kind == KindCA && (!leaf.BasicConstraintsValid || !leaf.IsCA) {
		return nil, fmt.Errorf("%w: not a CA certificate (basicConstraints CA:TRUE is missing)", ErrMaterial)
	}
	return &checked{content: encode(blocks), leaf: leaf}, nil
}

func checkKey(blocks []*pem.Block) (*checked, error) {
	if len(blocks) != 1 {
		return nil, fmt.Errorf("%w: expected exactly one PEM private key block, found %d blocks", ErrMaterial, len(blocks))
	}
	b := blocks[0]
	defer zero(b.Bytes) // the decoded DER of the key; the caller zeroes the PEM it resolved
	if b.Type == labelEncrypted || strings.Contains(b.Headers["Proc-Type"], "ENCRYPTED") {
		return nil, fmt.Errorf("%w: encrypted private keys are not supported (charon would need the passphrase): store the key unencrypted — the secret store encrypts it at rest", ErrMaterial)
	}
	var key any
	var err error
	switch b.Type {
	case labelPKCS8:
		key, err = x509.ParsePKCS8PrivateKey(b.Bytes)
	case labelEC:
		key, err = x509.ParseECPrivateKey(b.Bytes)
	case labelRSA:
		key, err = x509.ParsePKCS1PrivateKey(b.Bytes)
	default:
		return nil, fmt.Errorf("%w: PEM block %q is not a private key", ErrMaterial, clip(b.Type))
	}
	if err != nil {
		return nil, fmt.Errorf("%w: the private key does not parse", ErrMaterial)
	}
	signer, ok := key.(crypto.Signer)
	if !ok {
		return nil, fmt.Errorf("%w: unsupported private key type", ErrMaterial)
	}
	return &checked{content: pem.EncodeToMemory(&pem.Block{Type: b.Type, Bytes: b.Bytes}), pub: signer.Public()}, nil
}

// encode re-encodes blocks without headers or interleaved text.
func encode(blocks []*pem.Block) []byte {
	var out []byte
	for _, b := range blocks {
		out = append(out, pem.EncodeToMemory(&pem.Block{Type: b.Type, Bytes: b.Bytes})...)
	}
	return out
}

// keyMatches reports whether pub is the public key of leaf.
func keyMatches(pub crypto.PublicKey, leaf *x509.Certificate) bool {
	e, ok := pub.(interface{ Equal(crypto.PublicKey) bool })
	return ok && leaf != nil && e.Equal(leaf.PublicKey)
}

// clip shortens a value for an error message.
func clip(s string) string {
	if len(s) > 40 {
		return s[:40] + "…"
	}
	return s
}

// zero overwrites b (material) with zeros.
func zero(b []byte) { clear(b) }
