package ravpn

import (
	"context"
	"golang.org/x/sys/unix"
	"ngfw/agent/internal/vpp/bootid"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

func managerTripletFixture() (string, string, string) {
	socket, service := managerPairFixture()
	source := "ExecStart={ path=/usr/sbin/ngfw-agent ; argv[]=/usr/sbin/ngfw-agent ; }\nMainPID=" + strconv.Itoa(os.Getpid()) + "\nControlGroup=/system.slice/ngfw-agent.service\nFragmentPath=/usr/lib/systemd/system/ngfw-agent.service\nDropInPaths=\nUser=root\nGroup=ngfw\nId=ngfw-agent.service"
	return socket, service, source
}

func TestNumericPublisherTripletReorderedRolesAndMinimums(t *testing.T) {
	socket, service, source := managerTripletFixture()
	for _, blocks := range [][]string{{source, socket, service}, {service, source, socket}, {socket, service, source}} {
		snapshot, err := parseNumericPublisherManagerTriplet([]byte(strings.Join(blocks, "\n\n") + "\n"))
		if err != nil || !numericPublisherSocketState(snapshot.publisher.socket, false) || !numericPublisherCgroup(snapshot.publisher.service, false) {
			t.Fatalf("valid role minimums refused: %v", err)
		}
		if snapshot.source["MainPID"] != strconv.Itoa(os.Getpid()) || snapshot.publisher.service["MainPID"] != "0" || snapshot.source["ExecStart"] == snapshot.publisher.service["ExecStart"] {
			t.Fatal("source/server roles contaminated")
		}
		if _, ok := snapshot.source["Listen"]; ok {
			t.Fatal("Source adopted socket role")
		}
	}
}

func TestNumericPublisherTripletRejectsForeignAmbiguousRoles(t *testing.T) {
	socket, service, source := managerTripletFixture()
	valid := socket + "\n\n" + service + "\n\n" + source + "\n"
	cases := map[string]string{
		"missing-source":          socket + "\n\n" + service,
		"duplicate-source":        socket + "\n\n" + source + "\n\n" + source,
		"duplicate-publisher":     socket + "\n\n" + socket + "\n\n" + source,
		"foreign-source-id":       strings.Replace(valid, "Id=ngfw-agent.service", "Id=foreign.service", 1),
		"missing-source-property": strings.Replace(valid, "ControlGroup=/system.slice/ngfw-agent.service\n", "", 1),
		"cross-role-mainpid":      strings.Replace(valid, "MainPID="+strconv.Itoa(os.Getpid())+"\n", "", 1),
		"duplicate-property":      valid + "User=root\n",
		"duplicate-id":            valid + "Id=ngfw-agent.service\n",
		"unknown-property":        valid + "Foreign=yes\n",
		"non-key":                 valid + "unexpected\n",
		"missing-separator":       strings.Replace(valid, "\n\n", "\n", 1),
		"extra-unit":              valid + "\nId=foreign.service\n",
		"nul":                     valid + "\x00",
		"cr":                      strings.ReplaceAll(valid, "\n", "\r\n"),
		"invalid-utf8":            valid + string([]byte{255}),
		"oversized":               valid + strings.Repeat("x", 16384),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			if snapshot, err := parseNumericPublisherManagerTriplet([]byte(data)); err != ErrBoundary || snapshot != nil {
				t.Fatal("ambiguous/foreign triplet accepted or partial state exposed")
			}
		})
	}
}

func tripletMetadataProof(t *testing.T) *numericPublisherInstallationProof {
	t.Helper()
	if os.Geteuid() != 0 {
		t.Skip("root-owned private held metadata fixture")
	}
	proof := &numericPublisherInstallationProof{source: (bootid.Reader{}).ForPID(os.Getpid())}
	for i := 0; i < 4; i++ {
		path := filepath.Join(t.TempDir(), "owned-artifact")
		if err := os.WriteFile(path, []byte("private metadata guard fixture"), 0600); err != nil {
			t.Fatal(err)
		}
		artifact, err := openNumericPublisherArtifact(path, 1024, false)
		if err != nil {
			t.Fatal(err)
		}
		proof.files = append(proof.files, artifact)
	}
	t.Cleanup(func() {
		if err := proof.Close(); err != nil {
			t.Error(err)
		}
	})
	return proof
}

