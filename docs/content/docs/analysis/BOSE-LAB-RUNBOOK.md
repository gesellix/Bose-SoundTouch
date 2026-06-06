---
title: "Bose SoundTouch – Traffic Analysis Runbook"
---
> **Goal:** Set up a Raspberry Pi as a transparent access point to fully observe the traffic of the Bose SoundTouch app – specifically the pairing flow with the Bose Cloud. This serves as a basis for later reverse engineering / simulation of the cloud endpoints.

---

## Prerequisites

| Component          | Details                                                          |
|--------------------|------------------------------------------------------------------|
| Raspberry Pi       | Pi 3 or newer, Raspberry Pi OS (Bullseye, Bookworm, Trixie)      |
| Network interfaces | `eth0` → LAN cable to FritzBox, `wlan0` → own Access Point       |
| FritzBox           | Unchanged, assigns an IP to the Pi via DHCP on eth0              |
| Custom DNS Server  | Already present (or see Appendix A), incl. custom CA certificate |
| Phone              | Android, connects to the Pi's Wi-Fi                              |

### Network Architecture

```
Internet
    ↓
FritzBox  (existing, unchanged)
    ↓  LAN cable (eth0)
Raspberry Pi
  ├── DNS Server       → selective logging / redirection
  ├── hostapd          → custom Wi-Fi Access Point ("Bose-Lab")
  ├── dnsmasq          → DHCP for clients, DNS to custom server
  ├── iptables         → NAT, Forwarding eth0 ↔ wlan0
  ├── tcpdump          → full traffic capture
  └── (optional) mitmproxy → HTTPS decryption
    ↓  Wi-Fi ("Bose-Lab")
Android Phone
  └── Bose SoundTouch App
```

---

## Step 1 – Install Packages

```bash
sudo apt update && sudo apt install -y \
  hostapd \          # Wi-Fi Access Point daemon
  dnsmasq \          # DHCP + DNS forwarding
  nftables \         # Modern NAT / firewall / forwarding
  tcpdump \          # Packet capture at all levels
  wireshark-common   # tshark CLI (optional, for live analysis)
```

---

## Step 2 – Enable IP Forwarding

The Pi must forward packets between `wlan0` (phone) and `eth0` (FritzBox).

```bash
# Active immediately (no reboot required)
sudo sysctl -w net.ipv4.ip_forward=1

# Permanent (survives reboots)
# On modern Debian, using a dedicated file in sysctl.d/ is more reliable:
echo "net.ipv4.ip_forward=1" | sudo tee /etc/sysctl.d/99-ip-forward.conf

# Apply changes immediately
sudo sysctl --system
```

**Verify:**
```bash
# After a reboot, ensure it is still '1'
cat /proc/sys/net/ipv4/ip_forward
```

---

## Step 3 – Static IP on wlan0 (systemd-networkd)

On modern Debian (Bookworm/Trixie), `dhcpcd` is replaced by `systemd-networkd`.

```bash
# Create network configuration
sudo tee /etc/systemd/network/08-wlan0.network << 'EOF'
[Match]
Name=wlan0

[Network]
Address=198.51.100.1/24
IPForward=yes
ConfigureWithoutCarrier=yes
DHCP=no
IPv6AcceptRA=no
EOF

# Restart service
sudo systemctl enable systemd-networkd
sudo systemctl restart systemd-networkd

# Ensure wpa_supplicant and NetworkManager don't interfere
sudo nmcli device set wlan0 managed no
sudo systemctl stop wpa_supplicant@wlan0
sudo systemctl mask wpa_supplicant@wlan0
```

**Verify:**
```bash
ip addr show wlan0
# Expected: ONLY inet 198.51.100.1/24 (NO second DHCP IP)
```

---

## Step 4 – hostapd (Access Point)

```bash
sudo tee /etc/hostapd/hostapd.conf << 'EOF'
interface=wlan0
driver=nl80211
ssid=Bose-Lab
hw_mode=b
#hw_mode=g
channel=1
#channel=6
wmm_enabled=0
auth_algs=1
wpa=2
wpa_passphrase=secret123
wpa_key_mgmt=WPA-PSK
wpa_pairwise=CCMP
EOF

# The modern way is to just use hostapd.service which defaults to /etc/hostapd/hostapd.conf
sudo systemctl unmask hostapd
sudo systemctl enable --now hostapd
```

**Verify:**
```bash
sudo systemctl status hostapd
# Expected: active (running)
```

