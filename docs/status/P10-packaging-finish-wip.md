
### Runtime installer policy recovery checkpoint

The temporary no-start policy now uses a protected persistent sibling recovery
folder in `/usr/sbin`, so publishing/restoring the policy uses same-filesystem
atomic rename. The installer arms cleanup before publication and syncs recovery
state before replacing the original. After abrupt termination, a subsequent
installation refuses the stale recovery record before APT; operator recovery is
required, and an unexpected concurrently changed policy is never overwritten.
Fixture checks cover missing, regular, and symlink policies, success, APT failure,
and SIGKILL during APT followed by refusal to retry. These checks do not establish
real power-loss durability or appliance boot acceptance.

Source call-graph audit: agent startup RPC performs validation/generation only;
`apply-startup.sh` driver rebind writes run through its separate `systemd-run`
transient unit, or explicit operator `--foreground` execution. Thus the agent
unit's kernel protections do not restrict that separate unit. Target runtime
verification remains NOT RUN.
