package ravpn

import (
	"context"
	"fmt"
	"google.golang.org/protobuf/encoding/protojson"
	ngfwv1 "ngfw/agent/gen/ngfw/v1"
	"ngfw/agent/internal/renderers/strongswan"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestIntegrationSealedPreparationImmutableGenerationRecovery(t *testing.T) {
	if os.Getenv("NGFW_INTEGRATION") != "1" {
		t.Skip("requires owned namespace fixture")
	}
	profile := new(ngfwv1.RemoteAccessProfile)
	proposal := new(ngfwv1.IpsecProposal)
	if protojson.Unmarshal([]byte(`{"localAddr":"192.0.2.19","localId":"vpn.example.test","auth":"eap-mschapv2","certificate":"server","pools":[{"name":"clients","prefix":"10.19.200.0/24"}],"splitTunnel":["10.19.0.0/16"],"users":[{"username":"client","passwordRef":"password/client"}],"transport":{"outer":{"vpp":"198.18.19.0/31","namespace":"198.18.19.1/31"},"inner":{"vpp":"198.18.19.2/31","namespace":"198.18.19.3/31"}},"outerPolicy":{"ingress":["public-in"],"egress":["public-out"]},"accessPolicy":{"ingress":["client-in"],"egress":["client-out"]}}`), profile) != nil || protojson.Unmarshal([]byte(`{"ike":{"encr":"aes256","integ":"sha256","prf":"prfsha256","dh":"ecp256"},"esp":{"encr":"aes256gcm16","dh":"ecp256"}}`), proposal) != nil {
		t.Fatal("profile")
	}
	certRef, keyRef := "cert/server", "key/server"
	spec := EngineSpec{Owner: "w19-sealed", Profile: strconv.Itoa(os.Getpid()), Configuration: profile, Proposal: proposal, OuterID: 2432, InnerID: 2433, ServerCertificate: &ngfwv1.PkiCertificate{CertificateRef: &certRef, PrivateKeyRef: &keyRef}, Fingerprints: map[string]string{}}
	spec.Instance = InstanceID(spec.Owner, spec.Profile)
	plan, e := BuildNetworkPlan(spec.Owner, spec.Profile, profile)
	if e != nil {
		t.Fatal(e)
	}
	credentials, now := credentialsFixture(t)
	blobs := map[string][]byte{}
	for i, item := range []struct {
		ref  string
		data []byte
	}{{certRef, credentials.Certificate}, {keyRef, credentials.PrivateKey}, {"password/client", []byte("NGFW_TEST_PRIVATE_PASSWORD")}} {
		h := fmt.Sprintf("hmac:%064x", i+1)
		spec.Fingerprints[item.ref] = h
		blobs[h] = item.data
	}
	adapter := &SealedPreparation{Now: func() time.Time { return now }, Resolver: strongswan.SecretResolverFunc(func(_ context.Context, ref string) ([]byte, error) {
		data, ok := blobs[ref]
		if !ok {
			t.Error("literal/foreign reference resolved")
			return nil, ErrEngine
		}
		return append([]byte(nil), data...), nil
	})}
	if e := adapter.Validate(context.Background(), spec); e != nil {
		t.Fatal("pre-mutation sealed validation", e)
	}
	if _, e := os.Stat(filepath.Join(InstanceRoot, spec.Instance)); !os.IsNotExist(e) {
		t.Fatal("validation created private namespace/files")
	}
	originalKey := blobs[spec.Fingerprints[keyRef]]
	blobs[spec.Fingerprints[keyRef]] = []byte("invalid private material")
	if adapter.Validate(context.Background(), spec) == nil {
		t.Fatal("invalid sealed private key accepted before mutation")
	}
	blobs[spec.Fingerprints[keyRef]] = originalKey
	createEAPNamespace(t, plan)
	prepared, e := adapter.Prepare(context.Background(), spec)
	if e != nil {
		t.Fatal("prepare", e)
	}
	if _, e := adapter.Prepare(context.Background(), spec); e == nil {
		t.Fatal("existing generation overwritten")
	}
	recovered, e := adapter.Recover(context.Background(), spec)
	if e != nil {
		t.Fatal("recover", e)
	}
	path := filepath.Join(InstanceRoot, spec.Instance, "strongswan.conf")
	// #nosec G304 -- reads the fixed immutable snapshot fixture manifest before deliberate corruption and restoration.
	original, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	if os.WriteFile(path, []byte("changed same inode"), 0600) != nil {
		t.Fatal("tamper fixture")
	}
	if _, e := adapter.Recover(context.Background(), spec); e == nil {
		t.Fatal("changed generation recovered")
	}
	// #nosec G703 -- path is the exact fixed manifest in the owned fixture instance; restores authenticated test bytes only.
	if os.WriteFile(path, original, 0600) != nil {
		t.Fatal("restore")
	}
	if e := recovered.Cleanup(context.Background()); e != nil {
		t.Fatal(e)
	}
	if prepared.Load(context.Background(), nil) == nil {
		t.Fatal("nil VICI loaded")
	}
}
