package ravpn

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/godbus/dbus/v5"
	"ngfw/agent/internal/vpp/bootid"
)

func TestNumericPublisherManagerDBusTypesPreserveOriginalPredicates(t *testing.T) {
	values := map[string]any{
		"Id": "ngfw-ra-openfile.service", "MainPID": uint32(0), "ControlPID": uint32(0), "ActiveState": "inactive", "SubState": "dead", "ControlGroup": "", "FragmentPath": numericPublisherService, "DropInPaths": []string{}, "User": "root", "Group": "ngfw", "CapabilityBoundingSet": uint64(0), "NoNewPrivileges": true,
		"ExecStart": []managerDBusExec{{Path: unitObserverExecutable, Arguments: []string{unitObserverExecutable, "--publish-openfile"}}},
	}
	service := map[string]string{}
	for key, value := range values {
		text, err := managerDBusPropertyValue(key, dbus.MakeVariant(value))
		if err != nil {
			t.Fatal(key, err)
		}
		service[key] = text
	}
	socket := map[string]string{"Id": "ngfw-ra-openfile.socket", "FragmentPath": numericPublisherSocket, "DropInPaths": "", "ActiveState": "active", "SubState": "listening"}
	listen, err := managerDBusPropertyValue("Listen", dbus.MakeVariant([]managerDBusListen{{"SequentialPacket", numericPublisherSocketPath}}))
	if err != nil {
		t.Fatal(err)
	}
	socket["Listen"] = listen
	if !numericPublisherSocketState(socket, false) || !numericPublisherCgroup(service, false) {
		t.Fatal("native values changed original socket/inactive predicates")
	}
	proof := tripletMetadataProof(t)
	if err := numericPublisherManagerUsing(context.Background(), bootid.Identity{}, proof, func(context.Context) (numericPublisherManagerSnapshot, error) {
		return numericPublisherManagerSnapshot{socket: socket, service: service}, nil
	}); err != nil {
		t.Fatal("native values failed original manager predicates", err)
	}
	source, err := managerDBusPropertyValue("ExecStart", dbus.MakeVariant([]managerDBusExec{{Path: "/usr/sbin/ngfw-agent", Arguments: []string{"/usr/sbin/ngfw-agent"}}}))
	if err != nil || !strings.Contains(source, "path=/usr/sbin/ngfw-agent ; argv[]=/usr/sbin/ngfw-agent ;") {
		t.Fatal("Source executable predicate changed")
	}
	foreign := map[string]string{}
	for key, value := range socket {
		foreign[key] = value
	}
	foreign["Listen"] = strings.Replace(listen, "SequentialPacket", "Stream", 1)
	if numericPublisherSocketState(foreign, false) {
		t.Fatal("foreign socket type accepted")
	}
}

func TestNumericPublisherManagerDBusTypesRefuseMalformedValues(t *testing.T) {
	cases := []struct {
		key   string
		value any
	}{
		{"MainPID", int32(1)}, {"MainPID", uint32(0xffffffff)}, {"CapabilityBoundingSet", uint32(0)}, {"NoNewPrivileges", "yes"}, {"Id", "foreign\nId=ngfw-agent.service"}, {"DropInPaths", []string{"/path with spaces"}},
		{"Listen", []managerDBusListen{}}, {"Listen", []managerDBusListen{{"SequentialPacket", "one"}, {"SequentialPacket", "two"}}},
		{"ExecStart", []managerDBusExec{{Path: "/foreign ; path=/usr/sbin/ngfw-agent", Arguments: []string{"x"}}}}, {"ExecStart", []managerDBusExec{{Path: "/usr/sbin/ngfw-agent", Arguments: []string{"x ; argv[]=/usr/sbin/ngfw-agent"}}}},
		{"Unknown", "value"},
	}
	for _, test := range cases {
		if _, err := managerDBusPropertyValue(test.key, dbus.MakeVariant(test.value)); err != ErrBoundary {
			t.Fatal("malformed native property accepted", test.key)
		}
	}
	if value, err := managerDBusPropertyValue("CapabilityBoundingSet", dbus.MakeVariant(uint64(1))); err != nil || value == "" {
		t.Fatal("nonzero capabilities converted to empty")
	}
	if value, err := managerDBusPropertyValue("NoNewPrivileges", dbus.MakeVariant(false)); err != nil || value == "yes" {
		t.Fatal("false NNP converted to yes")
	}
}

func TestNumericPublisherManagerDBusFixedInterfacesAndForeignRoles(t *testing.T) {
	if managerDBusPropertyInterface(managerDBusSourceRole, "ControlGroup") != "org.freedesktop.systemd1.Service" || managerDBusPropertyInterface(managerDBusPublisherRoles[0], "Listen") != "org.freedesktop.systemd1.Socket" || managerDBusPropertyInterface(managerDBusSourceRole, "Id") != "org.freedesktop.systemd1.Unit" {
		t.Fatal("systemd259 fixed property interface changed")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := readManagerDBusRoles(ctx, managerDBusPublisherRoles); err != ErrBoundary {
		t.Fatal("cancelled caller reached manager")
	}
	if _, err := readManagerDBusRoles(context.Background(), []managerDBusRole{{name: "foreign"}, {name: "foreign"}}); err != ErrBoundary {
		t.Fatal("foreign role reached manager")
	}
}

func TestNumericPublisherManagerDBusDecodedCompositeConversion(t *testing.T) {
	for _, test := range []struct {
		key   string
		value any
	}{
		{"Listen", []managerDBusListen{{"SequentialPacket", numericPublisherSocketPath}}},
		{"ExecStart", []managerDBusExec{{Path: unitObserverExecutable, Arguments: []string{unitObserverExecutable, "--publish-openfile"}}}},
	} {
		frame := managerDBusReplyFixture(t, test.value)
		if _, err := validateManagerDBusFrame(frame); err != nil {
			t.Fatal(err)
		}
		message, err := dbus.DecodeMessage(bytes.NewReader(frame))
		if err != nil {
			t.Fatal(err)
		}
		variant, ok := message.Body[0].(dbus.Variant)
		if !ok {
			t.Fatal("decoded body is not variant")
		}
		got, err := managerDBusPropertyValue(test.key, variant)
		want, wantErr := managerDBusPropertyValue(test.key, dbus.MakeVariant(test.value))
		if err != nil || wantErr != nil || got != want {
			t.Fatal("decoded native composite changed predicates", test.key, err)
		}
	}
}
