package memif_test

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"go.fd.io/govpp/api"
	"google.golang.org/protobuf/proto"

	"ngfw/agent/binapi/interface_types"
	memifapi "ngfw/agent/binapi/memif"
	"ngfw/agent/internal/descriptors/interface/ifacetest"
	"ngfw/agent/internal/descriptors/memif"
	"ngfw/agent/internal/scheduler"
)

const owner = "w2"

var ctx = context.Background()

type fakeMemif struct {
	*ifacetest.VPP
	sockets map[uint32]string
	memifs  map[uint32]*memifapi.MemifDetails
}

func newFake() *fakeMemif {
	f := &fakeMemif{VPP: ifacetest.New(), sockets: map[uint32]string{0: "/run/vpp/memif.sock", 3001: "/run/vrx-test/w3/memif/w3-a.sock"}, memifs: map[uint32]*memifapi.MemifDetails{}}
	f.On("memif_socket_filename_add_del_v2", func(req api.Message) ([]api.Message, error) {
		r := req.(*memifapi.MemifSocketFilenameAddDelV2)
		if r.IsAdd {
			if _, dup := f.sockets[r.SocketID]; dup {
				return []api.Message{&memifapi.MemifSocketFilenameAddDelV2Reply{Retval: -100}}, nil
			}
			f.sockets[r.SocketID] = r.SocketFilename
		} else {
			delete(f.sockets, r.SocketID)
		}
		return []api.Message{&memifapi.MemifSocketFilenameAddDelV2Reply{SocketID: r.SocketID}}, nil
	})
	f.On("memif_socket_filename_dump", func(api.Message) ([]api.Message, error) {
		var out []api.Message
		for id, fn := range f.sockets {
			out = append(out, &memifapi.MemifSocketFilenameDetails{SocketID: id, SocketFilename: fn})
		}
		return out, nil
	})
	f.On("memif_create_v2", func(req api.Message) ([]api.Message, error) {
		r := req.(*memifapi.MemifCreateV2)
		if _, ok := f.sockets[r.SocketID]; !ok {
			return []api.Message{&memifapi.MemifCreateV2Reply{Retval: -1}}, nil
		}
		idx := f.Add(fmt.Sprintf("memif%d/%d", r.SocketID, r.ID), "memif", "")
		f.memifs[idx] = &memifapi.MemifDetails{SwIfIndex: interface_types.InterfaceIndex(idx), ID: r.ID, Role: r.Role, Mode: r.Mode, ZeroCopy: !r.NoZeroCopy, SocketID: r.SocketID, RingSize: r.RingSize, BufferSize: r.BufferSize}
		return []api.Message{&memifapi.MemifCreateV2Reply{SwIfIndex: interface_types.InterfaceIndex(idx)}}, nil
	})
	f.On("memif_delete", func(req api.Message) ([]api.Message, error) {
		r := req.(*memifapi.MemifDelete)
		delete(f.memifs, uint32(r.SwIfIndex))
		f.Remove(uint32(r.SwIfIndex))
		return []api.Message{&memifapi.MemifDeleteReply{}}, nil
	})
	f.On("memif_dump", func(api.Message) ([]api.Message, error) {
		var out []api.Message
		for _, m := range f.memifs {
			out = append(out, m)
		}
		return out, nil
	})
	return f
}

