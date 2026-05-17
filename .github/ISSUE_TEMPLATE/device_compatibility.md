---
name: Device compatibility report
about: Report compatibility with a new SoundTouch device model
title: 'Device Compatibility: [Device Model]'
labels: 'compatibility, documentation'
assignees: ''

---

**Device Information**
- **Model**: [e.g. SoundTouch 30, Wave SoundTouch IV, SoundTouch Portable]
- **Model Number**: [e.g. 738102-2100, found on device label]
- **Firmware Version**: [if known, from device settings or API response]
- **Purchase Date**: [approximate, helps identify firmware generation]

**Testing Results**

### Basic Functionality
- [ ] Device discovery (UPnP/mDNS)
- [ ] Basic device info (`GET /info`)
- [ ] Now playing status (`GET /now_playing`)
- [ ] Media controls (play/pause/stop)
- [ ] Volume control
- [ ] Source listing (`GET /sources`)

### Advanced Features
- [ ] Bass control (`GET/POST /bass`)
- [ ] Balance control (`GET/POST /balance`) - if stereo device
- [ ] Clock/time management (`GET/POST /clockTime`)
- [ ] Network information (`GET /networkInfo`)
- [ ] WebSocket events
- [ ] Multiroom zones (master)
- [ ] Multiroom zones (slave)

### Advanced Audio Controls (Professional/High-end Models)
- [ ] DSP controls (`GET/POST /audiodspcontrols`)
- [ ] Tone controls (`GET/POST /audioproducttonecontrols`)
- [ ] Level controls (`GET/POST /audioproductlevelcontrols`)

### Known Issues
List any features that don't work or behave unexpectedly:
- Feature name: Description of issue
- Command that fails: `soundtouch-cli command that doesn't work`

**Device Info Output**
```xml
<!-- Paste output from: soundtouch-cli --host YOUR_DEVICE_IP info get -->
<!-- This helps us understand device capabilities and variants -->
```

**Device Capabilities Output**
```xml
<!-- Paste output from: soundtouch-cli --host YOUR_DEVICE_IP capabilities -->
<!-- This shows what features the device reports as available -->
```

**Bass Capabilities (if supported)**
```xml
<!-- Paste output from: soundtouch-cli --host YOUR_DEVICE_IP bass capabilities -->
<!-- Only if the device supports bass control -->
```

**Available Sources**
```xml
<!-- Paste output from: soundtouch-cli --host YOUR_DEVICE_IP source list -->
<!-- Shows what audio sources this device supports -->
```

**Testing Commands Used**
```bash
# List the specific commands you used for testing
soundtouch-cli --host 192.0.2.100 info get
soundtouch-cli --host 192.0.2.100 play start
# ... etc
```

**Environment**
- **OS**: [e.g. macOS 14.0, Windows 11, Ubuntu 22.04]
- **Go version**: [e.g. 1.25.5]
- **Library version**: [e.g. v1.0.0, commit hash]
- **Network setup**: [home WiFi, corporate, etc.]

**Performance Notes**
- Response times: [normal, slow, timeouts]
- Specific timeouts: [any endpoints that timeout]
- WebSocket stability: [connects reliably, frequent disconnects, etc.]

**Comparison with Tested Models**
If you have experience with other SoundTouch models:
- **Similar to**: [e.g. works like SoundTouch 20]
- **Differences from**: [e.g. missing balance control compared to SoundTouch 30]

**Additional Notes**
Any other observations about device behavior, quirks, or special considerations:
- Does the device have unique features not seen in other models?
- Are there any setup requirements or configuration notes?
- Does it work differently in different network environments?

**Documentation Impact**
- [ ] Update supported devices list
- [ ] Add device-specific notes to documentation
- [ ] Update compatibility matrix
- [ ] Add to integration test suite

---

**Checklist**
- [ ] I have tested basic functionality (info, play, volume)
- [ ] I have tested advanced features available on this device
- [ ] I have provided complete device information output
- [ ] I have noted any issues or limitations
- [ ] I have tested in a typical network environment
- [ ] I understand this helps improve compatibility for all users