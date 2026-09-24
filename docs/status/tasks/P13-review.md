# P13 — review (`vrx` CLI, branch task/P13 @ a84258c)

Reviewer: independent review agent, run directly on the host. Nothing in the code was changed.

## What I ran

| check | result |
|---|---|
| `tools/ci.sh --base main` | **CI GATE PASSED** (wall 3m30s, logs `/root/ngfw-wt/logs/ci/P13-20260924-034959-2574381`), same steps and outcome as the output pasted in P13.md. The gate does not build/lint/test `apps/cli` (P13-questions #1). |
| `make -C apps/cli all` | check ok · gofmt ok · `go vet` ok · golangci-lint `0 issues.` · `go test -race` all packages ok · build ok |
| drift / reference tests with `-v` | `TestOperationsTableMatchesOpenAPI` **PASS** (it ran and did not skip, because ci.sh had just run `pnpm gen`), `TestReferenceDocIsCurrent` PASS |
| e2e on slot 3 (`pg-test.sh create w3`, `tools/lab lock shared devstack.sh start`, real vrx-api + vrx-agent + host VPP) | `--- PASS: TestREPLConfirmedCommitAutoRevertAndRBAC (12.87s)` |
| manual probes (real slot-3 stack + a local fake API on 127.0.0.1:3399 run from the scratchpad) | Reproduced H1, H2, M1, M2 and M3 below |
| cleanup | stack stopped by PID (`api stopped (pid 2600413)`, `agent stopped (pid 2600394)`); `vrx_w3` dropped; 12 `vrx:w3:cli:*` Valkey keys deleted, 0 left; `vppctl show int \| grep -c loop3` → 0; worktree clean. **Left behind:** `/run/vrx-test/w3/{admin.pw,jwt.key,api.log,agent.log}`, which the sandbox did not let me delete. See L6. |

Checklist: (1) no contract files changed · (2) the e2e runs against the real API, agent and VPP, and it passed on my run · (3)/(4) not
applicable (no VPP objects, no binapi) · `grep -rn vppctl apps/cli` finds nothing, and `make check` also keeps `/run/vpp`, govpp and the agent socket out ·
(5) the slot prefix is honoured and PIDs are stopped by PID, see L6 · (8)/(10) no UI · (9) see L7 · (11) matches.

## Findings (ranked)

### H1: Server-supplied strings reach the operator's terminal raw (ANSI/OSC escape injection)
- **Where:** `internal/cpath/cpath.go:107-143` (`NeedsQuote`/`Quote` escape only `"` `\` `\n` `\t` `\r`, and ESC does not even trigger
  quoting) → `internal/render/render.go:18-35` (`Scalar`, used by `show configuration` in every format and by `compare`/`diff`/`drift`);
  `internal/cli/cmd_op.go:581` (`show revisions` comment), `:635` (`show revision` header), `:652` (`show commit pending`),
  `internal/cli/cmd_config.go:457` (commit result comment), `:473`/`:545` (agent messages, warnings), `internal/cli/exit.go:325-346`
  (problem `detail`, `errors[].message`, `results[].message`, lock owner), `internal/cli/app.go:471` + `prompt()` (username from
  `/auth/me` goes into the prompt), `internal/lineedit/lineedit.go:363-386` (completion/help lists: map keys from the candidate).
- **Proof (real stack):** `vrx commit comment "$(printf 'cli review\033]0;PWNED\007\033[2K\rinnocuous')"` is accepted (the API
  validates `comment` only as `z.string().max(1024)`). Then `vrx show revisions 1 | od -c` gives
  `c l i   r e v i e w 033 ] 0 ; P W N E D \a 033 [ 2 K \r i n n o c u o u s`, and the commit output itself printed the raw
  sequences. Against the fake API, `detail: "bad \x1b]52;c;…\x07 value"` came out raw on stderr, and a username with `\x1b[31m` was
  put into the prompt.
- **Scenario:** an `operator` user commits with a comment, or sets one of the 26 `description` fields that have no control-character
  pattern (every `acl/*`, `nat/*`, `objects/*` description, found by walking RootConfig), or a key of `routing/bgp/neighbors` or
  `routing/ospf/areas` (keys without a pattern). When an admin later runs `show revisions`, `show configuration`, `compare` or
  `show drift`, `\x1b[2K\r` / `\x1b[1A` erase or overwrite lines. That can hide a malicious `+ set …` line in the diff the admin is
  reviewing before commit. OSC 52 writes the admin's clipboard (paste-jacking into the root shell), and OSC 0/2 changes the title.
  Some terminals also answer query sequences (DECRQSS, OSC 10/11) back into stdin, and in raw mode the REPL reads those bytes as typed
  input.
- **Fix:** add one output sanitiser used by every human-mode writer: render C0/C1 controls and DEL (and bidi overrides U+202A–U+202E,
  U+2066–U+2069) as `\xNN`/`\u{…}`, and make `Quote` quote *and* escape them so `set` lines still round-trip. The simplest wiring is
  to wrap `a.Stdout`/`a.Stderr` for text output, leaving `--json` untouched because JSON already escapes them. Add a unit test with ESC,
  BEL, CSI, OSC and U+009B in a description, a comment, a problem detail and a map key. Separately (API owner, not P13): comments and
  descriptions should get the same control-character pattern that `interfaces.*.description` already has.

### H2: One Ctrl-C (or SIGTERM) during a command leaves the REPL permanently unusable. Every later call reports "API unreachable"
- **Where:** `internal/cli/app.go:117` creates `signal.NotifyContext(… os.Interrupt, SIGTERM)` once per process. `repl()` (`:399`)
  passes that ctx to every command. While a command runs, the tty is back in cooked mode (ISIG on), so Ctrl-C raises SIGINT and
  cancels the process context for good. The notifier also stays registered, so later Ctrl-C/SIGTERM are swallowed.
- **Proof (pty against a slow fake API):**
  ```
  adm@vrx> show system        ^C
  error: API unreachable (GET /api/v1/state/system): … interrupt signal received
  adm@vrx> show whoami
  error: API unreachable (GET /api/v1/auth/me): … interrupt signal received      ← every command from now on
  ```
- **Scenario:** after a risky `commit confirm 60`, the operator presses Ctrl-C on a slow `show …`. `confirm` now fails with "API
  unreachable" (exit 9). The operator concludes the change cut off management, and the change reverts even though it was good. Or the
  operator believes the box is down. `kill -TERM` also no longer ends the REPL.
- **Fix:** give each REPL command its own `signal.NotifyContext` derived from a never-cancelled base and `stop()` it after the
  command. Keep the whole-process context only for one-shot mode. Print `interrupted` (not "API unreachable") for
  `context.Canceled`, and add a pty test: Ctrl-C during a command, then a command succeeds.

### M1: Session file: symlinks are followed and an existing file's mode is inherited, so the token can end up world-readable or overwrite an arbitrary file (fallback path)
- **Where:** `internal/cli/app.go:305-318` falls back to `os.TempDir()/vrx/session.json` when `XDG_RUNTIME_DIR`, `XDG_CACHE_HOME`
  and `HOME` are unset (for example `env -i`, a systemd unit without `User=`, some cron setups). `:342-355`
  `saveSession`: `MkdirAll(0700)` accepts an existing directory that another user owns with mode 0777. `os.WriteFile(path+".tmp", …, 0600)` has
  no `O_EXCL|O_NOFOLLOW` and keeps the mode of an existing file. `loadSession` (`:320`) uses `Stat` (follows symlinks) and never checks the owner.
- **Proof** (`TMPDIR=<scratch>/tmpd`, HOME/XDG unset, directory 0777):
  - a planted symlink `session.json.tmp → victim`: after `vrx login`, `victim` contains `{"api":…,"token":…}` with mode **644**. That
    is an arbitrary file overwrite as root, plus a readable token.
  - a planted `session.json.tmp` with mode 0666: after login, `session.json` has mode **666**.
- **Fix:** never fall back to a shared temp directory; with no HOME/XDG, do not persist and say so. Create the directory with
  `Mkdir`, then `Lstat` it and require the current uid as owner, mode 0700 and not a symlink. Write through `os.CreateTemp` in that
  directory, then `Fchmod 0600` and rename. When loading, use `Lstat` and require the owner to be the current uid. Apply the same
  rules to the history file (`:512-531`), whose mode also stays whatever the existing file has ("0600" in P13.md only holds on
  creation).

### M2: `delete <list> <value>` on an integer leaf-list deletes by *index*, not by value
- **Where:** `internal/cli/cmd_config.go:295-305` first tries the whole argument list as a path. `jschema.Child`
  (`internal/jschema/jschema.go:258-263`, `isIndex`) accepts any digit string as an array index, so the "remove this value" branch
  (`:306-333`) never runs for numbers.
- **Proof (real stack):** candidate `dataplane corelist [5,7,1]`. `delete dataplane corelist 1` → exit 0, and the list is now
  `[5,1]`: core 7 was removed instead of core 1. The same applies to `routing policy routeMaps * entries * set asPathPrepend`.
  `set … corelist 1` appends the *value*, so `set` and `delete` are asymmetric.
- **Fix:** when the node before the last word is a leaf-list and the last word is not quoted as a pointer, treat the last word as a
  value (Junos semantics). Address items by index only through `/…/corelist/1` pointer syntax. Add a unit test with an integer list.

### M3: Commit-confirm UX: nothing in the session keeps saying that a revert is pending, and the texts mislead during the window
- **Where:** `internal/cli/cmd_config.go:446` prints the deadline only once, as an ISO UTC instant (`…T00:25:40.048Z`, while the host
  runs at +0330), with no "in 60 s". The prompt (`app.go:384-397`) and the `[edit]` banner (`:413`) do not change while a commit is pending.
  `exitConfig` (`cmd_config.go:570-580`) and `quit`/`exit`/Ctrl-D (`app.go:423-443`) do not warn. Nothing is printed when the revert
  happens.
- **Proof (pty, real stack):** after `commit confirm 20`, `exit` printed
  `note: the candidate keeps 1 uncommitted change(s) — `commit` or `discard` them`. That is wrong for this state: the
  applied-but-unconfirmed change is still "uncommitted" in the candidate, `commit` then returns 409 (exit 6) "waits for
  confirmation", and `discard` succeeds silently. `show configuration … mtu` returned 404 during the window because running excludes
  the pending change. `exit` then left the REPL with the revert armed and no warning.
- **Fix:**
  - Track the pending commit, from the commit answer and `Config_pending` on each prompt, cached like `candCache`.
  - Show it in the prompt or banner: `[edit] (commit d97486… reverts in 42 s — type confirm)`.
  - Print the deadline as local time plus the seconds remaining.
  - Refuse or warn on `exit`/`quit`/EOF while one is pending: "pending commit will revert in N s; confirm first or type `exit` again".
  - While a commit is pending, make `exitConfig`'s note say "`confirm` to keep it" instead of "commit or discard".
  - At the next prompt after the deadline, print "commit … was reverted automatically".

### M4: HTTP to a non-loopback host silently sends the password and bearer token in cleartext
- **Where:** `internal/api/client.go:67-72` accepts any `http://` base. The CLI has no warning and no refusal.
- **Scenario:** `vrx --api http://10.0.0.1:3000 login admin` from a jump host sends the password and a 15-min token in cleartext.
  TLS verification is on by default and there is no `--insecure`, which is good.
- **Fix:** refuse `http://` unless the host is loopback, the future unix socket, or `--insecure-http` is given. Document
  `SSL_CERT_FILE` (or add `--ca-file`) for the product's self-signed certificate.

### M5: Self-written line editor: long lines, resize, wide/combining characters, unknown escape sequences
- **Where:** `internal/lineedit/lineedit.go:205-210` `refresh()` redraws with `\r` + prompt + the whole buffer and moves back with
  `\x1b[<runes>D`. There is no `TIOCGWINSZ`, no SIGWINCH handling, and no display width.
  - **Wrapped lines:** once prompt+line is wider than the terminal, each keystroke reprints the whole buffer from the last physical
    row. The screen fills with copies and the cursor position is wrong. Pasted `merge … {json}` and long `set` lines hit this.
  - **Width:** CJK/emoji (width 2) and Persian or Arabic combining marks (width 0, e.g. U+064E) put the cursor in the wrong place after
    ←/→ or Ctrl-A. RTL display is left to the terminal. That is acceptable, but it is not tested.
  - **`escape()` (`:263-307`):** `ESC [ 1 ; 5 C` (Ctrl-→) inserts the text `5C`, F5 `ESC [ 1 5 ~` inserts `~`, and a lone ESC or
    Alt-x swallows the next key.
  - **Paste:** Tab inside pasted text triggers completion. `?` outside quotes, for example in a URL, runs help and drops the character.
    There is no bracketed paste.
  - **No unit tests** (`lineedit [no test files]`).
- **Fix:**
  - Track the terminal width through `TIOCGWINSZ` and SIGWINCH.
  - Compute display width with a small wcwidth table. `golang.org/x/text` is in apps/agent's go.sum, not in apps/cli's, so using it
    means adding the dependency.
  - Handle multi-row redraw, or fall back to horizontal scrolling of the line.
  - Parse CSI sequences completely: parameters up to the final byte 0x40–0x7E.
  - Enable bracketed paste (`\x1b[?2004h`) and insert pasted text literally.
  - Add table tests feeding byte streams into `edit()` over a pipe pair, including Persian text.

### M6: Pasting a block keeps running after an error, including a trailing `commit`
- **Where:** `internal/cli/app.go:445-451`. In tty mode a failed line only `continue`s. A paste of `set …` lines that ends in `commit`
  therefore commits a partial change set when a middle line fails validation. Scripts on stdin do stop at the first failure.
- **Fix:** with bracketed paste (M5), treat one paste as a batch that stops at its first error. Or offer `load set terminal`, which
  collects lines, validates them all and then applies them. At minimum, document the behaviour.

### L1: `ping` / `traceroute` drop the target
`internal/cli/cmd_op.go:711-718`: `args[0]` (the host) is checked for count and then never sent: no body and no parameter. Today it is
hidden behind the 501 from the API. When P08 implements actions, the CLI will send pings without a destination. Send
`{"target": host}` per the action's schema, or keep the command out until the schema exists.

### L2: `delete` with no path at an edit level deletes the whole subtree without asking
`internal/cli/cmd_config.go:291-305`: at `[edit interfaces]`, a bare `delete` removes all interfaces from the candidate. Junos asks
`Delete everything under this level? [yes,no]`. Ask for confirmation on a tty, or require an explicit path.

### L3: The OpenAPI drift test is real but not enforced
`internal/api/api_test.go:18-22` passes the check when `packages/api-client/openapi.json` (gitignored, written by `pnpm gen`) is
missing, and ci.sh does not run apps/cli at all. A route change on main therefore breaks the CLI silently until someone runs `make` by hand. I confirmed
it runs and passes after `pnpm gen`. Fix: the manager adds the ci.sh step (questions #1). In the test, fail instead of skip when
`CI`/`VRX_CI` is set. The completion/validation unit tests use the hand-kept `internal/testdata/openapi-min.json`, which is not
checked against the real RootConfig.

### L4: `logout` in one-shot mode does not revoke anything
`cmd_op.go:752-771`: `vrx login` (one-shot) receives the refresh cookie and drops it. `vrx logout` only deletes the file, so the
refresh-token family stays in Valkey until it expires, and the access token stays valid for up to 15 min. Either call `Auth_logout` with the
access token (if the API supports it) or say "session file removed; the token stays valid until <exp>".

### L5: Completion and help block without any way to interrupt them
`internal/cli/complete.go:21,33` use `context.Background()`: the first Tab loads `/api/docs-json`, and later ones load the candidate.
With a hung API that can block the raw-mode editor for the full 90 s client timeout, and Ctrl-C cannot interrupt it because ISIG is off. Use
a context with a ~3 s timeout.

### L6: `devstack.sh stop` leaves the slot's secrets behind
`apps/cli/test/devstack.sh:52-66` stops the PIDs but leaves `admin.pw`, `jwt.key`, `api.log` and `agent.log` in `/run/vrx-test/<prefix>/`. Those
files are 0600 on tmpfs, but shared-host-rules expect a clean slot. My run left them there because I was not allowed to delete them.
The manager should remove them, and `stop` should do it itself, with `--keep` for debugging.

### L7: Scope notes (acceptable, listed per checklist 9)
Beyond the prompt the branch adds `merge`, `validate`, `show drift|lock|whoami|revision <n>`, `show commit pending`, `api-key
create|list|delete`, `login`/`logout` and a one-shot mode for config verbs. All are thin wrappers over documented operations, and
`api-key` is needed for the non-interactive credential path. No removal requested.

### L8: Nits
- `show commit pending` prints `— comment: ` with nothing after it when there is no comment.
- `printWarnings` classifies warnings by matching English message text (`cmd_config.go:537-550`). Use a warning code if the API gains
  one.
- Quoted `".."` still climbs a level (`cpath.Resolve` ignores `Quoted`). Keys `..`/`.` are possible under the two unconstrained map
  keys.
- `readSecretFile` does `Stat` and then `ReadFile`. Use `Open` + `Fstat` so the check and the read hit the same file.

## Things checked and found fine
- The API key is read only from `VRX_API_KEY`, `VRX_API_KEY_FILE` or `--api-key-file` (the file must not be group/other-readable). No flag puts the key in argv. `api-key create
  … file` uses `O_EXCL` 0600 and creates the file before it creates the key.
- The password prompt turns off echo, and the e2e asserts the password never reaches the pty. `--password-file` has the same mode
  check. The refresh cookie stays in memory, and only the access token is saved.
- `--debug` prints only method, escaped path, status and duration: no headers, bodies or query (so no comment).
  `TestProblemAndCredentialsNotLogged` covers this.
- The `--json` error document holds exit code, status, message and problem only. Secrets are redacted server-side
  (`show configuration management` shows `users [ ]`, with no hash).
- The client only calls documented operations, rejects undocumented query parameters, and escapes pointer segments first with
  RFC 6901 `~0`/`~1`, then per segment with `url.PathEscape`. `TenGigabitEthernet0/0/0` becomes `TenGigabitEthernet0~10~10`, and the
  round-trip tests exist.
- Exit codes follow a single table (`exit.go`), and both the reference and `--json` are generated from it. 401→3, 403→4, 404→5,
  409→6 and 501→10 were confirmed on the real stack. The exception is H2, where a cancel is reported as 9.
- History drops lines that mention `password`, `secret`, `psk` or `vrxk_`. The only raw secret in RootConfig is `passwordHash`;
  everything else is a `secretRef`.
- TLS verification uses Go's defaults and there is no way to disable it. Redirects drop Authorization and Cookie when the host changes.

**APPROVE WITH CHANGES** (fix H1, H2, M1–M3 before merge; M4–M6 and L* may go to tech-debt with an owner)
