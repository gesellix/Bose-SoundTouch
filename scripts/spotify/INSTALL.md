# On-Speaker Spotify Boot Primer for Bose SoundTouch
Self-contained boot-time Spotify primer that runs directly on the speaker.
No Spotify credentials on the device — it fetches a fresh token from a
[Bose-SoundTouch](https://github.com/gesellix/Bose-SoundTouch) server at boot.
No jq, no rootfs modification — just files on persistent storage.

## How It Works
Bose SoundTouch speakers run embedded Linux with a persistent writable volume
at `/mnt/nv`. The init script `shelby_local` (S97) has a built-in hook:
```
[ -x /mnt/nv/rc.local ] && /mnt/nv/rc.local
```
This runs before SoundTouch itself (S99), so we background a primer script
that waits for the Spotify Connect ZeroConf endpoint (port 8200) to come up,
fetches a fresh Spotify token from the service, and primes the speaker — all
within ~30 seconds of boot.

## File Layout
```
/mnt/nv/
  rc.local                                      boot hook (S97 checks this)
  .profile                                      PATH setup for interactive SSH
  bin/
    spotify-boot-primer                         main script
  BoseApp-Persistence/1/
    spotify-primer.conf                         service credentials (mode 600)
    Sources.xml, Presets.xml, ...               existing speaker data
```
Scripts live in `/mnt/nv/bin/` (added to PATH via `.profile`), config lives
alongside the speaker's own persistence files in `/mnt/nv/BoseApp-Persistence/1/`.

## Speaker Environment
Tested on SoundTouch 20. Other SoundTouch models likely similar.
| Item | Detail |
|------|--------|
| OS | Linux 3.14.43+ ARM (hostname `spotty`) |
| Root FS | Read-only ubifs (can be remounted rw) |
| Persistent storage | `/mnt/nv` — writable ubifs, ~24M free |
| curl | 7.50.3 with OpenSSL (HTTPS works) |
| bash/grep/sed/awk | Available via busybox |
| jq | **Not available** (not needed) |
| Init | SysV, runlevel 5 |
| Production mode | Yes — cron is disabled |

## Prerequisites
1. **SSH access to the speaker**:
   ```
   ssh -o HostKeyAlgorithms=+ssh-rsa -o PubkeyAcceptedAlgorithms=+ssh-rsa root@SPEAKER_IP
   ```
2. **A running [Bose-SoundTouch](https://github.com/gesellix/Bose-SoundTouch) server** with:
   - A linked Spotify account (via the management API OAuth flow)
   - The `GET /mgmt/spotify/token` endpoint (returns `{accessToken, username}`)
   - Management API credentials (HTTP Basic Auth)

## Installation
SSH into the speaker and run:
```bash
# 1. Create bin directory
mkdir -p /mnt/nv/bin
# 2. Create the config file with your service connection info
cat > /mnt/nv/BoseApp-Persistence/1/spotify-primer.conf << 'EOF'
SOUNDTOUCH_URL=https://soundtouch.example.com
SOUNDTOUCH_USER=admin
SOUNDTOUCH_PASS=secret
EOF
chmod 600 /mnt/nv/BoseApp-Persistence/1/spotify-primer.conf
# 3. Copy spotify-boot-primer to the speaker
#    From your local machine:
#    cat scripts/spotify/spotify-boot-primer | ssh root@SPEAKER_IP "cat > /mnt/nv/bin/spotify-boot-primer"
chmod +x /mnt/nv/bin/spotify-boot-primer
# 4. Create the boot hook
cat > /mnt/nv/rc.local << 'EOF'
#!/bin/bash
/mnt/nv/bin/spotify-boot-primer &
EOF
chmod +x /mnt/nv/rc.local
# 5. Set up PATH for interactive SSH sessions (optional but convenient)
cat > /mnt/nv/.profile << 'EOF'
export PATH="/mnt/nv/bin:$PATH"
EOF
```

## Testing
```bash
# Manual test (speaker must be running):
/mnt/nv/bin/spotify-boot-primer
# Check logs:
logread | grep spotify-primer
# Full test — reboot the speaker:
reboot
# Wait ~30s, then SSH back in and check:
logread | grep spotify-primer
curl -s "http://localhost:8200/zc?action=getInfo" | grep activeUser
```

## Related
- [Bose-SoundTouch](https://github.com/gesellix/Bose-SoundTouch) — Comprehensive Go toolkit with migration automation
