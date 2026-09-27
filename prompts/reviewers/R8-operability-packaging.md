# Reviewer R8 — operability & packaging   (prepend 00-CONTEXT.md, then ../REVIEW-PROMPT.md)

Mandatory when the diff touches `deploy/**`, `tools/**`, packaging (P10 `.deb`, P14 ISO), systemd units, `tools/ci.sh`, DB
migrations, logging, metrics or alarms.

## Check
1. **Install/upgrade/remove:** the `.deb` (P10) installs files with the right owners/modes, the postinst is idempotent, an upgrade
   from the previous package keeps config and data, purge removes only what the package created. New runtime dependency →
   declared in the package and `docs/09-os-packages.md`.
2. **Services:** systemd units with `Restart=`, sandboxing that does not break the function, ordering (`After=`/`Wants=`), no unit
   enabled on the shared dev host by a test.
3. **Migrations:** forward migration safe on a populated DB, bounded run time, documented rollback or a stated "one-way".
4. **Observability:** structured logs with the component and object id, no secrets (R2), log level sane by default; metrics named
   per convention with units; alarms have a clear condition and a clear message; a failure an operator must act on is visible in
   the UI or the CLI, not only in a log.
5. **CI and tooling:** `tools/ci.sh` only extended, never weakened (its header rule); scripts pass `shellcheck`; new tools listed in
   `tools/README.md`; no dependency on the operator's desktop.
6. **Recovery:** what happens after a crash, a reboot, a full disk — the change says or tests it.

## Output
`docs/status/tasks/<id>-review-R8.md` — findings, verdict line.
