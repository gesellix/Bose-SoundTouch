#!/usr/bin/env bash
# Per-session startup: restore the 'mitm-ready' emulator snapshot, refresh the
# proxy IP (the Mac's LAN IP can change between sessions), start frida-server,
# and print the Frida launch command for the Bose app.
#
# Prerequisites: run setup-mitm-avd.sh once first.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
ANDROID_DIR="${REPO_ROOT}/scripts/android"

# ── Config ────────────────────────────────────────────────────────────────────
SDK="${ANDROID_HOME:-${HOME}/Library/Android/sdk}"
EMULATOR="${SDK}/emulator/emulator"
ADB="${SDK}/platform-tools/adb"

AVD_NAME="bose-mitm"
EMULATOR_SERIAL="emulator-5554"
SNAPSHOT_NAME="mitm-ready"
PROXY_PORT=8080

FRIDA_SCRIPTS_DIR="${ANDROID_DIR}/frida"
FRIDA_VENV="${ANDROID_DIR}/frida-venv"
FRIDA_SERVER="${ANDROID_DIR}/frida-server"
# Keep in sync with scripts/android/Dockerfile ARG defaults
FRIDA_VERSION="17.9.1"
FRIDA_TOOLS_VERSION="14.8.1"
MITMPROXY_IMAGE="mitmproxy/mitmproxy:12.2.1"
# Detect native mitmproxy app location
MITMPROXY_NATIVE=""
for candidate in "/Applications/mitmproxy.app" "${HOME}/Downloads/mitmproxy.app"; do
  if [[ -x "${candidate}/Contents/MacOS/mitmweb" ]]; then
    MITMPROXY_NATIVE="${candidate}/Contents/MacOS/mitmweb"
    break
  fi
done
[[ -n "${MITMPROXY_NATIVE}" ]] || die "mitmproxy native app not found. See setup-mitm-avd.sh for download instructions."

# ── Helpers ───────────────────────────────────────────────────────────────────
info()  { echo "  → $*"; }
ok()    { echo "  ✓ $*"; }
die()   { echo "  ✗ $*" >&2; exit 1; }

wait_for_boot() {
  "${ADB}" -s "${EMULATOR_SERIAL}" wait-for-device
  until "${ADB}" -s "${EMULATOR_SERIAL}" shell getprop sys.boot_completed 2>/dev/null | grep -q "1"; do
    sleep 2
  done
  sleep 2
}

# ── Mac IP ────────────────────────────────────────────────────────────────────
MAC_IP=$(ipconfig getifaddr en0 2>/dev/null || ipconfig getifaddr en1 2>/dev/null)
[[ -n "${MAC_IP}" ]] || die "Could not determine Mac LAN IP. Are you connected to Wi-Fi?"
ok "Mac IP: ${MAC_IP}"

# ── Start emulator from snapshot ──────────────────────────────────────────────
echo ""
echo "Step 1/4  Start emulator from snapshot '${SNAPSHOT_NAME}'"
if "${ADB}" devices | grep -q "${EMULATOR_SERIAL}"; then
  ok "Emulator already running — skipping launch"
else
  "${EMULATOR}" -avd "${AVD_NAME}" -writable-system \
    -snapshot "${SNAPSHOT_NAME}" &
  info "Emulator starting (PID $!)..."
fi

wait_for_boot
"${ADB}" -s "${EMULATOR_SERIAL}" root
ok "Emulator ready"

# ── Refresh proxy IP ──────────────────────────────────────────────────────────
echo ""
echo "Step 2/4  Set proxy to ${MAC_IP}:${PROXY_PORT}"
"${ADB}" -s "${EMULATOR_SERIAL}" shell settings put global http_proxy "${MAC_IP}:${PROXY_PORT}"
ok "Proxy configured"

# ── Start frida-server ────────────────────────────────────────────────────────
echo ""
echo "Step 3/4  Start frida-server"
[[ -f "${FRIDA_SERVER}" ]] || die "frida-server not found at ${FRIDA_SERVER}. Run setup-mitm-avd.sh first."
"${ADB}" -s "${EMULATOR_SERIAL}" shell su 0 \
  'pgrep frida-server > /dev/null && echo already_running || (nohup /data/local/tmp/frida-server > /dev/null 2>&1 &)'
sleep 2
ok "frida-server running"

