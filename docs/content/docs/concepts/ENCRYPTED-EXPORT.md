---
title: "Encrypting Sensitive Data Exports with SSH/age or GPG"
---
## Problem

Allow users of our software to export potentially sensitive data, encrypt it locally, and send it to us. We decrypt on our side. Goal: no key exchange, minimal user friction.

Two viable options are documented here: **Option A — `age`** (simpler, modern) and **Option B — GPG** (widely known, interoperable with existing tooling). Both support fetching a recipient key from GitHub so users don't need to hand us anything.

## Key Findings

### GPG via SSH keys: not possible

- GitHub's `https://github.com/<user>.keys` serves SSH public keys, not GPG keys.
- SSH and GPG/OpenPGP use different formats, capability flags, and key material (auth vs. encrypt/sign/certify).
- Ed25519 SSH keys can't be directly reused for GPG encryption (encryption requires X25519/ECDH).

### GPG via published GPG keys: possible

- GitHub exposes GPG public keys at `https://github.com/<user>.gpg` — these are real OpenPGP armored keys, not SSH keys.
- Any key the user has uploaded to their GitHub account (or a keyserver like `keys.openpgp.org`) can be used directly for encryption.
- Decryption requires the matching GPG private key on our side.
- The `github.com/ProtonMail/go-crypto/openpgp` package is the actively maintained Go OpenPGP implementation (`golang.org/x/crypto/openpgp` is deprecated and points to it).

### `age` with SSH or native keys: possible (simpler)

