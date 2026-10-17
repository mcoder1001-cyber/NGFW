# agent-allifs-unbound-nics — review R6 (UX / web / i18n — en/fa, RTL, logical CSS)

- Reviewer: R6 (panel, added because the branch touches `docs/user/**`)
- PR: #241, branch `claude/agent-allifs-unbound-nics`
- Head reviewed: `873f1d946` (product tree `856bf743a`, the user-doc sentence from `0b57dc070`). Base: merge-base `99a774734`.
- Mode: read-only. No files were modified and nothing was created in the repository or worktree. I ran no gate, build, dev server or browser.
- Scope of the R6 surface in this diff: one table cell, `docs/user/security/global-blocking.md:27`. There are no changes under `apps/web/**`, `packages/ui-kit/**`, `packages/schema/**` or the locales. I also checked the new agent warning text as an operator sees it in the web Commit dialog.

## Findings (ranked)

### MAJOR-1 — `docs/user/security/global-blocking.md:27`: "every interface the data plane has" claims more than the code does

- **What the code does:** the all-interfaces target set is the top-level `interfaces` rows the projection keeps: `apps/agent/internal/desired/global_blocking.go:117` `allIfs = sortedKeys(kept)`. Several kinds of data-plane interface are not in that map:
  - **VLAN sub-interfaces** live under `interfaces.<parent>.subinterfaces` (`desired/interfaces.go:183`, VPP name `parent.id` from `SubName`, `:93`). Each is its own VPP sw_if_index.
  - **Tunnel interfaces** (GRE `gre<n>`, VXLAN, IPIP, VXLAN-GPE, GTP-U, L2TPv3, PPPoE sessions) live under `tunnels.*`. `docs/user/vpn/tunnels.md:8` says: "A tunnel is an interface. Its addresses … are set on the tunnel record itself".
  - **WireGuard `wg<n>`** lives under `vpn.wireguard.interfaces` (`docs/user/vpn/wireguard.md:8-9`).
  - **IPsec tunnel interfaces** also live outside `interfaces`.

  None of these is ever targeted by *All interfaces* or by the auto-block overlay.
- **The PR contradicts itself:** its own tech-debt row (`docs/tech-debt.md:228`, from R2 MINOR-4 and R4 O-2) says sub-interfaces "are never targeted". That row's fix is to "state the limitation in the schema help and `docs/user/security/global-blocking.md`". The sentence added in the same commit `0b57dc070` says the opposite. The old text ("also covers interfaces added later") was vague. The new text is a firm statement of scope, and the scope is wrong.
- **Scenario:** a router-on-a-stick with VLAN sub-interfaces, or a WireGuard road-warrior VPN. The operator relies on an all-interfaces block list to drop a listed source in transit. The page also says "Automatic blocking targets the same set", so they rely on auto-block too. Packets from that source on `ens161.100` or `wg0` are forwarded, because the acl plugin filters on the sub-interface's or tunnel's own RX/TX sw_if_index. Only traffic to the box is still dropped (nftables `in__gb`). This is a false security assurance in user-facing text.
- **Why the existing debt row does not cover it:** row 228 assumes the page will *state* the limitation, and it does not mention tunnels or WireGuard at all. A debt row cannot make newly written wrong text acceptable, and the fix is one table cell.
- **Fix in this branch:** reword the cell (MINOR-2 and NIT-6 are folded in). Suggested text:

  > Where the list is enforced. *All interfaces* means every interface on the **Interfaces** page that the engine has, including interfaces added there later. It does **not** include VLAN sub-interfaces or tunnel and VPN interfaces (**VPN › Tunnels**, WireGuard, IPsec). A built-in NIC shown as *awaiting engine* is covered once the engine takes it over. A NIC *released to host* is not covered. On both of these NICs the list drops only traffic to the appliance itself, and only when *Protect the box* is on. [Auto-block](auto-block.md) uses the same set. Naming an interface that does not exist is refused at commit.

  You may add "name a sub-interface under *Interfaces* to cover it", but only after checking that an explicit sub-interface binding works. The picker offers `parent.id` names (`GlobalBlockingPage.tsx:437`) and the schema accepts them (`vppInterfaceName`), but I did not verify the binding. Also extend `docs/tech-debt.md:228` to cover tunnel and VPN interfaces.

### MINOR-2 — `docs/user/security/global-blocking.md:27`: the Protect-the-box clause contradicts itself and has no condition

- The sentence says "a NIC released to the host is never covered by the list (its traffic to the appliance is covered by *Protect the box*)". But *Protect the box* is a setting of the same list. It drops listed sources on every host interface (`renderers/nftables/blocking.go:10-12`), so the list *does* cover that NIC for traffic to the box.
- The protection also only applies when the setting is on. The schema default is on (`packages/schema/src/domains/ext/global-blocking.ts:77`). With `protectHost: false` the NIC is not protected at all, as R2 MINOR-3 pointed out.
- The same host-side protection applies to an *awaiting engine* NIC, which is still a host interface, but the sentence only mentions the released NIC.
- **Fix:** use the wording in MAJOR-1: "On both of these NICs the list drops only traffic to the appliance itself, and only when *Protect the box* is on".

