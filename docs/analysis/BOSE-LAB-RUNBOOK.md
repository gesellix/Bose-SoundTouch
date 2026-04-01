# Bose SoundTouch – Traffic Analysis Runbook

> **Goal:** Set up a Raspberry Pi as a transparent access point to fully observe the traffic of the Bose SoundTouch app – specifically the pairing flow with the Bose Cloud. This serves as a basis for later reverse engineering / simulation of the cloud endpoints.

---

## Prerequisites

| Component          | Details                                                    |
|--------------------|------------------------------------------------------------|
| Raspberry Pi       | Pi 3 or newer, Raspberry Pi OS (Bullseye or newer)         |
| Network interfaces | `eth0` → LAN cable to FritzBox, `wlan0` → own Access Point |
| FritzBox           | Unchanged, assigns an IP to the Pi via DHCP on eth0        |
| Custom DNS Server  | Already present, incl. custom CA certificate               |
| Phone              | Android, connects to the Pi's Wi-Fi                        |

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
  iptables \         # NAT / firewall / forwarding
  iptables-persistent \ # Save rules across reboots
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
echo "net.ipv4.ip_forward=1" | sudo tee -a /etc/sysctl.conf
```

---

## Step 3 – Static IP on wlan0

`wlan0` gets a fixed IP – this is the gateway for the phone.

```bash
# Append to /etc/dhcpcd.conf
sudo tee -a /etc/dhcpcd.conf << 'EOF'

interface wlan0
  static ip_address=192.168.10.1/24
  nohook wpa_supplicant   # wlan0 becomes AP, not Wi-Fi client
EOF

sudo systemctl restart dhcpcd
```

**Verify:**
```bash
ip addr show wlan0
# Expected: inet 192.168.10.1/24
```

---

## Step 4 – hostapd (Access Point)

```bash
sudo tee /etc/hostapd/hostapd.conf << 'EOF'
interface=wlan0
driver=nl80211
ssid=Bose-Lab             # Wi-Fi name – phone connects here
hw_mode=g
channel=6
wmm_enabled=0
auth_algs=1
wpa=2
wpa_passphrase=secret123  # Adjust password
wpa_key_mgmt=WPA-PSK
wpa_pairwise=CCMP
EOF

# Enter config path
sudo sed -i \
  's|#DAEMON_CONF=""|DAEMON_CONF="/etc/hostapd/hostapd.conf"|' \
  /etc/default/hostapd

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
interface=wlan0                              # Only listen on AP interface
dhcp-range=192.168.10.100,192.168.10.200,24h # IP pool for clients
dhcp-option=3,192.168.10.1                  # Gateway = Pi
dhcp-option=6,192.168.10.1                  # DNS = Pi (custom DNS server)

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

## Step 6 – NAT and Forwarding (iptables)

The Pi routes the phone's traffic to the FritzBox and back.

```bash
# NAT: outgoing packets get the Pi's IP (eth0)
sudo iptables -t nat -A POSTROUTING -o eth0 -j MASQUERADE

# Forwarding: Phone → Internet
sudo iptables -A FORWARD -i wlan0 -o eth0 -j ACCEPT

# Forwarding: Responses back to the phone
sudo iptables -A FORWARD -i eth0 -o wlan0 \
  -m state --state RELATED,ESTABLISHED -j ACCEPT

# Save rules permanently (iptables-persistent)
sudo netfilter-persistent save
```

**Verify:**
```bash
sudo iptables -t nat -L -n -v
# Expected: MASQUERADE rule on POSTROUTING for eth0
```

---

## Step 7 – Install Custom CA Certificate on the Phone

Since a custom DNS server with a custom CA certificate is used, it must be trusted on the phone – otherwise, the app will block HTTPS connections to redirected domains.

### Copy CA Certificate to the Pi (if not already there)

```bash
# Certificate is located e.g. at /etc/my-dns-ca/ca.crt
# Temporarily make reachable via HTTP for easy download:
cd /etc/my-dns-ca/
python3 -m http.server 8080
# → Reachable at http://192.168.10.1:8080/ca.crt
```

### Install on Android

1. Connect phone to `Bose-Lab`
2. Open browser → `http://192.168.10.1:8080/ca.crt`
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
sudo tcpdump -i wlan0 -n host 192.168.10.101
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
scp pi@192.168.10.1:/tmp/bose-*.pcap ~/Desktop/
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

Only useful if the CA certificate on the phone is trusted and no certificate pinning is active.

```bash
sudo apt install -y mitmproxy

# Transparent proxy on port 8080
mitmproxy --mode transparent --listen-port 8080

# Alternatively: mitmdump for automatic logging to file
mitmdump --mode transparent --listen-port 8080 \
  -w /tmp/bose-https.mitm
```

**iptables rule: redirect HTTPS traffic to mitmproxy**

```bash
# Only for wlan0 traffic (phone) → Port 443 → mitmproxy on 8080
sudo iptables -t nat -A PREROUTING \
  -i wlan0 -p tcp --dport 443 \
  -j REDIRECT --to-port 8080
```

**Remove rule when no longer needed:**

```bash
sudo iptables -t nat -D PREROUTING \
  -i wlan0 -p tcp --dport 443 \
  -j REDIRECT --to-port 8080
```

> **Detecting Certificate Pinning:** If the app immediately disconnects after mitmproxy redirection (connection reset directly after TLS ClientHello), pinning is active. In this case, Frida + root is needed to patch the pinning.

---

## Helper Commands / Troubleshooting

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

# Show active iptables rules
sudo iptables -L -n -v
sudo iptables -t nat -L -n -v

# All running tcpdump processes
pgrep -a tcpdump

# Test the Pi's own DNS resolution
dig @127.0.0.1 -p 5353 global.api.bose.io

# Check network connectivity from the phone (from the Pi)
ping 192.168.10.101   # Phone IP from dnsmasq.leases
```

---

## Restart Sequence

After a Pi reboot, everything should come up automatically. If not:

```bash
sudo systemctl start dhcpcd
sudo systemctl start hostapd
sudo systemctl start dnsmasq
sudo netfilter-persistent reload
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
