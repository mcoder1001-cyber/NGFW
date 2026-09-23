# Task: Daily integration   (prepend 00-CONTEXT.md)

You are the integration agent. Run this once per day, end of day.

1. List open PRs approved by the review agent. Merge them into `main` in dependency
   order (contracts first, agent, api, web). Resolve trivial conflicts; for anything
   non-trivial, stop and report which two PRs conflict and why.
2. On `main`: `pnpm install && pnpm gen` — generated output must be clean
   (`git status` empty). If not, a PR hand-edited generated code: find it, revert, report.
3. `tools/lab down && tools/lab up tri`.
4. Run the full suite: lint, typecheck, unit, integration, then the topology E2E in
   `test/topology/` (three VRX nodes + traffic hosts). Paste the summary.
5. Chaos pass: `tools/lab kill-vpp vrx-a`; verify every
   configured object is back within 30 s by comparing `Retrieve` output before/after.
6. Tag `main` as `nightly-YYYYMMDD`. Write `docs/status/YYYY-MM-DD.md`: what merged,
   what is red, what is blocked, what the humans must verify tomorrow (see
   docs/10 §5 human gates).
7. If anything is red, open an issue per failure with the reproduction and assign the
   originating PR's branch name. Do not attempt large fixes yourself — your job is to
   keep `main` honest, not to write features.
