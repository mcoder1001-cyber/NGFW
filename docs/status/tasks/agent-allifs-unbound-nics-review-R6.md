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