---

## Step 5 – dnsmasq (DHCP + DNS)

dnsmasq gives the phone an IP and forwards DNS queries to the custom DNS server.

```bash
# Back up original config
sudo mv /etc/dnsmasq.conf /etc/dnsmasq.conf.bak

sudo tee /etc/dnsmasq.conf << 'EOF'
interface=wlan0
dhcp-range=198.51.100.100,198.51.100.200,24h
dhcp-option=3,198.51.100.1
dhcp-option=6,198.51.100.1

# DNS Upstream: custom server on localhost (adjust port if necessary)
server=127.0.0.1#5353    # Example: custom server on port 5353
# Alternatively: server=1.1.1.1 if DNS server runs directly on port 53

# Log all DNS queries (for initial analysis)
log-queries
log-facility=/var/log/dnsmasq.log
EOF

sudo systemctl restart dnsmasq
```

**Observe DNS log live:**
```bash
sudo tail -f /var/log/dnsmasq.log
```

---

## Step 6 – NAT and Forwarding (nftables)

On modern Debian (Bookworm/Trixie), `nftables` is the default and recommended way to manage NAT and traffic forwarding.

```bash
# Define the NAT and Forwarding rules
sudo tee /etc/nftables.conf << 'EOF'
#!/usr/sbin/nft -f

flush ruleset

table inet filter {
    chain forward {
        type filter hook forward priority 0; policy drop;

        # Allow traffic from phone (wlan0) to internet (eth0)
        iifname "wlan0" oifname "eth0" accept

        # Allow established/related traffic back to the phone
        iifname "eth0" oifname "wlan0" ct state established,related accept
    }
}

table ip nat {
    chain posterouting {
        type nat hook postrouting priority 100; policy accept;

        # MASQUERADE outgoing packets on eth0
        oifname "eth0" masquerade
    }
}
EOF

# Enable and start nftables
sudo systemctl enable nftables
sudo systemctl restart nftables
```

**Verify:**
```bash
sudo nft list ruleset
# Expected: ruleset showing the forward and nat chains
```

### WiFi "Bose-Lab" not visible?

If you cannot see the `Bose-Lab` SSID on your phone:

1.  **Check hostapd status:** `sudo systemctl status hostapd`. If it failed with "nl80211: Driver does not support configured mode", try changing `hw_mode=g` to `hw_mode=b`.
2.  **Interface blocking:** Ensure `rfkill` hasn't blocked WiFi: `sudo rfkill unblock wlan`.
3.  **Country Code:** Some systems require a country code in `hostapd.conf` to enable the radio. Add `country_code=DE` (or your country) to the top of `/etc/hostapd/hostapd.conf` and restart hostapd: `sudo systemctl restart hostapd`.
4.  **Local Radio Check:** You can verify that the radio is actually configured as an AP: `iw dev wlan0 info`. Look for `type AP` and your SSID.
    > **Note:** Do NOT rely on `iw dev wlan0 scan` for your own SSID; many WiFi drivers cannot "scan" and "broadcast" simultaneously.
5.  **Debug Mode:** If the scan still returns nothing, stop the service and run hostapd in the foreground to see real-time errors:
    ```bash
    sudo systemctl stop hostapd
    sudo hostapd -dd /etc/hostapd/hostapd.conf
    ```
    Look for messages like `nl80211: Failed to set interface wlan0 into AP mode`. This usually means the hardware is busy or doesn't support the current `hw_mode` / `channel` combination.
6.  **Conflicting Services:** Ensure nothing else is managing `wlan0`. NetworkManager is common on modern Debian:
    ```bash
    sudo nmcli device set wlan0 managed no
    ```
7.  **Ghost IP Conflict:** If `ip addr show wlan0` shows both `198.51.100.1` and another IP (like `192.0.2.x`), `hostapd` will fail. This is usually caused by NetworkManager managing the interface. Ensure you've run:
    ```bash
    sudo nmcli device set wlan0 managed no
    # If the ghost IP is still there, remove it manually:
    sudo ip addr del 192.0.2.0/24 dev wlan0
    ```

---

## Step 7 – Install Custom CA Certificate on the Phone

Since a custom DNS server with a custom CA certificate is used, it must be trusted on the phone – otherwise, the app will block HTTPS connections to redirected domains.

### Copy CA Certificate to the Pi (if not already there)

If you haven't created a CA yet, follow **Appendix A** first.

