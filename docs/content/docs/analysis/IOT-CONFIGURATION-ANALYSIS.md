---
title: "IoT Configuration Analysis"
---
## Overview

This document provides a detailed analysis of the AWS IoT configuration system used by Bose SoundTouch devices, based on firmware backup analysis from ST10 and ST20 models.

## Configuration Files

### IoT.xml Location and Content

The IoT configuration is stored in XML format at:
- **Path**: `/mnt/nv/BoseApp-Persistence/1/IoT.xml`
- **Purpose**: Contains AWS IoT Core connection parameters

#### ST20 Configuration
```xml
<?xml version="1.0" encoding="UTF-8" ?>
<Configuration clientID="uuid1"
               iotEndpoint="a2bhvr9c4wn4ya.iot.us-east-1.amazonaws.com"
               deployment="PROD" />
```

#### ST10 Configuration
```xml
<?xml version="1.0" encoding="UTF-8" ?>
<Configuration clientID="uuid2"
               iotEndpoint="a2bhvr9c4wn4ya.iot.us-east-1.amazonaws.com"
               deployment="PROD" />
```

### Key Observations
- Each device has a unique `clientID` (UUID format)
- Both devices use the same AWS IoT endpoint
- Both are configured for production deployment (`PROD`)

## Binary Analysis

### Primary IoT Service Binary

**Location**: `/opt/Bose/IoT`
- **Type**: ARM ELF 32-bit executable
- **Purpose**: Main IoT daemon process
- **Framework**: AWS IoT SDK for C++

### Certificate and Key Management

The IoT binary manages the following certificate files:

| File                  | Location            | Purpose                     |
|-----------------------|---------------------|-----------------------------|
| `iot-cert.pem.crt`    | `/mnt/nv/IoTCerts/` | Device client certificate   |
| `iot-private.pem.key` | `/mnt/nv/IoTCerts/` | Device private key          |
| `rootCA.crt`          | `/var/lib/iot/`     | AWS IoT Root CA certificate |

### Certificate Registration Process

1. **CSR Generation**: Device generates X.509 certificate signing request
2. **Registration Endpoint**: `https://voice.api.bose.io/alexa/certificate`
3. **Certificate Storage**: Certificates stored in `/mnt/nv/IoTCerts/`
4. **Automatic Provisioning**: Process appears to be automated during device setup

## Protocol Analysis

### Connection Details

- **Protocol**: MQTT over TLS 1.2
- **Port**: Standard MQTT over SSL (likely 8883)
- **Authentication**: X.509 client certificate mutual authentication
- **Endpoint Redundancy**:
  - Primary (hardcoded): `amqmidtcohfms.iot.us-east-1.amazonaws.com`
  - Fallback (XML config): `a2bhvr9c4wn4ya.iot.us-east-1.amazonaws.com`

### AWS IoT Device Shadow Integration

The system uses AWS IoT Device Shadows for state management:

#### Topic Structure
```
$aws/things/{thing_name}/shadow/update
$aws/things/{thing_name}/shadow/update/accepted
$aws/things/{thing_name}/shadow/update/rejected
$aws/things/{thing_name}/shadow/delete
```

#### Shadow JSON Format
```json
{
  "state": {
    "desired": {},
    "reported": {
      "deviceState": "CONNECTED|DISCONNECTED",
      "powerState": "ON|OFF",
      "zoneState": "...",
      "groupState": "..."
    }
  },
  "version": 0,
  "clientToken": "...",
  "timestamp": 0
}
```

### Message Types

1. **Device State Updates**
   - Connection status (`CONNECTED`/`DISCONNECTED`)
   - Power state changes
   - Audio zone configuration
   - Multi-room grouping status

2. **Shadow Delta Processing**
   - Receives desired state changes
   - Updates device configuration
   - Reports new state back to shadow

## System Integration

### Service Management

The IoT service is managed by the Shepherd daemon system:

**Configuration**: `/opt/Bose/etc/Shepherd-noncore.xml`
```xml
<ShepherdConfig>
  <daemon name="STSCertified"/>
  <daemon name="IoT"/>
  <daemon name="TPDA">
    <arg>-c</arg>
    <arg>/opt/Bose/etc/Voice.xml</arg>
  </daemon>
</ShepherdConfig>
```

### Directory Structure Creation

The SoundTouch init script (`/etc/init.d/SoundTouch`) ensures proper directory structure:

```bash
mkdir -p /mnt/nv/BoseLog /mnt/nv/IoTCerts /mnt/nv/BoseApp-Persistence/1
mkdir -m 700 -p /mnt/nv/BoseApp-Persistence/1/Keys
```

### Process Information

From runtime analysis (`/var/run/shepherd/pids`):
- IoT service runs as PID 1837
- BoseApp service runs as PID 1846
- Both services are active during normal operation

## Configuration Dependencies

### Files That Reference IoT Configuration

1. **IoT Binary** (`/opt/Bose/IoT`)
   - Primary consumer of IoT.xml configuration
   - Contains hardcoded backup endpoints
   - Manages certificate lifecycle

2. **BoseApp Binary** (`/opt/Bose/BoseApp`)
   - References BoseApp-Persistence directory structure
   - May trigger IoT updates based on device state changes

3. **SoundTouch Init Script** (`/etc/init.d/SoundTouch`)
   - Creates necessary directory structure
   - Ensures proper permissions for certificate storage

4. **Shepherd Configuration** (`/opt/Bose/etc/Shepherd-noncore.xml`)
   - Defines IoT service startup parameters
   - Manages service lifecycle

## Security Considerations

### Certificate Management
- Private keys stored with 700 permissions
- Certificates managed automatically by the device
- Registration process appears to use device-specific authentication

