# Product boundaries and outstanding acceptance

This is the STATUS-FINAL preparation register, not a release certificate. Source
integration, a passing quick gate and live acceptance are separate evidence.
The manager must refresh this register against the final integration/security
reports and the current board before closing whole-product acceptance.

## Final integrated source receipt — 2026-10-09

Source candidate `30de26ee6a5697a3713fe4375399b86c5588a472` passed the unchanged complete hosted quick gate
[37903333143](https://github.com/mcoder1001-cyber/NGFW/actions/runs/37903333143) and all applicable fixture workflows.
[PR214](https://github.com/mcoder1001-cyber/NGFW/pull/214) merged as `d58db1a673d716ebcf595a9e49847a54991c58da`; merge tree equals tested tree
`d1de8b7c01220ce03b311d0a7dcafd5fe12ab1ad`. Reviewed history is preserved at
`codex/archive-completion-corrected-20261009` (`3f98838119ed8d67aab9c226308829ac49af9def`).
Final R2/R4 correction receipts approve the scoped source; initial failed CI and
local environment failures remain historical evidence, not retroactive PASS.

| Final check | Result |
|---|---|
| Mandatory quick | PASS, complete unchanged hosted gate |
| [Packaging37903333245](https://github.com/mcoder1001-cyber/NGFW/actions/runs/37903333245) | 81 fixtures, zero failures/errors/skips; seven gate-policy controls PASS |
| [Provisioning37903333184](https://github.com/mcoder1001-cyber/NGFW/actions/runs/37903333184) | 46 strict +23 offline Debian +11 trusted installer +18 portable export PASS |
| [Python37903333309](https://github.com/mcoder1001-cyber/NGFW/actions/runs/37903333309) | 14 synthetic-wheel +5 release-contract tests PASS |

Board: **205 merged,7 parked,0 review,0 running**, total212. The eight reviewed
source rows are merged. PPP-host and MultiWAN-host source is integrated; their
native acceptance joins the five already parked rows. No live source worker is
claimed. All older pending-source/CI statements in this document are historical
and superseded by this receipt.

Native acceptance of this cumulative source remains **NOT RUN**. Known prior RA
supplier/post-ACK identity failures and P12 mgmtd startup failure at the unchanged
30-second deadline before the200-route proof still require diagnosis, any necessary
fix and rerun on the real target. No unit/fixture result closes these cases.
Product license text/name/copyright authority remains a separate release input
under `docs/decisions/PENDING-P10-product-license.md`; no license is invented.
Plan exclusions remain unchanged. This is source completion, not release certification.

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