- [`age`](https://github.com/FiloSottile/age) natively supports `ssh-rsa` and `ssh-ed25519` public keys as recipients, fetched from `https://github.com/<user>.keys`.
- Also supports its own `age1...` native keys (`age-keygen`), which are X25519-based.
- Written in Go; library is `filippo.io/age` + `filippo.io/age/agessh`.
- Output is age format (not GPG-interoperable). Decrypt with `age -i key file.age` or the Go library.

---

## Option A: `age`

### Architecture

1. Generate a dedicated age key: `age-keygen -o decrypt.key` (produces `age1...` public key).
2. Embed the public key as a constant in the binary — users need no setup.
3. Optionally accept a GitHub username and fetch their SSH keys as recipients so the user can verify independently.
4. Store the private key securely (secret manager, HSM, offline backup).

### Workflow

**User side:**
```
soundtouch-cli export --encrypt
# or: soundtouch-cli export --encrypt-for github:gesellix
```
The CLI encrypts the export using the embedded key (or fetched SSH keys) and writes `export.age`.
The user sends that file through any channel.

**Maintainer side:**
```bash
age -d -i decrypt.key -o export.tar.gz export.age
# or with an SSH private key:
age -d -i ~/.ssh/id_ed25519 -o export.tar.gz export.age
```

### Go Implementation

#### Encrypt with embedded key

```go
import (
    "io"
    "os"

    "filippo.io/age"
)

const recipientKey = "age1..." // embedded public key

func exportEncrypted(plaintext io.Reader, outPath string) error {
    recipient, err := age.ParseX25519Recipient(recipientKey)
    if err != nil {
        return err
    }
    out, err := os.Create(outPath)
    if err != nil {
        return err
    }
    defer out.Close()

    w, err := age.Encrypt(out, recipient)
    if err != nil {
        return err
    }
    defer w.Close()

    _, err = io.Copy(w, plaintext)
    return err
}
```

#### Encrypt to a GitHub user's SSH keys (alternative / verification path)

```go
import (
    "bufio"
    "io"
    "log"
    "net/http"
    "strings"

    "filippo.io/age"
    "filippo.io/age/agessh"
)

func recipientsFromGitHub(user string) ([]age.Recipient, error) {
    resp, err := http.Get("https://github.com/" + user + ".keys")
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    var recipients []age.Recipient
    scanner := bufio.NewScanner(resp.Body)
    for scanner.Scan() {
        line := strings.TrimSpace(scanner.Text())
        if line == "" {
            continue
        }
        r, err := agessh.ParseRecipient(line)
        if err != nil {
            log.Printf("skipping unsupported key: %v", err)
            continue
        }
        recipients = append(recipients, r)
    }
    return recipients, nil
}
```

#### Decrypt (maintainer side)

With a native age key:

```go
package main

import "filippo.io/age"

func decryptAge(encryptedReader io.Reader, privateKeyString string) (io.Reader, error) {
    identity, err := age.ParseX25519Identity(privateKeyString)
    if err != nil {
        return nil, err
    }
    return age.Decrypt(encryptedReader, identity)
}
```

With an SSH private key:

```go
package main

import (
    "io"
    "os"

    "filippo.io/age"
    "filippo.io/age/agessh"
)

func decryptAgeSSH(encryptedReader io.Reader, sshKeyPath string) (io.Reader, error) {
    pemBytes, err := os.ReadFile(sshKeyPath)
    if err != nil {
        return nil, err
    }
    identity, err := agessh.ParseIdentity(pemBytes)
    if err != nil {
        return nil, err
    }
    return age.Decrypt(encryptedReader, identity)
}
```

---

## Option B: GPG (OpenPGP)

### Architecture

1. Generate a dedicated GPG encryption subkey: `gpg --full-gen-key` (choose RSA or Ed25519+X25519).
2. Export and publish the public key, or embed the armored block directly in the binary.
3. Optionally fetch the user's GPG key from `https://github.com/<user>.gpg` or `keys.openpgp.org` so they can confirm the recipient.
4. Store the private key securely. Decryption is `gpg --decrypt export.gpg`.

### Workflow

**User side:**
```
soundtouch-cli export --encrypt-gpg
# or: soundtouch-cli export --encrypt-gpg-for github:gesellix
```
The CLI encrypts the export as an OpenPGP binary message and writes `export.gpg`.
The user sends that file through any channel.

**Maintainer side:**
```bash
# GPG must have the matching private key in its keyring
gpg --decrypt -o export.tar.gz export.gpg

# Or with a specific key file (without importing into the keyring):
gpg --no-default-keyring --secret-keyring ./decrypt.gpg \
    --decrypt -o export.tar.gz export.gpg
```

### Go Implementation

Uses `github.com/ProtonMail/go-crypto/openpgp` (the maintained successor to the deprecated `golang.org/x/crypto/openpgp`; API is compatible).

#### Fetch public key from GitHub

```go
package main

import (
    "io"
    "net/http"

    "github.com/ProtonMail/go-crypto/openpgp"
    "github.com/ProtonMail/go-crypto/openpgp/armor"
)

func gpgKeyFromGitHub(user string) (openpgp.EntityList, error) {
    resp, err := http.Get("https://github.com/" + user + ".gpg")
    if err != nil {
        return nil, err
    }
    defer resp.Body.Close()

    block, err := armor.Decode(resp.Body)
    if err != nil {
        return nil, err
    }
    return openpgp.ReadKeyRing(block.Body)
}
```

#### Encrypt with embedded or fetched public key

```go
package main

import (
    "io"
    "os"
    "strings"

    "github.com/ProtonMail/go-crypto/openpgp"
    "github.com/ProtonMail/go-crypto/openpgp/armor"
)

const embeddedPublicKey = `-----BEGIN PGP PUBLIC KEY BLOCK-----
...
-----END PGP PUBLIC KEY BLOCK-----`

func exportEncryptedGPG(plaintext io.Reader, outPath string) error {
    block, err := armor.Decode(strings.NewReader(embeddedPublicKey))
    if err != nil {
        return err
    }
    recipients, err := openpgp.ReadKeyRing(block.Body)
    if err != nil {
        return err
    }

    out, err := os.Create(outPath)
    if err != nil {
        return err
    }
    defer out.Close()

    // Encrypt directly (binary, no ASCII armor — smaller output)
    w, err := openpgp.Encrypt(out, recipients, nil, nil, nil)
    if err != nil {
        return err
    }
    defer w.Close()

    _, err = io.Copy(w, plaintext)
    return err
}
```

To produce ASCII-armored output (easier to paste into emails/issues), wrap `out` with `armor.Encode`:

```go
package main

import (
    "io"
    "os"
    "strings"

    "github.com/ProtonMail/go-crypto/openpgp"
    "github.com/ProtonMail/go-crypto/openpgp/armor"
)

func exportEncryptedGPGArmored(plaintext io.Reader, outPath, embeddedPublicKey string) error {
    block, err := armor.Decode(strings.NewReader(embeddedPublicKey))
    if err != nil {
        return err
    }
    recipients, err := openpgp.ReadKeyRing(block.Body)
    if err != nil {
        return err
    }

    out, err := os.Create(outPath)
    if err != nil {
        return err
    }
    defer out.Close()

    armorWriter, err := armor.Encode(out, "PGP MESSAGE", nil)
    if err != nil {
        return err
    }
    defer armorWriter.Close()

    w, err := openpgp.Encrypt(armorWriter, recipients, nil, nil, nil)
    if err != nil {
        return err
    }
    defer w.Close()

    _, err = io.Copy(w, plaintext)
    return err
}
```

#### Decrypt (maintainer side)

```go
package main

import (
    "io"
    "os"
    "strings"

    "github.com/ProtonMail/go-crypto/openpgp"
    "github.com/ProtonMail/go-crypto/openpgp/armor"
)

func decryptGPG(encryptedPath, privateKeyArmored string) (io.ReadCloser, error) {
    block, err := armor.Decode(strings.NewReader(privateKeyArmored))
    if err != nil {
        return nil, err
    }
    keyring, err := openpgp.ReadKeyRing(block.Body)
    if err != nil {
        return nil, err
    }

    f, err := os.Open(encryptedPath)
    if err != nil {
        return nil, err
    }

    msg, err := openpgp.ReadMessage(f, keyring, nil, nil)
    if err != nil {
        return nil, err
    }
    return msg.UnverifiedBody, nil
}
```

---

## Comparison and Recommendation

| Criterion                           | `age`                        | GPG                                           |
|-------------------------------------|------------------------------|-----------------------------------------------|
| User familiarity                    | Low (newer tool)             | High (widely known)                           |
| User already has a key to use       | Maybe (SSH on GitHub)        | Often (GPG on GitHub/keyserver)               |
| Go library quality                  | Excellent (`filippo.io/age`) | Good (`ProtonMail/go-crypto`)                 |
| Output interoperability             | age format only              | Standard OpenPGP — any GPG client can decrypt |
| CLI decrypt UX (maintainer)         | `age -d -i key file.age`     | `gpg --decrypt file.gpg`                      |
| Key embedding in binary             | Native `age1...` string      | Armored PEM block                             |
| Key rotation story                  | `age-keygen`, swap constant  | Standard GPG subkey rotation                  |
| Anonymous recipients                | Yes (native age keys)        | No (key ID visible)                           |
| Streaming large exports             | Yes                          | Yes                                           |

**Recommendation:** use `age` with an embedded native key for the primary path — simpler dependency, cleaner API, no GPG keyring management needed. Add GPG as an opt-in flag (`--gpg` or `--encrypt-gpg-for github:<user>`) for users who already manage GPG keys and want their own tooling to verify or store the export.

---

## Binary Size

Measured on macOS arm64, stripped binaries (`-ldflags="-s -w"`).

### Standalone cost (no shared deps)

| Option                              | Binary size | Added vs no-crypto baseline |
|-------------------------------------|-------------|-----------------------------|
| Baseline (no crypto)                | 1.44 MB     | —                           |
| `age` native key only (no `agessh`) | 2.40 MB     | +0.96 MB                    |
| `age` + `agessh` (SSH recipients)   | 3.06 MB     | +1.63 MB                    |
| GPG (`ProtonMail/go-crypto`)        | 3.59 MB     | +2.15 MB                    |

### Marginal cost for this project

This project already imports `golang.org/x/crypto/ssh`, which `agessh` depends on. That ~660 KB is shared and doesn't count against `age`. Against the ~12.9 MB `soundtouch-service` binary:

| Option           | Marginal cost | % of service binary |
|------------------|---------------|---------------------|
| `age` + `agessh` | +0.64 MB      | ~5%                 |
| GPG              | +1.30 MB      | ~10%                |

### Why GPG is larger

`age` pulls in only what it needs: `chacha20poly1305`, `hkdf`, `edwards25519`, and `filippo.io/hpke` (post-quantum). `ProtonMail/go-crypto` must ship the full OpenPGP spec: `cloudflare/circl` (Ed448, X448, Goldilocks curves), `bitcurves`, `brainpool`, `EAX`, `OCB`, `CAST5`, `BLAKE2b`, `SHA3`, `Argon2`, S2K key derivation, and zlib/bzip2 compression. Go's dead-code elimination works at the function level but can't remove entire algorithm families wired through a shared codec dispatch.

---

## Gotchas & Risks

| Concern                                          | Mitigation                                                                          |
|--------------------------------------------------|-------------------------------------------------------------------------------------|
| MITM / GitHub account compromise swaps the key   | Pin expected key fingerprint(s); prefer embedded key over runtime fetch             |
| SSH key rotation breaks old `agessh` decryption  | Use a dedicated long-lived age key, not the user's SSH key, as primary              |
| ECDSA SSH keys not supported by `agessh`         | Handle "no usable key" gracefully; warn and fall back                               |
| `agessh` recipients leak a 32-bit key ID         | Accept, or use native age keys for full anonymity                                   |
| GPG key expiry breaks encryption                 | Use a non-expiring encryption subkey, or check and warn before encrypting           |
| GPG key without encryption capability            | Filter `EntityList` to keys with `EncryptCommunications` flag set                   |
| Encryption ≠ authentication                      | Authenticate via the send channel, or require a detached signature                  |
| Sensitive data leaks via logs or memory dumps    | Audit all egress paths; the export must be the only cleartext exit                  |
| Large exports                                    | Both `age` and `openpgp.Encrypt` stream — never buffer the whole payload            |

---

## Dependencies

```bash
# age
go get filippo.io/age
go get filippo.io/age/agessh   # only if supporting SSH recipients

# GPG
go get github.com/ProtonMail/go-crypto/openpgp
```

## References

- `age` project: https://github.com/FiloSottile/age
- `age` Go docs: https://pkg.go.dev/filippo.io/age
- `agessh` docs: https://pkg.go.dev/filippo.io/age/agessh
- ProtonMail go-crypto: https://github.com/ProtonMail/go-crypto
- OpenPGP Go docs: https://pkg.go.dev/github.com/ProtonMail/go-crypto/openpgp
- GitHub GPG key endpoint: `https://github.com/<user>.gpg`
- OpenPGP keyserver: https://keys.openpgp.org
