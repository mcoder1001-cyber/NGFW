# F-sdk-terraform-ansible — questions for the manager

1. **Ansible collection not built.** D-085 lists "Ansible cut (see prompts)"; the prompt makes it priority 3 "if
   reached". I stopped after the Python SDK and the Terraform provider. Left over: `sdk/ansible/` (`vrx.appliance`:
   `vrx_config` present/absent + check-mode diff via the candidate, `vrx_commit`, `vrx_facts`) on top of `vrx.VrxSession`
   (`transaction()`, `diff()`, `state()` already cover what those modules need). Needs `ansible-core` in a worktree venv.
   → a follow-up task, or confirm it stays cut?

2. **CI hook.** `sdk/test.sh` (~15 s: pytest + gofmt/vet/golangci-lint/go test of the provider) and
   `sdk/gen.sh --openapi packages/api-client/openapi.json --check` (~3 s once `pnpm gen` has written that file) are not
   in `tools/ci.sh` (not mine, owner P09). Suggest a step after "generate + generated-output gate" that runs both;
   `sdk/test.sh` creates `sdk/python/.venv` from the hash-pinned lock (needs PyPI reachable, or a warm pip cache).

3. **Terraform: one commit per resource.** Each resource change is its own confirmed commit (provider serialises them
   with a mutex; a run with N changes = N revisions). Batching a run into one candidate would need an API session
   concept (e.g. `POST /config/sessions` → candidate id + lock, commit at the end) — the prompt's open question; I kept
   the default.

4. **Candidate lock is per user, not per API key.** An API key acts as the user that created it (the lock owner shows
   `admin` for the key). Two pipelines using keys of the same user can therefore edit each other's candidate. Both
   clients refuse to start on a dirty candidate (`GET /config/diff` non-empty), but that check is racy (diff → edit is
   not atomic). Suggest: lock ownership by key id, or an `If-Match: <base revision>` on edits/commit (P06 / F-aaa).

5. **OpenAPI shape of the generic config routes.** `{path}` is declared as one string parameter but carries `/`
   (Nest `*` route); generators must special-case the parameter named `path`. An extension such as
   `x-vrx-pointer: true` on that parameter would let generators treat it generically. `GET /config/{path}` responses
   are `{}` (any) — fine for a pointer API, noted only.

6. **`management.users` via automation** needs an enabled admin with a password in the list (semantic rule), and a
   config commit rewrites `app_user` (D-P06-3). The Terraform live test sets a random PHC hash for `admin` in the
   throw-away slot database; the user docs show the pattern (`sensitive_value` index-wise, `{}` keeps a hash). Is
   managing users from Terraform wanted at all, or should `management.users` be excluded from `vrx_config` like AAA?

7. **No terraform CLI on the host.** Per the envelope I did not install it. The provider is exercised through the
   plugin protocol by `sdk/terraform/internal/tfharness` (Terraform core's proposed-new-state rule, post-apply
   consistency check, refresh, import). A real `terraform plan/apply/destroy` run (Terraform ≥ 1.11 for the
   write-only attribute) should be done once where the CLI exists; `docs/user/system/sdk-terraform-ansible.md` has
   the HCL and the dev_overrides block.
