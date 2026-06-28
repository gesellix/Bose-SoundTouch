---
title: "HTTPS & Custom CA Certificate"
---
SoundTouch speakers communicate with cloud services over HTTPS. For the local service to work over HTTPS, speakers must trust the AfterTouch Root CA. The service manages this automatically — it generates a CA on first start and the web UI guides you through installing it on each speaker as part of the migration flow.

> ### ⚠️ Speakers connect to `:443`, AfterTouch defaults to `:8443`
>
> Speakers build their target URLs from Bose hostnames *without* an explicit port, so they connect on the default HTTPS port **443**. AfterTouch's built-in HTTPS listener defaults to **8443** because port 443 is privileged on most Unix systems.
>
> **If you do nothing, speakers will fail with `Curl 7` / connection refused and nothing will appear in the AfterTouch HTTP log.**
>
> Pick one of the three options under [Binding to port 443](#binding-to-port-443) below. The settings page in the web UI shows a ✅ / ❌ indicator for `:443` reachability so you can confirm the routing is in place.

---

## How TLS works in AfterTouch

The service includes a built-in HTTPS listener (default port `8443`) that presents a certificate covering all Bose cloud hostnames. The certificate is signed by the AfterTouch Root CA, which is generated automatically on first start and stored in `data/certs/`.

**Domain coverage** — the certificate covers:
- Wildcard: `*.api.bose.io`, `*.api.bosecm.com`
- Specific: `streaming.bose.com`, `bmx.bose.com`, `stats.bose.com`, `updates.bose.com`, `worldwide.bose.com`, `bose-prod.apigee.net`, `media.bose.io`, `downloads.bose.com`, `voice.api.bose.io`, and more

> **Note**: The hostname you configure as `HTTPS_SERVER_URL` (e.g. `https://soundtouch.fritz.box:8443`) is also added as a Subject Alternative Name, ensuring valid TLS for direct browser or API access.

---

## CA trust installation (via web UI)

The migration flow in the web UI includes a CA trust step that:
1. Uploads the Root CA to the speaker via SSH
2. Appends it to the speaker's shared trust store (`/etc/pki/tls/certs/ca-bundle.crt`)
3. Verifies connectivity over HTTPS

This is handled automatically — you don't need to manage CA files manually unless you're doing an advanced or manual setup.

---

## Downloading the CA certificate

You can download the Root CA for manual installation on other devices (phones, PCs, additional speakers):

```
http://<server>:8000/setup/ca.crt
```

---

## Binding to port 443

Speakers expect HTTPS on the default port 443. Since binding to port 443 requires elevated privileges, you have three options:

1. **Port forwarding (recommended)**: Run the service on port 8443 and forward port 443 to it using `iptables` or your firewall/router. Inside an LXC/Docker container or on the host:

    ```bash
    iptables -t nat -A PREROUTING -p tcp --dport 443 -j REDIRECT --to-port 8443
    iptables -t nat -A OUTPUT     -p tcp --dport 443 -j REDIRECT --to-port 8443
    ```

    The first rule covers traffic arriving from speakers; the second covers loopback connections from the host itself (useful for the in-built pre-flight probe).

    > **Caveat — OUTPUT chain.** The second rule catches **all** outbound `:443` traffic from this host, including the AfterTouch host's own connections to the wider internet (browsers, `go install` against `proxy.golang.org`, `apt-get`, `git clone https://...`, etc.). Speakers reaching AfterTouch from the LAN only ever pass through `PREROUTING`. If you don't run the in-built pre-flight probe from this host, or if you've seen other software break with TLS errors after adding both rules, add only the `PREROUTING` rule and skip `OUTPUT`. The pre-flight's "localhost:443" probe will then report unreachable — that's expected and harmless.

2. **Capabilities**: Grant the binary permission to bind low ports and start the listener directly on `:443`:

    ```bash
    sudo setcap 'cap_net_bind_service=+ep' ./soundtouch-service
    ./soundtouch-service --https-port=443
    ```

3. **Reverse proxy**: Use Nginx or Caddy on `:443` in front of the service (see below).

### Confirming `:443` is reachable

After applying any of the options above, open the AfterTouch web UI → **Settings**. The Target Domain row will show a second line:

* ✅ `:443 reachable on localhost and <IP> (forwarded to :8443)` — you're good.
* ❌ `Speakers connect to :443 but AfterTouch listens on :8443.` — the routing is missing or not yet active.

A third line follows from the browser itself, which sits on the LAN exactly where the speakers do. The browser can't distinguish an untrusted-CA TLS error from a connection refusal, so it uses timing as a heuristic: a fast error means "no listener / firewall reset", a slower one means "something answered TCP". When the server-side and browser-side checks disagree, the UI flags it — that almost always means NAT, split-horizon DNS, or a host firewall sitting between AfterTouch and the LAN.

The same check runs once at service startup and prints a `[WARN]` log line if `:443` is unreachable, with the exact iptables/setcap commands for your current listener port.

#### When this check is shown

The `:443` indicator is only displayed when **AfterTouch's DNS interception is enabled** (Settings → "Enable DNS Discovery Server"). The check is only meaningful for the **DNS migration method**, where speakers reach AfterTouch via intercepted Bose hostnames and therefore on the implicit `:443`. The other migration method — writing direct `https://<host>:8443/...` URLs into the speaker's private config via SSH — uses the port that's literally in the URL, so `:443` is irrelevant and the check would only add noise.

#### Not applicable in HTTP-only deployments

When AfterTouch's configured `--server-url` is `http://…`, the pre-flight short-circuits to an `ℹ️ :443 reachability check not applicable` info line. Speakers that were migrated to that HTTP URL never connect to `:443`, so the iptables / setcap / reverse-proxy work is only needed if you also expect unmigrated speakers to fall back to `streaming.bose.com:443` via DNS hijack. If that's not your situation, the iptables rules above are optional.

#### Adding extra hosts to the TLS certificate

If speakers reach AfterTouch via a hostname or IP that isn't already covered by the served certificate, the speaker rejects the TLS handshake (typical syslog: `CURLE_SSL_CACERT (60)`). Two paths to fix this:

* **One-click QuickFix on the Health tab.** The `speaker_marge_url` check detects the mismatch and offers an `Add <host> to TLS hosts` button. Clicking it appends the missing host to `settings.json` (`tls_extra_hosts`). A subsequent service restart regenerates the certificate.
* **Settings tab → "TLS extra hosts" textarea.** Add one host per line and click Save. Same persistence path; restart required to apply. The textarea is pre-filled with the persisted list; the read-only "Currently covered by TLS cert" line below it shows the full effective SAN list (including the values from `--server-url`, `--https-server-url`, the system hostname, and any `--tls-extra-host` / `TLS_EXTRA_HOST` CLI/env entries).

CLI/env values still win over persisted ones, so an operator who pinned a host via systemd unit doesn't have to migrate it into `settings.json` — the merge in `applyPersistedSettings` deduplicates while preserving order.

If you intercept Bose hostnames **outside** AfterTouch (Pi-hole, router DNS rule, `/etc/hosts` on a gateway), the UI gate above will hide the indicator. The data is still in the `GET /setup/settings` JSON response (`https_443_localhost_reachable`, `https_443_lan_reachable`, `https_443_lan_host`, `https_443_not_applicable`, `https_443_reason`) if you want to inspect it directly, or you can briefly enable AfterTouch's DNS server to see the indicator render.

---

## Reverse proxy (optional)

If you prefer to use Nginx or another proxy for TLS termination:

```nginx
server {
    listen 443 ssl;
    server_name streaming.bose.com bmx.bose.com stats.bose.com updates.bose.com;

    ssl_certificate /path/to/data/certs/server.crt;
    ssl_certificate_key /path/to/data/certs/server.key;

    ssl_protocols TLSv1.2;
    ssl_ciphers 'ECDHE-RSA-AES128-GCM-SHA256:ECDHE-RSA-AES256-GCM-SHA384:ECDHE-RSA-CHACHA20-POLY1305:AES128-GCM-SHA256:AES256-GCM-SHA384';

    location / {
        proxy_pass http://localhost:8000;
        proxy_set_header Host $host;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    }
}
```

> **Client IP behind a proxy.** A reverse proxy changes the source IP the
> service sees, which matters for the handlers that act on it. Configuring
> AfterTouch to recover the real speaker IP from `X-Forwarded-For`
> (`trust_forwarded_headers` / `trusted_proxy_cidrs`) is covered under
> [Client IP behind a proxy or load balancer](CLOUD-DEPLOY-WALKTHROUGH.md#client-ip-behind-a-proxy-or-load-balancer).

---

## Manual CA injection (advanced)

If you need to inject the CA manually (e.g. without the web UI migration flow):

```bash
# Copy the CA to the speaker
scp data/certs/ca.crt root@<SPEAKER-IP>:/tmp/

# Make the filesystem writable and append the CA to the trust store
ssh root@<SPEAKER-IP> "(rw || mount -o remount,rw /) && cat /tmp/ca.crt >> /etc/pki/tls/certs/ca-bundle.crt"
```

---

## TLS compatibility

SoundTouch speakers run OpenSSL 1.0.2, supporting up to TLS 1.2. The service is configured accordingly:

- **Minimum TLS version**: TLS 1.2
- **Preferred cipher suites**: `ECDHE-RSA-AES128-GCM-SHA256`, `ECDHE-RSA-AES256-GCM-SHA384`, `ECDHE-RSA-CHACHA20-POLY1305`
- **Legacy support**: `RSA-AES128-GCM-SHA256`, `RSA-AES256-GCM-SHA384`