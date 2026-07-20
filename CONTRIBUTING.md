# Contributing to smuler-registry

The smuler registry is a curated index of community plugins and themes for [smuler](https://github.com/lum1n/smuler). Every entry must be signed, integrity-checked, and approved by a maintainer before merge.

## Before you submit

1. **Author your plugin or theme** using the smuler CLI (`smuler plugin init` or `smuler theme init`).
2. **Validate and sign** your package:
   ```bash
   smuler plugin validate
   smuler plugin sign
   smuler plugin package
   smuler plugin publish --check
   ```
3. **Create a GitHub release** with the generated archive (`.tar.gz`).
4. **Update the registry entry** — set the `url` field in `.smuler/registry-entry.json` to your release asset URL.
5. **Open a pull request** adding your entry.

## Submission options

### Option A: Submission file (recommended)

Copy your registry entry JSON to:

```
submissions/plugins/<id>-<version>.json
submissions/themes/<id>-<version>.json
```

Maintainers merge approved submissions into `registry.json` during review. When a PR that touches `submissions/**` or `registry.json` is merged into `main`, the **Publish Registry Release** workflow copies any files under `submissions/plugins/` and `submissions/themes/` into `registry.json`, removes the submission files, validates the index, pushes the update if needed, and publishes the `registry.json` GitHub Release asset that Smuler downloads. Maintainers can also run that workflow manually from the Actions tab (**Run workflow**).

### Option B: Direct index edit

Add your entry directly to the `plugins` or `themes` array in `registry.json`.

## Required fields

Every community entry must include:

| Field | Description |
|-------|-------------|
| `id` | Stable plugin/theme identifier |
| `name` | Display name |
| `version` | Semver version |
| `description` | Short summary |
| `url` | HTTPS download URL for signed archive |
| `publicKey` | Ed25519 public key (base64) used to sign the package |
| `sha256` | SHA256 hex digest of the archive file |

Optional: `author`, `category`, `homepage`, `minimumHostVersion`, `capabilities` (plugins), `previewUrl` (themes).

## CI checks

Pull requests run automated validation:

- JSON Schema compliance
- Archive download from allowlisted hosts (`github.com`, `objects.githubusercontent.com`)
- SHA256 integrity match
- Ed25519 signature verification (manifest + binary/bundle)
- Public key binding between registry entry and signed manifest
- No duplicate `(id, version)` pairs

All checks must pass. A maintainer must still approve the PR.

## Updating an existing entry

Bump `version`, publish a new release, and submit a new entry. Do not overwrite existing version entries.

## Revocation

To remove a malicious or deprecated entry, open a PR deleting it from `registry.json`. Maintainers will merge promptly for security issues.

## Questions

Open a discussion or issue in this repository.