```bash
# Certificate is located e.g. at /etc/my-dns-ca/ca.crt
# Temporarily make reachable via HTTP for easy download:
cd /etc/my-dns-ca/
python3 -m http.server 8080
# → Reachable at http://198.51.100.1:8080/ca.crt
```

### Install on Android

1. Connect phone to `Bose-Lab`
2. Open browser → `http://198.51.100.1:8080/ca.crt`
3. Download certificate
4. **Settings → Security → Credentials → Install CA Certificate**
5. Select certificate and confirm

> **Note:** Android distinguishes between system CAs and user CAs. User-installed CAs are accepted by many apps, but apps with certificate pinning (hardcoded certificate hashes) ignore them. Whether Bose uses pinning will be visible in the capture (Connection Reset after TLS ClientHello).

### Android 14+ Special Case

From Android 14 onwards, apps do not trust user CAs by default unless explicitly declared in the manifest. If the Bose app rejects the CA certificate:

```bash
# Option A: Root + Magisk module "MagiskTrustUserCerts"
#   → moves user CAs to the system store

# Option B: Root + manually copy to system CA directory
adb push ca.crt /system/etc/security/cacerts/
adb shell chmod 644 /system/etc/security/cacerts/ca.crt
```

---

## Step 8 – Capture Traffic

### All at once (recommended)

```bash
# Full capture of all protocols on wlan0
# Filename with timestamp for multiple sessions
sudo tcpdump -i wlan0 \
  -w /tmp/bose-$(date +%Y%m%d-%H%M%S).pcap \
  -s 0        # full packet length (no truncation)

# End session: Ctrl+C
```

### Targeted by protocol

```bash
# DNS only (Port 53) – shows if app uses standard DNS
sudo tcpdump -i wlan0 -n port 53

# HTTPS only – TLS connections to Bose Cloud
sudo tcpdump -i wlan0 -n 'tcp port 443'

# mDNS (ZeroConf) – device discovery in LAN
# Multicast group 224.0.0.1, Port 5353
sudo tcpdump -i wlan0 -n 'udp port 5353'

# SSDP/UPnP – alternative device discovery
sudo tcpdump -i wlan0 -n 'udp port 1900'

# Everything except DNS (reduces noise)
sudo tcpdump -i wlan0 -n 'not port 53' -w /tmp/bose-nodns.pcap

# Traffic of a specific host only (filter by phone IP)
# Read phone IP from dnsmasq.leases beforehand (see below)
sudo tcpdump -i wlan0 -n host 198.51.100.101
```

### Read SNI from TLS Traffic (without decryption)

```bash
# Extract domains from TLS ClientHello (SNI is unencrypted)
sudo tcpdump -i wlan0 -n 'tcp port 443' -A 2>/dev/null \
  | grep -oP '(?<=\x00)([a-zA-Z0-9.-]+\.(?:com|net|io|cloud|bose\.com))'
```

### Readable mDNS Announcements output

```bash
# tshark decodes mDNS directly
sudo tshark -i wlan0 -f 'udp port 5353' -T fields \
  -e dns.qry.name \
  -e dns.resp.name \
  -e dns.a
```

---

## Step 9 – Analysis with Wireshark (on PC)

Transfer `.pcap` files from the Pi to the PC:

```bash
# From the PC (scp)
scp pi@198.51.100.1:/tmp/bose-*.pcap ~/Desktop/
```

**Important Wireshark Filters:**

```
# DNS only
dns

# HTTPS only
tcp.port == 443

# WebSocket connections (HTTP Upgrade)
websocket

# mDNS
mdns

# TLS Handshakes (SNI visible)
tls.handshake.extensions_server_name

# Traffic of a specific domain (resolve by IP)
http.host contains "bose"

# WebSocket frames
websocket.payload
```

> **Tip:** Wireshark decodes WebSocket frames automatically if it sees the HTTP Upgrade handshake in the same capture. For the pairing flow: filtering for `tls.handshake.extensions_server_name` shows all domains the app contacts, even without decryption.

---

## Step 10 – mitmproxy (optional, for HTTPS content)

Only useful if the CA certificate on the phone is trusted and no certificate pinning is active. `mitmproxy` acts as a Man-in-the-Middle by generating fake, on-the-fly certificates for any domain (e.g., `global.api.bose.io`) using your custom CA.

### 1. Configure mitmproxy to use your Custom CA

