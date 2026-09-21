# `@wailsio/runtime/cancellable` export reproduction

This reproduction installs an unmodified `@wailsio/runtime@3.0.0-beta.24` in a temporary directory and verifies that:

1. `dist/cancellable.js` and `types/cancellable.d.ts` ship in the package.
2. The root entry exports the cancellable API.
3. The standalone subpath fails with `ERR_PACKAGE_PATH_NOT_EXPORTED`.
4. Adding the missing export makes the standalone subpath resolve.

Run it with Node.js and pnpm available:

```sh
./repro.sh
```

The temporary installation is removed when the script exits. Set `WAILS_RUNTIME_VERSION` to test another published version.
