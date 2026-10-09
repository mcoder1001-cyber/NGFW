package pppoe

import (
	"bytes"
	"errors"
	"path/filepath"

	"ngfw/agent/internal/renderers"
)

// CarrierPaths uses a per-namespace private peer tree mounted read-only at
// /etc/ppp by the fixed packaged service. State remains shared with the agent.
func CarrierPaths(base, state string) Paths {
	return Paths{PeersDir: filepath.Join(base, "peers"), ChapSecrets: filepath.Join(base, "chap-secrets"), PapSecrets: filepath.Join(base, "pap-secrets"), IPUpDir: filepath.Join(base, "ip-up.d"), IPDownDir: filepath.Join(base, "ip-down.d"), IPv6UpDir: filepath.Join(base, "ipv6-up.d"), IPv6DownDir: filepath.Join(base, "ipv6-down.d"), HelperDir: base, UnitDir: filepath.Join(base, "unused-units"), StateDir: state}
}

// RenderCarrier renders one isolated session and never emits a systemd unit.
// Dispatchers are packaged immutable assets and must be installed separately.
func (r *Renderer) RenderCarrier(s Session) (renderers.Files, error) {
	if s.Carrier == nil {
		return nil, errors.New("kernel PPP carrier specification is required")
	}
	files, err := r.Render([]Session{s})
	if err != nil {
		return nil, err
	}
	delete(files, filepath.Join(r.paths.UnitDir, "ngfw-pppoe-"+s.HostIf+".service"))
	peer := filepath.Join(r.paths.PeersDir, "ngfw-"+s.HostIf)
	files[filepath.Join(r.paths.PeersDir, "carrier")] = files[peer]
	delete(files, peer)
	// Templates execute inside the fixed service's private /etc/ppp mount. Only
	// generated executable/config text changes path; credentials are untouched.
	for path, file := range files {
		if !file.Secret {
			file.Content = bytes.ReplaceAll(file.Content, []byte(r.paths.HelperDir+"/"), []byte("/etc/ppp/"))
			files[path] = file
		}
	}
	return files, files.Validate()
}