### MINOR-3 — `apps/web/src/locales/en/global-blocking.json:108`, `apps/web/src/locales/fa/global-blocking.json:108`, `packages/schema/src/domains/ext/global-blocking.ts:66`: the UI help text is now broader than the doc

- **Current help text:**
  - en: "every interface, including interfaces added later"
  - fa: «همهٔ اینترفیس‌ها، از جمله اینترفیس‌هایی که بعداً افزوده می‌شوند»
  - schema: "every L3 interface, including interfaces added later"
- **Why fixing the schema help alone is not enough:** the edit dialog localises the schema through `localizeSchema` in `apps/web/src/domains/firewall/host-acl-nftables/model.ts:120-137`. The locale key `field.allInterfaces.help` wins, and the schema help is only a fallback. So the fix in row 228 ("state the limitation in the schema help") would not reach users in either language.
- **The column label:** the list table's "Enforced on" column still shows *All interfaces* (`GlobalBlockingPage.tsx:280-281`). That is acceptable under the doc's definition once MAJOR-1 is fixed.
- **Should it be fixed here?** No. It is not a hard contradiction: the hint does not mention released NICs. A web or locale change would bring web scope (and R6 screenshots) into an agent-only PR. Make it a follow-up for the manager, with R3 owning the schema help.
- **Fix (follow-up):** extend `docs/tech-debt.md:228` to name both locale keys. Suggested strings:
  - en: "every engine interface, including interfaces added later — not sub-interfaces, tunnels, or NICs awaiting the engine or released to the host"
  - fa: «همهٔ اینترفیس‌های موتور، از جمله اینترفیس‌هایی که بعداً افزوده می‌شوند — به‌جز زیراینترفیس‌ها، تونل‌ها و کارت‌های شبکهٔ «در انتظار موتور» یا «واگذارشده به میزبان»»

  These use the terms the fa locales already use: «زیراینترفیس», «تونل», «در انتظار موتور», «واگذارشده به میزبان».

### MINOR-4 — `apps/agent/internal/desired/global_blocking.go:171`: the warning repeats on every commit and the UI shows it as plain text

- **How it reaches the operator:**
  - The API sends every implemented subsystem to the agent on every validate and commit (`apps/api/src/commit/validation.service.ts:140-146`, `commit.service.ts:412`).
  - Every projection applies the auto-block overlay (`agent/service.go:1389`), whose list always selects all interfaces (`autoblock/runtime.go:123`).
  - The API turns every non-error issue into a severity-less `warnings` entry (`validation.service.ts:152-154`).
  - `CommitDialog.tsx:149` turns the result amber when any warning exists, and shows `pointer — message` verbatim (`CommitDialog.tsx:154-165`, `CommitResultView.tsx:79-86`).
- **Scenario:** while any NIC is *awaiting engine* or *released to host*, every Commit dialog shows:
  - one line per all-interfaces list;
  - one line for `block list "auto-block"`, a list the operator never created and cannot find on the page;
  - the existing per-NIC `agent.nic-not-bound` / `agent.nic-released` lines.

  Release is the documented permanent state for a removed NIC (`docs/user/interfaces/default-dataplane-nics.md:108-110`), so this never goes away. Operators learn to ignore warnings.
- **Rule code and locales:** the web does not look up warnings by rule code. `grep` finds no use of `acl.global-blocking`, `nic-not-bound` or `nic-released` in `apps/web/src`. So **no locale entry is needed**, and the rule code is not new. As for every agent message, the English text also appears in the fa UI. That is a pre-existing pattern. The line renders correctly inside `dir="auto"`. `productWording` has nothing to rewrite because the text does not name VPP, which is good for D-155.
- **Fix (follow-up acceptable):**
  - emit one warning per transaction at `/acl/globalBlocking` naming the lists, or warn only for awaiting-engine NICs (a released NIC is an intentional state that `agent.nic-released` already reports);
  - for the overlay, say "automatic blocking (system list `auto-block`)".

  Record it as a tech-debt row or answer "won't fix".

### MINOR-5 — `apps/web/src/domains/security/global-blocking/GlobalBlockingPage.tsx:433-438` and `docs/user/security/global-blocking.md:27`: the explicit *Interfaces* picker offers NICs that break the commit

- **The trap:** the picker lists every candidate interface row, including *awaiting engine* and *released to host* NICs, with no marking.
  - Choosing an awaiting NIC refuses the commit with a raw descriptor message: `acl.interface-binding/ens192: mandatory dependency interface/ens192 is neither desired nor present` (status report line 76).
  - Choosing a released NIC is accepted at first, then breaks every acl-domain transaction and auto-block once the NIC leaves the engine (`docs/tech-debt.md:227`).
