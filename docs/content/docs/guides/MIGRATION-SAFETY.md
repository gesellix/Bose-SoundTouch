---
title: "Migration & Safety Guide"
---
Starting a migration on real hardware requires a "Safety First" approach. This guide outlines the safety features implemented in the `soundtouch-service` and provides a checklist for a successful migration. The available safeguards depend on the migration method: SSH-backed methods can preserve files, while telnet-only URL migration is sequential and creates no filesystem backup.

#### 🛠 Technical Safety Enhancements

The following features are built into the `soundtouch-service` to ensure stability and easy rollbacks:

1.  **Off-Device Backups**: Before an SSH-backed migration starts, the service fetches the original `SoundTouchSdkPrivateCfg.xml` and `/etc/hosts` from your speaker and saves them locally in your `data/default/devices/<SERIAL>/` directory. This ensures you have a recovery path even if the speaker's filesystem becomes inaccessible. Telnet-only migration does not create these files.
2.  **Pre-flight Write Verification**: SSH-backed migration checks for write access (`rw`) before modifying files. Telnet migration instead checks each command response and reads back all four runtime URL fields; its writes remain sequential rather than atomic.
3.  **Automatic Safety on Sync**: Running a "Sync" in the Web UI or CLI automatically triggers an off-device backup, making it the perfect first step for any new device discovery.

#### 📋 Professional Migration Checklist

Before you proceed with the actual migration, follow these steps:

1.  **Enable SSH Access (SSH-backed methods only)**: SSH is not enabled by default. Skip this step for a telnet-only URL migration.
    - Create a file named `remote_services` on a FAT-formatted USB drive. The drive may need its bootable flag set — see [SoundCork issue #172](https://github.com/deborahgu/soundcork/issues/172) for details.
    - Insert the USB stick into the SoundTouch speaker's **SERVICE** port.
    - Reboot the speaker (unplug and replug).
    - The speaker will now allow SSH connections as `root` with no password.
    - **Verify**: Run `ssh -oHostKeyAlgorithms=+ssh-rsa root@<SPEAKER-IP>` to confirm access. (Note: older devices may require enabling `ssh-rsa` support).
2.  **Network Isolation (Optional but Recommended)**: Ensure the device is on a stable wired connection if possible, or a dedicated 2.4GHz SSID to avoid drops during SSH operations.
3.  **Initial Discovery & Sync**: 
    - Run `soundtouch-cli discover devices` to ensure connectivity.
    - Use the Web UI or CLI to "Sync" the device. This will automatically backup your presets and system configuration files to your local server.
4.  **Validate SSH Access (SSH-backed methods only)**: Confirm the device responds to SSH without a password.
    - In the Web UI **Migration** tab, select your speaker and verify that the "SSH Connection" status shows ✅ Success.
    - This toolkit automatically handles the necessary SSH parameters (ciphers and key exchanges) required by older Bose firmware.
5.  **Migration Methods**:
    - **XML redirect (default)**: Uploads a config file to the speaker via the Web API. Less invasive — only changes the application-level service URLs. Best for testing or single-device migration.
    - **Telnet URL redirect**: Writes the four service URLs through the port-17000 diagnostic shell without SSH. The commands are sequential, so a failed run can leave partial runtime state and must be inspected before retry or reboot.
    - **DNS/DHCP redirect**: Configures the speaker to use a custom DNS server that resolves Bose hostnames to the local service. Best for all-device coverage; requires the AfterTouch DNS server running on port 53. The service includes a pre-flight check before applying this method.
    
    The web UI walks you through the available methods. When the target uses HTTPS, its CA certificate must be trusted on the speaker; the web UI handles this as part of the migration flow.
6.  **Monitor Logs**: Run the `soundtouch-service` with `DEBUG` or `INFO` logging to see the step-by-step progress of the migration.

#### 🔄 Rollback Strategy

If something goes wrong or you want to return to the original Bose cloud services:

*   **Standard Revert**: Use the "Revert Migration" button in the Web UI or the corresponding CLI command. This restores the `.original` files created on the device.
*   **Telnet URL Restore**: A telnet-only migration creates no filesystem backup. Use **Restore Bose URLs via Telnet** or `setup revert --method telnet` to restore the four canonical Bose URL fields, then reboot and verify them. This does not restore DNS, CA, SSH, account, or filesystem state; pass explicit URL overrides when the device's original values differ from the canonical defaults. If any command fails, read back and reconcile all four fields before rebooting because earlier runtime writes or the `envswitch` persistence commit may already have taken effect.
*   **Concurrent Telnet Operations**: The service keeps URL-changing telnet sequences and telnet reboot operations contiguous per speaker. This process-local serialization prevents two HTTP requests from interleaving commands, but it cannot coordinate a separate CLI process or another service instance and does not make the device's multi-command update transactional.
*   **Emergency Recovery**: If the device is unreachable via the UI but SSH still works, you can manually restore the files from your local `data/` directory using `scp` or the backups created on-device (`.original`).
*   **Factory Reset**: As a last resort, Bose SoundTouch devices can be factory reset (usually by holding '1' and 'Volume Down' while plugging in). This will wipe all settings and return the device to the stock firmware configuration (the firmware itself remains at the current version, but configurations are reset).

By using the built-in off-device backups and pre-flight checks, the risk of "bricking" or losing configuration during the transition is significantly reduced.
