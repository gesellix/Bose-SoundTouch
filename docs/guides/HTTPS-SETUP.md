# HTTPS & Custom CA Certificate

SoundTouch speakers communicate with cloud services over HTTPS. For the local service to work over HTTPS, speakers must trust the AfterTouch Root CA. The service manages this automatically — it generates a CA on first start and the web UI guides you through installing it on each speaker as part of the migration flow.

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

1. **Port forwarding (recommended)**: Run the service on port 8443 and forward port 443 to it using `iptables` or your firewall/router.
2. **Capabilities**: Grant the binary permission to bind low ports: `sudo setcap 'cap_net_bind_service=+ep' ./soundtouch-service`
3. **Reverse proxy**: Use Nginx or Caddy in front of the service (see below).

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
        proxy_set_header X-Real-IP $remote_addr;
    }
}
```

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