- **What the doc says:** only "Naming an interface that does not exist is refused at commit". These rows do exist, on the Interfaces page.
- **Status:** pre-existing and outside the task's scope ("explicit interface lists unchanged"). Row 227 covers the agent and schema, but not the picker or this page.
- **Fix (follow-up):** extend row 227 to:
  - (a) mark or disable those rows in the picker; the labels `interfaces:awaitingDataplane` and `interfaces:releasedToHost` already exist in en and fa;
  - (b) add one clause to this page linking to the explicit-reference paragraph that PR #238 will add to `default-dataplane-nics.md`.

### NIT-6 — `docs/user/security/global-blocking.md:27`: terminology

- **Feature name:** use "[Auto-block](auto-block.md)" instead of "Automatic blocking". That is the feature's name: `auto-block.md:1` and the nav/page label.
- **Awaiting engine:** "still awaiting the engine … once it is bound" should be "*awaiting engine* … once the engine takes it over". That matches:
  - the Interfaces screen label `awaitingDataplane` "awaiting engine";
  - `awaitingInfo` "once the engine takes the NIC over";
  - `default-dataplane-nics.md:68-73` ("taken over").
- **Released:** write "*released to host*", the UI label `releasedToHost`.
- **Links:** link both to `../interfaces/default-dataplane-nics.md#awaiting-engine` and `#releasing-a-nic-to-the-host-and-reclaiming-it`.
- **One term:** do not mix "data plane" and "engine" in one sentence.

### NIT-7 — `apps/agent/internal/desired/global_blocking.go:171`: the warning points at the wrong field

- The warning pointer is `/acl/globalBlocking/lists/<name>/interfaces`, but the operator set `allInterfaces`. The Commit dialog therefore shows a path to a field that is empty.
- Point it at `…/allInterfaces`. It is only a warning pointer, not a contract field.
- Optional: the text says "not bound to the data plane yet" while the Interfaces screen says "awaiting engine". This matches the sibling `agent.nic-not-bound` message, so changing it is optional.

### NIT-8 — `docs/user/security/global-blocking.md:27`: the table cell is long

The cell now holds four clauses. Consider a short subsection after the table, "What *All interfaces* covers", which also gives the MAJOR-1 exclusions room.

### NIT-9 — `docs/user/security/auto-block.md:52-59`: no link to where blocks are enforced

The auto-block page never says where blocks are enforced in the data plane. One line would let a reader find the scope rule: "on the same interfaces as a block list set to *All interfaces*; see Global blocking".

## Checked, no finding

- **Persian counterpart:** none is needed. The only Persian user doc is `docs/user/system/notifications.fa.md`. The index generator (`tools/docs/generate-reference.py`) reads page titles only, and the title did not change.
- **i18n, RTL, logical CSS, screenshots:** not applicable. The diff touches no file under `apps/web/**`, `packages/ui-kit/**`, `packages/schema/**` or the locales.
- **Menu path:** the doc's "**Firewall › Global blocking**" (line 3) matches `apps/web/src/nav/nav.ts:153` (firewall group, `/firewall/global-blocking`). The brief's "Security → Global blocking" is the doc folder, not the menu.
- **Accurate parts of the sentence:**
  - "including interfaces added later" and "covered once it is bound" are correct: the set is computed at projection time, and R4 verified the connect resync.
  - *Protect the box* covers every host interface (`blocking.go:10-12`).
  - The auto-block overlay always sets `ProtectHost: true` (`autoblock/runtime.go:123`).
- **New warning text:** it does not name VPP, so it complies with D-155.

## Notes for other reviewers

- **[other: R7]** The PR #241 body (`gh pr view 241`, head `873f1d946`) is stale:
  - it still cites D-250;
  - it describes the superseded design ("`desired.ACL` takes the Linux netdev lookup … (chosen …)");
  - it says `tools/ci.sh quick` is "**running**";
  - it says "agent only", without the `docs/user` change or R6;
  - it lists the review as "In progress".

  The status report's review table marks R7 #7 ("PR body") as handled.
- **[other: R3]** The schema help "every L3 interface" (`packages/schema/src/domains/ext/global-blocking.ts:66`) is R3's call, as R2 MINOR-3 said. See MINOR-3: the locale override means the schema text alone does not reach users.

## Evidence (commands run in `/root/NGFW/.claude/worktrees/funny-meninsky-c9ef19`, read-only)

