---
title: "Device Customization Setup Guide"
sidebar:
  exclude: true
---
This guide documents the manual steps required to configure your Bose SoundTouch device for customization using the SoundCork approach.

Based on: https://github.com/deborahgu/soundcork

## Overview

SoundCork allows you to customize your SoundTouch device by intercepting and modifying its firmware update process. This requires specific manual configuration steps to prepare your device.

## Prerequisites

- Bose SoundTouch device
- Network access to device
- Administrative access to your router/network

## Configuration Steps

### Step 1: Prepare USB Drive
- Insert USB stick into computer
- Create remote services file: `touch /path/to/mounted/usb/root-directory/remote_services`

### Step 2: Connect to Device
- Insert USB stick into SoundTouch 20 device
- Restart device (unplug power, plug it back in)

### Step 3: Access Device via SSH or Telnet

After the restart, remote access is enabled.

#### Option A: SSH
- SSH access: `ssh -oHostKeyAlgorithms=ssh-rsa root@<device-ip>`
- Device will show network interfaces and system info
- No password required for root access

Example output:
```text
gesellix@Mac Bose-SoundTouch % ssh -oHostKeyAlgorithms=ssh-rsa root@<device-ip>
Last login: Sun Feb  1 19:12:47 2026
eth0      Link encap:Ethernet  HWaddr CA:FE:BA:BE:A3:25
          inet addr:<device-ip>  Bcast:0.0.0.0  Mask:255.255.255.0
lo        Link encap:Local Loopback
          inet addr:127.0.0.1  Mask:255.0.0.0
usb0      Link encap:Ethernet  HWaddr CA:FE:BA:BE:1E:47
          inet addr:123.12.123.12  Bcast:0.0.0.0  Mask:255.255.255.252

Sun Feb  1 20:35:24 CET 2026

Device name: "Kitchen SoundTouch"
Country EU, Region (not set)
Module type: scm
root@spotty:~#
```

#### Option B: Telnet via Docker
If you don't have a telnet client installed, you can use Docker:
```bash
docker run --rm -it alpine:edge ash -c 'apk add -U inetutils-telnet && telnet <device-ip> 23'
```

Example output:
```text
Trying <device-ip>...
Connected to <device-ip>.
Escape character is '^]'.

... --- ..- -. -.. - --- ..- -.-. ....

        ____  ____  _____ _________
       / __ )/ __ \/ ___// _______/
      / __  / / / /\__ \/ __/
 ____/ /_/ / /_/ /___/ / /___
/_________/\____//____/_____/


spotty login: root
eth0      Link encap:Ethernet  HWaddr CA:FE:BA:BE:A3:25
          inet addr:<device-ip>  Bcast:0.0.0.0  Mask:255.255.255.0
lo        Link encap:Local Loopback
          inet addr:127.0.0.1  Mask:255.0.0.0
usb0      Link encap:Ethernet  HWaddr CA:FE:BA:BE:1E:47
          inet addr:123.12.123.12  Bcast:0.0.0.0  Mask:255.255.255.252

Sun Feb  1 19:12:47 CET 2026

Device name: "Kitchen SoundTouch"
Country EU, Region (not set)
Module type: scm
root@spotty:~#
```

### Step 4: Check Current Configuration
- View current configuration: `cat /opt/Bose/etc/SoundTouchSdkPrivateCfg.xml`
- Note the URLs for streaming, stats, software updates, and BMX registry

## Notes

- Keep your device's original firmware backed up
- Ensure stable network connection during setup
- Document your device's current firmware version before starting

## Troubleshooting

*Common issues and solutions will be added here...*
