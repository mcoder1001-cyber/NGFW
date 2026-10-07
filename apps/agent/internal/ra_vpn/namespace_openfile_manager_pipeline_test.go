package ravpn

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
)

// This owned protocol primitive deliberately has no PID1 trust attestation.
// It exercises the same no-FD transport and pinned SDK on a private socketpair.
func managerDBusPipelinePeer(peer io.ReadWriter, roles []managerDBusRole, mode string, cancel context.CancelFunc) error {
	reader := bufio.NewReader(peer)
	for _, step := range []struct{ want, reply string }{{"\x00AUTH\r\n", "REJECTED EXTERNAL ANONYMOUS\r\n"}, {"AUTH EXTERNAL 30\r\n", "OK 0123456789abcdef0123456789abcdef\r\n"}, {"BEGIN\r\n", ""}} {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			return err
		}
		if string(line) != step.want {
			return ErrBoundary
		}
		if step.reply != "" {
			if _, err := io.WriteString(peer, step.reply); err != nil {
				return err
			}
		}
	}
	serial := uint32(1)
	receive := func(role managerDBusRole, key string) (*dbus.Message, error) {
		message, err := dbus.DecodeMessage(reader)
		if err != nil {
			return nil, err
		}
		if message.Type != dbus.TypeMethodCall || message.Headers[dbus.FieldPath].Value() != dbus.ObjectPath(role.path) || message.Headers[dbus.FieldInterface].Value() != "org.freedesktop.DBus.Properties" || message.Headers[dbus.FieldMember].Value() != "Get" || len(message.Body) != 2 || message.Body[0] != managerDBusPropertyInterface(role, key) || message.Body[1] != key {
			return nil, ErrBoundary
		}
		return message, nil
	}
	reply := func(request *dbus.Message, value dbus.Variant, foreign bool) error {
		replySerial := request.Serial()
		if foreign {
			replySerial = 0x7fffffff
		}
		message := dbus.Message{Type: dbus.TypeMethodReply, Headers: map[dbus.HeaderField]dbus.Variant{dbus.FieldReplySerial: dbus.MakeVariant(replySerial), dbus.FieldSignature: dbus.MakeVariant(dbus.SignatureOf(value))}, Body: []any{value}}
		var b bytes.Buffer
		if err := message.EncodeTo(&b, binary.LittleEndian); err != nil {
			return err
		}
		frame := b.Bytes()
		binary.LittleEndian.PutUint32(frame[8:12], serial)
		serial++
		_, err := peer.Write(frame)
		return err
	}
	for _, role := range roles {
		keys := strings.Split(role.fields, ",")
		first, err := receive(role, "Id")
		if err != nil {
			return err
		}
		id := role.name
		if mode == "pre-id" {
			id = "foreign.service"
		}
		if err := reply(first, dbus.MakeVariant(id), false); err != nil {
			return err
		}
		if mode == "pre-id" {
			var b [1]byte
			_, err := reader.Read(b[:])
			return err
		}
		pending := make([]*dbus.Message, 0, len(keys)-1)
		for _, key := range keys[1:] {
			request, err := receive(role, key)
			if err != nil {
				return err
			}
			pending = append(pending, request)
		}
		if mode == "cancel-wait" {
			cancel()
			var b [1]byte
			_, err := reader.Read(b[:])
			return err
		}
		// Reply in reverse order, retaining later successes before the first result.
		for i := len(pending) - 1; i >= 0; i-- {
			value, err := managerDBusQueryFixture(role, keys[i+1])
			if err != nil {
				return err
			}
			if mode == "first-type" && i == 0 {
				value = dbus.MakeVariant(uint32(1))
			}
			if err := reply(pending[i], value, mode == "foreign-serial"); err != nil {
				return err
			}
			if mode == "duplicate-serial" {
				if err := reply(pending[i], value, false); err != nil {
					return err
				}
			}
		}
		if mode == "first-type" || mode == "foreign-serial" || mode == "duplicate-serial" {
			var b [1]byte
			_, err := reader.Read(b[:])
			return err
		}
		last, err := receive(role, "Id")
		if err != nil {
			return err
		}
		id = role.name
		if mode == "post-id" {
			id = "foreign.service"
		}
		if err := reply(last, dbus.MakeVariant(id), false); err != nil {
			return err
		}
		if mode == "post-id" {
			var b [1]byte
			_, err := reader.Read(b[:])
			return err
		}
	}
	var b [1]byte
	_, err := reader.Read(b[:])
	return err
}

