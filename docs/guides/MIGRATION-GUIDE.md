# THIS IS A PLANNED TO BE THE MIGRATION GUIDE

> This migration guide is not finalized, yet.
> We're using it as an orientation for the required implementation.

---

# Complete Migration Guide - From Bose Cloud to Local SoundTouch Service

## Overview

This guide will walk you through migrating your Bose SoundTouch speakers from Bose's cloud services to AfterTouch, your own local SoundTouch service. By the end of this process, your speakers will be completely independent of Bose's servers while retaining all their functionality.

> **💡 Why Migrate?** Bose announced the shutdown of their SoundTouch cloud services in May 2026. This migration ensures your speakers continue working indefinitely with enhanced local control and monitoring.

## What You'll Need

### Hardware Requirements
- **Raspberry Pi 4 or similar** (minimum: Raspberry Pi Zero 2W)
- **MicroSD card** (16GB or larger)
- **USB drive** (for device preparation)
- **Network connection** for your Raspberry Pi

### Before You Start
- **List all your SoundTouch devices** and their current locations
- **Note your current presets and favorites** (they will be preserved)
- **Ensure devices are on the same network** as your future SoundTouch service
- **Basic computer skills** (following instructions, using a web browser)

### Time Estimate
- **Setup**: 30-60 minutes for the service installation
- **Per Device**: 10-15 minutes for each speaker migration
- **Total**: 1-3 hours depending on number of devices

## Step 1: Install SoundTouch Service

### Option A: Raspberry Pi Installation (Recommended)

#### 1.1 Prepare Your Raspberry Pi

1. **Flash Raspberry Pi OS** to your SD card using Raspberry Pi Imager (see the raspberrypi.com documentation)
2. **Enable SSH** during imaging or create an empty `ssh` file on the boot partition
3. **Boot your Pi** and connect it to your network
4. **Find your Pi's IP address** (check your router or use `ping raspberrypi.local`)

#### 1.2 Install SoundTouch Service

Connect to your Pi via SSH and run:

```bash
# Download and install
curl -sSL https://github.com/gesellix/Bose-SoundTouch/releases/latest/download/install.sh | bash

# Start the service
sudo systemctl enable soundtouch-service
sudo systemctl start soundtouch-service
```

#### 1.3 Verify Installation

