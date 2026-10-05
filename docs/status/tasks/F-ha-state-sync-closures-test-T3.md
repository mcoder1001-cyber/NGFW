# Independent HA T3 fixtures

Source 48e5af12fd6a42323f3c83d04129eb6563cef650; /dev/shm/r4h; 2026-10-05 UTC.

Command: `cd /dev/shm/r4h && python3 -m unittest discover -s test/topology/ha-state-sync -v`

```text
test_candidate_drift_prevents_cleanup (test_acceptance.ConcreteEvidence.test_candidate_drift_prevents_cleanup) ... ok
test_exact_tuple_and_truncation (test_acceptance.ConcreteEvidence.test_exact_tuple_and_truncation) ... ok
test_fault_refuses_shared_host_boot_and_pid_reuse (test_acceptance.ConcreteEvidence.test_fault_refuses_shared_host_boot_and_pid_reuse) ... ok
test_foreign_candidate_lease_never_discarded (test_acceptance.ConcreteEvidence.test_foreign_candidate_lease_never_discarded) ... ok
test_foreign_pending_never_reverted (test_acceptance.ConcreteEvidence.test_foreign_pending_never_reverted) ... ok
test_full_exercise_requires_peer_tuple_roles_and_forwarding (test_acceptance.ConcreteEvidence.test_full_exercise_requires_peer_tuple_roles_and_forwarding) ... ok
test_kill_never_runs_without_exact_handover (test_acceptance.ConcreteEvidence.test_kill_never_runs_without_exact_handover) ... ok
test_one_connection_survives_echo_delayed_beyond_three_seconds (test_acceptance.ConcreteEvidence.test_one_connection_survives_echo_delayed_beyond_three_seconds) ... ok
test_one_real_socket_many_challenges (test_acceptance.ConcreteEvidence.test_one_real_socket_many_challenges) ... ok
test_replayed_challenge_fails (test_acceptance.ConcreteEvidence.test_replayed_challenge_fails) ... ok
test_tls_origins_and_namespace_guard (test_acceptance.ConcreteEvidence.test_tls_origins_and_namespace_guard) ... ok
test_exercise_requires_all_explicit_probes (test_driver.Safety.test_exercise_requires_all_explicit_probes) ... usage: python3 -m unittest [-h] --node-a NODE_A --node-b NODE_B
                           --output OUTPUT [--isolated-lab] [--exercise]
                           [--kill-vpp] [--after-handover]
                           [--create-session CREATE_SESSION]
                           [--verify-session-on-b VERIFY_SESSION_ON_B]
                           [--failover-command FAILOVER_COMMAND]
                           [--continuity-probe CONTINUITY_PROBE]
python3 -m unittest: error: exercise requires isolated lab and all four explicit argv probes
ok
test_fetch_refuses_non_origin_before_sending_credentials (test_driver.Safety.test_fetch_refuses_non_origin_before_sending_credentials) ... ok
test_fetch_rejects_foreign_and_downgrade_redirects (test_driver.Safety.test_fetch_rejects_foreign_and_downgrade_redirects) ... ok
test_readonly_never_executes_a_probe (test_driver.Safety.test_readonly_never_executes_a_probe) ... ok
test_same_node_and_unguarded_kill_refused (test_driver.Safety.test_same_node_and_unguarded_kill_refused) ... usage: python3 -m unittest [-h] --node-a NODE_A --node-b NODE_B
                           --output OUTPUT [--isolated-lab] [--exercise]
                           [--kill-vpp] [--after-handover]
                           [--create-session CREATE_SESSION]
                           [--verify-session-on-b VERIFY_SESSION_ON_B]
                           [--failover-command FAILOVER_COMMAND]
                           [--continuity-probe CONTINUITY_PROBE]
python3 -m unittest: error: two distinct appliances are required
usage: python3 -m unittest [-h] --node-a NODE_A --node-b NODE_B
                           --output OUTPUT [--isolated-lab] [--exercise]
                           [--kill-vpp] [--after-handover]
                           [--create-session CREATE_SESSION]
                           [--verify-session-on-b VERIFY_SESSION_ON_B]
                           [--failover-command FAILOVER_COMMAND]
                           [--continuity-probe CONTINUITY_PROBE]
python3 -m unittest: error: --kill-vpp requires --exercise --after-handover and explicit failover-command
ok

----------------------------------------------------------------------
Ran 16 tests in 3.238s

OK
```

Actual loopback TCP challenge exchanges and 3.2s delayed echo: PASS. Candidate/fault/API fixtures: PASS; no real appliance mutation or fault attempted. Two-appliance session replication/VRRP transition/SSH SIGKILL: NOT RUN, unprovisioned (no dedicated endpoints, credentials, VM markers).

Verdict: PASS (fixtures only); BLOCKED-ENV for actual two-appliance acceptance.

| Scenario | Expected | Observed | Result |
|---|---|---|---|
| Python fixtures | All pass | 16/16 | PASS |
| Same socket delayed3.2s | Survive without reconnect | Actual loopback echo succeeded | PASS |
| Two appliances/SSH fault | EI replica traffic continuity | Unprovisioned | NOT RUN |