```
$ git rev-parse HEAD; git merge-base HEAD origin/main
873f1d946a0c8e65f08603db4891025560bcaa66
99a774734e2cf455f71563ff0dba5afb58051685

$ git diff --name-only 99a774734...873f1d946 -- apps/web packages/ui-kit packages/schema docs/user
docs/user/security/global-blocking.md

$ git diff 99a774734...873f1d946 -- docs/user/
-| All interfaces / Interfaces | Where the list is enforced. *All interfaces* also covers interfaces added later. Naming an interface that does not exist is refused at commit. |
+| All interfaces / Interfaces | Where the list is enforced. *All interfaces* means every interface the data plane has, including interfaces added later; a physical NIC still awaiting the engine is covered once it is bound, and a NIC released to the host is never covered by the list (its traffic to the appliance is covered by *Protect the box*). Automatic blocking targets the same set. Naming an interface that does not exist is refused at commit. |

$ sed -n 116,118p apps/agent/internal/desired/global_blocking.go
			}
			allIfs = sortedKeys(kept)
			for _, ifn := range sortedKeys(ds.GetInterfaces()) {

$ sed -n 183,184p apps/agent/internal/desired/interfaces.go; sed -n 93p apps/agent/internal/desired/interfaces.go
		for _, id := range sortedKeys(itf.GetSubinterfaces()) {
			sub := itf.GetSubinterfaces()[id]
func SubName(parent, id string) string { return parent + "." + id }

$ sed -n 8,9p docs/user/vpn/tunnels.md; sed -n 8,9p docs/user/vpn/wireguard.md
A tunnel is an interface. Its addresses, MTU, VRF and (L2 tunnels) bridge-domain membership are set on the tunnel
record itself; other features (routes, VRRP, LLDP …) reference it by its engine interface name.
NGFW runs WireGuard in the VPP data plane (plugin `wireguard`, VPP 26.06). Each configured interface becomes the VPP
interface `wg<instance>`; its peers, addresses, MTU, VRF and (optionally) the routes of the peers' allowed IPs are

$ git diff 99a774734...873f1d946 -- docs/tech-debt.md   (row 228, excerpt)
+- 2026-10-17 (PR #241 review R2 #4, R4 O-2, pre-existing): the all-interfaces target set of the block lists and the auto-block overlay is the top-level `interfaces` rows only; VLAN sub-interfaces (`subinterfaces`, …) are never targeted … the schema help says "every L3 interface". Fix: include sub-interfaces in the implicit set … or state the limitation in the schema help and `docs/user/security/global-blocking.md`. …

$ sed -n 108p apps/web/src/locales/en/global-blocking.json; sed -n 108p apps/web/src/locales/fa/global-blocking.json
      "help": "every interface, including interfaces added later"
      "help": "همهٔ اینترفیس‌ها، از جمله اینترفیس‌هایی که بعداً افزوده می‌شوند"

$ sed -n 64,67p packages/schema/src/domains/ext/global-blocking.ts
    allInterfaces: withUi(z.boolean().default(false), {
      title: 'All interfaces',
      help: 'every L3 interface, including interfaces added later',
    }),

$ sed -n 77p packages/schema/src/domains/ext/global-blocking.ts
    protectHost: withUi(z.boolean().default(true), {

$ grep -n "localizeSchema" apps/web/src/domains/security/global-blocking/GlobalBlockingPage.tsx
43:import { localizeSchema } from '../../firewall/host-acl-nftables/model';
431:  const schema = useMemo(() => localizeSchema(listSchema(), (k, o) => t(k, o ?? {})), [t]);
$ sed -n 126-136p apps/web/src/domains/firewall/host-acl-nftables/model.ts   (excerpt)
        t(`field.${scope}${name}.${what}`, {
          defaultValue: t(`field.${name}.${what}`, { defaultValue: fallback }),
      ...
      const help = text('help', typeof hints.help === 'string' ? hints.help : '');

$ grep -rn -e "acl.global-blocking" -e "nic-not-bound" -e "nic-released" apps/web/src | wc -l
0

$ sed -n 149,165p apps/web/src/config/CommitDialog.tsx   (excerpt)
<Alert severity={validate.data.notApplied.length > 0 || validate.data.warnings.length > 0 ? 'warning' : 'success'} …>
  {validate.data.warnings.map((w, i) => ( … {diffPointerLabel(w.pointer || '/')} … {' — '} {serviceText(w.message)} …

$ sed -n 149,154p apps/api/src/commit/validation.service.ts
    const errors = report.errors
      .filter((i) => i.severity === IssueSeverity.ISSUE_SEVERITY_ERROR)
      .map(issue);
    const warnings = report.errors
      .filter((i) => i.severity !== IssueSeverity.ISSUE_SEVERITY_ERROR)
      .map(issue);

$ sed -n 1385,1389p apps/agent/internal/agent/service.go   (excerpt)
	if (contains(domains, "security") || contains(domains, "interfaces")) && … len(s.autoBlock.entries) > 0) {
		projectionDomains = union(projectionDomains, []string{"acl"})
	effective, overlayErr := autoblock.Overlay(view, s.autoBlock.entries, s.now())

$ grep -n "AllInterfaces: proto.Bool(true)" apps/agent/internal/autoblock/runtime.go | cut -c1-200
123:	out.Acl.GlobalBlocking.Lists[ListName] = &ngfwv1.GlobalBlockingList{Enabled: proto.Bool(true), AllInterfaces: proto.Bool(true), Direction: proto.String("both"), ProtectHost: proto.Bool(true), Log

$ sed -n 10,12p apps/agent/internal/renderers/nftables/blocking.go
// established traffic too (a list is a block, not a policy). It applies to every host interface: which
// Linux interface carries traffic of a VPP interface is not known here (management included). Accept
// rules in this chain only end this chain; the host lists' chains still decide for everything else.

$ grep -n -i "awaiting\|releas" apps/web/src/locales/en/interfaces.json | head -4
16:  "awaitingDataplane": "awaiting engine",
17:  "releasedToHost": "released to host",
19:    "release": "Release to host",
25:    "awaitingInfo": "Built-in NIC ({{pci}}) waiting for the engine: its settings apply once the engine takes the NIC over.",
$ grep -n -i "awaiting\|releas" apps/web/src/locales/fa/interfaces.json | head -2
16:  "awaitingDataplane": "در انتظار موتور",
17:  "releasedToHost": "واگذارشده به میزبان",

$ sed -n 153p apps/web/src/nav/nav.ts | cut -c1-130
  groups.get('firewall')!.push({ id: 'global-blocking', path: '/firewall/global-blocking', labelKey: 'global-blocking:nav', fallba

$ find docs/user -name "*.fa.md"
docs/user/system/notifications.fa.md

$ gh pr view 241 --repo mcoder1001-cyber/NGFW --json headRefOid,baseRefName,state,title
873f1d946a0c8e65f08603db4891025560bcaa66 / main / OPEN / fix(agent): all-interfaces block lists skip unbound and released NICs
  (body: "Options considered (decision log D-250): pass the lookup and re-split inside the acl projection (chosen …)";
   "`tools/ci.sh quick` on the head: **running**"; "agent only"; Review "In progress")
```