func tripletMetadataSnapshot(t *testing.T, proof *numericPublisherInstallationProof) *numericPublisherTrustSnapshot {
	t.Helper()
	socket, service, source := managerTripletFixture()
	snapshot, err := parseNumericPublisherManagerTriplet([]byte(socket + "\n\n" + service + "\n\n" + source))
	if err != nil {
		t.Fatal(err)
	}
	snapshot.proof = proof
	snapshot.identity = (bootid.Reader{}).ForPID(os.Getpid())
	return snapshot
}

func TestNumericPublisherTripletSingleUseAndFreshHeldGuards(t *testing.T) {
	proof := tripletMetadataProof(t)
	snapshot := tripletMetadataSnapshot(t, proof)
	if _, err := snapshot.sourceProperties(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := snapshot.publisherProperties(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := snapshot.sourceProperties(context.Background()); err != ErrBoundary {
		t.Fatal("Source role reused across a boundary")
	}
	if _, err := snapshot.publisherProperties(context.Background()); err != ErrBoundary {
		t.Fatal("publisher role reused across a boundary")
	}
	fresh := tripletMetadataSnapshot(t, proof)
	if _, err := fresh.sourceProperties(context.Background()); err != nil {
		t.Fatal(err)
	}
	// The owned metadata fixture is not an installed-engine readiness attestation.
	// Changing its held artifact after one consumer must refuse the other consumer.
	if err := os.WriteFile(proof.files[0].path, []byte("replaced byte content"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := fresh.publisherProperties(context.Background()); err != ErrBoundary {
		t.Fatal("changed held proof accepted between role consumers")
	}
}

func TestNumericPublisherTripletRefusesChangedIdentityAndCaller(t *testing.T) {
	proof := tripletMetadataProof(t)
	changed := tripletMetadataSnapshot(t, proof)
	changed.identity.StartTime++
	if _, err := changed.sourceProperties(context.Background()); err != ErrBoundary {
		t.Fatal("changed Source start identity accepted")
	}
	fresh := tripletMetadataSnapshot(t, proof)
	ctx, cancel := context.WithTimeout(context.Background(), time.Nanosecond)
	defer cancel()
	<-ctx.Done()
	if _, err := fresh.publisherProperties(ctx); err != ErrBoundary {
		t.Fatal("short caller deadline ignored")
	}
	if _, err := fresh.sourceProperties(ctx); err != ErrBoundary {
		t.Fatal("short Source deadline ignored")
	}
	if numericPublisherPeerFromTrust(context.Background(), nil, bootid.Identity{}, fresh) != ErrBoundary {
		t.Fatal("foreign Source identity accepted")
	}
}

func TestNumericPublisherTripletFixedCommandAndSourceGuardBeforeGetter(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	command := numericPublisherTripletCommand(ctx)
	want := []string{"/usr/bin/systemctl", "show", "--all", "--property=" + numericPublisherManagerProperties, "ngfw-ra-openfile.socket", "ngfw-ra-openfile.service", "ngfw-agent.service"}
	if !reflect.DeepEqual(command.Args, want) || command.WaitDelay != time.Second {
		t.Fatal("fixed three-unit argv/reap bound changed")
	}
	if err := command.Run(); err == nil {
		t.Fatal("cancelled command executed")
	}
	if command.Process != nil {
		t.Fatal("already-cancelled context launched host manager process")
	}
	called := false
	getter := func(context.Context) (map[string]string, error) { called = true; return map[string]string{}, nil }
	if verifyFixedAgentPeerUsing(context.Background(), nil, bootid.Identity{}, getter) != ErrBoundary || called {
		t.Fatal("nil foreign peer reached manager getter")
	}
	if verifyFixedAgentIdentityUsing(context.Background(), &unix.Ucred{Pid: 0, Uid: 0, Gid: 0}, bootid.Identity{}, getter) != ErrBoundary || called {
		t.Fatal("zero-PID foreign identity reached getter")
	}
}
