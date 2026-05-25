---
title: "MAC Address to Serial Number Mapping"
---
**Understanding and troubleshooting device identification in SoundTouch service**

This guide explains how the SoundTouch service handles device identification through MAC address to serial number mapping, and how to troubleshoot related issues.

## 📋 **Overview**

The SoundTouch service uses two different identifiers for devices:

- **MAC Address** (`AABBCCDDEEFF`) - Used in HTTP API requests and UPnP discovery
- **Serial Number** (`I6332527703739342000020`) - Used for internal file storage

The service automatically maps between these identifiers so that API requests using MAC addresses can access files stored using serial numbers.

## 🔍 **How It Works**

### Request Flow
```
1. HTTP Request: GET /streaming/account/1000001/device/AABBCCDDEEFF/presets
2. MAC Resolution: AABBCCDDEEFF → I6332527703739342000020
3. File Access: accounts/1000001/devices/I6332527703739342000020/Presets.xml
```

### UPnP Discovery Integration
The service extracts MAC addresses from UPnP device descriptions:

```xml
<!-- From http://192.0.2.100:8091/XD/BO5EBO5E-F00D-F00D-FEED-AABBCCDDEEFF.xml -->
<root xmlns="urn:schemas-upnp-org:device-1-0">
    <device>
        <friendlyName>Sound Machinery</friendlyName>
        <modelName>SoundTouch 10</modelName>
        <serialNumber>AABBCCDDEEFF</serialNumber>  <!-- MAC address here -->
    </device>
</root>
```

## ⚙️ **Automatic Setup**

The mapping is created automatically when the service starts:

1. **Directory Scan**: Service scans `data/accounts/{account}/devices/{serial}/`
2. **DeviceInfo.xml**: Reads MAC address from each device's info file
3. **Mapping Creation**: Creates MAC → Serial mapping in memory
4. **Normalization**: Handles different MAC address formats automatically

## 🛠️ **Supported MAC Address Formats**

The service handles all common MAC address formats automatically:

| Format      | Example             | Status      |
|-------------|---------------------|-------------|
| Standard    | `AABBCCDDEEFF`      | ✅ Supported |
| Lowercase   | `a81b6a536a98`      | ✅ Supported |
| With Colons | `AA:BB:CC:DD:EE:FF` | ✅ Supported |
| With Dashes | `AA-BB-CC-DD-EE-FF` | ✅ Supported |
| Mixed Case  | `a81B6a536A98`      | ✅ Supported |
| With Spaces | ` AABBCCDDEEFF `    | ✅ Supported |

## 🔧 **Troubleshooting**

### Problem: API requests fail with "file not found" errors

**Symptoms:**
```
GET /streaming/account/1000001/device/AABBCCDDEEFF/presets
→ 500 Internal Server Error
→ Log: "open .../devices/AABBCCDDEEFF/Presets.xml: no such file or directory"
```

**Diagnosis:**
1. Check if mapping exists:
   ```bash
   # Look for device directory
   ls data/accounts/1000001/devices/
   # Should show serial numbers like: I6332527703739342000020
   ```

2. Check DeviceInfo.xml:
   ```bash
   cat data/accounts/1000001/devices/I6332527703739342000020/DeviceInfo.xml
   # Look for <macAddress> field
   ```

**Solutions:**

#### Solution 1: Restart the Service
The mapping is created at startup. Simply restart:
```bash
sudo systemctl restart soundtouch-service
```

#### Solution 2: Check DeviceInfo.xml Format
Ensure the MAC address is present:
```xml
<info deviceID="I6332527703739342000020">
    <networkInfo type="SCM">
        <macAddress>AABBCCDDEEFF</macAddress>  <!-- Must be present -->
        <ipAddress>192.0.2.10</ipAddress>
    </networkInfo>
</info>
```

#### Solution 3: Manual Device Addition
If the device was added manually, ensure proper structure:
```bash
# Create device directory using serial number
mkdir -p data/accounts/1000001/devices/I6332527703739342000020

# Create DeviceInfo.xml with MAC address
cat > data/accounts/1000001/devices/I6332527703739342000020/DeviceInfo.xml << EOF
<?xml version="1.0" encoding="UTF-8"?>
<info deviceID="I6332527703739342000020">
    <name>My SoundTouch Device</name>
    <networkInfo type="SCM">
        <macAddress>AABBCCDDEEFF</macAddress>
        <ipAddress>192.0.2.100</ipAddress>
    </networkInfo>
</info>
EOF
```

### Problem: UPnP discovery not creating mappings

**Check UPnP accessibility:**
```bash
# Test UPnP endpoint directly
curl http://192.0.2.100:8091/XD/BO5EBO5E-F00D-F00D-FEED-AABBCCDDEEFF.xml

# Should return XML with <serialNumber> field
```

**Enable debug logging:**
```bash
# Check service logs for UPnP activity
journalctl -u soundtouch-service -f | grep UPnP
```

### Problem: Case or format mismatches

This should be handled automatically, but you can verify:

**Test different formats:**
```bash
# All of these should work the same:
curl http://localhost:8000/streaming/account/1000001/device/AABBCCDDEEFF/presets
curl http://localhost:8000/streaming/account/1000001/device/a81b6a536a98/presets
curl http://localhost:8000/streaming/account/1000001/device/AA:BB:CC:DD:EE:FF/presets
```

## 📊 **Monitoring and Diagnostics**

### Check Current Mappings
The service logs mapping creation at startup:
```bash
journalctl -u soundtouch-service | grep "MAC.*serial"
```

### Verify File Structure
Ensure proper directory organization:
```
data/
└── accounts/
    └── 1000001/
        └── devices/
            └── I6332527703739342000020/     # Serial number directory
                ├── DeviceInfo.xml           # Contains MAC address
                ├── Presets.xml
                └── Sources.xml
```

## 🔗 **Related Documentation**

- [Device Initial Setup](DEVICE-INITIAL-SETUP.md) - Setting up new devices
- [Troubleshooting Guide](TROUBLESHOOTING.md) - General troubleshooting steps
- [SoundTouch Service](SOUNDTOUCH-SERVICE.md) - Service configuration and management

## 🏗️ **Technical Implementation**

For developers interested in the technical details:

### Normalization Algorithm
```go
// MAC addresses are normalized by:
// 1. Removing spaces, colons, and dashes
// 2. Converting to uppercase
// Examples:
// "a8:1b:6a:53:6a:98" → "AABBCCDDEEFF"
// "AA-BB-CC-DD-EE-FF" → "AABBCCDDEEFF"
```

### Lookup Process
```go
// 1. Try exact match first
// 2. If not found, try normalized version
// 3. Return serial number for file access
```

### Performance
- **Lookup Time**: O(1) - Hash map lookup
- **Memory Usage**: ~40 bytes per device mapping
- **Initialization**: Scans all devices once at startup

## 📝 **Best Practices**

1. **Use Discovery**: Let UPnP discovery create mappings automatically
2. **Consistent Format**: Store MAC addresses consistently in DeviceInfo.xml
3. **Service Restart**: Restart service after manual device additions
4. **Monitoring**: Check logs for mapping creation during startup
5. **Backup**: Keep DeviceInfo.xml files backed up

## ⚠️ **Known Limitations**

- Mappings are created only at service startup
- Manual device additions require service restart
- MAC addresses must be present in DeviceInfo.xml
- No automatic cleanup of stale mappings (restart required)
