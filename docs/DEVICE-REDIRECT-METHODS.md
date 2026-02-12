# Device Redirect Methods & Custom Service Setup

To enable offline operation or use custom services like **SoundCork** or **ÜberBöse API**, SoundTouch devices must be redirected from Bose's official cloud endpoints to a local or custom server. This document outlines the three known methods to achieve this, gathered from community reverse-engineering efforts in the **SoundCork** and **ÜberBöse API** projects.

## Overview of Redirection Targets

SoundTouch devices primarily communicate with the following domains:
- `streaming.bose.com`: Marge (Account and streaming services)
- `updates.bose.com`: Software updates
- `stats.bose.com`: Telemetry and analytics
- `bmx.bose.com`: Bose Media eXchange registry

---

## Method 1: XML Configuration Modification (Recommended)

The most robust and granular method involves modifying the device's private configuration file. This is the primary method used by **SoundCork**'s migration logic to redirect devices to a local service instance.

### Technical Details
- **File Path**: `/opt/Bose/etc/SoundTouchSdkPrivateCfg.xml`
- **Mechanism**: The device firmware reads this XML file at boot to determine service URLs.
- **Fields to Modify**:
  - `<margeServerUrl>`: Redirects account/streaming calls.
  - `<statsServerUrl>`: Redirects telemetry.
  - `<swUpdateUrl>`: Redirects update checks.
  - `<bmxRegistryUrl>`: Redirects service discovery.

### Implementation
Requires SSH access to the device.
```xml
<SoundTouchSdkPrivateCfg>
  <margeServerUrl>http://192.168.1.10:8000/marge</margeServerUrl>
  <statsServerUrl>http://192.168.1.10:8000</statsServerUrl>
  <swUpdateUrl>http://192.168.1.10:8000/updates/soundtouch</swUpdateUrl>
  <bmxRegistryUrl>http://192.168.1.10:8000/bmx/registry/v1/services</bmxRegistryUrl>
</SoundTouchSdkPrivateCfg>
```

### Pros & Cons
| Pros | Cons |
| :--- | :--- |
| **Granular Control**: Redirect specific services while leaving others (e.g., updates) intact. | **Requires SSH**: Must have root/SSH access to the device. |
| **Persistent**: Survives software updates (usually). | **Syntax Sensitive**: Errors in XML can cause boot issues or service failures. |
| **Native**: Uses the device's built-in configuration mechanism. | |

---

## Method 2: `/etc/hosts` DNS Override

This method uses the standard Linux hosts file to redirect traffic at the network level within the device. It is often used as a quick alternative in the **ÜberBöse API** community for global redirection.

### Technical Details
- **File Path**: `/etc/hosts`
- **Mechanism**: Overrides DNS resolution for Bose domains to point to a local IP.
- **Resolution Order**: SoundTouch devices use the standard Linux Name Service Switch (`/etc/nsswitch.conf`). The default configuration (`hosts: files dns`) ensures that `/etc/hosts` is consulted *before* any external DNS lookups. This makes the redirection highly reliable for all system processes, including `curl`, `BoseApp`, and `IoT`.

### Implementation
Requires SSH access. Add entries for the target domains:
```text
192.168.1.10  streaming.bose.com
192.168.1.10  updates.bose.com
192.168.1.10  stats.bose.com
```

### Pros & Cons
| Pros | Cons |
| :--- | :--- |
| **Simple**: Easy to understand and implement. | **Requires SSH**: Must have root access. |
| **Universal**: Affects all processes on the device attempting to reach those domains. | **HTTPS Issues**: Redirecting HTTPS domains to a local IP will cause SSL certificate errors unless the device is patched to skip verification or trust a custom CA. |
| | **Brittle**: Some firmware versions may overwrite `/etc/hosts` on reboot. |

---

## Method 3: Binary Patching

