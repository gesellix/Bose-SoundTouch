---
title: "Placeholder values for examples"
---
This repo is public. Documentation, READMEs, example configs, and test
fixtures must never carry real LAN IPs, real device MACs, real Bose
account IDs, or personal device names from any maintainer or
contributor.

This file is the **canonical mapping table** for the placeholders we
use across the codebase. Use these values in new examples and tests.

## Placeholder mapping

| Concept                | Placeholder                                                            |
|------------------------|------------------------------------------------------------------------|
| Example IP (primary)   | `192.0.2.10`                                                           |
| Example IP (secondary) | `192.0.2.11`                                                           |
| Example IP (third)     | `192.0.2.12`                                                           |
| Network / CIDR         | `192.0.2.0/24`                                                         |
| External / non-LAN IP  | `198.51.100.10` or `203.0.113.10`                                      |
| Gateway IP             | `192.0.2.1`                                                            |
| Device MAC (primary)   | `AA:BB:CC:DD:EE:FF` (no separator: `AABBCCDDEEFF`)                     |
| Device MAC (secondary) | `AA:BB:CC:DD:EE:01` (no separator: `AABBCCDDEE01`)                     |
| Device ID (some XML)   | `ABCD1234EFGH` — legacy placeholder still in some fixtures             |
| Device display name    | `Living Room SoundTouch` / `Kitchen SoundTouch` / `Bedroom SoundTouch` |
| Bose account ID        | `1000001` / `1000002`                                                  |

`192.0.2.0/24`, `198.51.100.0/24`, and `203.0.113.0/24` are reserved
by [RFC 5737](https://www.rfc-editor.org/rfc/rfc5737) exclusively for
documentation. They won't ever route on a real network, so readers
know at a glance that they're placeholders and not addresses they
need to think about.

`AA:BB:CC:DD:EE:FF` is the conventional "locally administered" MAC
placeholder used in many vendor docs.

`1000001` / `1000002` are well outside the range of real Bose customer
account IDs (which are typically 6–7 digits with no leading 1 0 0…
pattern) but stay numeric for parsers that expect integer-looking IDs.

## Why we don't use 192.168.1.x

An earlier anonymisation pass used `192.168.1.x` as its target. That
range is RFC-1918 private space — perfectly valid on real networks,
which means a reader can't tell whether `192.168.1.10` is a
placeholder or a documented LAN address. RFC-5737 ranges fix that:
because they're reserved for documentation only, any reader knows on
sight that they don't represent a real device.

The `.md` / `.txt` portion of the `192.168.1.*` → `192.0.2.x` sweep
is complete. Test files (`.go` / `.xml` / `.http`) still carry the
old placeholder pending Phase 2 in the audit at
`_/RFC-5737-cleanup/assessment.md`.

## How to audit before committing

When you add or edit examples that contain IP addresses, MACs, account
IDs, or device names, mentally answer: "would I be comfortable
publishing this on a postcard?" If not, swap in a placeholder from
the table above.

Some patterns flag clearly-non-placeholder values:

```sh
# Any IPv4 not in a documentation range or the 192.168.1.x default:
git ls-files | xargs grep -hoE "[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+" 2>/dev/null \
  | grep -vE "^(192\.0\.2\.|198\.51\.100\.|203\.0\.113\.|0\.0\.0\.0|127\.0\.0\.1|255\.255\.255\.255|192\.168\.1\.[0-9])" \
  | sort -u

# Any colon-separated MAC that doesn't start with AA:BB:CC:DD:EE:
git ls-files | xargs grep -hoE "[0-9A-F]{2}(:[0-9A-F]{2}){5}" 2>/dev/null \
  | grep -vE "^AA:BB:CC:DD:EE:" \
  | sort -u
```

If real values slip into a commit, treat it as a sanitisation task:
revert or fix, then audit nearby files for sibling leaks. Personal
device names and Bose account IDs don't have a regex-friendly shape —
catch those at review time.