## Verdict

**BLOCK** — 0 BLOCKER, 1 MAJOR, 4 MINOR, 4 NIT.

- The block clears when the one table cell (`docs/user/security/global-blocking.md:27`) is reworded in this branch to fix MAJOR-1 and MINOR-2. Suggested text is in MAJOR-1; NIT-6 can go in with it.
- MINOR-3, MINOR-4 and MINOR-5 can be follow-ups by extending tech-debt rows 227 and 228, or answered "won't fix".
- Nothing in the web app or the en/fa locales has to change in this PR, and the agent warning needs no locale entry.
- A verify round needs only the reworded cell.

## Verify round (head `b74ad2b1b`)

- **Reviewer:** R6, verify round.
- **Head verified:** `b74ad2b1b`. The product change is in `1377ae788` and the docs change in `0e96796ae`. `gh pr view 241` shows headRefOid `b74ad2b1ba5cddd91f583278794d45dcbf2392e2`, state OPEN.
- **Mode:** read-only. No file was created, modified or deleted in the repository or worktree. I ran no gate, build, dev server or browser.
- **Scope:** my nine round-1 findings, plus the operator-visible text in the delta:
  - `docs/user/security/{global-blocking,auto-block}.md`
  - `docs/tech-debt.md`
  - the warnings in `apps/agent/internal/desired/global_blocking.go`
  - the reconcile log line in `apps/agent/internal/agent/rpc_autoblock.go`
- Nothing under `apps/web/**`, `packages/ui-kit/**`, `packages/schema/**` or the locales changed: `git diff --name-only 873f1d946 b74ad2b1b -- apps/web packages/ui-kit packages/schema` prints nothing.

### Per finding

**MAJOR-1 — fixed.**
- **The text:** `docs/user/security/global-blocking.md:27` now holds my suggested text word for word, plus two links. With the links stripped, a script compare against the round-1 suggestion returns `True`.
- **The links resolve.** GitHub-style slugs of the headings in `docs/user/interfaces/default-dataplane-nics.md`:
  - `## "Awaiting engine"` (line 68) gives `awaiting-engine`.
  - `## Releasing a NIC to the host, and reclaiming it` (line 75) gives `releasing-a-nic-to-the-host-and-reclaiming-it`.
  - Both sections say what the cell claims.
- **The table still parses.** The row still has 2 columns; the text contains no stray `|`.
- **The scope statement matches the code.**
  - The set is `sortedKeys(kept)`, so top-level `interfaces` rows only.
  - Bonds, loopbacks and the PPPoE logical interface (`interfaces.pppwan.pppoe`, `pppoe-delegation.md`) are top-level rows, so "every interface on the **Interfaces** page" is right.
  - Sub-interfaces, `tunnels.*`, WireGuard and IPsec are excluded, as the cell says.
- **The menu path is correct.** "**VPN › Tunnels**" matches `nav.json` (`groups.vpn` "VPN", `domains.tunnels` "Tunnels") and `nav.ts:24` (`tunnels: 'vpn'`).
- **The optional sentence was correctly left out.** That is the one I made conditional on verifying explicit sub-interface binding.