A low-level approach where the actual compiled binaries (e.g., `BoseApp`, `IoT`) are modified to change hardcoded URL patterns. Research into these patterns has been documented in both **SoundCork** (Issue #128) and **ÜberBöse API** research.

### Technical Details
- **Target Binaries**: `/opt/Bose/BoseApp`, `/opt/Bose/IoT`, `/opt/Bose/lib/libBmxAccountHsm.so`
- **Mechanism**:
    - **URL Replacement**: Using a hex editor to search for string patterns like `https://streaming.bose.com` and replacing them with a custom URL of the **exact same length**.
    - **Regex Neutralization**: Some libraries (like `libBmxAccountHsm.so`) perform a validation check called `IsItBose` using a hardcoded regex. This regex prevents the device from connecting to non-Bose domains even if the URL is changed in the configuration.

#### The `IsItBose` Regex Patch
Research in the **SoundCork** community (Issue #62) identified a specific regex in `libBmxAccountHsm.so` that enforces Bose/Apigee domain usage:
`^https:\/\/bose-[a-zA-Z0-9\.\_\-\$\%]\+\.apigee\.net\/`

By patching this regex to be more "lax", the device can be made to accept any custom domain.

**Example Patch**:
Using `sed` to replace the strict regex with a broad match while preserving the original string length:
```bash
sed "s#\^https:....bose.\+apigee..net..#http[aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa]*#g" \
< libBmxAccountHsm.so.orig > libBmxAccountHsm.so.patched
```

### Implementation
1. Copy the target binary or library from the device to a PC.
2. Use a hex editor or `sed` to locate and patch the URL strings or regex patterns.
3. Copy the patched file back to the device.
4. Restore execution permissions and reboot.

### Pros & Cons
| Pros | Cons |
| :--- | :--- |
| **Bypass Config**: Works even if the firmware ignores XML settings. | **High Risk**: Modifying binaries can lead to permanent bricks or boot loops. |
| **Hardcoded Redirects**: Can catch URLs that aren't exposed in configuration files. | **Length Constraint**: Custom URLs must fit within the space of the original strings. |
| | **Firmware Specific**: Patches must be reapplied after every software update. |
| | **Complexity**: Requires understanding of binary structures and potential checksums. |

---

## Comparison & Usage Strategy

### Summary Table

| Method | Primary Use Case | Ease | Safety | Persistence | Granularity |
| :--- | :--- | :---: | :---: | :---: | :---: |
| **XML Config** | Logical service redirection | ⭐⭐⭐⭐ | ⭐⭐⭐⭐ | ⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ |
| **`/etc/hosts`** | Quick global DNS override | ⭐⭐⭐⭐⭐ | ⭐⭐⭐ | ⭐⭐ | ⭐⭐ |
| **Binary Patch** | Bypassing hardcoded checks | ⭐ | ⭐ | ⭐ | ⭐⭐⭐ |

---

## Combining Methods: When is one not enough?

A common question is whether these methods can be used in isolation or if they must be combined. The answer depends on your specific firmware version and the target service.

### Scenario A: XML Config Only (The Ideal Case)
If your firmware does not strictly enforce the `IsItBose` check for the specific URLs you are changing, **Method 1 (XML)** is sufficient. This is the cleanest approach and is used by the `soundtouch-service` migration tool.

### Scenario B: XML Config + Binary Patching (The "Locked" Case)
On some newer firmware versions, even if you change the `<margeServerUrl>` in the XML to `http://192.168.1.10`, the internal library (`libBmxAccountHsm.so`) will validate the string against the hardcoded Bose regex.
*   **Symptom**: The device ignores the XML setting or fails to connect despite the correct URL being present.
*   **Solution**: You **must** apply the **Binary Patch (Method 3)** to neutralize the `IsItBose` check *in addition* to the XML change.

### Scenario C: `/etc/hosts` + Custom CA (The "Clean Deep Redirect")
If you use `/etc/hosts` to point `streaming.bose.com` to a local IP and want to avoid binary patching.
*   **Requirement 1**: Your local server must handle HTTPS (port 443).
*   **Requirement 2**: You must inject your Root CA into the device's trust store.
*   **Automated Tool**: The `soundtouch-service` now supports this via the `/setup/migrate/{deviceIP}?method=hosts` endpoint.
*   **CA Download**: You can download the auto-generated Root CA from `http://<your-server>:8000/setup/ca.crt`.
*   **Benefit**: Maintains system integrity (no binary changes) and full end-to-end encryption.

### Scenario D: `/etc/hosts` + Binary Patching (The "Legacy Deep Redirect")
If you cannot or do not want to manage certificates, but still use `/etc/hosts` for DNS redirection.
*   **Requirement 1**: Your local server must handle HTTPS (port 443).
*   **Requirement 2**: Since the certificate will be invalid (mismatched domain/CA), you must patch the binary to **skip SSL verification** (see [Option 2](#option-2-ssl-verification-bypass) below).
*   **Risk**: Less secure and higher risk of bricking due to binary modification.

### Scenario E: The Triple-Threat (Total Control)
For developers creating a completely isolated "dark" environment (no internet at all):
1.  **XML**: Point all URLs to local services.
2.  **Binary Patch**: Neutralize `IsItBose` to allow non-Bose domains/IPs.
3.  **`/etc/hosts`**: Redirect hardcoded domains that aren't exposed in the XML (like analytics or NTP) to prevent leakage to the real Bose cloud.
4.  **Process Instrumentation**: Use [SoundTouch Hook](https://github.com/CodeFinder2/bose-soundtouch-hook) to monitor and override internal behavior in real-time.

---

## Handling HTTPS & SSL Certificates

When redirecting HTTPS traffic to a custom service, SoundTouch devices will fail the SSL handshake because they do not trust your local server's certificate.

### Option 1: Custom CA Certificate (Recommended)

As suggested by community members, you can configure the device to trust your own Root CA. This allows for secure HTTPS communication without patching binaries.

**Technical Steps**:
1.  **Generate a Root CA** and issue a certificate for the target domain (e.g., `streaming.bose.com`).
2.  **SSH into the device** and copy your `rootCA.crt` to `/usr/share/ca-certificates/custom/`.
3.  **Update the Trust Store**:
    - **Method A (Append to Bundle)**: `cat /usr/share/ca-certificates/custom/rootCA.crt >> /etc/pki/tls/certs/ca-bundle.crt`
    - **Method B (Symlinks)**: Add the certificate to `/etc/ssl/certs/` and create a hash symlink using `c_rehash` (if available) or manual mapping.

**Pros & Cons**:
| Pros | Cons |
| :--- | :--- |
| **Secure**: Maintains end-to-end encryption. | **Requires SSH**: Must have root access to modify the trust store. |
| **Clean**: No binary patching required for SSL bypass. | **Update Risk**: Firmware updates might overwrite the `ca-bundle.crt`. |

### Option 2: SSL Verification Bypass

If you cannot or do not want to manage certificates, you can patch the binary to skip certificate verification.

**Target**: `libBmxAccountHsm.so` or `BoseApp`
**Mechanism**: Locating the SSL verification function (often in the internal curl-based or openssl-based logic) and forcing it to return "Success" regardless of the certificate status.

---

## Recommendation

1.  **Start with Method 1 (XML Modification)**. It is the least invasive and most likely to work across different models.
2.  **Verify connectivity**. If the device refuses to connect to your custom endpoint, check logs for "IsItBose" or validation failures.
3.  **Apply Method 3 (Binary Patching)** only if Method 1 is being actively blocked by the firmware's validation logic.
4.  **Avoid Method 2 (`/etc/hosts`)** unless you are prepared to handle SSL certificate complexities or are performing quick temporary tests.
