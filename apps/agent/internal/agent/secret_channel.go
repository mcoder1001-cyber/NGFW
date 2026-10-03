package agent

import (
	vrxv1 "ngfw/agent/gen/vrx/v1"
	"ngfw/agent/internal/descriptors/vpn"
)

// Drop the decoded RPC plaintext as soon as projection/application finishes.
func clearSecretBundle(bundle *vrxv1.SecretBundle) {
	if bundle == nil {
		return
	}
	for key, value := range bundle.Values {
		vpn.Zero(value)
		delete(bundle.Values, key)
	}
}

func (s *Service) restoreSecretSelection() {
	// Leave all snapshots available to the outer panic containment/recovery path.
	if fault := recover(); fault != nil {
		panic(fault)
	}
	if s.secrets == nil {
		return
	}
	if e := s.secrets.Activate(s.st.meta.SecretBundle); e != nil {
		s.setDegraded(true, "durable secret snapshot unavailable")
		return
	}
	if !s.isDegraded() {
		if e := s.secrets.Retain(s.st.meta.SecretBundle, s.st.meta.ConfirmedSecretBundle); e != nil {
			s.log.Warn("obsolete secret snapshot cleanup deferred")
		}
	}
}
