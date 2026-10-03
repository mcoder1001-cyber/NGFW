# Independent P10 main storage/scope composition — R2/R4/R8

Exact local head `b07a5c4077156c60993d961ad8411f3cc51e83ae`, tree `3fc888ec`, compared against fresh main `8f07ce68744ac3ec87d0c4ec87661c64b59c3884`. Isolated task/P10-punt-composition-review; reviewer made no production edits.

**APPROVE this composition.** The agent unit preserves main's dedicated appliance comment and `NGFW_VPP_ID_RANGE=all` verbatim, plus reviewed punt activation `NGFW_BASE_POLICY=1` and nftables Requires/After ordering. Capability set remains exactly NET_ADMIN/SYS_ADMIN/IPC_LOCK, writable paths unchanged, no shared-lab activation authorized. `all` came from approved main, not a new privilege/range decision introduced by this composition.

Compared file identities with main: Debian control/postinst packaging files, API unit, API storage helper inputs, runtime installer and pending agent ownership decision have no diff. Only owned packaging overlaps are the reviewed dynamic-set bootstrap renderer/test plus agent activation unit; source delta is the previously reviewed typed punt lifecycle and scheduler uncertainty marker. Main storage directory ownership/hardlink/symlink safeguards and direct Python package dependency remain retained. Installer still requires explicit NGFW_INSTALL_APPLIANCE=1 and verified artifact input, warns never shared development host; ngfw-meta.postinst registers future boot without starting/restarting VPP.

Personally executed complete combined packaging fixture suite in exact frozen worktree:

```
python3 -m unittest discover -s deploy/debian/ngfw/tests -v
Ran 30 tests in 4.967s
OK (skipped=1)
```

Actual result **29 PASS, 1 SKIP**, not30PASS: real-GPG repository fixture skipped because isolated gpg-agent cannot start; signing NOT RUN. New API storage child-swap, helper shipping/dependency, reconfigure preservation and link-refusal tests each passed, along with dynamic base-policy bootstrap/boot-order and three runtime policy boundary regressions. Fixtures execute isolated temporary paths, not real appliance installation or host nft/VPP/systemd calls.

Full hosted quick on the new final integration head must be green before merge; an earlier base/head run is not proof for this new composition. No unnecessary whole-Go repetition performed. Real Ubuntu/appliance traffic/boot/restart acceptance and CAP_CHOWN/global /etc decisions remain unchanged and unresolved.
