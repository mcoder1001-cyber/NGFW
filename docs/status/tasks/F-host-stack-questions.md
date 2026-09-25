# F-host-stack — questions (written, not waited on)

1. **Should host-stack configuration ship in the 21-day scope at all (T3)?** List it as *partial* in STATUS-FINAL:
   - nothing is host-verified;
   - namespaces, the TCP pool and http_static are write-only.
2. **http_static cannot be disabled via the API.** It is kept opt-in (`VRX_HOSTSTACK_HTTP_STATIC=1`, globals owner only,
   and DryRun refuses it otherwise). Options: (a) keep it opt-in (current); (b) drop the descriptor and the schema leaf.
3. **Which session rt engine does the product's globals owner select?** rule-table serves session rules; sdl serves
   F-rpf-adl-pbr's Auto-SDL. Current behaviour:
   - The owner enables rule-table only when the layer is off.
   - It never re-engines the layer.
   - A document with both features gets a semantic error.
   The manager or F-rpf-adl-pbr must pick the default.
4. **Start-up generator proposals** (F-startup-gen owns them; nothing was added here):
   - `session { enable, use-app-socket-api, evt_qs_memfd_seg, event-queue-length }`
   - `tcp { cc-algo cubic|newreno, max-rx-fifo, max-tx-fifo }`
   - `udp` buffer sizes as optional `dataplane.hostStack` leaves.
   The generator renders no `session`/`tcp` stanza today.
5. **Adding the `services` domain has side effects outside my files.**
   - Two assertions in `apps/agent/internal/agent/service_test.go` list the implemented domains and needed `+services`.
     This hunk is outside the anchors.
   - Retrieve of `services` now returns only `hostStack.sessionRules`. Other services leaves (dhcp, dns, … prefaulted)
     and the write-only host-stack leaves are absent, so the API's running-vs-actual diff will report them as drift until
     each services feature assembles its own leaves, or until the service overlays stored write-only leaves (D-073b-like).
     This needs a manager decision.
6. **Example fixture.** `packages/schema/examples/host-stack-*.json` (my glob) is rejected by
   `packages/schema/src/examples.test.ts` (the SIBLING regex does not know `host-stack-`), and that file is not mine. The
   fixture therefore lives only in `packages/proto/test/fixtures/host-stack-basic.json`, which the drift test and my
   schema tests use. Proposal: add `host-stack` to SIBLING.
7. **Missing anchors.** There is no `wave-BC: F-host-stack` anchor in:
   - the subsystems register block;
   - projection.go project() and assemble();
   - services.ts keys;
   - schema index.ts and semantic/index.ts;
   - proto.md;
   - nav BUILT_DOMAINS and nav.test.
   The hunks are appended after the last anchor and marked "unanchored".
8. **CI environment** (not a product issue). Every worker in this container will hit these:
   - Turbo fails with ENOEXEC on `/root/.local/share/pnpm/.tools/pnpm/12.5.1/bin/pnpm`, a shebang-less launcher.
   - Host go is 1.24.7 while go.mod needs 1.26.
   - The installed golangci-lint is 2.5.0; 2.13.2 is pinned.
   - golangci-lint 2.13.2 flags a pre-existing finding: `internal/agent/service.go:549 revertRetryMin`.
   - `--base main` flags a gitleaks hit in base commit c2a8ab7.
9. **Namespace secret.** VPP's `secret` is a u64. Once the secret channel lands, the agent needs a mapping from the
   `key/<name>` material to the u64 (for example, the first 8 bytes of an HMAC). This is not decided here.
