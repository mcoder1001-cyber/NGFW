package ravpn

import (
	"context"
	"strings"
	"testing"
	"time"
)

func managerPairFixture() (string, string) {
	return "Listen=" + numericPublisherSocketPath + " (SequentialPacket)\nFragmentPath=" + numericPublisherSocket + "\nDropInPaths=\nActiveState=active\nSubState=listening\nControlPID=0\nUser=root\nId=ngfw-ra-openfile.socket", "ExecStart={ path=" + unitObserverExecutable + " ; argv[]=" + unitObserverExecutable + " --publish-openfile ; }\nMainPID=0\nControlPID=0\nActiveState=inactive\nSubState=dead\nControlGroup=\nFragmentPath=" + numericPublisherService + "\nDropInPaths=\nUser=root\nGroup=ngfw\nCapabilityBoundingSet=\nNoNewPrivileges=yes\nId=ngfw-ra-openfile.service"
}
func TestNumericPublisherManagerPairBindsReorderedRoles(t *testing.T) {
	socket, service := managerPairFixture()
	for _, data := range []string{socket + "\n\n" + service + "\n", service + "\n\n" + socket} {
		s, e := parseNumericPublisherManagerPair([]byte(data))
		if e != nil || !numericPublisherSocketState(s.socket, false) || !numericPublisherCgroup(s.service, false) {
			t.Fatalf("valid role snapshot rejected: %v", e)
		}
		if _, ok := s.socket["ExecStart"]; ok {
			t.Fatal("service contaminated socket")
		}
		if _, ok := s.service["Listen"]; ok {
			t.Fatal("socket contaminated service")
		}
		if v, ok := s.service["CapabilityBoundingSet"]; !ok || v != "" {
			t.Fatal("empty required value lost")
		}
	}
}
func TestNumericPublisherManagerPairRejectsAmbiguity(t *testing.T) {
	socket, service := managerPairFixture()
	valid := socket + "\n\n" + service + "\n"
	cases := map[string]string{
		"missing-role": socket, "duplicate-role": socket + "\n\n" + socket, "foreign-id": strings.Replace(valid, "Id=ngfw-ra-openfile.service", "Id=foreign.service", 1), "missing-id": strings.Replace(valid, "\nId=ngfw-ra-openfile.service", "", 1), "duplicate-key": strings.Replace(valid, "Listen=", "Listen=other\nListen=", 1), "duplicate-id": valid + "Id=ngfw-ra-openfile.service\n", "unknown-key": valid + "Foreign=yes\n", "non-property": valid + "unexpected text\n", "missing-required-empty": strings.Replace(valid, "CapabilityBoundingSet=\n", "", 1), "merged": strings.Replace(valid, "\n\n", "\n", 1), "extra-block": valid + "\nId=foreign.service\n", "extra-separator": strings.Replace(valid, "\n\n", "\n\n\n", 1), "nul": valid + "\x00", "crlf": strings.ReplaceAll(valid, "\n", "\r\n"), "invalid-utf8": valid + string([]byte{255}), "oversized": valid + strings.Repeat("x", 16384), "cross-role-missing": strings.Replace(socket, "Listen="+numericPublisherSocketPath+" (SequentialPacket)\n", "", 1) + "\n\n" + service + "\nListen=" + numericPublisherSocketPath + " (SequentialPacket)\n"}
	for n, d := range cases {
		t.Run(n, func(t *testing.T) {
			s, e := parseNumericPublisherManagerPair([]byte(d))
			if e != ErrBoundary || s.socket != nil || s.service != nil {
				t.Fatal("ambiguous input accepted or partial maps leaked")
			}
		})
	}
}
func TestNumericPublisherManagerPairPreservesForeignPredicateRefusal(t *testing.T) {
	socket, service := managerPairFixture()
	s, e := parseNumericPublisherManagerPair([]byte(strings.Replace(socket, "FragmentPath="+numericPublisherSocket, "FragmentPath=/foreign.socket", 1) + "\n\n" + service))
	if e != nil {
		t.Fatal(e)
	}
	if numericPublisherSocketState(s.socket, false) {
		t.Fatal("foreign fragment accepted")
	}
	s, e = parseNumericPublisherManagerPair([]byte(socket + "\n\n" + strings.Replace(service, "MainPID=0", "MainPID=123", 1)))
	if e != nil {
		t.Fatal(e)
	}
	if numericPublisherCgroup(s.service, false) {
		t.Fatal("live process adopted as inactive")
	}
}
func TestNumericPublisherManagerPairFixedNativeQueryAndCancelledCaller(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if managerDBusQueryBudget != 2*time.Second || managerDBusSocket != "/run/systemd/private" || len(managerDBusPublisherRoles) != 2 || managerDBusPublisherRoles[0].name != "ngfw-ra-openfile.socket" || managerDBusPublisherRoles[1].name != "ngfw-ra-openfile.service" {
		t.Fatal("fixed native manager roles or original query budget changed")
	}
	start := time.Now()
	s, e := numericPublisherManagerPair(ctx)
	if e != ErrBoundary || s.socket != nil || s.service != nil || time.Since(start) > time.Second {
		t.Fatal("caller cancellation ignored")
	}
}
