# F-bruteforce-detectors — R4 dataplane/shared-host review

Reviewed exact product SHA `5c88cb5f2b09f59a161dcfa6d72cf89b831c207a` against `06e4368c`. Independent reviewer; no product edits.

No BLOCKER, MAJOR or MINOR source findings.

The change adds destination-port evidence to existing trusted kernel observations and moves distinct-port counting to the API. Existing product-only journal gating, configured detector/allowlist gating, kernel transport provenance, bounded runner output and cursor deduplication remain. Native VPN ownership-safe state reading is unchanged; its literal adapts to the expanded observation struct. No VPP/kernel object creation, ownership mutation, shared-table/global operations, system units, port allocations, binding cleanup or generated API changes are introduced. Existing runtime enforcement remains the owner of VPP/nftables programming. The topology extension is shell-free, requires pinned SSH host keys/disposable identity, rejects transport failures as authentication evidence and uses the explicitly supplied test addresses.

Exact-source independent commands:

```text
# cwd: detectors/apps/agent
/workspace/scratch/e4f791ef53f7/go/bin/go test -race -count=1 ./internal/detectors
ok ngfw/agent/internal/detectors 1.032s
/workspace/scratch/e4f791ef53f7/go/bin/go test -race -count=1 ./internal/agent -run TestAutoBlockObservationPreservesPortThroughStream
ok ngfw/agent/internal/agent 1.078s
# cwd: detectors/test/topology/autoblock
python3 -m unittest test_driver.py
Ran 4 tests in 0.007s
OK
```

No real journal/nftables/VPP packets, SSH authentication or live restart were run here. Actual local-in/forwarding/allowlist/manual-removal/expiry acceptance remains lab-deferred under the owner policy; this report makes no full CI or hardware acceptance claim.

Verdict: **APPROVE** (source review; lab acceptance deferred).