**MINOR-2 — fixed.**
- The cell now says "On both of these NICs the list drops only traffic to the appliance itself, and only when *Protect the box* is on."
- This matches `renderers/nftables/blocking.go:41`: `if (l.Enabled != nil && !l.GetEnabled()) || !l.GetProtectHost() || len(l.GetEntries()) == 0 {` skips the host part.

**MINOR-3 — follow-up accepted.**
- `docs/tech-debt.md:232`:
  - names `field.allInterfaces.help` in `apps/web/src/locales/{en,fa}/global-blocking.json`;
  - explains that the locale overrides the schema help, so all three texts must change together, with R3 owning the schema text;
  - has an owner and a due date.
- The suggested en and fa strings are byte-identical to round 1 (script check: `fa in tech-debt: True`, `en in tech-debt: True`).
- One remark for whoever does the follow-up. The doc now excludes "tunnel and VPN interfaces (… WireGuard, IPsec)", but my suggested help strings only say "tunnels" / «تونل‌ها». Add "VPN interfaces" / «اینترفیس‌های VPN» when the strings land so the UI and the doc say the same thing.

**MINOR-4 — partially fixed.**

What is fixed:
- The auto-block overlay is now reported at `/security/autoBlock` as "automatic blocking …" (`global_blocking.go:169-171`).
- The hidden list name `auto-block` is gone from these two lines.
- Unbound NICs are a warning (`:174`); released NICs are `infof` (`:177`).

What is not fixed, as an operator sees it in the Commit dialog:
- The API drops severity and puts every non-error issue into `warnings`. `apps/api/src/commit/validation.service.ts:39-43`: `issue()` copies `pointer`, `message` and `rule` only. Lines `152-154`: `.filter((i) => i.severity !== IssueSeverity.ISSUE_SEVERITY_ERROR)`.
- `apps/web/src/config/CommitDialog.tsx:149` turns the panel amber when `warnings.length > 0`.
- `grep ISSUE_SEVERITY_INFO apps/api/src apps/web/src` (non-test) finds nothing, so the API and web never treat info notes differently.
- So the released-NIC "note" looks exactly like a warning. While a NIC is released (the documented permanent state for a removed NIC), every validate still shows one amber line per all-interfaces list, plus one for auto-block while it has active blocks.
- When awaiting and released NICs coexist there are now **two** lines per list instead of one.
- The same flattening already applies to the existing `agent.nic-released` info (`projection.go:250`), so the pattern is consistent, but the noise part of the finding is not addressed.

Still MINOR and optional. Any one of these closes it:
- (a) Emit nothing for released NICs in the acl projection. `agent.nic-released` at `/interfaces/<nic>` already reports them on every validate, and the doc now states that released NICs are not covered.
- (b) Add a tech-debt row so the API keeps `severity` in `ProblemIssue` and the web shows info notes in a neutral colour. That would also quiet `agent.nic-released` and the cnat infos.
- (c) A one-line "won't fix".

Optional wording point: the page and menu call the feature "Auto-block" (`locales/en/auto-block.json:2-3`). "Auto-block selects all interfaces, …" would match the page better than "automatic blocking" — my own round-1 wording.

**MINOR-5 — follow-up accepted.**
- `docs/tech-debt.md:231` part (iii) covers both things I asked for:
  - mark or disable awaiting-engine and released rows in the `GlobalBlockingPage.tsx` *Interfaces* picker (the labels `interfaces:awaitingDataplane` / `releasedToHost` already exist in en and fa);
  - link the user doc to the explicit-reference paragraph that PR #238 adds.
- It has an owner and a due date.

**NIT-6 — fixed.**
- The cell uses "[Auto-block](auto-block.md)", "*awaiting engine*" (linked), "*released to host*" (linked) and "once the engine takes it over".
- It uses "engine" only and never mixes in "data plane".

**NIT-7 — fixed.**
- The pointer is now `lp+"/allInterfaces"` (`global_blocking.go:169`).
- The tests assert `W /acl/globalBlocking/lists/a/allInterfaces acl.global-blocking block list "a" selects all interfaces, but these physical NICs are not bound to the data plane yet and are skipped: ens192` (`TestGlobalBlockingTwoAllInterfacesLists`).
- The optional wording "not bound to the data plane yet" was kept, which round 1 allowed. It contains no "VPP", so `productWording` leaves it unchanged and it complies with D-155.

**NIT-8 — won't fix accepted** (one cell, as I allowed).

**NIT-9 — fixed.**
- `docs/user/security/auto-block.md:60` now reads: "Blocks are enforced on the same interfaces as a block list set to *All interfaces* (see … [Global blocking](global-blocking.md)) and, for traffic to the appliance itself, on every host interface."
- This is accurate:
  - the overlay list sets `AllInterfaces` and `ProtectHost` to true (`autoblock/runtime.go:123`);
  - the host part covers every host interface (`blocking.go:10-12`).

