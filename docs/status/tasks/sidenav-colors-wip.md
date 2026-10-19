# Sidenav colors WIP

Branch codex/ngfw-sidenav-colors. Parent local4c38c9a18; remote parent3f99bdeeb8175897cd1718e586b7796dc009c36c.
Chosen colors: dashboard/routing blue60A5FA, interfaces cyan22D3EE, objects/system slate94A3B8, firewall amberFBBF24, VPN violetA78BFA, services emerald34D399, tools/dev orangeFB923C.
Stable module-scope wrappers forward icon props/accessibility/style; only color changes. Shared dark rail in both themes; selected row highlight unchanged. Independent read-only review approved collapsed/expanded rendering. Contrast checked against dark rail and representative selected surface (minimum4.89:1). Web lint and tools/ci.sh check pass. Production build (including TypeScript, bundle budget, no-dev-routes) passed. No new tests for this low-impact presentation change.
Next: confirm build budget/isolation pass, publish commit, deploy new versioned static release on444, verify served asset hashes.

Deployed /usr/share/ngfw/web-releases/ui-d5015f6de on appliance444. nginx validation/reload passed; external verified TLS fetch confirms HTML/JS match local build and all7 colors present. API health200. Entry SHA256102201733e8e8f802122bfc2af9019d40f42e01c3551f5b1a95cd732cd25028b. Rollback config /etc/ngfw/nginx.conf.before-icon-colors-20261019150041; previous release retained. Remote code checkpoint b987b4e02b2afa277cf5a92c9926e1f1d7528923 verified publication success. Production budget599.8kB passes; no main merge/full quick CI. Browser visual inspection not performed.
