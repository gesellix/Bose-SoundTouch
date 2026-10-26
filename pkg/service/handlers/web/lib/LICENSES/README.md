# Vendored Frontend Dependencies

The JavaScript files in the parent directory are copied unmodified from the
exact npm packages pinned in the repository's `package-lock.json`.

This directory travels with the embedded web UI and contains each package's
upstream license. Its copy of `package-lock.json` records the exact versions,
npm tarball URLs, and integrity hashes used to generate the assets:

- `preact.module.js` and `preact-hooks.module.js`: `preact`
- `htm.module.js`: `htm`
- `es-module-shims.js`: `es-module-shims`

Run `make update-static-deps` to regenerate the assets, licenses, and package
metadata together.
