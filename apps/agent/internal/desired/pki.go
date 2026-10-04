package desired

import (
	"context"
	"errors"
	"time"

	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/pki"
)

// PKI stages configured operational certificates, public CAs and matching private keys in an
// agent-owned directory. CSR-only keys stay API-side. Native IKEv2 certificate consumers are a
// separate feature; no removed strongSwan daemon is enabled by this projection.

// PKIFilesKey is the key of the singleton (pki.Key).
var PKIFilesKey = pki.Key

// Rule ids of the builder's findings.
const (
	RulePKIUnsupported = "agent.unsupported-field"
	RulePKISecret      = "agent.secret-unavailable"
	RulePKIMaterial    = "vpn.pki-material"
	RulePKIReference   = "vpn.pki-reference-exists"
)

// PKIOptions are the agent-side facts the PKI builder needs (subsystems.PKIProjection).
type PKIOptions struct {
	// Disabled: an explicitly disabled materializer stages nothing.
	Disabled bool
	// Plan resolves and validates the files and returns the object's value (pki.Materialiser.Plan). nil = no
	// materialiser in this agent: every configured file is refused with agent.secret-unavailable.
	Plan func(ctx context.Context, files []pki.File) (*ngfwv1.PkiFileStateSet, []*pki.FileError)
}

// pkiPlanTimeout bounds the material resolution of one projection (the source is the agent's in-memory store).
const pkiPlanTimeout = 10 * time.Second

// pkiNeed is one file of the projection with the pointer of the leaf that names its material.
type pkiNeed struct {
	file    pki.File
	pointer string
}

// PKI projects the pki.files object for a transaction that includes `vpn` (see the file comment).
func PKI(s Sink, ds *ngfwv1.DesiredState, in map[string]bool, o PKIOptions) {
	if !in["vpn"] {
		return
	}
	cfg := ds.GetVpn().GetPki()
	for _, name := range sortedKeys(cfg.GetCertificates()) {
		if cfg.GetCertificates()[name].GetAcme() != nil {
			// a warning while nothing uses it (the schema examples carry ACME blocks); a tunnel that authenticates with
			// it is refused in pkiNeeds
			s.Warnf(Ptr("vpn", "pki", "certificates", name, "acme"), RulePKIUnsupported,
				"ACME is not supported in this build: the certificate is never issued; obtain it elsewhere and import it (POST /api/v1/actions/pki/import)")
		}
	}
	if h := cfg.GetHsm(); h != nil && h.GetEnabled() {
		s.Errorf(Ptr("vpn", "pki", "hsm"), RulePKIUnsupported, "PKCS#11 / HSM keys are not supported in this build")
	}
	if o.Disabled { // no charon for this agent: nothing to materialise (the IPsec builder reports the tunnels)
		return
	}
	needs, ok := pkiNeeds(ds)
	if !ok || len(needs) == 0 {
		return
	}
	files := make([]pki.File, len(needs))
	for i, n := range needs {
		files[i] = n.file
	}
	if o.Plan == nil {
		for _, n := range needs {
			if !n.file.Optional {
				s.Errorf(n.pointer, RulePKISecret, "%s: this agent has no PKI materialiser (%v)", n.file.Ref, pki.ErrNoSource)
			}
		}
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), pkiPlanTimeout)
	defer cancel()
	set, errs := o.Plan(ctx, files)
	for _, fe := range errs {
		rule := RulePKIMaterial
		if !isMaterialErr(fe) {
			rule = RulePKISecret
		}
		ptr := Ptr("vpn", "pki")
		if fe.Index >= 0 && fe.Index < len(needs) {
			ptr = needs[fe.Index].pointer
		}
		s.Errorf(ptr, rule, "%v", fe)
	}
	if len(errs) > 0 || set == nil {
		return
	}
	s.Add(PKIFilesKey, set, Ptr("vpn", "pki"))
}

func isMaterialErr(fe *pki.FileError) bool {
	return fe != nil && errors.Is(fe.Err, pki.ErrMaterial)
}

// pkiNeeds lists the files the enabled strongSwan certificate tunnels need, deduplicated, in tunnel order. A tunnel
// naming a certificate or CA that does not exist is reported at its leaf (the schema's vpn.pki-reference-exists rule
// says the same at commit time) and makes the projection fail.
func pkiNeeds(ds *ngfwv1.DesiredState) ([]pkiNeed, bool) {
	cfg := ds.GetVpn().GetPki()
	var out []pkiNeed
	for _, name := range sortedKeys(cfg.GetCas()) {
		ca := cfg.GetCas()[name]
		out = append(out, pkiNeed{pki.File{Kind: pki.KindCA, Name: name, Ref: ca.GetCertificateRef()}, Ptr("vpn", "pki", "cas", name, "certificateRef")})
		if len(name) <= 59 {
			out = append(out, pkiNeed{pki.File{Kind: pki.KindCRL, Name: name, Ref: "cert/" + name + ".crl", Optional: true}, Ptr("vpn", "pki", "cas", name, "crl")})
		}
	}
	for _, name := range sortedKeys(cfg.GetCertificates()) {
		cert := cfg.GetCertificates()[name]
		if cert.GetCertificateRef() == "" {
			continue
		} // A CSR awaiting issuance has no operational material.
		out = append(out, pkiNeed{pki.File{Kind: pki.KindCert, Name: name, Ref: cert.GetCertificateRef()}, Ptr("vpn", "pki", "certificates", name, "certificateRef")})
		if cert.GetPrivateKeyRef() != "" {
			out = append(out, pkiNeed{pki.File{Kind: pki.KindKey, Name: name, Ref: cert.GetPrivateKeyRef()}, Ptr("vpn", "pki", "certificates", name, "privateKeyRef")})
		}
	}
	return out, true
}