func TestNumericPublisherPipelineReorderedRepliesAndFailureRetirement(t *testing.T) {
	for _, mode := range []string{"valid", "pre-id", "post-id", "first-type", "foreign-serial", "duplicate-serial", "cancel-enqueue", "cancel-wait"} {
		t.Run(mode, func(t *testing.T) {
			baselineWorkers := managerDBusPipelineSDKWorkers()
			client, peer := managerDBusOwnedPair(t)
			transport := &managerDBusTransport{conn: client, pending: make(map[uint32]bool)}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			callbackDone := make(chan struct{})
			stop := context.AfterFunc(ctx, func() { defer close(callbackDone); _ = transport.Close() })
			defer func() {
				if !stop() {
					<-callbackDone
				}
			}()
			roles := append(append([]managerDBusRole(nil), managerDBusPublisherRoles...), managerDBusSourceRole)
			peerDone := make(chan error, 1)
			go func() { peerDone <- managerDBusPipelinePeer(peer, roles, mode, cancel) }()
			bus, err := dbus.NewConn(transport, dbus.WithContext(ctx))
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := bus.Close(); err != nil {
					t.Error(err)
				}
			}()
			if err := bus.Auth([]dbus.Auth{dbus.AuthExternal("0")}); err != nil {
				t.Fatal(err)
			}
			var calls []*dbus.Call
			var requests []string
			result, err := readManagerDBusPipelineProperties(ctx, roles, func(callContext context.Context, role managerDBusRole, key string) *dbus.Call {
				call := bus.Object("org.freedesktop.systemd1", dbus.ObjectPath(role.path)).GoWithContext(callContext, "org.freedesktop.DBus.Properties.Get", dbus.FlagNoAutoStart, make(chan *dbus.Call, 1), managerDBusPropertyInterface(role, key), key)
				calls = append(calls, call)
				requests = append(requests, role.name+":"+key)
				if mode == "cancel-enqueue" && len(calls) == 3 {
					cancel()
				}
				return call
			}, bus.Close)
			if mode == "valid" {
				if err != nil || len(result) != 3 || len(calls) != 30 {
					t.Fatalf("fixed roles failed: %v/%d/%d", err, len(result), len(calls))
				}
				var want []string
				for _, role := range roles {
					for _, key := range strings.Split(role.fields, ",") {
						want = append(want, role.name+":"+key)
					}
					want = append(want, role.name+":Id")
				}
				if fmt.Sprint(requests) != fmt.Sprint(want) {
					t.Fatal("dispatch count/order changed")
				}
			} else {
				if err != ErrBoundary || result != nil {
					t.Fatal("failure accepted")
				}
				dispatchLimit := len(strings.Split(roles[0].fields, ","))
				if mode == "pre-id" {
					dispatchLimit = 1
				}
				if mode == "post-id" {
					dispatchLimit++
				}
				if len(calls) > dispatchLimit {
					t.Fatalf("postId/nextRole dispatched after failure: %v", requests)
				}
			}
			for _, call := range calls {
				if len(call.Done) != 0 {
					t.Fatal("owned queued Done completion abandoned")
				}
				select {
				case <-call.Context().Done():
				default:
					t.Fatal("owned call context not retired")
				}
			}
			if err := bus.Close(); err != nil {
				t.Fatal(err)
			}
			select {
			case peerErr := <-peerDone:
				if peerErr != nil && !errors.Is(peerErr, io.EOF) && mode == "valid" {
					t.Fatal(peerErr)
				}
			case <-time.After(time.Second):
				t.Fatal("owned peer/call/reader did not retire")
			}
			if err := transport.Close(); err != nil {
				t.Fatal(err)
			}
			until := time.Now().Add(time.Second)
			for managerDBusPipelineSDKWorkers() > baselineWorkers {
				if time.Now().After(until) {
					t.Fatal("pinned SDK call/reader workers did not retire within owned fixture bound")
				}
				time.Sleep(time.Millisecond)
			}
		})
	}
}

// Pinned SDK exposes completion channels, not a private worker join API.
// Observe bounded retirement without emitting stacks or pretending to join it.
func managerDBusPipelineSDKWorkers() int {
	buffer := make([]byte, 1<<20)
	n := runtime.Stack(buffer, true)
	return bytes.Count(buffer[:n], []byte("github.com/godbus/dbus/v5.(*Conn).send.func1")) + bytes.Count(buffer[:n], []byte("github.com/godbus/dbus/v5.(*Conn).inWorker"))
}

func TestNumericPublisherPipelineClosedPreconditions(t *testing.T) {
	role, err := fixedManagerDBusSingleRole(managerDBusSingleSource)
	if err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	for _, test := range []struct {
		ctx   context.Context
		roles []managerDBusRole
	}{{canceled, []managerDBusRole{role}}, {context.Background(), []managerDBusRole{{name: "foreign"}}}} {
		invoked := false
		_, err := readManagerDBusPipelineProperties(test.ctx, test.roles, func(context.Context, managerDBusRole, string) *dbus.Call { invoked = true; return nil }, func() error { invoked = true; return nil })
		if err != ErrBoundary || invoked {
			t.Fatal("closed precondition invoked SDK/connection")
		}
	}
}
