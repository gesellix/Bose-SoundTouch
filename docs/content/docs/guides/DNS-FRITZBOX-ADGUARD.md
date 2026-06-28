---
title: "FRITZ!Box + AdGuard Home: DNS-based bose Hostname"
---

This guide covers a setup that trips up a lot of people: running AfterTouch
behind a local DNS resolver (AdGuard Home, Pi-hole, or the FRITZ!Box itself)
and addressing it by a short hostname like `bose` instead of a raw IP. When the
pieces don't line up, speakers report `INVALID_SOURCE` for TuneIn / internet
radio, the Health tab warns about missing source types
(`LOCAL_INTERNET_RADIO`, `RADIO_BROWSER`, `TUNEIN`), and pre-flight shows an
HTTP-connection / URL-mismatch failure even though AfterTouch itself is running
correctly.

The root cause is almost always the same: **the speaker cannot resolve the
hostname you configured, or the TLS certificate doesn't cover it.** This is a
real-world setup contributed by a user who hit exactly this and worked out the
fix.

> The IP addresses below use the documentation range `192.0.2.0/24`
> ([RFC 5737](https://datatracker.ietf.org/doc/html/rfc5737)). Substitute your
> own AfterTouch host IP. The hostname `bose` and FQDN `bose.fritz.box` are
> examples; any short name works as long as DNS and TLS agree on it.

## The setup

- AfterTouch runs as a container (here: Proxmox + Docker, `--network host`,
  data directory bind-mounted), reachable at `192.0.2.10`.
- The FRITZ!Box forwards all DNS queries to **AdGuard Home** as the LAN resolver.
- AdGuard already had DNS rewrites for the Bose cloud hostnames pointing at
  AfterTouch:

  | Name                           | Answer       |
  |--------------------------------|--------------|
  | `productregistration.bose.com` | `192.0.2.10` |
  | `streaming.bose.com`           | `192.0.2.10` |
  | `select.bose.com`              | `192.0.2.10` |
  | `update.bose.com`              | `192.0.2.10` |

That part is the standard "intercept Bose hostnames outside AfterTouch"
approach (see [HTTPS & Custom CA Certificate](HTTPS-SETUP.md)). What was missing
was making the **short hostname** you point speakers at resolvable *and*
TLS-valid.

## The fix

### 1. Add DNS rewrites for the short hostname

In AdGuard Home, add rewrites so the name you plan to use in the service URLs
resolves to AfterTouch:

| Name             | Answer       |
|------------------|--------------|
| `bose`           | `192.0.2.10` |
| `bose.fritz.box` | `192.0.2.10` |

Both forms matter: speakers and clients may append the FRITZ!Box search domain
(`.fritz.box`), so covering the bare label and the FQDN avoids surprises.

### 2. Include the hostname in the TLS certificate

If speakers (or your browser) reach AfterTouch by `bose`, that name must be in
the certificate's SAN list, otherwise the TLS handshake is rejected
(`CURLE_SSL_CACERT (60)`). Start the container with the host added:

```bash
TLS_EXTRA_HOST="192.0.2.10,bose"
```

`TLS_EXTRA_HOST` is a comma-separated (and repeatable) list of extra DNS names
or IPs added to the certificate SAN list. You can also manage it from the web
UI: **Settings → "TLS extra hosts"**, or the one-click **"Add &lt;host&gt; to TLS
hosts"** QuickFix on the Health tab. Either path persists to `settings.json`
(`tls_extra_hosts`) and takes effect after a service restart, which regenerates
the certificate. See
[Adding extra hosts to the TLS certificate](HTTPS-SETUP.md#adding-extra-hosts-to-the-tls-certificate).

### 3. Point the service URLs at the hostname

In AfterTouch, under **System Settings / Target Domain / Service URLs**, switch
from the raw IP to the hostname:

```
http://192.0.2.10:8000   →   http://bose:8000
```

After this, the per-device config should read:

```
margeServerUrl = http://bose:8000
statsServerUrl = http://bose:8000
bmxRegistryUrl = http://bose:8000/bmx/registry/v1/services
```

### 4. Re-migrate the speakers

Re-run the migration for each speaker (XML over SSH), then reboot and send a
`sourcesUpdated` notification so the runtime layer reconciles. See the
[Migration Guide](MIGRATION-GUIDE.md).

## Verifying it worked

- `http://bose:8000/health` responds, and `https://bose:8443/admin` loads with a
  valid certificate.
- The Health tab no longer warns about URL mismatch or HTTP reachability (a
  brief runtime-vs-XML hint right after migration clears on reboot).
- TuneIn / internet radio plays again; `INVALID_SOURCE` is gone.
- `/sources` lists the expected source types and `sources_xml_diff` is green.

## Why this is the stumbling block

Technically AfterTouch was serving correctly the whole time. The failure was
purely in name resolution and certificate coverage: the speaker asked the
nameserver for `bose`, got nothing usable (or reached a host whose certificate
didn't list `bose`), and fell back toward the now-dead Bose cloud. Using a raw
IP avoids the resolution step entirely; using a hostname is cleaner but only
works once **DNS** and the **TLS certificate** both agree on that name.

> Prefer the raw IP if you want the simplest possible path with one fewer moving
> part. Prefer the hostname if you run split-horizon DNS anyway and want a
> stable name that survives an IP change. Either is fine, the key is that DNS,
> the certificate, and the configured service URLs all reference the same
> target.

## Related

- [HTTPS & Custom CA Certificate](HTTPS-SETUP.md): TLS, SAN coverage, `:443` routing
- [Migration Guide](MIGRATION-GUIDE.md): DNS vs. SSH/XML migration methods
- [Troubleshooting](TROUBLESHOOTING.md): `nslookup` / `dig` checks for name resolution
