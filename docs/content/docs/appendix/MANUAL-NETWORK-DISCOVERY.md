---
title: "Manual Network Discovery on macOS"
sidebar:
  exclude: true
---
This document provides comprehensive guidance for manually discovering network services and devices using built-in macOS tools and command-line utilities. This is particularly useful for troubleshooting network discovery issues or understanding what services are available on your local network.

## Overview

Network service discovery typically relies on two main protocols:

- **mDNS (Multicast DNS)** - Used by Apple devices, printers, and many local services
- **SSDP (Simple Service Discovery Protocol)** - Used by UPnP devices, media servers, and smart home devices

## mDNS (Multicast DNS) Discovery

**Multicast Address:** `224.0.0.251:5353`

mDNS is the underlying protocol for Bonjour/Zeroconf services. It allows devices to advertise services on the local network using `.local` domain names.

### Built-in Tools (Recommended)

macOS includes `dns-sd`, a powerful command-line tool for service discovery:

```bash
# Browse for all available service types
dns-sd -B _services._dns-sd._udp local.

# Browse for specific service types
dns-sd -B _http._tcp local.           # Web servers
dns-sd -B _airplay._tcp local.        # AirPlay devices
dns-sd -B _ipp._tcp local.           # Internet Printing Protocol
dns-sd -B _soundtouch._tcp local.    # Bose SoundTouch devices
dns-sd -B _ssh._tcp local.           # SSH servers
dns-sd -B _afpovertcp._tcp local.    # AFP file sharing

# Resolve a specific service to get IP address and port
dns-sd -L "ServiceName" _http._tcp local.

# Register a test service (useful for testing)
dns-sd -R "TestService" _http._tcp local 8080

# Query for a specific record type
dns-sd -Q hostname.local A          # Get IPv4 address
dns-sd -Q hostname.local AAAA       # Get IPv6 address
```

### Using dig Command

The `dig` command can also query mDNS directly:

```bash
# Query for a specific hostname
dig @224.0.0.251 -p 5353 hostname.local

# Query for all service types
dig @224.0.0.251 -p 5353 _services._dns-sd._udp.local PTR

# Query for specific service instances
dig @224.0.0.251 -p 5353 _http._tcp.local PTR

# Get detailed information with additional records
dig @224.0.0.251 -p 5353 _soundtouch._tcp.local PTR +additional
```

### Advanced mDNS Monitoring

```bash
# Monitor all mDNS traffic (requires sudo)
sudo tcpdump -i any -n -s 0 'port 5353'

# Monitor specific service announcements
sudo tcpdump -i any -n -s 0 -A 'port 5353 and host 224.0.0.251'

# Monitor with human-readable timestamps
sudo tcpdump -i any -n -s 0 -t -A 'port 5353'
```

### With Homebrew (Optional)

For additional tools, you can install Avahi:

```bash
brew install avahi

# Browse all services
avahi-browse -a

# Browse with verbose details
avahi-browse -a -v -t

# Browse only for a limited time
avahi-browse -a -t --timeout=10

# Resolve a specific service
avahi-resolve -n hostname.local

# Publish a test service
avahi-publish -s "Test Service" _http._tcp 8080
```

## SSDP (Simple Service Discovery Protocol)

**Multicast Address:** `239.255.255.250:1900`

SSDP is used by UPnP devices to advertise and discover services. It uses HTTP-like messages over UDP multicast.

### Active Discovery (M-SEARCH)

This method sends out discovery requests and waits for responses:

**Terminal 1 - Capture responses:**
```bash
# Monitor all SSDP traffic
sudo tcpdump -i any -n -A 'udp port 1900'

# Monitor with better formatting
sudo tcpdump -i any -n -s 0 -A 'udp port 1900' | grep -E '(M-SEARCH|HTTP|NOTIFY|ST:|USN:|LOCATION:)'
```

**Terminal 2 - Send discovery requests:**
```bash
# Basic discovery for all devices
echo -e "M-SEARCH * HTTP/1.1\r\nHost:239.255.255.250:1900\r\nST:ssdp:all\r\nMan:\"ssdp:discover\"\r\nMX:3\r\n\r\n" | nc -u 239.255.255.250 1900

# Search for specific device types
echo -e "M-SEARCH * HTTP/1.1\r\nHost:239.255.255.250:1900\r\nST:urn:schemas-upnp-org:device:MediaRenderer:1\r\nMan:\"ssdp:discover\"\r\nMX:3\r\n\r\n" | nc -u 239.255.255.250 1900

# Search for root devices only
echo -e "M-SEARCH * HTTP/1.1\r\nHost:239.255.255.250:1900\r\nST:upnp:rootdevice\r\nMan:\"ssdp:discover\"\r\nMX:3\r\n\r\n" | nc -u 239.255.255.250 1900

# Search with longer timeout for slow devices
echo -e "M-SEARCH * HTTP/1.1\r\nHost:239.255.255.250:1900\r\nST:ssdp:all\r\nMan:\"ssdp:discover\"\r\nMX:10\r\n\r\n" | nc -u 239.255.255.250 1900
```

