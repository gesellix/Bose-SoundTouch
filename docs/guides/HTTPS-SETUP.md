# HTTPS Setup & Custom CA Certificate

To use the `/etc/hosts` redirection method safely, SoundTouch devices must communicate over HTTPS. This requires the device to trust the Root CA certificate used by the local `soundtouch-service`.

## 1. Automated Migration (Hosts Method)

The `soundtouch-service` can automatically configure a device to use the `/etc/hosts` method:

```bash
curl -X POST "http://localhost:8000/setup/migrate/{deviceIP}?method=hosts"
```

This command will:
1.  Connect to the device via SSH.
2.  Update `/etc/hosts` to point Bose domains to the service IP.
3.  Inject the auto-generated Root CA into the device's trust store (`/etc/pki/tls/certs/ca-bundle.crt`).
4.  Reboot the device.

## 2. Managing the Root CA

The `soundtouch-service` automatically generates a Root CA when it first starts.

- **CA Certificate**: `data/certs/ca.crt`
- **CA Private Key**: `data/certs/ca.key`

### Downloading the CA Certificate
You can download the CA certificate for manual installation on other devices (like your phone or PC) from:
`http://<server-ip>:8000/setup/ca.crt`

### 3. Built-in HTTPS Support

The `soundtouch-service` now includes a built-in HTTPS listener. This simplifies the `/etc/hosts` redirection method by automatically presenting the correct certificates for Bose domains.

- **HTTPS Port**: Configurable via `HTTPS_PORT` environment variable (defaults to `8443`).
- **HTTPS Server URL**: Configurable via `HTTPS_SERVER_URL` (e.g., `https://mysoundtouch.local:8443`). If not set, the service attempts to guess it using the system hostname.
- **Domain Coverage**: Automatically presents a certificate for `streaming.bose.com`, `updates.bose.com`, `stats.bose.com`, `bmx.bose.com`, and `content.api.bose.io`.
- **Automatic Setup**: On first start, it generates a server certificate signed by your local Root CA.

#### TLS Security

The built-in HTTPS listener is configured to use modern and secure TLS settings while maintaining compatibility with SoundTouch devices (which support up to TLS 1.2 with OpenSSL 1.0.2).

- **Minimum TLS Version**: TLS 1.2
- **Preferred Cipher Suites**:
  - `ECDHE-RSA-AES128-GCM-SHA256`
  - `ECDHE-RSA-AES256-GCM-SHA384`
  - `ECDHE-RSA-CHACHA20-POLY1305`
  - `RSA-AES128-GCM-SHA256` (Legacy support)
  - `RSA-AES256-GCM-SHA384` (Legacy support)

#### Binding to Port 443
SoundTouch devices expect HTTPS on the default port 443. Since binding to port 443 usually requires root privileges, you have two options:

1.  **Port Forwarding (Recommended)**: Run the service on a high port (e.g., 8443) and use `iptables` or your firewall to forward traffic from 443 to 8443.
2.  **Capabilities**: Grant the binary permission to bind to low ports: `sudo setcap 'cap_net_bind_service=+ep' ./soundtouch-service`.
3.  **Reverse Proxy**: Use Nginx or Caddy as described below.

### 4. Reverse Proxy (Optional)

1.  **Generate a certificate** for the Bose domains signed by your Root CA.
2.  **Configure Nginx** to use this certificate and proxy requests to `soundtouch-service`.

```nginx
server {
    listen 443 ssl;
    server_name streaming.bose.com bmx.bose.com stats.bose.com updates.bose.com;

    ssl_certificate /path/to/generated-cert.crt;
    ssl_certificate_key /path/to/generated-cert.key;

    # Secure TLS configuration (matches soundtouch-service defaults)
    ssl_protocols TLSv1.2;
    ssl_ciphers 'ECDHE-RSA-AES128-GCM-SHA256:ECDHE-RSA-AES256-GCM-SHA384:ECDHE-RSA-CHACHA20-POLY1305:AES128-GCM-SHA256:AES256-GCM-SHA384';

    location / {
        proxy_pass http://localhost:8000;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}
```

## 5. Manual CA Injection (Legacy/Manual)

If you prefer to inject the CA certificate manually:

1.  Copy `ca.crt` to the device:
    ```bash
    scp data/certs/ca.crt root@{deviceIP}:/tmp/
    ```
2.  Append it to the trust store on the device:
    ```bash
    ssh root@{deviceIP} "(rw || mount -o remount,rw /) && cat /tmp/ca.crt >> /etc/pki/tls/certs/ca-bundle.crt"
    ```

    ## 6. Verifying Connectivity

    You can verify that your device can correctly reach the `soundtouch-service` over HTTPS using the management web UI.

    In the **Migration Summary** for a device, you will find an **HTTPS Connection Test** section:
    - **Test with Explicit CA.crt**: Uploads a temporary copy of the Root CA to the device and uses `curl --cacert` to verify the connection. Use this to verify your HTTPS setup *before* modifying the device's shared trust store.
    - **Test with Shared Trust Store**: Uses the device's default trust store. Use this to verify that your CA injection was successful and the device now natively trusts your local server.
