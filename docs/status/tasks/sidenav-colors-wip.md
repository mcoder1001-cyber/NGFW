# Sidenav colors WIP

Branch codex/ngfw-sidenav-colors. Parent local4c38c9a18; remote parent3f99bdeeb8175897cd1718e586b7796dc009c36c.
Chosen colors: dashboard/routing blue60A5FA, interfaces cyan22D3EE, objects/system slate94A3B8, firewall amberFBBF24, VPN violetA78BFA, services emerald34D399, tools/dev orangeFB923C.
Stable module-scope wrappers forward icon props/accessibility/style; only color changes. Shared dark rail in both themes; selected row highlight unchanged. Independent read-only review approved collapsed/expanded rendering. Contrast checked against dark rail and representative selected surface (minimum4.89:1). Web lint and tools/ci.sh check pass. Production build (including TypeScript, bundle budget, no-dev-routes) passed. No new tests for this low-impact presentation change.
Next: confirm build budget/isolation pass, publish commit, deploy new versioned static release on444, verify served asset hashes.
