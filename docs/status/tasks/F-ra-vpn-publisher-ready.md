# Publisher validation-ready checkpoint

Branch `codex/ready-f-ra-vpn-20261005`, owned worktree `/root/ngfw-wt/ready-f-ra-vpn-20261005`. Contract first remote `7930083b8591a7c0a678e35b1232cc62bbf4d775`, local `c00ade4ca`, exact tree `765c4bbc996b586a0e656bfb23b9ab8b82d693d9`.

Boot21 real original-unit guest evidence establishes server installation entry at 368ms, listener entry at 7762ms, peer at 7938ms, after the client's five-second receive deadline. The manager later enforced service RuntimeMaxSec=10. This is a measured phase-order failure; the peer-stage internal failure remains unknown. No successful supplier or full RA campaign is claimed.

The closed validation-ready frame carries complete current source/server boot identities and exactly one manager-opened source executable FD. The client independently checks current fixed server manager/process/installed executable identity and held source image before sending a request. Both probe and publish use distinct freshly validated one-shot servers. Final response must match the authenticated readiness server and image. No publication mutation occurs before this validation.

Each validation wait is bounded by the existing public validation20/public40 context and caller deadline. Once ready, client and server restore the unchanged socket/request-reply IPC5 boundary. Server installation proofs remain per-process, held with canonical stamp rechecks; there is no across-call or restart cache. Strict UID/GID, capabilities, source reference, role, descriptor and image proofs remain. Service RuntimeMaxSec=10 is unchanged and may still prevent the allowed validation phase completing; it remains an explicit unresolved installation/runtime budget limitation requiring real replay and a reviewed finite phase-consistent bound if necessary.

Actual selected unchanged `go test -race -count=1 ./internal/ra_vpn -run '^TestNumericPublisher'` PASS 1.284s. New tests reject stale/foreign/incomplete readiness, prove validation socket20 resets to IPC5, and preserve canceled/short caller deadlines. These are source tests, not live protocol or guest acceptance.

Owned changed paths: namespace_openfile_client.go, namespace_openfile_server.go, namespace_openfile_ready.go, namespace_openfile_ready_test.go, and this receipt. Next command: run the full RA package race and lint, publish the exact consumer checkpoint, obtain independent protocol/security review, then build a distinct coherent guest artifact. Existing VM failures and immutable ELFs remain intact.
