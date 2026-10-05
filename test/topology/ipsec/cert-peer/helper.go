// Test-only helper appended to the isolated native packet fixture via Go overlay.
package desired

func certPeerMaterial(t *testing.T) (map[string][]byte, []byte, []byte, []byte) {
	t.Helper()
	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	ca := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "w8-private-ca"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	caDER, err := x509.CreateCertificate(rand.Reader, ca, ca, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	leaf := func(name string, serial int64) ([]byte, []byte) {
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			t.Fatal(err)
		}
		cert := &x509.Certificate{SerialNumber: big.NewInt(serial), Subject: pkix.Name{CommonName: name}, DNSNames: []string{name}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth}}
		der, err := x509.CreateCertificate(rand.Reader, cert, ca, &key.PublicKey, caKey)
		if err != nil {
			t.Fatal(err)
		}
		return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	}
	localCert, localKey := leaf("local.test", 2)
	peerCert, peerKey := leaf("remote.test", 3)
	return map[string][]byte{"cert/local": localCert, "key/local": localKey, "cert/peer": peerCert}, peerCert, peerKey, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})
}

func certPeerLoadKey(t *testing.T, ctx context.Context, socket string, key []byte) {
	t.Helper()
	block, _ := pem.Decode(key)
	if block == nil {
		t.Fatal("private peer key missing")
	}
	defer clear(block.Bytes)
	vc, err := strongswan.DialVICI(ctx, socket)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = vc.Close() }()
	message := vici.NewMessage()
	if err := message.Set("type", "RSA"); err != nil {
		t.Fatal(err)
	}
	if err := message.Set("data", string(block.Bytes)); err != nil {
		t.Fatal(err)
	}
	response, err := vc.Call(ctx, "load-key", message)
	if err != nil {
		t.Fatal("private VICI key load failed", err)
	}
	if response.Get("success") != "yes" {
		t.Fatal("private VICI key load refused")
	}
}

func certPeerLoadCertificate(t *testing.T, ctx context.Context, socket string, data []byte, flag string) {
	t.Helper()
	block, _ := pem.Decode(data)
	if block == nil {
		t.Fatal("public peer fixture certificate missing")
	}
	vc, err := strongswan.DialVICI(ctx, socket)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = vc.Close() }()
	request := vici.NewMessage()
	for key, value := range map[string]string{"type": "X509", "flag": flag, "data": string(block.Bytes)} {
		if err := request.Set(key, value); err != nil {
			t.Fatal(err)
		}
	}
	response, err := vc.Call(ctx, "load-cert", request)
	if err != nil || response.Get("success") != "yes" {
		t.Fatal("private VICI public certificate load refused", err)
	}
}