By default, `mitmproxy` creates its own CA in `~/.mitmproxy/`. To ensure the phone (which already trusts your `ca.crt`) accepts the traffic, you must tell `mitmproxy` to use your existing CA:

```bash
# mitmproxy expects the CA in a specific PEM format (cert + key in one file)
sudo mkdir -p ~/.mitmproxy
sudo cat /etc/my-dns-ca/ca.crt /etc/my-dns-ca/ca.key | sudo tee ~/.mitmproxy/mitmproxy-ca.pem > /dev/null
```

### 2. Install and Start mitmproxy

```bash
# Install mitmproxy binary (stable version for aarch64)
cd /tmp
wget https://downloads.mitmproxy.org/12.2.1/mitmproxy-12.2.1-linux-aarch64.tar.gz
tar -xzf mitmproxy-12.2.1-linux-aarch64.tar.gz
sudo mv mitmproxy mitmdump mitmweb /usr/local/bin/
rm mitmproxy-12.2.1-linux-aarch64.tar.gz

mitmproxy --version

# Transparent proxy on port 8080
# It will now use the CA from ~/.mitmproxy/mitmproxy-ca.pem
mitmproxy --mode transparent --listen-port 8080

# Alternatively: mitmdump for automatic logging to file
# mitmdump --mode transparent --listen-port 8080 -w /tmp/bose-https.mitm
```

### 3. Troubleshooting: TLS Handshake Failures

If you see `Client TLS handshake failed. The client does not trust the proxy's certificate for www.google.com` (or other domains) in the `mitmproxy` logs:

1.  **HSTS and Pre-installed Pinning:** High-security sites like `www.google.com` use **HSTS (HTTP Strict Transport Security)** and have their certificates hardcoded (pinned) into browsers like Chrome and the Android system. **These will always fail with a User-installed CA.**
2.  **User vs. System CA Store:** On Android 7.0+, apps **do not trust User-installed CAs by default**. They only trust the "System" store.
    *   **The Bose app:** If it fails, it's because it only trusts the System store or uses its own certificate pinning.
    *   **The Fix (Rooted Phone):** Use a Magisk module like `AlwaysTrustUserCerts` or manually move your `ca.crt` to `/system/etc/security/cacerts/` (see Step 7).
3.  **The "Golden Rule" - Verify the Proxy is Working:**
    To confirm your CA and `mitmproxy` are correctly configured, test with a non-HSTS site on the phone's browser (e.g., `http://neverssl.com`). Once redirected to HTTPS, **inspect the certificate**. It should say it was issued by your "Bose-Lab Root CA" (or "SoundTouch Root CA").

    *   **If this works:** Your "factory" (mitmproxy + CA) is 100% correct. Any failure in the Bose app is due to its own security policy (ignore User Store or Pinning).
    *   **If this fails:** Your CA is not trusted by the browser or `mitmproxy` is not using your PEM file.

    Alternatively, use `curl` from a terminal emulator on the phone:
    ```bash
    # This should work if the CA is in the user store and curl is told to use it
    curl -v --cacert /path/to/ca.crt https://example.com
    ```
4.  **Check mitmproxy CA:** Ensure `mitmproxy` is actually using your CA. When it starts, it should NOT generate a new CA in `~/.mitmproxy/mitmproxy-ca.pem` if you've already placed yours there.

---

**nftables rule: redirect HTTPS traffic to mitmproxy**

```bash
# Create a temporary file for the redirection rule
sudo nft add table ip mitm
sudo nft add chain ip mitm prerouting { type nat hook prerouting priority -100 \; }
sudo nft add rule ip mitm prerouting iifname "wlan0" tcp dport 443 redirect to :8080
```

**Remove rule when no longer needed:**

```bash
sudo nft delete table ip mitm
```

> **Detecting Certificate Pinning:** If the app immediately disconnects after mitmproxy redirection (connection reset directly after TLS ClientHello), pinning is active. In this case, Frida + root is needed to patch the pinning.

---

## Step 11 – Bypassing Android Trust Restrictions

If `neverssl.com` works in the browser but the Bose app shows `TLS handshake failed` in `mitmproxy`, the app is either ignoring the **User CA store** (common on Android 7+) or using **Certificate Pinning**.

### Option A: Move CA to System Store (Requires Root/Magisk)

This is the most reliable way to make apps trust your CA without modifying the app itself.

