# Mounted public PKI inventory review

Immutable tree `e618a11885d9a9b07e02308348cd8acf60b750b0`.
Verdict: APPROVE WITH LIMITS for mounting the previously reviewed public panel.

PKI is appended to vpnTabs, preserving the existing first/default tab. Its stable
`pki` identifier selects the panel via `/vpn?tab=pki`. VpnPage uses this same tab
registry and DomainTabsPage links the selected tab to its labelled tabpanel.
The existing VPN navigation domain is already built.

English/Persian JSON resources register centrally under `pkiInventory` and the
namespace is included in NAMESPACES. Thus the cross-namespace tab label is available
before the lazy panel loads; panel import no longer mutates global resources.
Both locale files retain all public panel labels, including Expired. The prior
precise expiry/status priority and corrected `/api/v1/state/pki` GET remain intact.
The bounded ROW_SCOPE constant retains literal row-header semantics.

Two language integration regressions render the actual shared tabs component with
vpnTabs and the memory-router URL, assert selected localized tab, await public
inventory content and restrict observed requests to the state GET. They exercise
mounted routing rather than only standalone panel rendering.

Read-only source review, no test launches or product edits. Coordinator owns type/
lint/tests and final integration; runtime browser/RTL/screen-reader acceptance is
not claimed by this report. No actionable new finding within this bounded scope.