### New finding

**NIT-10 (optional) — `apps/agent/internal/desired/global_blocking.go:181`: the warning just below the new ones keeps the old pointer and list name.**
- **Code:** `x.s.Warnf(lp+"/interfaces", "acl.global-blocking", "block list %q selects no interface: it is not enforced in the data plane", name)`
- **Why it is new:** this PR makes the line reachable for all-interfaces lists. It fires when every row is awaiting the engine or released, for example on first boot before the engine restart, and `TestGlobalBlockingAllInterfacesNothingBound` asserts it.
- **What the operator then sees:**
  ```
  /security/autoBlock — automatic blocking selects all interfaces, but these physical NICs are not bound to the data plane yet and are skipped: ens192
  /acl/globalBlocking/lists/auto-block/interfaces — block list "auto-block" selects no interface: it is not enforced in the data plane
  ```
  - For auto-block, the hidden list name and the pointer to a field that does not exist — both removed by MINOR-4 and NIT-7 — come back in exactly the case where the warning matters most.
  - For a user list, "selects all interfaces" is followed by "selects no interface", pointing at the empty `interfaces` field.
- **Fix:** in the `GetAllInterfaces()` branch, reuse `what` and `at` for this warning, for example: "%s has no interface the engine has yet: it is not enforced in the data plane (traffic to the appliance is still dropped when *Protect the box* is on)". Leave out the Protect-the-box clause for auto-block, where it is always on.

### Notes for other reviewers (not graded)

**[other: R8/R1] `apps/agent/internal/agent/rpc_autoblock.go:130-146`**
- **Problem 1 — wrong prefix.** `logSkippedNICsLocked` collects every `acl.global-blocking` warning of the reconcile under the prefix `auto-block:`. That includes the user lists' skipped-NIC warnings and the `selects no interface` warning, which can come from a user list with an empty explicit `interfaces`.
- **Problem 2 — wrong all-clear message.** When those warnings clear, it logs `auto-block: every physical NIC is bound to the data plane again; automatic blocking covers all interfaces` (line 145). That is wrong if the cleared warning was a user list's "selects no interface".
- **Problem 3 — overstated scope.** "covers all interfaces" claims more than the doc now allows: sub-interfaces, tunnels and VPN interfaces are not covered.
- **Suggested fix:** filter on the auto-block pointer or message, and say "covers every interface the engine has".

### Evidence (commands run in `/root/NGFW/.claude/worktrees/funny-meninsky-c9ef19`, read-only)

