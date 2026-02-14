### Professional Migration & Safety Guide

Starting a migration on real hardware requires a "Safety First" approach. This guide outlines the safety features implemented in the `soundtouch-service` and provides a checklist for a successful migration.

#### 🛠 Technical Safety Enhancements

The following features are built into the `soundtouch-service` to ensure stability and easy rollbacks:

1.  **Off-Device Backups**: Before any migration starts, the service automatically fetches the original `SoundTouchSdkPrivateCfg.xml` and `/etc/hosts` from your speaker and saves them locally in your `data/default/devices/<SERIAL>/` directory. This ensures you have a recovery path even if the speaker's filesystem becomes inaccessible.
2.  **Pre-flight Write Verification**: The migration process includes a mandatory check for SSH write access (`rw`) before attempting any modifications. This prevents "half-baked" migrations where a script might fail halfway through due to a read-only filesystem.
3.  **Automatic Safety on Sync**: Running a "Sync" in the Web UI or CLI automatically triggers an off-device backup, making it the perfect first step for any new device discovery.

#### 📋 Professional Migration Checklist

Before you proceed with the actual migration, follow these steps:

1.  **Enable SSH Access (Prerequisite)**: This toolkit requires SSH access to your speakers, which is not enabled by default. 
    - Create an empty file named `remote_services` on a USB stick.
    - Insert the USB stick into the SoundTouch speaker's **SERVICE** port.
    - Reboot the speaker (unplug and replug).
    - The speaker will now allow SSH connections as `root` with no password.
    - **Verify**: Run `ssh -oHostKeyAlgorithms=+ssh-rsa root@<SPEAKER-IP>` to confirm access. (Note: older devices may require enabling `ssh-rsa` support).
2.  **Network Isolation (Optional but Recommended)**: Ensure the device is on a stable wired connection if possible, or a dedicated 2.4GHz SSID to avoid drops during SSH operations.
3.  **Initial Discovery & Sync**: 
    - Run `soundtouch-cli discover devices` to ensure connectivity.
    - Use the Web UI or CLI to "Sync" the device. This will automatically backup your presets and system configuration files to your local server.
4.  **Validate SSH Access**: Confirm the device responds to SSH without a password. 
    - In the Web UI **Migration** tab, select your speaker and verify that the "SSH Connection" status shows ✅ Success.
    - This toolkit automatically handles the necessary SSH parameters (ciphers and key exchanges) required by older Bose firmware.
5.  **Use XML Migration First**: The `XML` migration method is less invasive than the `Hosts` method. It only changes the application config and doesn't require modifying the system's DNS/CA trust store if you don't need full HTTPS interception initially.
6.  **Monitor Logs**: Run the `soundtouch-service` with `DEBUG` or `INFO` logging to see the step-by-step progress of the migration.

#### 🔄 Rollback Strategy

If something goes wrong or you want to return to the original Bose cloud services:

*   **Standard Revert**: Use the "Revert Migration" button in the Web UI or the corresponding CLI command. This restores the `.original` files created on the device.
*   **Emergency Recovery**: If the device is unreachable via the UI but SSH still works, you can manually restore the files from your local `data/` directory using `scp` or the backups created on-device (`.original`).
*   **Factory Reset**: As a last resort, Bose SoundTouch devices can be factory reset (usually by holding '1' and 'Volume Down' while plugging in). This will wipe all settings and return the device to the stock firmware configuration (the firmware itself remains at the current version, but configurations are reset).

By using the built-in off-device backups and pre-flight checks, the risk of "bricking" or losing configuration during the transition is significantly reduced.
