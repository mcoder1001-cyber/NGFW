# P05 — questions / notes for the manager

1. **ErrRetrieveUnsupported (D-063)** — defined in the reconciler side as `scheduler.ErrRetrieveUnsupported`
   (`internal/scheduler/reconciler.go`, not in the frozen `descriptor.go`) with the same message as
   `df2.ErrRetrieveUnsupported` ("vpp has no dump for this object type"). `scheduler.IsRetrieveUnsupported(err)` accepts
   either sentinel (errors.Is or the message) so DF-2 works unchanged; after both merge DF-2 should alias:
   `ErrRetrieveUnsupported = scheduler.ErrRetrieveUnsupported`. Write-only rules implemented as in D-063; counter exposed
   as metric `vrx_agent_retrieve_unsupported_objects` (+ `..._descriptors`).
2. **Normalizer** — optional interface `scheduler.Normalizer{ Normalize(proto.Message) proto.Message }` (DF-2's
   proposed shape). The scheduler normalises every desired value before diffing. DF-2's exported `Normalize*`
   functions need a one-line method per descriptor to be picked up.
3. **Interface key convention conflict between factories.** DF-1 references interfaces by FULL keys
   (`interface.loopback/loop201`, `tapv2.tap/w2-tap0`); DF-2…DF-6 depend on the generic `interface/<name>`. P05 adds an
   optional `scheduler.KeyProvider{ ProvidedKeys(obj) []Key }` (aliases that satisfy dependencies) and the core loopback
   provides `interface/<name>`; P05 core's own references (addresses, table binding, route egress) use `interface/<name>`.
   For DF-2…6 dependencies on non-loopback interfaces to resolve, DF-1's interface-creating descriptors (tap,
   host-interface, bond, memif, subinterface) should implement `ProvidedKeys` → `interface/<name>` (one method each).
   VRFs: `vrf/<id>` as all DF prompts assume.
4. **DF-2 classify store path** — proposal accepted for wiring time: `$VRX_AGENT_STATE_DIR/classify-<owner>.json`; P05
   provides `internal/ownertable` (`owned-<owner>.json`) for untaggable objects (routes use it).
