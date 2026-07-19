# Maintainer review checklist

Use this checklist when reviewing registry submissions. CI validates signatures and integrity; humans validate trust and intent.

## Automated (CI must pass)

- [ ] JSON Schema valid
- [ ] Archive downloads from allowlisted HTTPS host
- [ ] SHA256 matches downloaded archive
- [ ] Manifest + binary/bundle signatures verify (Ed25519)
- [ ] Registry `publicKey` matches manifest `publicKey`
- [ ] Semver version; no duplicate `(id, version)`

## Human review

### Author identity

- [ ] Author field or GitHub profile is identifiable
- [ ] First-time contributor: verify intent via PR description or linked homepage/repo

### Permissions and scope

- [ ] Declared permissions match plugin/theme behavior (read manifest)
- [ ] Network access is justified by stated capabilities
- [ ] No excessive filesystem or credential scope

### Package contents

- [ ] Plugin binary is not obfuscated or packed without explanation
- [ ] No embedded secrets, API keys, or hardcoded tokens in manifest or archive
- [ ] Description accurately reflects functionality

### Version policy

- [ ] Version bump is appropriate (not re-publishing same artifact under new version without reason)
- [ ] `minimumHostVersion` is reasonable if set

### Red flags (reject or request changes)

- Unsigned or mismatched signatures (CI should catch)
- Download URL points outside allowlisted hosts
- Typosquatting on popular plugin IDs
- Requests broad permissions without clear need
- Binary differs significantly from source repo without explanation

## Merge process

1. Confirm CI green
2. Complete checklist above
3. Merge PR
4. If using submission files, copy approved entry into `registry.json` and remove submission file
5. Release workflow publishes updated `registry.json` asset

## Revocation

For reported malware or policy violations:

1. Remove entry from `registry.json` immediately
2. Open issue documenting reason (without amplifying exploit details)
3. Consider blocking publisher public key in future index metadata
