package strongswan

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/netip"
	"path/filepath"
	"regexp"
	"strings"

	ngfwv1 "ngfw/agent/gen/ngfw/v1"
)

// RAFiles are exclusively private daemon inputs, never a state/RPC payload.
// String deliberately prevents fmt from serializing credential-bearing bytes.
type RAFiles struct {
	Connection []byte `json:"-"`
	Secrets    []byte `json:"-"`
	Daemon     []byte `json:"-"`
}

func (RAFiles) String() string         { return "remote-access private files <redacted>" }
func (files RAFiles) GoString() string { return files.String() }

var raUserName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.@-]{0,63}$`)

// BuildRAFiles renders an independent kernel-netlink daemon. It never passes
// these profiles through the retired kernel-vpp/site-to-site renderer.
// root is the trusted private instance directory; credentials are copied there
// by the runtime after PKI verification. Every emitted file must be mode0600.
func BuildRAFiles(ctx context.Context, name string, profile *ngfwv1.RemoteAccessProfile,
	proposal *ngfwv1.IpsecProposal, root string, resolve SecretResolver) (*RAFiles, error) {
	refuse := func(field string) (*RAFiles, error) { return nil, fmt.Errorf("remote-access: invalid %s", field) }
	if !objectNameRe.MatchString(name) || profile == nil || proposal == nil {
		return refuse("profile or proposal")
	}
	if !filepath.IsAbs(root) || filepath.Clean(root) != root || strings.ContainsAny(root, "\n\r\x00") {
		return refuse("private root")
	}
	if _, err := netip.ParseAddr(profile.GetLocalAddr()); err != nil {
		return refuse("local address")
	}
	if !objectNameRe.MatchString(profile.GetCertificate()) {
		return refuse("server certificate")
	}
	if profile.GetAuth() == "eap-tls" || profile.GetAuth() == "pubkey" {
		if !objectNameRe.MatchString(profile.GetClientCa()) {
			return refuse("client CA")
		}
	}
	ike, err := IKEProposal(proposal.GetIke().GetEncr(), proposal.GetIke().GetInteg(), proposal.GetIke().GetPrf(), proposal.GetIke().GetDh())
	if err != nil {
		return refuse("IKE proposal")
	}
	esp, err := ChildProposal("esp", proposal.GetEsp().GetEncr(), proposal.GetEsp().GetInteg(), proposal.GetEsp().GetDh(), false)
	if err != nil {
		return refuse("ESP proposal")
	}
	id := profile.GetLocalId()
	if id == "" {
		id = profile.GetLocalAddr()
	}
	if _, err := Identity(id); err != nil {
		return refuse("local identity")
	}
	qid, _ := Quote(id)
	connName, err := ConnName(name)
	if err != nil {
		return refuse("profile name")
	}
	connName = "ra-" + connName
	if _, err := SectionName(connName); err != nil {
		return refuse("profile name length")
	}
	quoted := func(value string) (string, error) { return Quote(value) }
	certificate, err := quoted(filepath.Join(root, "x509", profile.GetCertificate()+".pem"))
	if err != nil {
		return refuse("certificate path")
	}
	auth := profile.GetAuth()
	if auth != "eap-mschapv2" && auth != "eap-tls" && auth != "eap-radius" && auth != "pubkey" {
		return refuse("authentication method")
	}
	var config, secrets, daemon strings.Builder
	fmt.Fprintf(&config, "connections { %s { version = 2\n local_addrs = %s\n remote_addrs = %%any\n proposals = %s\n send_certreq = yes\n fragmentation = yes\n mobike = yes\n local { auth = pubkey\n id = %s\n certs = %s\n }\n remote { auth = %s\n", connName, profile.GetLocalAddr(), ike, qid, certificate, auth)
	if auth != "pubkey" {
		config.WriteString(" eap_id = %any\n")
	}
	if auth == "eap-tls" || auth == "pubkey" {
		ca, err := quoted(filepath.Join(root, "x509ca", profile.GetClientCa()+".pem"))
		if err != nil {
			return refuse("CA path")
		}
		fmt.Fprintf(&config, " cacerts = %s\n revocation = strict\n", ca)
	}
	config.WriteString(" }\n")
	dpd := profile.GetDpd()
	if dpd == nil || dpd.Enabled == nil || dpd.GetEnabled() {
		delay := uint32(30)
		if dpd != nil && dpd.GetDelaySec() != 0 {
			delay = dpd.GetDelaySec()
		}
		if delay > 86400 {
			return refuse("DPD delay")
		}
		fmt.Fprintf(&config, " dpd_delay = %ds\n", delay)
	}
	rekey := profile.GetRekey()
	ikeTime, espTime := uint32(14400), uint32(3600)
	if rekey != nil {
		if rekey.GetIkeSec() != 0 {
			ikeTime = rekey.GetIkeSec()
		}
		if rekey.GetEspSec() != 0 {
			espTime = rekey.GetEspSec()
		}
	}
	if ikeTime < 60 || ikeTime > 604800 || espTime < 60 || espTime > 604800 {
		return refuse("rekey interval")
	}
	if rekey.GetReauth() {
		fmt.Fprintf(&config, " rekey_time = 0s\n reauth_time = %ds\n", ikeTime)
	} else {
		fmt.Fprintf(&config, " rekey_time = %ds\n", ikeTime)
	}
	pools := profile.GetPools()
	if len(pools) == 0 || len(pools) > 16 {
		return refuse("pool count")
	}
	names := make([]string, 0, len(pools))
	seenPools := map[string]bool{}
	for _, pool := range pools {
		if pool == nil || !objectNameRe.MatchString(pool.GetName()) || seenPools[pool.GetName()] {
			return refuse("pool names")
		}
		seenPools[pool.GetName()] = true
		pname, err := ConnName(pool.GetName())
		if err != nil {
			return refuse("pool name")
		}
		names = append(names, pname)
		if p, err := netip.ParsePrefix(pool.GetPrefix()); err != nil || p.Masked().String() != pool.GetPrefix() {
			return refuse("pool prefix")
		}
	}
	selectors := profile.GetSplitTunnel()
	if len(selectors) == 0 {
		selectors = []string{"0.0.0.0/0", "::/0"}
	}
	if len(selectors) > 64 {
		return refuse("split route count")
	}
	for _, selector := range selectors {
		if _, err := TrafficSelector(selector); err != nil {
			return refuse("split route")
		}
	}
	fmt.Fprintf(&config, " pools = %s\n children { protected { local_ts = %s\n remote_ts = dynamic\n esp_proposals = %s\n if_id_in = 1\n if_id_out = 1\n rekey_time = %ds\n start_action = none\n dpd_action = clear\n } }\n } }\npools {\n", strings.Join(names, ", "), strings.Join(selectors, ", "), esp, espTime)
	for index, pool := range pools {
		if len(pool.GetDns()) > 4 {
			return refuse("DNS server count")
		}
		for _, dns := range pool.GetDns() {
			if _, err := netip.ParseAddr(dns); err != nil {
				return refuse("DNS server")
			}
		}
		fmt.Fprintf(&config, " %s { addrs = %s\n", names[index], pool.GetPrefix())
		if len(pool.GetDns()) > 0 {
			fmt.Fprintf(&config, " dns = %s\n", strings.Join(pool.GetDns(), ", "))
		}
		config.WriteString(" }\n")
	}
	config.WriteString("}\n")
	secrets.WriteString("secrets {\n")
	if auth == "eap-mschapv2" {
		if len(profile.GetUsers()) == 0 || len(profile.GetUsers()) > 1024 || resolve == nil {
			return refuse("EAP users or sealed resolver")
		}
		seen := map[string]bool{}
		for index, user := range profile.GetUsers() {
			if user == nil || !raUserName.MatchString(user.GetUsername()) || seen[user.GetUsername()] {
				return refuse("EAP username")
			}
			seen[user.GetUsername()] = true
			if !secretRefRe.MatchString(user.GetPasswordRef()) || !strings.HasPrefix(user.GetPasswordRef(), "password/") {
				return refuse("password reference")
			}
			value, err := resolve.Resolve(ctx, user.GetPasswordRef())
			if err != nil || len(value) == 0 || len(value) > 1024 {
				return refuse("EAP secret resolution")
			}
			quser, _ := Quote(user.GetUsername())
			fmt.Fprintf(&secrets, " eap-%d { id = %s\n secret = 0s%s\n }\n", index, quser, base64.StdEncoding.EncodeToString(value))
		}
	}
	secrets.WriteString("}\n")
	socket, err := quoted("unix://" + filepath.Join(root, "vici.sock"))
	if err != nil {
		return refuse("VICI socket")
	}
	plugins := "random nonce openssl pem pkcs1 pkcs8 x509 pubkey revocation constraints socket-default kernel-netlink vici eap-identity " + auth
	if auth == "pubkey" {
		plugins = strings.TrimSuffix(plugins, " pubkey")
	}
	if auth == "eap-mschapv2" {
		plugins += " md4"
	}
	if auth == "eap-tls" {
		plugins += " tls"
	}
	fmt.Fprintf(&daemon, "charon { load_modular = no\n load = \"%s\"\n install_routes = no\n install_virtual_ip = no\n plugins { kernel-netlink { install_routes_xfrmi = no\n }\n vici { socket = %s\n }\n", plugins, socket)
	if auth == "eap-radius" {
		radius := profile.GetRadius()
		if radius == nil || len(radius.GetServers()) == 0 || len(radius.GetServers()) > 8 || resolve == nil {
			return refuse("RADIUS servers or sealed resolver")
		}
		daemon.WriteString(" eap-radius { servers {\n")
		for index, server := range radius.GetServers() {
			if server == nil || !secretRefRe.MatchString(server.GetSecretRef()) || !strings.HasPrefix(server.GetSecretRef(), "psk/") {
				return refuse("RADIUS secret reference")
			}
			// Numeric addresses keep routing deterministic; hostname resolution would
			// require a separately configured namespace DNS path.
			if _, err := netip.ParseAddr(server.GetAddress()); err != nil {
				return refuse("RADIUS numeric address")
			}
			port := server.GetPort()
			if port == 0 {
				port = 1812
			}
			if port > 65535 {
				return refuse("RADIUS port")
			}
			value, err := resolve.Resolve(ctx, server.GetSecretRef())
			if err != nil || len(value) == 0 || len(value) > 1024 {
				return refuse("RADIUS secret resolution")
			}
			qsecret, err := Quote(string(value))
			if err != nil {
				return refuse("RADIUS secret encoding")
			}
			fmt.Fprintf(&daemon, " server%d { address = %s\n port = %d\n secret = %s\n }\n", index, server.GetAddress(), port, qsecret)
		}
		daemon.WriteString(" } }\n")
	}
	daemon.WriteString(" }\n journal { default = -1\n }\n syslog { daemon { default = -1\n } }\n}\n")
	return &RAFiles{Connection: []byte(config.String()), Secrets: []byte(secrets.String()), Daemon: []byte(daemon.String())}, nil
}