1.  **Using Magisk (Recommended):**
    Install the **"AlwaysTrustUserCerts"** or **"Move Certificates"** module in Magisk. It automatically mirrors all certificates from the User store to the System store on every boot.

2.  **Manual Move (via ADB):**
    Android system certificates are stored in `/system/etc/security/cacerts/` and must be named using the hash of the certificate.

    ```bash
    # 1. Get the hash of your certificate
    hash=$(openssl x509 -inform PEM -subject_hash_old -in ca.crt | head -1)

    # 2. Rename the certificate locally
    cp ca.crt ${hash}.0

    # 3. Push to the phone (requires remounting /system as read-write)
    adb push ${hash}.0 /sdcard/
    adb shell
    su
    mount -o rw,remount /
    cp /sdcard/${hash}.0 /system/etc/security/cacerts/
    chmod 644 /system/etc/security/cacerts/${hash}.0
    chown root:root /system/etc/security/cacerts/${hash}.0
    reboot
    ```

### Option B: Patching the App (No Root Required)

If you cannot root your phone, you can modify the app's APK to trust user-installed certificates. This involves obtaining the APK, decompiling it, adding a network security configuration, and then repackaging and signing it.

#### 0. How to get the .apk file?

You have two main ways to get the official Bose SoundTouch APK:

**Method 1: Extract from your phone (Safest)**
If the app is already installed on your phone, you can pull it using `adb`:
```bash
# 1. Find the package name (usually com.bose.soundtouch)
adb shell pm list packages | grep bose

# 2. Get the full path to the APK on the phone
adb shell pm path com.bose.soundtouch
# Output: package:/data/app/~~...==/com.bose.soundtouch-.../base.apk

# 3. Pull the file to your computer
adb pull /data/app/~~...==/com.bose.soundtouch-.../base.apk Bose-SoundTouch.apk
```