### Network Security
- All communication over TLS 1.2
- Mutual authentication using X.509 certificates
- AWS IoT Core provides additional access controls

### Configuration Protection
- Configuration files stored in persistent storage
- Directory structure created with appropriate permissions
- No hardcoded credentials in binaries (uses certificate-based auth)

## Integration Points

### AWS Services
- **AWS IoT Core**: Primary messaging and device management
- **AWS IoT Device Management**: Certificate provisioning
- **AWS IoT Device Shadows**: State synchronization

### Bose Services
- **Mobile Applications**: Remote control and monitoring
- **Alexa Integration**: Voice control capabilities
- **Multi-room Audio**: Zone and group coordination

### Device Functions
- **Power Management**: Remote power on/off
- **Audio Control**: Volume, source selection
- **Network Configuration**: WiFi and connectivity settings
- **Firmware Updates**: OTA update coordination

## Troubleshooting

### Common Issues

1. **Certificate Problems**
   - Check `/mnt/nv/IoTCerts/` for valid certificates
   - Verify certificate registration endpoint accessibility
   - Ensure proper file permissions (600 for keys)

2. **Connection Issues**
   - Verify both primary and fallback endpoints
   - Check TLS 1.2 support and cipher suites
   - Validate clientID uniqueness

3. **Configuration Issues**
   - Ensure IoT.xml has proper XML format
   - Verify clientID is valid UUID format
   - Check deployment parameter matches environment

### Debug Information

The IoT binary provides extensive logging for:
- MQTT connection attempts and status
- Certificate loading and validation
- Shadow message processing
- Network state changes

## MQTT Monitoring and Security Considerations

### Direct MQTT Access with Device Credentials

With access to the device's private key and certificate, it's technically possible to subscribe to MQTT events:

```bash
# Subscribe to device shadow events
mosquitto_sub -h a2bhvr9c4wn4ya.iot.us-east-1.amazonaws.com \
              -p 8883 --cafile /var/lib/iot/rootCA.crt \
              --cert /mnt/nv/IoTCerts/iot-cert.pem.crt \
              --key /mnt/nv/IoTCerts/iot-private.pem.key \
              -t '$aws/things/_uuid_/shadow/#'
```

### Security Constraints and Limitations

#### AWS IoT Policy Restrictions
Device certificates are bound to specific policies that typically restrict:
- Access to device-specific topics only (`$aws/things/{clientID}/shadow/*`)
- No wildcard subscriptions across multiple devices
- Limited publish/subscribe permissions
- Possible IP geolocation restrictions

#### Example Policy Structure
```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": "iot:Connect",
      "Resource": "arn:aws:iot:us-east-1:*:client/${iot:ClientId}"
    },
    {
      "Effect": "Allow",
      "Action": ["iot:Publish", "iot:Subscribe", "iot:Receive"],
      "Resource": [
        "arn:aws:iot:us-east-1:*:topic/$aws/things/${iot:ClientId}/shadow/*",
        "arn:aws:iot:us-east-1:*:topicfilter/$aws/things/${iot:ClientId}/shadow/*"
      ]
    }
  ]
}
```

#### Additional Security Measures
- Certificate revocation for unusual activity
- Device fingerprinting and connection frequency limits
- Service shutdown timeline (May 2026) affecting endpoint availability

### Alternative Monitoring Approaches

#### Network Traffic Capture
A less intrusive method to analyze MQTT communication patterns:

```bash
# Capture encrypted MQTT traffic from the actual device
tcpdump -i eth0 -s0 -w soundtouch_iot.pcap host a2bhvr9c4wn4ya.iot.us-east-1.amazonaws.com

# Monitor connection patterns
tcpdump -i eth0 -n "host a2bhvr9c4wn4ya.iot.us-east-1.amazonaws.com and port 8883"
```

#### Local MQTT Broker Setup
For development and testing, create a local MQTT broker that mimics AWS IoT behavior:

```bash
# Install and configure Mosquitto
sudo apt-get install mosquitto mosquitto-clients

# Create test shadow topics
mosquitto_pub -h localhost -t '$aws/things/test-device/shadow/update' \
              -m '{"state":{"reported":{"deviceState":"CONNECTED"}}}'
```

### Ethical and Legal Considerations

- **Device Ownership**: Only monitor devices you own
- **Terms of Service**: Using credentials outside device context may violate Bose ToS
- **Unauthorized Access**: Accessing Bose's AWS infrastructure could be considered inappropriate
- **Research Purpose**: Limit monitoring to understanding message formats for local alternatives

### Expected Message Examples

If monitoring is successful, typical shadow messages include:

```json
// Power state change
{
  "state": {
    "reported": {
      "powerState": "ON",
      "deviceState": "CONNECTED",
      "timestamp": 1703875200
    }
  }
}

// Volume adjustment
{
  "state": {
    "reported": {
      "volume": 25,
      "muted": false
    }
  }
}

// Zone configuration
{
  "state": {
    "reported": {
      "zoneState": "master",
      "groupMembers": ["device1", "device2"]
    }
  }
}
```

### Recommended Research Approach

1. **Document Message Formats**: Capture and analyze JSON structures
2. **Understand State Transitions**: Map device actions to shadow updates
3. **Build Local Alternative**: Use insights to create local MQTT shadow service
4. **Prepare for Service Shutdown**: Develop migration strategy before May 2026

## Conclusion

The Bose SoundTouch IoT configuration system is a sophisticated implementation using AWS IoT Core for real-time device management. The system provides:

- Secure, certificate-based authentication
- Reliable bi-directional communication
- Comprehensive device state management
- Integration with voice assistants and mobile applications
- Robust error handling and retry mechanisms

This architecture enables seamless remote control, monitoring, and coordination of SoundTouch devices across multiple platforms and services.
