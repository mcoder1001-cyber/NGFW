# Product boundaries and outstanding acceptance

This is the STATUS-FINAL preparation register, not a release certificate. Source
integration, a passing quick gate and live acceptance are separate evidence.
The manager must refresh this register against the final integration/security
reports and the current board before closing whole-product acceptance.

## Explicit exclusions from the compressed plan

Per D-059 and D-125 in [the decision log](../decisions/LOG.md):

| Item | Current boundary |
|---|---|
| D10.1 VDOM | Multi-tenancy deferred; single-tenant guardrails apply |
| D12.3 pentest | Independent penetration testing is not certified by source review |
| D12.4 full docs | This user-guide index and feature documentation do not constitute full release documentation |
| D12.5 QA/interop | Full interoperability matrix has not been accepted |
| D12.6 72h soak/chaos | No 72-hour stability or chaos certification is claimed |
| D12.7 certification | No external certification is claimed |
| Ansible | Collection is not built; Python SDK and Terraform source are available, not registry publications |
| NETCONF | Not provided by the RESTCONF/YANG feature; consult its feature guide for actual supported operations |

## Acceptance still requiring evidence

- Final integrated E2E and whole-tree security reports must identify the tested
  commit, actual PASS/FAIL/NOT RUN results and any outstanding source failures.
- [Deferred acceptance](DEFERRED-ACCEPTANCE.md) remains authoritative for live
  VPP/daemon/packet, installed appliance, real HA/failover and browser scenarios.
  NOT RUN is not PASS. Source and security gaps cannot be reclassified as lab-only.
- [Task board](../../plan/tasks.yaml) and [progress](PROGRESS.md) show source task
  state; merged task counts are not a product readiness percentage or live worker
  inventory. The manager must reconcile outstanding tasks after the campaign.
- Package provenance/licensing, host privilege decisions, provisioning trust pins
  and incomplete feature paths remain open whenever the current task report or
  decision file records them. This register does not override those documents.
- No throughput, hardware performance or deployment certification is inferred
  from the unit gate or generated user documentation.

## Available source documentation

[User guide](../user/README.md) links the existing feature pages and
[source reference](../user/source-reference.md) links API controllers and schema
sources. The installed API's OpenAPI contract describes requests and responses;
feature pages state configuration workflows and limits. Generated navigation is
validated against the checkout, not against an installed appliance.
