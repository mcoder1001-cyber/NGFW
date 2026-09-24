# F-object-model — WIP

Updated 2026-09-24 18:45 (slot 3, branch task/F-object-model; W-seed@df67a8e merged at 748694c per the manager's A1 safety note).

Done (committed):
- contract(proto): FqdnObjectState (e94ad5f) + questions Q1–Q4
- agent: internal/objects (Expand/ExpandService/Active/store/objects.* family/FQDN resolver) + unit tests
- agent: objects domain wired (subsystems A1, projection A2, rpc_object_model.go), agent-level tests
- api: ObjectModelController (/state/objects/fqdn, /state/objects/usage), fake, e2e file; api-client + CLI table regenerated
- web: Objects page, ObjectPicker/TagPicker, en/fa, W1–W3 hunks, unit tests

Next:
- API e2e green on slot 3 (host load caused vitest worker timeouts twice)
- docs/agent/objects.md, docs/user/firewall/object-model.md
- test/topology/object-model: real agent + API + DB, in-process DNS responder, agent-restart simulation, screenshots
- tools/ci.sh --base main; status file with pasted evidence