# ── Check SSL bypass scripts ──────────────────────────────────────────────────
echo ""
echo "Step 4/4  Ensure Frida SSL scripts are present"
[[ -f "${FRIDA_SCRIPTS_DIR}/config.js" ]] || \
  die "Frida SSL scripts not found at ${FRIDA_SCRIPTS_DIR}. Run setup-mitm-avd.sh first."

# Patch config.js with current MAC IP and mitmproxy cert
CERT_PEM=$(openssl x509 -in ~/.mitmproxy/mitmproxy-ca-cert.pem)
FRIDA_SCRIPTS_DIR="${FRIDA_SCRIPTS_DIR}" MAC_IP="${MAC_IP}" \
  PROXY_PORT="${PROXY_PORT}" CERT_PEM="${CERT_PEM}" \
  python3 - <<'PYEOF'
import re, pathlib, os
cfg_path = pathlib.Path(os.environ["FRIDA_SCRIPTS_DIR"] + "/config.js")
cfg = cfg_path.read_text()
cfg = re.sub(r"const PROXY_HOST = '[^']*'", f"const PROXY_HOST = '{os.environ['MAC_IP']}'", cfg)
cfg = re.sub(r"const PROXY_PORT = [0-9]+", f"const PROXY_PORT = {os.environ['PROXY_PORT']}", cfg)
cfg = re.sub(r"const CERT_PEM = `[^`]*`", f"const CERT_PEM = `{os.environ['CERT_PEM']}`", cfg)
cfg_path.write_text(cfg)
print(f"  ✓ config.js updated (proxy={os.environ['MAC_IP']}:{os.environ['PROXY_PORT']})")
PYEOF

# Ensure frida Python package matches server version
if [[ ! -d "${FRIDA_VENV}" ]]; then
  info "Creating frida venv at ${FRIDA_VENV}..."
  python3 -m venv "${FRIDA_VENV}"
  "${FRIDA_VENV}/bin/pip" install -q "frida==${FRIDA_VERSION}" "frida-tools==${FRIDA_TOOLS_VERSION}"
fi

ok "Frida scripts ready at ${FRIDA_SCRIPTS_DIR}"

# ── Instructions ──────────────────────────────────────────────────────────────
CAPTURES_DIR="${ANDROID_DIR}/captures"
mkdir -p "${CAPTURES_DIR}"

cat <<INSTRUCTIONS

Session ready. In a separate terminal:

  1. Start mitmweb (native macOS app):
       CAPTURE="bose-pairing-\$(date +%Y%m%d-%H%M%S).mitm"
       ${MITMPROXY_NATIVE} \\
         --web-host 0.0.0.0 --listen-port ${PROXY_PORT} --mode regular \\
         --set web_password=bose \\
         -w "${CAPTURES_DIR}/\${CAPTURE}"
     Captures → ${CAPTURES_DIR}/
     Web UI   → http://127.0.0.1:8081/?token=bose
     Note: Docker mitmproxy does not work — its NAT layer blocks emulator traffic.

  2. Launch Bose app with SSL unpinning:
       ${FRIDA_VENV}/bin/frida \\
         -U \\
         -f com.bose.soundtouch \\
         -l ${FRIDA_SCRIPTS_DIR}/config.js \\
         -l ${FRIDA_SCRIPTS_DIR}/native-connect-hook.js \\
         -l ${FRIDA_SCRIPTS_DIR}/android/android-system-certificate-injection.js \\
         -l ${FRIDA_SCRIPTS_DIR}/android/android-proxy-override.js \\
         -l ${FRIDA_SCRIPTS_DIR}/android/android-certificate-unpinning.js \\
         -l ${FRIDA_SCRIPTS_DIR}/android/android-certificate-unpinning-fallback.js

  3. Operate the Bose app in the emulator.

  To dump the current UI for adb automation:
       adb -s ${EMULATOR_SERIAL} shell uiautomator dump /sdcard/ui.xml
       adb -s ${EMULATOR_SERIAL} pull /sdcard/ui.xml /tmp/ui.xml

  To take a screenshot:
       adb -s ${EMULATOR_SERIAL} shell screencap /sdcard/screen.png
       adb -s ${EMULATOR_SERIAL} pull /sdcard/screen.png /tmp/screen.png && open /tmp/screen.png

INSTRUCTIONS
