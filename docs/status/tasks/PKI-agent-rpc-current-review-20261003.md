# PKI unavailable RPC current-tree review

Immutable `519425efc220741bfbd7c02e636a92f12c1badbd`, two Go files vs f469d56.
Verdict: APPROVE WITH LIMITS for truthful unavailable-state RPC.

The RPC checks the existing owner policy before clock access and returns configured
owner plus observed timestamp and fixed unavailable reason. Empty owner remains
accepted by the shared policy; this change does not create a new exception.
Root/files remain empty and there is no filesystem, secret provider, scheduler or
VPP access. It does not imply PKI materialization is implemented.

Tests use a deliberately minimal Service, reject foreign owner before clock access,
check exact timestamp/empty payload and exercise registered generated gRPC via
bufconn for configured/empty/foreign owners. This proves the handler overrides the
inherited unimplemented RPC rather than testing only its direct method.

Read-only source review; no new tests launched. Root owns current-tree validation.
PKI secret transport/materializer remains a separate P11-owned dependency.

Next independent ISO coverage candidate: exercise numeric overflow of successful
database count responses in console cleanup. The code accepts arbitrary digit
strings then applies Bash arithmetic to `n`; values exceeding signed machine range
can wrap to zero/negative and pass the positive-count retention check. The new
uncertainty suite covers syntax/exit failure, not numeric range. A bounded successor
can require decimal zero explicitly before removal, preserving every positive count
without arithmetic, and add large-count retention/recovery regressions. This is a
proposal only; no product changes or runtime reproducer executed in this review.