### Passive Listening (NOTIFY messages)

Devices periodically send NOTIFY messages to announce their presence:

```bash
# Simple listening (may miss some messages)
nc -ul 1900

# More reliable listening with proper multicast join
# First, install socat if not available
brew install socat

# Listen to multicast SSDP traffic
socat - UDP4-RECVFROM:1900,ip-add-membership=239.255.255.250:0.0.0.0,fork

# Alternative: bind to specific interface
socat - UDP4-RECVFROM:1900,ip-add-membership=239.255.255.250:en0,fork
```

### Python Script for SSDP Discovery

For more reliable and detailed discovery, use this Python script:

```python
#!/usr/bin/env python3
"""
SSDP Discovery Script
Sends M-SEARCH requests and collects responses from UPnP devices.
"""

import socket
import time
import re
from urllib.parse import urlparse

# M-SEARCH message for discovering all SSDP devices
MSEARCH_MSG = \
    'M-SEARCH * HTTP/1.1\r\n' \
    'HOST:239.255.255.250:1900\r\n' \
    'ST:ssdp:all\r\n' \
    'MX:3\r\n' \
    'MAN:"ssdp:discover"\r\n' \
    '\r\n'

def discover_devices(timeout=5, retries=2):
    """Discover UPnP devices using SSDP."""
    devices = {}
    
    for attempt in range(retries):
        print(f"\n--- Discovery attempt {attempt + 1} ---")
        
        # Create UDP socket
        sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM, socket.IPPROTO_UDP)
        sock.settimeout(timeout)
        
        try:
            # Send M-SEARCH request
            sock.sendto(MSEARCH_MSG.encode(), ('239.255.255.250', 1900))
            
            # Collect responses
            start_time = time.time()
            while time.time() - start_time < timeout:
                try:
                    data, addr = sock.recvfrom(8192)
                    response = data.decode('utf-8', errors='ignore')
                    
                    # Parse the response
                    device_info = parse_ssdp_response(response, addr)
                    if device_info:
                        # Use USN as unique identifier
                        usn = device_info.get('USN', f"{addr[0]}:unknown")
                        devices[usn] = device_info
                        
                except socket.timeout:
                    continue
                except Exception as e:
                    print(f"Error receiving data: {e}")
                    continue
                    
        except Exception as e:
            print(f"Discovery attempt {attempt + 1} failed: {e}")
        finally:
            sock.close()
    
    return devices

def parse_ssdp_response(response, addr):
    """Parse SSDP response and extract device information."""
    lines = response.split('\r\n')
    
    # Check if it's a valid HTTP response
    if not lines[0].startswith('HTTP/1.1 200 OK'):
        return None
    
    device_info = {
        'IP': addr[0],
        'Port': addr[1],
        'Raw': response
    }
    
    # Parse headers
    for line in lines[1:]:
        if ':' in line:
            key, value = line.split(':', 1)
            device_info[key.strip().upper()] = value.strip()
    
    return device_info

def print_device_summary(devices):
    """Print a summary of discovered devices."""
    if not devices:
        print("\nNo devices discovered.")
        return
    
    print(f"\n--- Discovered {len(devices)} devices ---")
    
    for usn, device in devices.items():
        print(f"\nDevice: {device.get('SERVER', 'Unknown')}")
        print(f"  IP: {device['IP']}")
        print(f"  USN: {device.get('USN', 'N/A')}")
        print(f"  ST: {device.get('ST', 'N/A')}")
        
        location = device.get('LOCATION')
        if location:
            parsed = urlparse(location)
            print(f"  Location: {location}")
            print(f"  Host: {parsed.hostname}:{parsed.port}")

def print_detailed_info(devices):
    """Print detailed information for all devices."""
    for i, (usn, device) in enumerate(devices.items(), 1):
        print(f"\n{'='*60}")
        print(f"Device {i}: {device['IP']}")
        print(f"{'='*60}")
        print(device['Raw'])

if __name__ == "__main__":
    print("SSDP Device Discovery")
    print("Searching for UPnP devices on the network...")
    
    # Discover devices
    devices = discover_devices(timeout=5, retries=2)
    
    # Print results
    print_device_summary(devices)
    
    # Ask if user wants detailed info
    if devices:
        response = input("\nShow detailed device information? (y/N): ")
        if response.lower() == 'y':
            print_detailed_info(devices)
```

