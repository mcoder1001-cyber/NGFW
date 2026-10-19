# Common UI development

NGFW consumes the shared Common UI package through the ignored local directory `.yalc/@amnafzar/common-ui-lib`. Component implementations and generic theme tokens belong to common-ui. The NGFW theme adapter adds appliance status colors and density to the shared theme; API/schema contracts remain in NGFW.

With Node 22 and the project's pinned pnpm installed, install the common-ui checkout's dependencies, then run:

```sh
bash tools/update-common-ui.sh /absolute/path/to/common-ui
pnpm exec turbo run build --filter=@ngfw/schema --filter=@ngfw/ui-kit --filter=@ngfw/api-client
NGFW_UI_PREVIEW=1 npm --prefix apps/web run dev
```

The refresh script builds all shared entry points, stages an npm package locally, and refreshes workspace dependencies. It does not publish to a registry. The preview is available at http://127.0.0.1:5173 with sample data and disabled device changes. Set `NGFW_WEB_PORT` to use a different local port. Omit `NGFW_UI_PREVIEW` when connecting to a real backend at `NGFW_HTTP_PORT` (default 3000).

Run the library build after shared source edits; NGFW consumes the compiled package. Keep shared React/MUI dependency versions aligned through the workspace overrides, and do not replace installed package symlinks by hand. Production bundles must exclude the offline preview transport.


To connect this local UI to an installed appliance, disable sample-data mode and set the upstream URL. For a private CA, supply a certificate file verified with the appliance administrator:

```bash
NODE_EXTRA_CA_CERTS=/absolute/path/to/trusted-appliance-ca.pem \
NGFW_API_TARGET=https://172.30.126.250 NGFW_UI_PREVIEW=0 NGFW_WEB_PORT=5174 \
pnpm --filter @ngfw/web dev
```

The proxy forwards API requests and WebSockets while retaining TLS certificate verification. The certificate must cover the target IP or DNS name. Use a trusted DNS name instead of the IP when required by the certificate. Without a private CA, omit `NODE_EXTRA_CA_CERTS`. Keep this development server bound to loopback.

If the administrator-approved appliance certificate covers a different TLS name than its connection IP, set `NGFW_API_TLS_SERVERNAME` to that certificate name. For the appliance at `172.30.126.250`, its current certificate covers `localhost`; use `NGFW_API_TLS_SERVERNAME=localhost` together with the trusted certificate file. This sets TLS SNI/hostname verification without disabling certificate validation. Trust is scoped to this Node process, not installed system-wide.
