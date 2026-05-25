---
title: "Encrypted Diagnostic Export"
sidebar:
  exclude: true
---
AfterTouch can produce an encrypted diagnostic report that users can download and
send to the project maintainer without exposing sensitive data to third parties.
The report is encrypted with an SSH public key using
[`age`](https://github.com/FiloSottile/age); only the holder of the matching
private key can read it.

---

## What the report contains

The encrypted `.age` file decrypts to a `.tar.gz` archive with:

- `diagnostic.json` — structured summary:
  - Service version and build info
  - Full health-check results (same data as the Health tab)
  - Per-device state: sources (IDs, names, SourceKeyTypes), presets (slot, name,
    Source, SourceID, location), device product code, firmware version, IP, name
- `datastore/accounts/{id}/devices/{id}/*.xml` — raw XML files verbatim from
  the sender's datastore (`Presets.xml`, `Sources.xml`, `Recents.xml`, …)

Having both the structured JSON and the raw XML lets you compare what the
service serves via HTTP against what is actually stored on disk.

**What is excluded from the JSON:** authentication tokens, credentials, OAuth
secrets, Spotify refresh tokens. The raw XML files are included as-is.

---

## Maintainer setup (one-time)

> This section is for the project maintainer only.
> Users never need to touch keys.

### 1. Generate the key pair

```bash
bash scripts/setup-diagnostic-key.sh
```

This creates:
- `keys/private/diagnostic` — SSH ed25519 private key (**gitignored**, never commit)
- `keys/private/diagnostic.pub` — copy for reference (**gitignored**)
- `keys/public/diagnostic.pub` — public key committed to the repo

### 2. Add the public key to GitHub

Go to <https://github.com/settings/ssh/new> and paste the contents of
`keys/public/diagnostic.pub`. This makes the key visible at
<https://github.com/gesellix.keys> so users can independently verify that the
key embedded in the binary matches a key actually controlled by the maintainer.

### 3. Embed the public key in the binary

Open `pkg/service/export/encrypt.go` and update the `DiagnosticPublicKey`
constant to match the new public key:

```go
const DiagnosticPublicKey = "ssh-ed25519 AAAA... aftertouch-diagnostic@gesellix"
```

### 4. Commit

```bash
git add keys/public/diagnostic.pub pkg/service/export/encrypt.go
git commit -m "keys: add diagnostic SSH public key"
```

`keys/private/` is `.gitignore`d — the private key will not be committed.

### 5. Back up the private key

The private key is **not** stored in git. Keep a copy in a secure location
(password manager, encrypted USB drive, etc.). If it is lost, a new key pair
must be generated and the constant in `encrypt.go` updated.

---

## Verifying the embedded key (users)

Users who want to confirm that the key embedded in their running binary matches
the maintainer's GitHub SSH keys can run:

```bash
# Compare the raw key text — both should show the same line:
curl -s https://github.com/gesellix.keys
cat keys/public/diagnostic.pub
```

The key should appear verbatim in both outputs.

---

## Decrypting a received report (maintainer)

When a user sends you an `aftertouch-diagnostic-*.age` file, use the helper
script (no extra tools needed — only Go and the private key). Run from the
repository root directory:

```bash
# Decrypt and extract in one step:
go run scripts/decrypt-diagnostic.go aftertouch-diagnostic-<timestamp>.age | tar xz

# Or decrypt to a .tar.gz first, then inspect:
go run scripts/decrypt-diagnostic.go aftertouch-diagnostic-<timestamp>.age > report.tar.gz
tar xzf report.tar.gz
# → diagnostic.json
# → datastore/accounts/{id}/devices/{id}/Presets.xml  (and Sources.xml, Recents.xml, …)
```

The script uses only the `filippo.io/age` Go module — no separate `age` CLI
installation required.

---

## User workflow

1. Open the AfterTouch admin UI and go to the **Health** tab.
2. Click **Download diagnostic report**.
3. The browser downloads `aftertouch-diagnostic-<timestamp>.age`.
4. Attach the file to the GitHub issue or send it via a direct channel.

The file is opaque binary — the user cannot read it. All they see is that the
report was generated and downloaded.

---

## Key rotation

If the private key is compromised or lost:

1. Run `scripts/setup-diagnostic-key.sh` (delete the old `keys/private/diagnostic` first).
2. Add the new public key to GitHub and remove the old one.
3. Update `DiagnosticPublicKey` in `encrypt.go`.
4. Commit and tag a new release.

Old reports encrypted with the previous key cannot be decrypted with the new key.
