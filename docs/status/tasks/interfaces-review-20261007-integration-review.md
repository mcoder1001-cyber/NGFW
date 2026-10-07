# Bounded Interfaces / wizard integration review

Compared independently via git show/diff without cherry-picking wizard product:
- Interfaces: e9e94d17e56470caf7526a61c59d915cc2c0866e (previous R1–R7 source approvals).
- Wizard: dfb7844a84c8653f7fbf59845da5654d68355697 against3ddb1680e.
- Exact combined integration tree is not supplied yet. No product writes or live host actions.

## R6 MAJOR: partial observation metadata must reach wizard user

Wizard SetupWizardPage.tsx only warns on `interfaces.isError`. New Interfaces state contract intentionally converts retrieve/live/HostNics failures to HTTP200 with available rows plus `observationErrors` and availability flags. In a combined tree, VPP observation may fail while configured rows remain. The wizard then presents that subset without its existing failure/retry warning, concealing incomplete discovery. The Interfaces page itself correctly surfaces this metadata.

Fix: show translated partial observation warning plus retry when the successful payload indicates failed sources. Preserve available configured/live selections. Regression must cover HTTP200 with retrieve/live observationErrors and configured selections still accessible. Root accepted this finding and will implement the contract-consumer fix in its own combined integration worktree; independent reviewer must verify final delta.

This is a combined-contract issue, not a blocker on isolated wizard PR198 against the old all-or-error endpoint.

## Compatible behavior inspected

- Host inventoryOnly rows have state:null and cannot pass wizard live eligibility. Management/host-only discovery remains observational and cannot trigger implicit adoption.
- Configured existing rows retain running-document selection; host-owned/local0 are excluded. Live unconfigured rows must be parentless, unmanaged, eligible physical engine types/defaultVRF with valid parent name. The selected peer cannot also be WAN/LAN.
- API setup preview/stage does not trust inventory UI metadata. Missing selected config names must be observed independently from actual InterfaceState, with eligible type, unmanaged/defaultVRF and nonzero index. It clones running before default insertion; preview is non-mutating, stage retains existing transactional password+config flow and fresh revision/candidate safeguards.
- Optional additive REST fields do not break wizard parsing; explicit state:null short-circuit avoids null dereference. Interface API/client field names remain unchanged.

No command/test pass claimed for the combined tree; final integrated SHA, consumer warning fix/regression and full unchanged gate still pending.

Verdict: BLOCK combined integration pending R6 MAJOR fix; prior isolated source approvals remain applicable.
