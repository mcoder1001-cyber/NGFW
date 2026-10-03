# P10 standalone WIP

Branch: `codex/p10-standalone-20261003`; base/local before checkpoint `71cee90b2a28421480dfd67a6d64ca7f47f37200`; remote publication pending.
Owned files: see P10-standalone-envelope.md.
Completed: policy read and actual Python/Bash dependency inventory; bounded trust design.
Tests: full VPP static verifier currently running; no code tests yet.
Remaining: helper exporter, authenticated bootstrap, real outside-checkout fixtures and adversarial refusals, docs, PR and hosted gate.
Current failure: none observed yet; system dependencies include Git/APT/patch beyond the initial Python/Bash/dpkg description.
Next command: implement `deploy/debian/bundle/helpers.py` and `recipient.py`, preserving canonical dependency paths and the unchanged VPP verifier.
