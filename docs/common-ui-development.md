# Common UI development

Shared source lives in `/Users/mohammad/webngf/common-ui`. NGFW does not maintain a second source copy. Its applications consume the original library’s compiled package through yalc (`.yalc/@amnafzar/common-ui-lib`). The generated package is ignored by Git.

Run `bash tools/update-common-ui.sh /Users/mohammad/webngf/common-ui` after changing shared components. This builds both the existing root entry point and the new scoped entries, publishes to the local yalc store, refreshes NGFW, and installs dependencies. A fresh checkout needs Node, npm, pnpm, and yalc available, plus that original library checkout.

NGFW imports shared controls from `/forms`, tables from `/grid`, layout components from `/primitives`, icons from `/icons`, and theme helpers from `/styles`. MUI remains an internal library dependency. NGFW-specific dependency overrides retain its existing MUI 7 and React versions; the original library’s regular dependencies and legacy root API are retained for existing consumers.

No registry publication or Common UI commit is performed by this script. Production builds do not include the offline preview transport.
