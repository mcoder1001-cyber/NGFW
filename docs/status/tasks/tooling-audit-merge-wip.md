# CLI/SDK audit integration

Root owns isolated .scratch/tooling-integration, branch codex/tooling-audit-merge-20261003. Frozen baseactualPR135mergea0922c7e9; PR102 board/status-only integration may advance base before final publication.

Scoped files: apps/cli/internal/cli/{cmd_op.go,actions_test.go,docs.go}, generated docs/user/cli/reference.md, sdk/python/tools/gen.py, sdk/python/tests/test_gen_hostile.py and this report/envelope. Import only independently reviewed ready deltas from PR103/104. Preserve current native-IPsec CLI documentation, NGFW test names and main SDK whitespace-injection regression. CLI actions validate IPv4/IPv6 target/no zones before HTTP and send JSON target; traceroute501 remains. Python generator rejects normalized method/argument collisions before writing output. No product protocol changes, daemon operations or CI configuration changes.

Historical PR103/104 branch CI successes are not current-main validation. Run meaningful CLI request tests and all SDK tests, independently review final integration, publish checkpoint, then integrate current actualmain and unchanged complete local/hosted gates before expected-head merge. Original PR103/104 histories must remain archived.
