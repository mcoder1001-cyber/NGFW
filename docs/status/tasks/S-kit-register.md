# S-kit-register — TD-16 item 5: descriptor registration on kit.Register

## What
- Families whose `Register(r, c, owner)` takes only client+owner now build through
  `kit.Register(r, kit.Env{Client, Owner}, ctors...)`: interface, l2, ipip, bond, gre, l3xc, tapv2.
  Public signatures, registration order and descriptors are unchanged (no caller edits).
- Every other `Register(r, client, owner, ...)` gained a one-line comment: it takes per-family
  options or extra dependencies (IDRange, Store, Secrets, BootStore) beyond `kit.Env`, or returns a
  plugin handle, so it stays as is. core keeps its own richer `core.Env`.
- Untouched per scope: ipfix, flowprobe, sflow, desired/ipfix_sflow.go.

## Verification (apps/agent)
- gofmt -l internal: no output
- go vet ./...: clean
- go test -race ./...: all ok, no failures
- golangci-lint run ./...: `0 issues.`

## Out of scope / open questions
- Extending `kit.Env`/`Ctor` to carry options would let the df2/df6/df7/natcommon families
  migrate; that is a signature change for callers and was not done here.
- No real-VPP runs (cloud container); fake client only.