1. Open your web browser
2. Go to `http://[PI_IP_ADDRESS]:8000` (replace with your Pi's IP)
3. You should see the **SoundTouch Service Dashboard**

![SoundTouch Service Dashboard](../images/dashboard-home.png)
*Example: SoundTouch Service main dashboard*

### Option B: Docker Installation

If you prefer Docker, run:

```bash
docker run -d \
  --name soundtouch-service \
  --restart unless-stopped \
  -p 8000:8000 \
  -p 8443:8443 \
  -v soundtouch-data:/data \
  gesellix/soundtouch-service:latest
```

## Step 2: Create Your Account

### 2.1 Initial Setup

1. **Open the dashboard** at `http://[SERVICE_IP]:8000`
2. Click **"Create New Account"**
3. **Fill in your details**:
   - Account Name: `My Home Audio`
   - Email: `your@email.com` (optional, for notifications)
   - Migration Strategy: `Gradual` (recommended)

![Account Creation](../images/account-creation.png)
*Example: Account creation form*

### 2.2 Account Configuration

After creation, you'll see your **Account Dashboard**:
- **Account ID**: Unique identifier (e.g., `acc_home_audio_001`)
- **Status**: `Active - Ready for Migration`
- **Device Count**: Initially 0
- **Migration Status**: `Prepared`

![Account Dashboard](../images/account-dashboard.png)
*Example: Fresh account dashboard ready for device migration*

### 2.3 Initial Settings

Once your account is created, configure the global settings:
1. **Settings**:
   - Check **Target Domain**: Ensure it's reachable from the speaker (e.g., `soundtouch.fritz.box`).
   - **DNS Discovery**: Enable DNS discovery on port `:53`. This is crucial for the DNS hook migration method.
2. **Devices**:
   - Go to the **"Device Discovery"** tab.
   - Click **"Scan Network"** or manually add a speaker via IP address.
   - Your devices should appear with **SSH Status**: `Enabled`.

![Device Discovery](../images/device-discovery.png)
*Example: Discovered devices with remote access enabled*

## Step 3: Prepare Your Devices

> **⚠️ Important**: This step temporarily enables SSH access on your speakers. SSH will be automatically disabled after migration unless you choose to keep it enabled.

### 3.1 Enable Remote Services

For each SoundTouch device:

1. **Prepare a USB drive**:
   - Format as FAT32
   - Create an empty file named `remote_services` (no extension)
   - (Optional) Firmware update/reset, see [Bose SoundTouch USB Update](https://downloads.bose.com/ced/soundtouch/soundtouch_usb/index.html)

2. **Insert USB drive** into your SoundTouch speaker
3. **Power cycle** the device (unplug for 10 seconds, then reconnect)

![USB Preparation](../images/usb-remote-services.png)
*Example: USB drive setup for enabling remote services*

## Step 4: Discover and Register Devices

### 4.1 Automatic Discovery

The service automatically scans for SoundTouch devices every 5 minutes. To trigger immediate discovery:

1. **Dashboard** → **"Devices"** → **"Discover Devices"**
2. **Wait 30-60 seconds** for scan completion
3. **Review discovered devices** in the list

### 4.2 Register Devices to Your Account

For each discovered device:

1. **Click device name** in the discovery list
2. **Verify device information**:
   - Name: `Living Room Speaker`
   - Model: `SoundTouch 30`
   - MAC Address: `A8:1B:6A:53:6A:98`
   - IP Address: `192.168.1.100`
   - Status: `Discovered - Ready for Registration`

3. **Click "Register to Account"**
4. **Choose registration type**:
   - **Fresh Setup**: For new or factory-reset devices
   - **Migrate from Bose**: For devices with existing Bose account (recommended)

![Device Registration](../images/device-registration.png)
*Example: Device registration dialog with migration options*

### 4.3 Device Registration Results

After registration, you'll see:
- **Device Status**: `Registered - Active`
- **Account Association**: Your account name
- **Lifecycle State**: `Active`
- **Data Sources**: `Mirror Primary` (initially uses Bose, falls back to local)

## Step 5: Migrate Individual Devices

### 5.1 Step 3: Data Sync

1. **Dashboard** → **"Devices"** → Select your device
2. Click **"Data Sync"**
3. This fetches configuration (presets, recents, sources) from the speaker to the SoundTouch service.

### 5.2 Step 4: Migration

Once data is synced, proceed to the migration tab for the device:

1. **Backup XML**: Create an off-device backup of the current configuration.
2. **Enable Persistent Remote Service**: This ensures SSH remains available after reboots.
   - *Note*: If you see `'rw: command not found'`, you can safely ignore it.
3. **CA Certificate Configuration**:
   - **Test with explicit CA**: Verify the speaker can communicate using the local CA.
   - **Trust CA now**: Inject the local Root CA into the speaker's trust store.
   - **Test with shared trust store**: Verify general HTTPS communication.
4. **Migration Method**:
   - Select **"Redirect via DNS hook"**.
   - **Test DNS Redirection**: Ensure the speaker correctly resolves the service domain.
5. **Confirm Migration**: Apply the final changes to the speaker.

#### Example Migration Output:
```text
Successfully created off-device backup of current configuration.
Pre-flight: Write access verified.
Resolved soundtouch.fritz.box to 192.168.1.100
Uploaded /mnt/nv/soundtouch-service/aftertouch.resolv.conf
/mnt/nv/rc.local already contains Aftertouch hook logic
(rw || mount -o remount,rw /): sh: rw: command not found

cp /etc/udhcpc.d/50default /etc/udhcpc.d/50default.original:
Applied patch to /etc/udhcpc.d/50default
Verified patch on /etc/udhcpc.d/50default
cp /opt/Bose/udhcpc.script /opt/Bose/udhcpc.script.original:
Applied patch to /opt/Bose/udhcpc.script
Verified patch on /opt/Bose/udhcpc.script
CA certificate already trusted, skipping injection
```

## Step 7: Complete Account Migration

### 7.1 Migrate All Devices

Repeat the migration process for each of your SoundTouch devices. You can migrate multiple devices simultaneously, but we recommend doing 1-2 at a time to monitor progress.

**Migration Dashboard** shows overall progress:
- **Devices Migrated**: `2 of 4 completed`
- **Currently Migrating**: `Living Room Speaker, Kitchen Speaker`
- **Pending Migration**: `Bedroom Speaker, Office Speaker`
- **Estimated Completion**: `3 days remaining`

![Account Migration Status](../images/account-migration.png)
*Example: Account-wide migration progress*

### 7.2 Verify Complete Migration

When all devices are migrated:

1. **Account Status**: `Active - Fully Migrated`
2. **Bose Dependency**: `None`
3. **Local Control**: `100%`
4. **Device Health**: All devices show `Healthy - Local Only`

![Migration Complete](../images/migration-complete.png)
*Example: Completed migration dashboard*

## Step 8: Post-Migration Tasks

1. **Remove USB stick** from the speaker.
2. **Reboot** the device to apply all changes.

### 8.1 Disable Remote Services (Optional)

For enhanced security, you can disable SSH on migrated devices. However, keeping it enabled allows for easier future maintenance or reverts.

### 8.2 Configure Backups

Set up automatic backups of your device configurations:

1. **Dashboard** → **"Settings"** → **"Backup"**
2. **Enable Automatic Backups**: ✅
3. **Backup Schedule**: `Daily at 2 AM`
4. **Retention**: `Keep 30 days`
5. **Export Location**: `/data/backups` or external storage

![Backup Configuration](../images/backup-setup.png)
*Example: Backup configuration settings*

### 8.3 Set Up Monitoring Alerts (Optional)

Configure notifications for important events:

1. **Dashboard** → **"Settings"** → **"Notifications"**
2. **Email Notifications**: Enter your email
3. **Alert Types**:
   - ✅ Device goes offline
   - ✅ Migration failures
   - ✅ Service errors
   - ✅ Daily health summary

## Troubleshooting Common Issues

### Device Not Discovered

**Problem**: Device doesn't appear in discovery scan

**Solutions**:
1. **Check network**: Ensure device and service are on same network
2. **Verify USB setup**: Confirm `remote_services` file was processed
3. **Power cycle**: Unplug device for 30 seconds, reconnect
4. **Manual add**: Dashboard → "Devices" → "Add Manually" with IP address

### Migration Stuck

**Problem**: Device stuck in "Migrating" status

**Solutions**:
1. **Check device health**: Dashboard → Device → "Health Status"
2. **Review logs**: Dashboard → Device → "View Logs"
3. **Restart migration**: Device → "Migration" → "Restart Process"
4. **Rollback**: Device → "Migration" → "Rollback to Bose"

### Presets Not Working

**Problem**: Saved presets don't work after migration

**Solutions**:
1. **Verify sources**: Check configured sources are still available
2. **Re-authenticate**: Re-login to music services (Spotify, etc.)
3. **Rebuild presets**: Dashboard → Device → "Presets" → "Rebuild from Backup"

### Service Unreachable

**Problem**: Cannot access SoundTouch Service dashboard

**Solutions**:
1. **Check service status**: `sudo systemctl status soundtouch-service`
2. **Restart service**: `sudo systemctl restart soundtouch-service`
3. **Check network**: Verify Pi is connected and accessible
4. **Check ports**: Ensure ports 8000 and 8443 are not blocked

## Advanced Features

### Multi-Zone Management

After migration, your multi-zone setups work seamlessly:

1. **Dashboard** → **"Zones"**
2. **Create Zone**: Select primary device and slaves
3. **Zone Control**: Play, pause, volume control for entire zone
4. **Individual Control**: Override individual speakers in zone

### Custom Sources

Add custom streaming sources:

1. **Dashboard** → **"Sources"** → **"Add Custom"**
2. **Configure**:
   - Name: `Local Radio Station`
   - Stream URL: `http://stream.example.com:8000`
   - Image URL: `http://example.com/logo.png`
3. **Assign to devices**: Select which devices can access this source

### API Access

For developers and advanced users:

- **REST API**: `http://[SERVICE_IP]:8000/api/v1/`
- **Documentation**: `http://[SERVICE_IP]:8000/docs`
- **WebSocket Events**: Real-time device status updates
- **Export Data**: JSON/XML export of all device configurations

## Maintenance and Monitoring

### Daily Monitoring

Check your **Dashboard Summary**:
- **All Devices Online**: ✅ Green indicators
- **Response Times**: < 100ms average
- **Error Rate**: < 1%
- **Storage Usage**: Monitor disk space

### Weekly Tasks

1. **Review Health Reports**: Check weekly device health summaries
2. **Update Service**: Check for SoundTouch service updates
3. **Backup Verification**: Ensure backups are completing successfully
4. **Log Review**: Check for any recurring issues or warnings

### Monthly Tasks

1. **Full System Backup**: Export complete account and device data
2. **Performance Review**: Analyze response times and error patterns
3. **Security Update**: Update Raspberry Pi OS and service
4. **Capacity Planning**: Monitor storage and consider expansion

## Getting Help

### Documentation Resources

- **Technical Reference**: `/docs/reference/` - Detailed API and configuration docs
- **Troubleshooting Guide**: `/docs/guides/TROUBLESHOOTING.md` - Common issues and solutions
- **Community Forum**: GitHub Discussions for community support

### Diagnostic Information

When seeking help, provide:

1. **System Information**: Dashboard → "System" → "Download Diagnostic Report"
2. **Device Logs**: Dashboard → Device → "Export Logs"
3. **Migration History**: Dashboard → "Migration" → "Export Timeline"
4. **Current Status**: Screenshot of main dashboard

### Support Channels

- **GitHub Issues**: Technical bugs and feature requests
- **Community Discussions**: User questions and experiences
- **Documentation Updates**: Corrections and improvements

---

## Summary

Congratulations! 🎉 You've successfully migrated your SoundTouch speakers to local control. Your devices are now:

- ✅ **Independent** of Bose cloud services
- ✅ **Fully functional** with all original features preserved
- ✅ **Enhanced** with better monitoring and control
- ✅ **Future-proof** against service shutdowns

**What's Next?**

- **Enjoy your music** with enhanced local control
- **Monitor your system** through the dashboard
- **Share your experience** with the community
- **Explore advanced features** as you become more comfortable

Your SoundTouch speakers will now continue working indefinitely, regardless of external service availability. Welcome to true audio independence! 🔊
