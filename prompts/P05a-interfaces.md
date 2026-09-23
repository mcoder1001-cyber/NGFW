# Task P05a — Agent interfaces: scheduler Descriptor, fake VPP client, renderer interface   (prepend 00-CONTEXT.md)

## Goal
Publish, in a few hours, the **interfaces every factory and P05 build against** — so that DF-1…DF-8 and RF-1…RF-4 can
start in parallel while P05 implements the real scheduler. Keep it small and stable; after merge these files are frozen
(changes = `contract/<id>` branch).

## Build exactly this
1. `apps/agent/internal/scheduler/descriptor.go`:
   ```go
   type Key string
   type KV struct { Key Key; Value proto.Message; Meta any }
   type Dependency struct { Key Key; Optional bool }
   var ErrRecreate = errors.New("update requires recreate")
   type Descriptor interface {
     Name() string
     KeyOf(obj proto.Message) Key
     Dependencies(obj proto.Message) []Dependency
     Create(ctx context.Context, obj proto.Message) (meta any, err error)
     Update(ctx context.Context, old, new proto.Message, meta any) (any, error)   // may return ErrRecreate
     Delete(ctx context.Context, obj proto.Message, meta any) error
     Retrieve(ctx context.Context) ([]KV, error)                                    // ACTUAL state from VPP/daemon
   }
   type Registry interface { Register(d Descriptor) }
   type Plan struct{ Create, Update, Delete []KV }; type Result struct{ Key Key; Op string; Err error }
   ```
   plus a `Registry` implementation (map, duplicate-name error) and doc comments explaining ordering/rollback semantics
   (see `docs/01-architecture.md` AD-3) — the real scheduler lands in P05.
2. `apps/agent/internal/vpp/client.go`: the `Client` interface P05 will implement (`Invoke(ctx, req, reply)`, `Stream`, `Connected()`),
   and `internal/vpp/fake`: an in-memory fake recording calls and serving canned replies, for descriptor unit tests.
3. `apps/agent/internal/renderers/renderer.go`: `type Renderer interface { Name() string; Render(ctx, desired proto.Message) (Files, error);
   Validate(ctx, Files) error; Apply(ctx, Files) error; Retrieve(ctx) (proto.Message, error) }` + `Files map[string]File{Mode, Owner, Content}` +
   helpers: atomic write (temp+rename), fixed-argv runner with an allowlist (`ALLOWLIST.md` skeleton), strict template escaping helper.
4. `apps/agent/internal/descriptors/README.md` and `internal/renderers/README.md`: how to write one, naming (`VRX_TEST_PREFIX`), Retrieve rules,
   test isolation on the shared host.
5. `make lint test` green; one example descriptor unit test using the fake.

## Acceptance
- [ ] Interfaces compile; `go vet` clean; example test green
- [ ] READMEs answer: where do I put files, how do I name objects, how do I test without touching other workers' objects

## Out of scope
The real scheduler, govpp connection, gRPC server, any real descriptor or renderer (P05, DF-*, RF-*).