**Method 2: Download from a Mirror (Easiest)**
You can download the APK from reputable third-party sites.
> **Warning:** Always verify the site's reputation.
*   [APKMirror](https://www.apkmirror.com/apk/bose-corporation/bose-soundtouch/)
*   [APKPure](https://apkpure.com/bose-soundtouch/com.bose.soundtouch)

#### 1. Automated Method: apk-mitm (Recommended)
The easiest way is to use `apk-mitm`, which automates the entire process including fixing common certificate pinning libraries.

```bash
# Requires Node.js installed on your PC
npx apk-mitm Bose-SoundTouch.apk
```
This will produce a `Bose-SoundTouch-patched.apk` which you can install on your phone.

#### 2. Manual Method: Network Security Config
If you prefer to do it manually:

1.  **Decompile the APK:**
    ```bash
    apktool d Bose-SoundTouch.apk
    ```
2.  **Create/Modify `res/xml/network_security_config.xml`:**
    ```xml
    <?xml version="1.0" encoding="utf-8"?>
    <network-security-config>
        <base-config>
            <trust-anchors>
                <certificates src="system" />
                <certificates src="user" />
            </trust-anchors>
        </base-config>
    </network-security-config>
    ```
3.  **Update `AndroidManifest.xml`:**
    Ensure the `<application>` tag includes: `android:networkSecurityConfig="@xml/network_security_config"`.
4.  **Repackage and Sign:**
    ```bash
    apktool b Bose-SoundTouch -o Bose-SoundTouch-patched.apk
    # Sign with your own key
    # 1. Generate a keystore (if you don't have one)
    # Note: You can use ANY name/values here. The phone does not need to "know" or "trust" this key beforehand.
    # It only needs the APK to be digitally signed so the Android installer accepts it.
    keytool -genkey -v -keystore my-release-key.keystore -alias alias_name -keyalg RSA -keysize 2048 -validity 10000

    # 2. Sign the APK
    apksigner sign --ks my-release-key.keystore --out Bose-SoundTouch-patched-signed.apk Bose-SoundTouch-patched.apk

    # Alternatively, use uber-apk-signer (recommended for simplicity)
    # It handles zipalign and signing automatically.
    java -jar uber-apk-signer.jar --apk Bose-SoundTouch-patched.apk
    ```

#### 3. Install the Patched APK

Once you have your `Bose-SoundTouch-patched.apk` (and it is signed), you need to install it on your phone.

**Important:** You must **uninstall the original Bose app first**. Android will not allow you to "update" the official app with your patched version because the digital signatures won't match.

**Method 1: via ADB (Recommended)**
```bash
# 1. Uninstall the original app
adb uninstall com.bose.soundtouch

# 2. Install your patched version
adb install Bose-SoundTouch-patched.apk
```

**Method 2: Manual Transfer**
1.  Copy the `Bose-SoundTouch-patched.apk` to your phone's storage (via USB, Google Drive, or the Pi's HTTP server).
2.  On the phone, use a File Manager to open the APK.
3.  If prompted, allow "Install from Unknown Sources" for your File Manager.

### Option C: Using the macOS Bose SoundTouch App (No Root/Patching Required)

If you have a Mac, using the macOS version of the Bose SoundTouch app is often a good alternative. However, because the app is built on an **older version of Qt (5.7.0)**, it has specific trust and TLS compatibility issues that require extra steps.

#### 1. Install the Custom CA in macOS Keychain

1.  Open **Keychain Access** on your Mac.
2.  Select the **System** keychain (or **login** if System is locked).
3.  Drag and drop your `ca.crt` file into the list.
4.  Double-click the newly added certificate (e.g., "Bose-Lab Root CA").
5.  Expand the **Trust** section.
6.  Set "When using this certificate" to **Always Trust**.
7.  Close the window and authenticate with your Mac password.

#### 2. Configure the Proxy

You can either configure the macOS system proxy manually or use `mitmproxy`'s automatic interception.

**Method 1: System Proxy (Manual)**
1.  Go to **System Settings → Network → Wi-Fi → Details... → Proxies**.
2.  Enable **HTTP Proxy** and **HTTPS Proxy**.
3.  Set Server to your Pi's IP (`198.51.100.1`) and Port to `8080`.
4.  Click **OK** and **Apply**.

**Method 2: mitmproxy Local Redirect (Automatic)**
If you are running `mitmproxy` directly on your Mac (instead of the Pi), you can use the modern "Local Redirect" mode which doesn't require proxy settings:
```bash
# Install mitmproxy via Homebrew
brew install mitmproxy

# Start mitmproxy in local redirect mode
# This uses a macOS Network Extension to intercept traffic from specific apps
mitmproxy --mode local
```

#### 3. Special Troubleshooting: Legacy Qt 5.7.0 SSL Failures

If you see `SSL handshake failed` in the `mitmproxy` logs or the app's internal log (`log.txt`), the app's older networking stack is rejecting the connection. This is common because Qt 5.7.0 (2016) lacks support for **TLS 1.3** and many modern root certificates (like Let's Encrypt's **ISRG Root X1**).

**The Solution: Launch with SSL Bypass Flags**

Since the Bose macOS app is a hybrid of **Qt/Chromium** and **Node.js**, you must bypass the trust checks for both engines by launching the app from the terminal:

```bash
# 1. Bypass QtWebEngine/Chromium (Qt 5.7) trust
export QTWEBENGINE_CHROMIUM_FLAGS="--ignore-certificate-errors"

# 2. Bypass Node.js (SoundTouch Music Server) trust
export NODE_TLS_REJECT_UNAUTHORIZED=0

# 3. (Optional) Provide your custom CA directly to Node.js
export NODE_EXTRA_CA_CERTS="/path/to/your/ca.crt"

# 4. Launch the application
"/Applications/SoundTouch/SoundTouch.app/Contents/MacOS/SoundTouch"
```

#### 4. Verify and Capture

1.  Open Safari and visit `https://neverssl.com`. Verify the certificate is issued by your custom CA.
2.  Launch the Bose app using the terminal command above.
3.  Watch the traffic flow in `mitmproxy`.

> **Note:** Even on macOS, **Certificate Pinning** is still possible if Bose implemented it specifically in the desktop app code. However, it is much less common on desktop apps than on mobile apps. If it works, you've saved yourself hours of Android patching!

### Option D: Patching the App with Frida (Requires Root)

If the app uses **Certificate Pinning** (hardcoded hashes), even moving the CA to the System store won't work. You must disable the pinning check in the app's code.

1.  **Install Frida** on your PC and `frida-server` on the rooted phone.
2.  **Use a universal bypass script:**
    ```bash
    frida -U -f com.bose.soundtouch -l https://codeshare.frida.re/@pcipolloni/universal-android-ssl-pinning-bypass-with-frida/ --no-pause
    ```
    *(Replace `com.bose.soundtouch` with the actual package name if different).*

## Step 12 – Alternative: Regular HTTP Proxy Mode

If the **Transparent AP** setup (Steps 1–6) is too complex or you are experiencing routing issues, you can use `mitmproxy` as a **Regular HTTP Proxy**.

### 1. How it works
In this mode, the Pi acts as a simple server on port 8080. You tell your phone's Wi-Fi settings to send all traffic to `198.51.100.1:8080`.

*   **Pros:** No complex `nftables` or NAT rules required.
*   **Cons:** Many Android apps (and background processes) ignore system-wide proxy settings. **HTTPS still requires a trusted CA for decryption.**

### 2. Start mitmproxy in Regular Mode
```bash
# Stop transparent mode first if it's running
# No special flags needed for regular mode
mitmproxy --listen-port 8080
```

### 3. Configure the Phone
1.  Go to **Settings → Wi-Fi → Bose-Lab**.
2.  Select **Modify Network** (or the "i" icon).
3.  Set **Proxy** to **Manual**.
4.  **Proxy hostname:** `198.51.100.1`
5.  **Proxy port:** `8080`
6.  Save and try to browse a site.

---

## Step 13 – Extracting for soundtouch-service

You can extract interactions (especially unencrypted WebSockets on port 8090) from a `.pcap` and format them for use in `soundtouch-service`.

### 1. Extract Traffic using Go

A helper script is provided in `scripts/extract-ws.go`. It automatically detects, unmasks, and decompresses (GZIP) WebSocket frames, and also extracts DNS, MDNS, and SSDP traffic.

```bash
# Install dependencies
go get github.com/google/gopacket

# Run extraction (outputs multiple files: .ws.http, .dns.txt, .mdns.txt, .ssdp.txt)
# The results will be saved beside your .pcap file
go run scripts/extract-ws.go your_capture.pcap [filter_ip]

# Example: Filter for a specific speaker's IP in WebSocket messages
go run scripts/extract-ws.go capture.pcap 203.0.113.1
```

### 2. Manual Extraction with tshark

If you only need a quick look at the payloads:

```bash
# Extract all WebSocket text payloads
tshark -r your_capture.pcap -Y "websocket.payload.text" -T fields -e websocket.payload.text
```

---

## Step 14 – Extracting from Internal App Logs (macOS)

If you are using the macOS app and cannot decrypt the cloud traffic due to pinning, you can still extract the JSON/XML messages from the app's internal communication log.

A helper script is provided in `scripts/extract-log-interactions.go`. It parses the interleaved "Native" and "Network" calls to reconstruct the application's internal state and cloud requests.

```bash
# Run extraction from the log file
# Outputs a chronological record of internal events and network URLs
go run scripts/extract-log-interactions.go path/to/log.txt > extracted-interactions.http
```

**What this shows:**
- **TO NETWORK:** The URLs the app is about to call (intercepted before encryption).
- **FROM NATIVE:** Data being returned from the OS or Cloud to the UI.
- **TO NATIVE:** Commands being sent from the UI to the underlying engines.

This is a powerful "Plan B" when HTTPS decryption is blocked, as the app essentially logs its own decrypted data for you.

---

## Helper Commands / Troubleshooting

After a Pi reboot, everything should come up automatically. If not:

```bash
# Restart and enable all core services
sudo systemctl restart systemd-networkd
sudo systemctl enable --now hostapd
sudo systemctl enable --now dnsmasq
sudo systemctl restart nftables

# Verify the unmanaged state of wlan0 (nmcli)
sudo nmcli device set wlan0 managed no
```

---

## What to Expect

| Protocol             | Port       | Tool                     | Visibility                                     |
|----------------------|------------|--------------------------|------------------------------------------------|
| DNS (Standard)       | UDP 53     | tcpdump, dnsmasq log     | Full, plaintext                                |
| HTTPS / REST         | TCP 443    | tcpdump (SNI), mitmproxy | SNI without decryption, content with mitmproxy |
| WebSockets           | TCP 443/80 | Wireshark                | Frames decoded if TLS is broken                |
| mDNS / ZeroConf      | UDP 5353   | tcpdump, tshark          | Full, plaintext                                |
| SSDP / UPnP          | UDP 1900   | tcpdump                  | Full, plaintext                                |
| SoundTouch local API | TCP 8090   | tcpdump                  | Full, plaintext (no TLS)                       |

> **Expectation for Bose SoundTouch:** The app likely uses standard DNS (older app generation), REST/HTTPS for the pairing flow with the cloud, WebSockets for push events from the device, and mDNS for local device discovery. The local device API on port 8090 is HTTP without TLS – this traffic is always readable.

---

## Next Steps After Analysis

1. Extract domains from DNS log and SNI → List of all Bose endpoints
2. HTTP methods and paths from mitmproxy log → Reconstruct API structure
3. Document auth flow (OAuth2? Proprietary? Token format?)
4. Build a minimal mock server simulating the critical endpoints
5. Testing: App against mock server → does pairing work offline?

---

## Appendix A – Generating a Custom CA Certificate

If you don't have a custom DNS server with a CA yet, you can create one directly on the Pi. Alternatively, if you are already using the `soundtouch-service` from this repository, you can reuse its CA certificate located in the `data/certs/` directory.

### 0. (Optional) Copy an Existing CA from another host

If you are already using the `soundtouch-service` on another machine (e.g., your notebook), you can copy the existing CA to the Pi instead of generating a new one:

```bash
# On your Pi:
sudo mkdir -p /etc/my-dns-ca
sudo chown $USER:$USER /etc/my-dns-ca

# Run this on your notebook (replace hostnames and paths):
# Note: This is easiest if your SSH key is added to the Pi and soundtouch-service host.
# If you run into permission issues with sudo, ensure the source user has passwordless sudo for 'cat'.

# Step A: Download from source to your notebook
ssh soundtouch-service "sudo cat /var/lib/soundtouch-service/certs/ca.crt" > ca.crt
ssh soundtouch-service "sudo cat /var/lib/soundtouch-service/certs/ca.key" > ca.key

# Step B: Upload from notebook to the Pi
scp ca.crt ca.key soundtouch-access-point:/tmp/
ssh soundtouch-access-point "sudo mv /tmp/ca.crt /tmp/ca.key /etc/my-dns-ca/ && sudo chown root:root /etc/my-dns-ca/ca.*"
rm ca.crt ca.key
```

### 1. Create CA Key and Certificate

```bash
sudo mkdir -p /etc/my-dns-ca
cd /etc/my-dns-ca

# Generate CA private key
sudo openssl genrsa -out ca.key 4096

# Generate Root CA certificate
# Note: we explicitly add basicConstraints=CA:TRUE for modern TLS clients
sudo openssl req -x509 -new -nodes -key ca.key -sha256 -days 3650 \
  -out ca.crt \
  -subj "/C=DE/O=Bose-Lab/CN=Bose-Lab Root CA" \
  -addext "basicConstraints=critical,CA:TRUE" \
  -addext "keyUsage=critical,keyCertSign,cRLSign"
```

### 2. Generate a Certificate for Interception (Example)

To intercept `global.api.bose.io`, you need a certificate for it, signed by your CA:

```bash
# Generate server key
sudo openssl genrsa -out bose.key 2048

# Create CSR (Certificate Signing Request) configuration
sudo tee bose.ext << 'EOF'
authorityKeyIdentifier=keyid,issuer
basicConstraints=CA:FALSE
keyUsage = digitalSignature, nonRepudiation, keyEncipherment, dataEncipherment
subjectAltName = @alt_names

[alt_names]
DNS.1 = global.api.bose.io
DNS.2 = *.bose.io
EOF

# Generate CSR
sudo openssl req -new -key bose.key -out bose.csr \
  -subj "/C=DE/O=Bose-Lab/CN=global.api.bose.io"

# Sign the certificate with your CA
sudo openssl x509 -req -in bose.csr -CA ca.crt -CAkey ca.key \
  -CAcreateserial -out bose.crt -days 365 -sha256 -extfile bose.ext
```

### 3. Usage in your DNS/HTTPS Server

Your custom server (e.g., a small Go or Python script) would then use `bose.crt` and `bose.key` to serve HTTPS traffic for those domains.

## Appendix B – Helpful Commands

```bash
# Which IPs did the phone receive?
cat /var/lib/misc/dnsmasq.leases

# Is the access point active?
sudo systemctl status hostapd

# Is dnsmasq active?
sudo systemctl status dnsmasq

# Check interfaces and IPs
ip addr show

# Check routing table
ip route show

# Show active nftables rules
sudo nft list ruleset

# All running tcpdump processes
pgrep -a tcpdump

# Test the Pi's own DNS resolution
dig @127.0.0.1 -p 5353 global.api.bose.io

# Check network connectivity from the phone (from the Pi)
ping 198.51.100.101   # Phone IP from dnsmasq.leases
```