```
$ git rev-parse HEAD; git log --oneline 873f1d946..b74ad2b1b
b74ad2b1ba5cddd91f583278794d45dcbf2392e2
b74ad2b1b docs(status): report and WIP after the verify round and the gates on 0e96796ae (PR #241)
0e96796ae docs: All interfaces scope per R6, tech-debt rows extended, verify-round hand-backs saved (PR #241)
1377ae788 fix(agent): report the NICs an all-interfaces list skips at the operator's field; log them in the auto-block reconcile

$ git diff --name-only 873f1d946 b74ad2b1b -- apps/web packages/ui-kit packages/schema
(empty)
$ git diff --name-only 873f1d946 b74ad2b1b -- docs/user
docs/user/security/auto-block.md
docs/user/security/global-blocking.md

$ git diff 873f1d946 b74ad2b1b -- docs/user/security/global-blocking.md
+| All interfaces / Interfaces | Where the list is enforced. *All interfaces* means every interface on the **Interfaces** page that the engine has, including interfaces added there later. It does **not** include VLAN sub-interfaces or tunnel and VPN interfaces (**VPN › Tunnels**, WireGuard, IPsec). A built-in NIC shown as *[awaiting engine](../interfaces/default-dataplane-nics.md#awaiting-engine)* is covered once the engine takes it over. A NIC *[released to host](../interfaces/default-dataplane-nics.md#releasing-a-nic-to-the-host-and-reclaiming-it)* is not covered. On both of these NICs the list drops only traffic to the appliance itself, and only when *Protect the box* is on. [Auto-block](auto-block.md) uses the same set. Naming an interface that does not exist is refused at commit. |

$ python3 -I (strip the two ../interfaces links from the cell, compare with the MAJOR-1 suggested text in this file)
identical to suggestion (links stripped): True
$ sed -n 27p docs/user/security/global-blocking.md | awk -F'|' '{print "columns:", NF-2}'
columns: 2

$ grep "^## " docs/user/interfaces/default-dataplane-nics.md | python3 -I (GitHub slug: lower, drop [^\w\- ], space→-)
'"Awaiting engine"' -> awaiting-engine
'Releasing a NIC to the host, and reclaiming it' -> releasing-a-nic-to-the-host-and-reclaiming-it

$ sed -n 41p apps/agent/internal/renderers/nftables/blocking.go
		if (l.Enabled != nil && !l.GetEnabled()) || !l.GetProtectHost() || len(l.GetEntries()) == 0 {

$ sed -n 7p apps/web/src/locales/en/nav.json; grep -n '"tunnels"' apps/web/src/locales/en/nav.json
    "vpn": "VPN",
30:    "tunnels": "Tunnels",

$ python3 -I (search the round-1 en/fa strings in docs/tech-debt.md)
fa in tech-debt: True
en in tech-debt: True
$ grep -n "field.allInterfaces.help\|(iii) the web picker" docs/tech-debt.md | cut -c1-40
231:- 2026-10-17 (PR #241 review R2 #2,
232:- 2026-10-17 (PR #241 review R2 #4,

$ grep -n 'what, at :=\|what, at = \|x.s.Warnf(at\|infof(x.s, at\|selects no interface' apps/agent/internal/desired/global_blocking.go
169:			what, at := fmt.Sprintf("block list %q", name), lp+"/allInterfaces"
171:				what, at = "automatic blocking", Ptr("security", "autoBlock")
174:				x.s.Warnf(at, "acl.global-blocking", "%s selects all interfaces, but these physical NICs are not bound to the data plane yet and are skipped: %s"
177:				infof(x.s, at, "acl.global-blocking", "%s selects all interfaces; the physical NICs released to the host are skipped: %s", ...
181:			x.s.Warnf(lp+"/interfaces", "acl.global-blocking", "block list %q selects no interface: it is not enforced in the data plane", name)

$ sed -n 152,154p apps/agent/internal/agent/projection.go; sed -n 1288-1292p apps/agent/internal/agent/service.go (excerpt)
func (p *projected) Infof(...) { p.issues = append(p.issues, issue{..., severity: ngfwv1.IssueSeverity_ISSUE_SEVERITY_INFO, ...}) }
	for _, is := range pj.issues {
		rep.Errors = append(rep.Errors, &ngfwv1.ValidationIssue{Pointer: is.pointer, Message: is.message, Severity: is.severity, Rule: is.rule})

$ sed -n 39,43p apps/api/src/commit/validation.service.ts; sed -n 152,154p apps/api/src/commit/validation.service.ts
function issue(i: ValidationIssue): ProblemIssue {
  const out: ProblemIssue = { pointer: i.pointer, message: i.message };
  if (i.rule) out.rule = i.rule;
  return out;
}
    const warnings = report.errors
      .filter((i) => i.severity !== IssueSeverity.ISSUE_SEVERITY_ERROR)
      .map(issue);
$ grep -rn "ISSUE_SEVERITY_INFO" apps/api/src apps/web/src | grep -v test
(empty)
$ sed -n 149p apps/web/src/config/CommitDialog.tsx
                <Alert severity={validate.data.notApplied.length > 0 || validate.data.warnings.length > 0 ? 'warning' : 'success'} data-testid="validate-ok">
$ sed -n 250p apps/agent/internal/agent/projection.go | cut -c1-120
			p.Infof(ptr("interfaces", name), "agent.nic-released", "physical NIC %q is released to the host (physical.owner = hos

$ sed -n 107,123p apps/agent/internal/autoblock/runtime.go (excerpt)
	if len(prefixes) == 0 { return out, nil }
	out.Acl.GlobalBlocking.Lists[ListName] = &ngfwv1.GlobalBlockingList{Enabled: proto.Bool(true), AllInterfaces: proto.Bool(true), Direction: proto.String("both"), ProtectHost: proto.Bool(true), Log: ...
$ grep -n "every physical NIC is bound" apps/agent/internal/agent/rpc_autoblock.go
145:		s.log.Info("auto-block: every physical NIC is bound to the data plane again; automatic blocking covers all interfaces")
$ grep -n '"nav"\|"title"' apps/web/src/locales/en/auto-block.json | head -2
2:  "nav": "Auto-block",
3:  "title": "Auto-block",

$ grep -n "Blocks are enforced on the same interfaces" docs/user/security/auto-block.md | cut -c1-60
60:Blocks are enforced on the same interfaces as a block lis

$ gh pr view 241 --repo mcoder1001-cyber/NGFW --json headRefOid,state
b74ad2b1ba5cddd91f583278794d45dcbf2392e2 / OPEN
```

### Verdict

**APPROVE** — 0 BLOCKER, 0 MAJOR open.
- **Fixed:** MAJOR-1, MINOR-2, NIT-6, NIT-7, NIT-9.
- **Follow-up rows accepted:** MINOR-3, MINOR-5.
- **Won't fix accepted:** NIT-8.
- **Partially fixed, optional:** MINOR-4.
- **New, optional:** NIT-10.

None of the open items has to change in this PR. The web app, ui-kit, schema and en/fa locales are untouched, so no screenshots and no new locale keys are needed.
