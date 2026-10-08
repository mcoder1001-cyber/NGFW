# TD19 offline Python lock envelope

Branch: codex/td19-python-lock-20261008. Base: 4d4723f.
Owned: scripts/lab-python-lock.py; scripts/tests/lab-python-lock.py; .github/workflows/lab-python-lock-fixtures.yml (manager-authorized narrow extension); docs/status/tasks/td19-python-lock-20261008*.

Implement an offline wheel-only resolver and hash-closure generator for the independently versioned lab environment. Inputs: exactly six operator-approved pinned direct packages and a trusted local wheelhouse. Output: complete requirements.lock and digest/runtime provenance. No package installation, host services, network, trust-anchor changes, guessed release versions/hashes, board or shared decision edits. Preserve current default installer refusal. Tests use explicitly synthetic wheels; these are not release pins.

Release completion still needs reviewed direct version selections, official upstream artifact provenance, and generation/acceptance in the Ubuntu 26.04 x86_64 target runtime. Publish coherent checkpoints immediately, independent review and full unchanged hosted quick gate required before manager merge.
