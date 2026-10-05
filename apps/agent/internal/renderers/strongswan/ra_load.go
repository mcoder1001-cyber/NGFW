package strongswan

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"github.com/strongswan/govici/vici"
	"slices"
	"strings"
)

var ErrRALoad = errors.New("remote-access private configuration load refused")

// RAMaterial contains exactly the verified profile files; no arbitrary path
// read is performed while building requests or following a VICI response.
type RAMaterial struct {
	Certificates map[string][]byte `json:"-"`
	PrivateKey   []byte            `json:"-"`
	CRL          []byte            `json:"-"`
}

func (RAMaterial) String() string     { return "remote-access VICI material <redacted>" }
func (m RAMaterial) GoString() string { return m.String() }

func raSectionMessage(section *Section, material RAMaterial) (*vici.Message, error) {
	message := vici.NewMessage()
	for _, item := range section.Items {
		switch item.Kind {
		case KindSection:
			child, err := raSectionMessage(item.Section, material)
			if err != nil {
				return nil, ErrRALoad
			}
			if message.Set(item.Name, child) != nil {
				return nil, ErrRALoad
			}
		case KindKey:
			var value any = item.Value
			if slices.Contains(listKeys, item.Name) {
				value = splitList(item.Value)
			}
			if slices.Contains(fileListKeys, item.Name) {
				var blobs []string
				for _, path := range splitList(item.Value) {
					data, exists := material.Certificates[path]
					if !exists || len(data) == 0 || len(data) > MaxCertFile {
						return nil, ErrRALoad
					}
					blobs = append(blobs, string(data))
				}
				value = blobs
			}
			if message.Set(item.Name, value) != nil {
				return nil, ErrRALoad
			}
		}
	}
	return message, nil
}

// LoadRA loads into a newly started, empty per-profile daemon. A failure must
// cause the controller to stop that daemon; partial trust is never reused.
// Verification of keys, certificate/CRL trust and private socket PID is caller's
// prerequisite. All remote error text is discarded to prevent secret echo.
func LoadRA(ctx context.Context, client ViciConn, files *RAFiles, material RAMaterial) (string, error) {
	if client == nil || files == nil || len(material.PrivateKey) == 0 || len(material.PrivateKey) > 16384 || len(material.Certificates) == 0 || len(material.Certificates) > 2 {
		return "", ErrRALoad
	}
	config, err := RoundTrip("remote-access connection", files.Connection)
	if err != nil {
		return "", ErrRALoad
	}
	secrets, err := RoundTrip("remote-access secrets", files.Secrets)
	if err != nil {
		return "", ErrRALoad
	}
	connections := config.Sub("connections")
	if connections == nil || len(connections.Sections()) != 1 {
		return "", ErrRALoad
	}
	connection := connections.Sections()[0]
	if !strings.HasPrefix(connection.Name, "ra-") {
		return "", ErrRALoad
	}
	prior, err := client.Call(ctx, "get-conns", nil)
	if err != nil || prior == nil {
		return "", ErrRALoad
	}
	priorConnections, ok := prior.Get("conns").([]string)
	if !ok || len(priorConnections) != 0 {
		return "", ErrRALoad
	}
	load := func(command string, message *vici.Message) error {
		response, err := client.Call(ctx, command, message)
		if err != nil || response == nil || str(response, "success") != "yes" {
			return ErrRALoad
		}
		return nil
	}
	for _, data := range material.Certificates {
		remaining := data
		for len(strings.TrimSpace(string(remaining))) != 0 {
			block, rest := pem.Decode(remaining)
			if block == nil || block.Type != "CERTIFICATE" {
				return "", ErrRALoad
			}
			certificate, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				return "", ErrRALoad
			}
			flag := "NONE"
			if certificate.IsCA {
				flag = "CA"
			}
			if load("load-cert", msg("type", "X509", "flag", flag, "data", string(block.Bytes))) != nil {
				return "", ErrRALoad
			}
			remaining = rest
		}
	}
	if len(material.CRL) > 0 {
		if len(material.CRL) > 4<<20 || load("load-cert", msg("type", "X509_CRL", "flag", "NONE", "data", string(material.CRL))) != nil {
			return "", ErrRALoad
		}
	}
	keyMessage := msg("type", "any", "data", string(material.PrivateKey))
	keyErr := load("load-key", keyMessage)
	keyMessage.Unset("data")
	if keyErr != nil {
		return "", ErrRALoad
	}
	if root := secrets.Sub("secrets"); root != nil {
		for _, item := range root.Sections() {
			if !strings.HasPrefix(item.Name, "eap-") {
				return "", ErrRALoad
			}
			identity, ok := item.Section.Get("id")
			if !ok || !raUserName.MatchString(identity) {
				return "", ErrRALoad
			}
			encoded, ok := item.Section.Get("secret")
			if !ok {
				return "", ErrRALoad
			}
			data, err := decodeSecret(encoded)
			if err != nil || len(data) > 1024 {
				return "", ErrRALoad
			}
			message := msg("id", item.Name, "type", "EAP", "owners", []string{identity}, "data", string(data))
			clear(data)
			err = load("load-shared", message)
			message.Unset("data")
			if err != nil {
				return "", ErrRALoad
			}
		}
	}
	if pools := config.Sub("pools"); pools != nil {
		for _, item := range pools.Sections() {
			body := vici.NewMessage()
			for _, key := range item.Section.Keys() {
				var value any = splitList(key.Value)
				if key.Name == "addrs" {
					value = key.Value
				}
				if body.Set(key.Name, value) != nil {
					return "", ErrRALoad
				}
			}
			if load("load-pool", msg(item.Name, body)) != nil {
				return "", ErrRALoad
			}
		}
	}
	body, err := raSectionMessage(connection.Section, material)
	if err != nil {
		return "", ErrRALoad
	}
	if load("load-conn", msg(connection.Name, body)) != nil {
		return "", ErrRALoad
	}
	actual, err := client.Call(ctx, "get-conns", nil)
	if err != nil || actual == nil || !slices.Equal(strs(actual, "conns"), []string{connection.Name}) {
		return "", ErrRALoad
	}
	return connection.Name, nil
}