Save this script and run it:

```bash
# Save the script
cat > ssdp_discovery.py << 'EOF'
# [paste the Python script above]
EOF

# Make it executable
chmod +x ssdp_discovery.py

# Run the discovery
python3 ssdp_discovery.py
```

### SSDP Message Types

Understanding SSDP message types helps interpret the traffic:

**M-SEARCH Request:**
```
M-SEARCH * HTTP/1.1
HOST:239.255.255.250:1900
ST:ssdp:all
MAN:"ssdp:discover"
MX:3
```

**NOTIFY Advertisement:**
```
NOTIFY * HTTP/1.1
HOST:239.255.255.250:1900
CACHE-CONTROL:max-age=1800
LOCATION:http://192.0.2.100:8090/device_description.xml
NT:upnp:rootdevice
NTS:ssdp:alive
USN:uuid:12345678-1234-1234-1234-123456789012::upnp:rootdevice
```

**HTTP Response:**
```
HTTP/1.1 200 OK
CACHE-CONTROL:max-age=1800
DATE:Wed, 18 Dec 2024 10:30:00 GMT
EXT:
LOCATION:http://192.0.2.100:8090/device_description.xml
SERVER:Linux/3.0 UPnP/1.0 Device/1.0
ST:upnp:rootdevice
USN:uuid:12345678-1234-1234-1234-123456789012::upnp:rootdevice
```

## Network Interface Discovery

### Find Your Network Interfaces

```bash
# List all network interfaces
ifconfig

# Show only active interfaces with IP addresses
ifconfig | grep -A 1 "inet "

# Show routing table to find default interface
netstat -rn | grep default

# Use route command (alternative)
route get default
```

### Find Your Network Segment

```bash
# Get your IP and netmask
ifconfig en0 | grep inet

# Show ARP table (devices that have communicated recently)
arp -a

# Scan local network segment (requires nmap)
brew install nmap
nmap -sn 192.0.2.0/24  # Adjust network range as needed

# Quick ping sweep (built-in)
for i in {1..254}; do ping -c 1 -t 1 192.168.1.$i >/dev/null 2>&1 && echo "192.168.1.$i is up"; done
```

## Troubleshooting Discovery Issues

### Common Problems and Solutions

**1. No responses to mDNS queries:**
```bash
# Check if mDNS daemon is running
sudo launchctl list | grep mDNSResponder

# Restart mDNS if needed (rarely required)
sudo launchctl kickstart -k system/com.apple.mDNSResponder

# Test basic mDNS functionality
dns-sd -B _services._dns-sd._udp local.
```

**2. No responses to SSDP queries:**
```bash
# Check if firewall is blocking multicast
sudo pfctl -sr | grep 1900

# Test multicast connectivity
ping 239.255.255.250

# Check interface supports multicast
ifconfig en0 | grep MULTICAST
```

**3. Network interface issues:**
```bash
# Check which interface is being used
route get 239.255.255.250

# Force specific interface for testing
ping -I en0 239.255.255.250
sudo tcpdump -i en0 'port 5353 or port 1900'
```

**4. Firewall blocking discovery:**
```bash
# Check macOS firewall status
sudo /usr/libexec/ApplicationFirewall/socketfilterfw --getglobalstate

# Temporarily disable firewall for testing (BE CAREFUL)
sudo /usr/libexec/ApplicationFirewall/socketfilterfw --setglobalstate off

# Re-enable firewall after testing
sudo /usr/libexec/ApplicationFirewall/socketfilterfw --setglobalstate on
```

### Debugging Tools

**Monitor all discovery traffic:**
```bash
# Watch both mDNS and SSDP traffic
sudo tcpdump -i any -n -s 0 'port 5353 or port 1900'

# Save traffic to file for analysis
sudo tcpdump -i any -n -s 0 -w discovery.pcap 'port 5353 or port 1900'

# Analyze with specific filters
sudo tcpdump -i any -n -A 'port 5353' | grep -i soundtouch
```

