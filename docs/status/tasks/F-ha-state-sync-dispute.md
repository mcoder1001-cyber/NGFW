# HA third-round API mapping case

The manager accepts independent R3 finding R3-HA-01. An administrator calling HA resync through a non-globals-owner agent receives HTTP 502 for the intentional gRPC PERMISSION_DENIED refusal, while the controller documents HTTP 403. The generic AgentClient problem translator lacks that status case. The real API with private PostgreSQL and Valkey reproduced this twice through fake-agent RPC transport. Readonly and operator guards still refused dispatch with HTTP 403; the other nineteen API cases and concurrent operator writes passed.

Preserved source commit: `48e5af12fd6a42323f3c83d04129eb6563cef650`.
Published R3 evidence commit: `180e61da0814d1e024c419412eaf7abf57b9dfdf`.

The previously reported complete source quick gate passed in 32m02s, but R3 BLOCK and T2 FAIL prevent merging. Two previous correction rounds covered the concrete failover probe and credential redirect guard. A fresh A1 ruling is required for the third correction. The proposed narrow correction restores HTTP 403 classification through the existing generic translator and adds actual AgentClient transport and real API replay regressions. Existing HTTP 409 and 503 mappings, administrator guard, failure audit and withheld completion must remain intact. No contract fields or security privileges change. Fresh independent R1, R2 and R3 closure, T2 replay, current-main complete quick and hosted gates remain mandatory.


A1 third-round disposition (2026-10-05): REASSIGN; R3-HA-01 upheld. See F-ha-state-sync-ruling.md. Existing error mapping correction needs no owner PENDING; current source remains blocked. Restart fixture failure establishes no source failure; restart remains UNVERIFIED. Manager provisions fresh developer and independent closure/current-main gates.