func TestSocketAndMemif(t *testing.T) {
	f := newFake()
	dir := t.TempDir()
	r := scheduler.NewRegistry()
	memif.Register(r, f, owner, memif.WithSocketDir(dir))
	if r.Len() != 2 {
		t.Fatal(r.Names())
	}
	sd := memif.NewSocket(f, owner, dir)
	sock := &memif.Socket{Id: 2001, Filename: filepath.Join(dir, "w2-memif1.sock")}
	if sd.KeyOf(sock) != "memif.socket/2001" {
		t.Fatal(sd.KeyOf(sock))
	}
	if kvs, _ := sd.Retrieve(ctx); len(kvs) != 0 {
		t.Fatalf("default socket and the other owner's socket must be invisible: %+v", kvs)
	}
	for _, bad := range []*memif.Socket{{Id: 0, Filename: filepath.Join(dir, "x")}, {Id: 5, Filename: "/run/vrx-test/w3/memif/x.sock"}, {Id: 5, Filename: filepath.Join(dir, "sub", "x.sock")}} {
		if _, err := sd.Create(ctx, bad); err == nil {
			t.Fatalf("accepted %v", bad)
		}
	}
	smeta, err := sd.Create(ctx, sock)
	if err != nil {
		t.Fatal(err)
	}
	kvs, _ := sd.Retrieve(ctx)
	if len(kvs) != 1 || kvs[0].Key != "memif.socket/2001" || !proto.Equal(kvs[0].Value, sock) || kvs[0].Meta != smeta {
		t.Fatalf("socket Retrieve = %+v", kvs)
	}
	if _, err := sd.Update(ctx, sock, sock, smeta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatal(err)
	}

	md := memif.NewMemif(f, owner)
	desired := &memif.Memif{Name: "w2-memif1", Id: 1, Socket: 2001, Role: memif.Role_ROLE_MASTER, Mode: memif.Mode_MODE_ETHERNET, RingSize: 1024, BufferSize: 2048}
	if md.KeyOf(desired) != "memif.memif/w2-memif1" {
		t.Fatal(md.KeyOf(desired))
	}
	if deps := md.Dependencies(desired); len(deps) != 1 || deps[0].Key != "memif.socket/2001" {
		t.Fatalf("deps = %+v", deps)
	}
	if deps := md.Dependencies(&memif.Memif{Name: "x"}); deps != nil {
		t.Fatalf("socket 0 has no dependency: %+v", deps)
	}
	if _, err := md.Create(ctx, &memif.Memif{Name: "x", Socket: 2001, RingSize: 1000, BufferSize: 2048}); err == nil {
		t.Fatal("non power-of-two ring accepted")
	}
	meta, err := md.Create(ctx, desired)
	if err != nil {
		t.Fatal(err)
	}
	req := f.CallsNamed("memif_create_v2")[0].(*memifapi.MemifCreateV2)
	if req.Role != memifapi.MEMIF_ROLE_API_MASTER || req.SocketID != 2001 || req.RingSize != 1024 || req.BufferSize != 2048 || !req.NoZeroCopy || req.Secret != "" {
		t.Fatalf("memif_create_v2 = %+v", req)
	}
	// the other owner's memif on the default socket is invisible
	oidx := f.Add("memif0/7", "memif", "w3:w3-m")
	f.memifs[oidx] = &memifapi.MemifDetails{SwIfIndex: interface_types.InterfaceIndex(oidx), ID: 7}
	kvs, _ = md.Retrieve(ctx)
	if len(kvs) != 1 || kvs[0].Key != "memif.memif/w2-memif1" || !proto.Equal(kvs[0].Value, desired) || kvs[0].Meta != meta {
		t.Fatalf("memif Retrieve = %+v", kvs)
	}
	if _, err := md.Update(ctx, desired, &memif.Memif{Name: "w2-memif1", Id: 2}, meta); !errors.Is(err, scheduler.ErrRecreate) {
		t.Fatal(err)
	}
	if err := md.Delete(ctx, desired, meta); err != nil {
		t.Fatal(err)
	}
	if err := sd.Delete(ctx, sock, smeta); err != nil {
		t.Fatal(err)
	}
	if kvs, _ = md.Retrieve(ctx); len(kvs) != 0 {
		t.Fatalf("after Delete: %+v", kvs)
	}
	if kvs, _ = sd.Retrieve(ctx); len(kvs) != 0 {
		t.Fatalf("after socket Delete: %+v", kvs)
	}
	if _, ok := f.sockets[3001]; !ok {
		t.Fatal("the other owner's socket was touched")
	}
}