**Network connectivity tests:**
```bash
# Test multicast group membership
netstat -g

# Test UDP connectivity
nc -u 192.0.2.100 8090  # Replace with actual device IP

# Test HTTP connectivity to discovered devices
curl -i http://192.0.2.100:8090/info  # SoundTouch info endpoint
```

## Protocol Comparison

| Protocol | Port | Multicast Address | Use Case | Discovery Method |
|----------|------|------------------|----------|------------------|
| **mDNS** | 5353 | 224.0.0.251 | Apple devices, printers, local services | Query `.local` names, browse service types |
| **SSDP** | 1900 | 239.255.255.250 | UPnP devices, media servers, smart home | M-SEARCH requests, NOTIFY advertisements |

## Advanced Techniques

### Continuous Monitoring

Create a script to continuously monitor for new devices:

```bash
#!/bin/bash
# continuous_discovery.sh

echo "Starting continuous network discovery monitoring..."
echo "Press Ctrl+C to stop"

# Function to handle cleanup
cleanup() {
    echo -e "\nStopping monitoring..."
    kill $TCPDUMP_PID 2>/dev/null
    kill $MDNS_PID 2>/dev/null
    exit 0
}

trap cleanup INT TERM

# Start background monitoring
sudo tcpdump -i any -n -l 'port 5353 or port 1900' &
TCPDUMP_PID=$!

# Periodic active discovery
while true; do
    echo -e "\n--- $(date) - Active Discovery Sweep ---"
    
    # mDNS discovery
    timeout 5 dns-sd -B _services._dns-sd._udp local. &
    MDNS_PID=$!
    
    # SSDP discovery
    echo -e "M-SEARCH * HTTP/1.1\r\nHost:239.255.255.250:1900\r\nST:ssdp:all\r\nMan:\"ssdp:discover\"\r\nMX:3\r\n\r\n" | nc -u 239.255.255.250 1900
    
    # Wait before next sweep
    sleep 30
done
```

### Device-Specific Queries

For SoundTouch devices specifically:

```bash
# Look for SoundTouch-specific services
dns-sd -B _soundtouch._tcp local.

# Query for SoundTouch device descriptions
dns-sd -L "Bose SoundTouch" _soundtouch._tcp local.

# SSDP query for media renderers (SoundTouch devices often respond)
echo -e "M-SEARCH * HTTP/1.1\r\nHost:239.255.255.250:1900\r\nST:urn:schemas-upnp-org:device:MediaRenderer:1\r\nMan:\"ssdp:discover\"\r\nMX:5\r\n\r\n" | nc -u 239.255.255.250 1900
```

### Creating Test Services

For testing your discovery setup:

```bash
# Register a test mDNS service
dns-sd -R "TestDevice" _http._tcp local 8080 &
TEST_PID=$!

# Test that it can be discovered
dns-sd -B _http._tcp local.

# Clean up
kill $TEST_PID
```

## Security Considerations

- **Network exposure**: Discovery protocols broadcast device information
- **No authentication**: Discovery traffic is typically unauthenticated
- **Information disclosure**: Device details may be visible to entire network
- **Firewall configuration**: Consider allowing only necessary multicast traffic

## Quick Reference

### Essential Commands

```bash
# Quick mDNS service browse
dns-sd -B _services._dns-sd._udp local.

# Quick SSDP discovery
echo -e "M-SEARCH * HTTP/1.1\r\nHost:239.255.255.250:1900\r\nST:ssdp:all\r\nMan:\"ssdp:discover\"\r\nMX:3\r\n\r\n" | nc -u 239.255.255.250 1900

# Monitor all discovery traffic
sudo tcpdump -i any -n 'port 5353 or port 1900'

# Test specific device connectivity
curl -i http://device-ip:8090/info
```

### Common Service Types

| Service Type | Protocol | Description |
|-------------|----------|-------------|
| `_http._tcp` | mDNS | Web servers |
| `_airplay._tcp` | mDNS | AirPlay devices |
| `_soundtouch._tcp` | mDNS | Bose SoundTouch |
| `_ipp._tcp` | mDNS | Printers |
| `_ssh._tcp` | mDNS | SSH servers |
| `upnp:rootdevice` | SSDP | UPnP root devices |
| `urn:schemas-upnp-org:device:MediaRenderer:1` | SSDP | Media players |

This guide provides comprehensive tools for manually discovering and troubleshooting network services on macOS. Use these techniques to understand what devices and services are available on your network, debug discovery issues, and verify that your applications are correctly implementing discovery protocols.