# P11-host: independent R1 review

Frozen local source f59c3ea7. Production native IPsec path was already integrated; this task adds guarded disposable acceptance driver and explicit owned rollback assertions. Reviewer writes evidence only.

Independent actual commands:
```text
python3 -m unittest discover -s test/topology/ipsec -p test_host_acceptance.py -v
Ran 2 tests in 0.001s
OK
shellcheck test/topology/ipsec/run.sh
(exit 0, no findings)
```

Source review: reserves fixed existing fixture slot8 with collision refusal and exclusive task lock/shared lab lock; parses lab exports as data; builds fresh product agent; executes both initiator/responder on disposable VPP; refuses SKIP or missing lifecycle markers; retains private logs and safe capture hashes; verifies shared VPP PID/restart identity and fixture cleanup. Rollback explicitly supplies authoritative subsystem list before Retrieve and direct IKE/protection/interface inspection. Assertions fail before disposable teardown, so teardown cannot masquerade as rollback. Driver relies on existing peer campaign for traffic/rekey/restart/loss checks. Reviewer did not rerun the packet campaign; T3 owns independent real data-plane execution.

Verdict: APPROVE. No source correctness finding identified. Independent unchanged complete quick PASS9m55s on exactf59c3ea7b9cf0d6ad52a0f1c4db82bb0f6bddd1b; generation/tracked product clean. T1 report details built-in cache reuse and benign Python artifact warning. Real packet evidence remains separate T3 responsibility.
