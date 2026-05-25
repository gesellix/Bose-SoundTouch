---
title: "IoT Configuration Quick Reference"
---
## Key Files and Locations

| File/Location                           | Purpose                | Notes                                   |
|-----------------------------------------|------------------------|-----------------------------------------|
| `/mnt/nv/BoseApp-Persistence/1/IoT.xml` | Main IoT configuration | Contains clientID, endpoint, deployment |
| `/opt/Bose/IoT`                         | IoT service binary     | ARM executable, AWS IoT SDK             |
| `/mnt/nv/IoTCerts/`                     | Certificate storage    | Device certs and private keys           |
| `/etc/init.d/SoundTouch`                | System startup script  | Creates directory structure             |
| `/opt/Bose/etc/Shepherd-noncore.xml`    | Service configuration  | Defines IoT daemon startup              |

## Configuration Parameters

### IoT.xml Structure
```xml
<Configuration 
    clientID="[UUID]" 
    iotEndpoint="[AWS_IOT_ENDPOINT]" 
    deployment="PROD" />
```

### Device-Specific Values
- **ST20**: `clientID="577ecfcc-2db3-4989-92c9-76d7704f9fb3"`
- **ST10**: `clientID="eb1a6d8f-0bb1-4aa7-9113-ea673fcef96e"`
- **Endpoint**: `a2bhvr9c4wn4ya.iot.us-east-1.amazonaws.com` (XML)
- **Backup Endpoint**: `amqmidtcohfms.iot.us-east-1.amazonaws.com` (hardcoded)

## Protocol Stack

```
Application Layer:    AWS IoT Device Shadows (JSON)
Presentation Layer:   RapidJSON parsing/serialization
Session Layer:        MQTT v3.1.1
Transport Layer:      TLS v1.2
Network Layer:        TCP/IP
```

## Certificate Files

| File                  | Location            | Purpose                   |
|-----------------------|---------------------|---------------------------|
| `iot-cert.pem.crt`    | `/mnt/nv/IoTCerts/` | Device client certificate |
| `iot-private.pem.key` | `/mnt/nv/IoTCerts/` | Device private key        |
| `rootCA.crt`          | `/var/lib/iot/`     | AWS IoT Root CA           |

## MQTT Topics

### Shadow Operations
```
$aws/things/{clientID}/shadow/update
$aws/things/{clientID}/shadow/update/accepted
$aws/things/{clientID}/shadow/update/rejected
$aws/things/{clientID}/shadow/delete
```

### JSON Payload Examples

#### Device State Report
```json
{
  "state": {
    "reported": {
      "deviceState": "CONNECTED",
      "powerState": "ON",
      "zoneState": "...",
      "groupState": "..."
    }
  }
}
```

#### Disconnection Message
```json
{
  "state": {
    "reported": {
      "deviceState": "DISCONNECTED"
    }
  }
}
```

## Process Information

- **IoT Service PID**: 1837
- **BoseApp PID**: 1846
- **Daemon Manager**: Shepherd
- **Service Type**: Non-core (stopped during updates)

## Registration Flow

1. Device generates X.509 CSR
2. Calls `https://voice.api.bose.io/alexa/certificate`
3. Receives device certificate
4. Stores cert/key in `/mnt/nv/IoTCerts/`
5. Connects to AWS IoT using certificate auth

## Directory Creation (Init Script)

```bash
mkdir -p /mnt/nv/BoseLog /mnt/nv/IoTCerts /mnt/nv/BoseApp-Persistence/1
mkdir -m 700 -p /mnt/nv/BoseApp-Persistence/1/Keys
```

## Error Messages and Debugging

### Common Log Messages
- `"Connection attempt %u to MQTT port at host %s"`
- `"MQTT port not available. Retrying in %u seconds"`
- `"Device connected with MQTT"`
- `"got shadow response: accepted. Payload: %s"`
- `"Failed to register device and get certificate, retrying"`

### Connection States
- `"MQTT port is open"`
- `"Successfully connected to MQTT server"`
- `"Disconnecting from IoT server"`
- `"UpdateShadow called when network is not ready"`

## Integration Points

### AWS Services
- AWS IoT Core (MQTT broker)
- AWS IoT Device Management (certificates)
- AWS IoT Device Shadows (state sync)

### Bose Ecosystem
- Mobile apps (remote control)
- Alexa integration (voice commands)
- Multi-room audio (zone coordination)
- OTA updates (firmware management)

## Quick Troubleshooting

1. **No IoT connectivity**: Check certificate files in `/mnt/nv/IoTCerts/`
2. **Certificate errors**: Verify registration endpoint accessibility
3. **MQTT failures**: Check both primary and backup endpoints
4. **Config issues**: Validate IoT.xml format and clientID uniqueness
5. **Service not starting**: Check Shepherd configuration and process status

## MQTT Monitoring Capabilities

### Direct Access with Device Credentials
```bash
# Subscribe to device shadow events (own device only)
mosquitto_sub -h a2bhvr9c4wn4ya.iot.us-east-1.amazonaws.com \
              -p 8883 --cafile /var/lib/iot/rootCA.crt \
              --cert /mnt/nv/IoTCerts/iot-cert.pem.crt \
              --key /mnt/nv/IoTCerts/iot-private.pem.key \
              -t '$aws/things/577ecfcc-2db3-4989-92c9-76d7704f9fb3/shadow/#'
```

### AWS IoT Policy Restrictions
- Device certificates limited to own clientID topics only
- No wildcard subscriptions across devices
- IP/location restrictions may apply
- Certificate revocation for unusual activity

### Alternative Monitoring Methods
```bash
# Network traffic capture (less intrusive)
tcpdump -i eth0 -s0 -w soundtouch_iot.pcap host a2bhvr9c4wn4ya.iot.us-east-1.amazonaws.com

# Monitor connection patterns
tcpdump -i eth0 -n "host a2bhvr9c4wn4ya.iot.us-east-1.amazonaws.com and port 8883"
```

### Expected Message Examples
```json
// Power state change
{"state":{"reported":{"powerState":"ON","deviceState":"CONNECTED"}}}

// Volume adjustment  
{"state":{"reported":{"volume":25,"muted":false}}}

// Zone configuration
{"state":{"reported":{"zoneState":"master","groupMembers":["device1"]}}}
```

## Security Notes

- TLS 1.2 encryption for all communications
- X.509 mutual authentication
- Private keys stored with 700 permissions
- No hardcoded credentials in binaries
- Automatic certificate lifecycle management
- **Monitoring Constraints**: Device credentials restricted to own device topics
- **Ethical Consideration**: Only monitor devices you own
