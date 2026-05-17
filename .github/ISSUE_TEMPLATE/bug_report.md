---
name: Bug report
about: Create a report to help us improve
title: ''
labels: 'bug'
assignees: ''

---

**Describe the bug**
A clear and concise description of what the bug is.

**To Reproduce**
Steps to reproduce the behavior:
1. Go to '...'
2. Click on '....'
3. Scroll down to '....'
4. See error

**Expected behavior**
A clear and concise description of what you expected to happen.

**Environment (please complete the following information):**
- OS: [e.g. macOS 14.0, Windows 11, Ubuntu 22.04]
- Go version: [e.g. 1.25.5]
- Library version: [e.g. v1.0.0, commit hash if using main branch]
- SoundTouch device model: [e.g. SoundTouch 10, SoundTouch 20]
- Device firmware version: [if known]

**Command/Code that failed**
```bash
# If using CLI tool, provide the exact command
soundtouch-cli --host 192.0.2.100 info get

# If using Go library, provide minimal code example
```

**Error output**
```
Paste the complete error message here, including stack traces if available
```

**Device Information (if applicable)**
```xml
<!-- If the issue is device-specific, include output from: -->
<!-- soundtouch-cli --host YOUR_DEVICE_IP info get -->
```

**Network Configuration**
- Network setup: [e.g. home WiFi, corporate network, VPN]
- Firewall/proxy: [any network restrictions]
- Device connectivity: [how device connects to network - WiFi, Ethernet]

**Additional context**
Add any other context about the problem here. For example:
- Does this happen consistently or intermittently?
- Did this work in a previous version?
- Are there any workarounds?
- Any relevant log files or debug output

**Logs (if applicable)**
```
# Enable verbose logging with --verbose flag or debug environment variable
# and paste relevant log output here
```

**Screenshots**
If applicable, add screenshots to help explain your problem.

---

**Checklist**
- [ ] I have searched existing issues to avoid duplicates
- [ ] I have tested with the latest version
- [ ] I have included all relevant environment information
- [ ] I have provided a minimal reproduction case
- [ ] I have included complete error messages