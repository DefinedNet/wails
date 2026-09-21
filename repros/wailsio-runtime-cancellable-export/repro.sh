#!/usr/bin/env bash
# Reproduces the missing `./cancellable` subpath export in @wailsio/runtime.
#
# dist/cancellable.js and types/cancellable.d.ts both ship in the published
# tarball (see the `files` field in package.json: "./dist", "./types"), but
# the package's `exports` map only lists "." and "./plugins/*". Node's ESM
# resolver treats any `exports` map as an exhaustive allowlist, so every
# path not listed --- including one that points at a file physically
# present on disk --- resolves to ERR_PACKAGE_PATH_NOT_EXPORTED. This script
# demonstrates the failure against a pristine install, then confirms adding
# one `exports` entry (in the same style already used by "." and
# "./plugins/*") is sufficient to resolve it.
set -euo pipefail

VERSION="${WAILS_RUNTIME_VERSION:-3.0.0-beta.24}"
WORKDIR="$(mktemp -d -t wails-cancellable-repro)"
trap 'rm -rf "$WORKDIR"' EXIT

echo "== Installing pristine @wailsio/runtime@${VERSION} into ${WORKDIR} =="
cd "$WORKDIR"
cat > package.json <<PKGJSON
{
  "name": "wails-cancellable-repro",
  "private": true,
  "type": "module"
}
PKGJSON
pnpm add "@wailsio/runtime@${VERSION}" >/dev/null

PKG_JSON="node_modules/@wailsio/runtime/package.json"
DIST_FILE="node_modules/@wailsio/runtime/dist/cancellable.js"
TYPES_FILE="node_modules/@wailsio/runtime/types/cancellable.d.ts"

echo
echo "== Step 1: dist/cancellable.js and types/cancellable.d.ts ship in the published package =="
if [[ ! -f "$DIST_FILE" ]]; then
  echo "FAIL: $DIST_FILE is missing; this reproduction targets a different package layout." >&2
  exit 1
fi
if [[ ! -f "$TYPES_FILE" ]]; then
  echo "FAIL: $TYPES_FILE is missing; this reproduction targets a different package layout." >&2
  exit 1
fi
echo "PASS: $DIST_FILE exists ($(wc -c < "$DIST_FILE") bytes)"
echo "PASS: $TYPES_FILE exists ($(wc -c < "$TYPES_FILE") bytes)"

echo
echo "== Step 2: root entry point resolves and already exports the cancellable API =="
node -e "
import('@wailsio/runtime').then(m => {
  const got = ['CancelError', 'CancellablePromise', 'CancelledRejectionError'].filter(k => k in m);
  if (got.length !== 3) {
    console.error('FAIL: root entry point does not export the cancellable API', got);
    process.exit(1);
  }
  console.log('PASS: import(\"@wailsio/runtime\") resolved and exports', got);
});
"

echo
echo "== Step 3: ./cancellable subpath is rejected by the exports map =="
set +e
node -e "
import('@wailsio/runtime/cancellable')
  .then(() => { console.error('FAIL: import unexpectedly succeeded'); process.exit(1); })
  .catch(err => {
    if (err.code !== 'ERR_PACKAGE_PATH_NOT_EXPORTED') {
      console.error('FAIL: unexpected error code', err.code, err);
      process.exit(1);
    }
    console.log('PASS: got the expected ERR_PACKAGE_PATH_NOT_EXPORTED');
    console.log(err.stack);
  });
"
STEP3_EXIT=$?
set -e
if [[ $STEP3_EXIT -ne 0 ]]; then
  exit $STEP3_EXIT
fi

echo
echo "== Step 4: adding one exports entry is sufficient to fix it =="
node -e "
const fs = require('fs');
const pkg = JSON.parse(fs.readFileSync('$PKG_JSON', 'utf8'));
pkg.exports['./cancellable'] = { types: './types/cancellable.d.ts', default: './dist/cancellable.js' };
fs.writeFileSync('$PKG_JSON', JSON.stringify(pkg, null, 2));
"
node -e "
import('@wailsio/runtime/cancellable').then(m => {
  const expected = ['CancelError', 'CancellablePromise', 'CancelledRejectionError'];
  const got = Object.keys(m).sort();
  if (JSON.stringify(got) !== JSON.stringify(expected.slice().sort())) {
    console.error('FAIL: unexpected export set', got);
    process.exit(1);
  }
  console.log('PASS: import(\"@wailsio/runtime/cancellable\") now resolves, exports:', got);
});
"

echo
echo "All steps passed. The fix is adding this entry to package.json's exports map,"
echo "matching the style already used by the \".\" and \"./plugins/*\" entries:"
cat <<'EOF'
    "./cancellable": {
      "types": "./types/cancellable.d.ts",
      "default": "./dist/cancellable.js"
    }
EOF
