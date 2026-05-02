#!/usr/bin/env bash
# One-time setup: install mitmproxy, obtain the Bose APK, create the Android
# emulator AVD, and save a ready-to-use snapshot so subsequent sessions skip
# the cert/APK install cycle.
#
# Run once, then use start-mitm-session.sh for day-to-day use.
# Tested on: Apple Silicon Mac (arm64)

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
ANDROID_DIR="${REPO_ROOT}/scripts/android"

# ── Config ────────────────────────────────────────────────────────────────────
SDK="${ANDROID_HOME:-${HOME}/Library/Android/sdk}"
SDKMANAGER="${SDK}/cmdline-tools/latest/bin/sdkmanager"
AVDMANAGER="${SDK}/cmdline-tools/latest/bin/avdmanager"
EMULATOR="${SDK}/emulator/emulator"
ADB="${SDK}/platform-tools/adb"

SYSTEM_IMAGE="system-images;android-33;google_apis;arm64-v8a"
AVD_NAME="bose-mitm"
AVD_DEVICE="pixel_6"
EMULATOR_SERIAL="emulator-5554"
SNAPSHOT_NAME="mitm-ready"

BOSE_APK="${BOSE_APK:-${ANDROID_DIR}/bose.apk}"
MITM_CA="${HOME}/.mitmproxy/mitmproxy-ca-cert.pem"
# Keep in sync with scripts/android/Dockerfile ARG defaults
MITMPROXY_IMAGE="mitmproxy/mitmproxy:12.2.1"
# Native macOS app (recommended for capture sessions — Docker NAT blocks emulator traffic)
MITMPROXY_NATIVE_URL="https://downloads.mitmproxy.org/12.2.2/mitmproxy-12.2.2-macos-arm64.tar.gz"
MITMPROXY_NATIVE_APP="/Applications/mitmproxy.app"
FRIDA_VERSION="17.9.1"
FRIDA_SERVER="${ANDROID_DIR}/frida-server"

# ── Helpers ───────────────────────────────────────────────────────────────────
info()  { echo "  → $*"; }
ok()    { echo "  ✓ $*"; }
warn()  { echo "  ⚠ $*"; }
die()   { echo "  ✗ $*" >&2; exit 1; }

confirm() {
  local prompt="$1"
  local reply
  read -r -p "  ${prompt} [y/N] " reply
  [[ "${reply}" =~ ^[Yy]$ ]]
}

wait_for_boot() {
  info "Waiting for emulator to finish booting..."
  "${ADB}" -s "${EMULATOR_SERIAL}" wait-for-device
  until "${ADB}" -s "${EMULATOR_SERIAL}" shell getprop sys.boot_completed 2>/dev/null | grep -q "1"; do
    sleep 2
  done
  sleep 3
  ok "Boot complete"
}

# ── Step 1: Check prerequisites ───────────────────────────────────────────────
echo ""
echo "Step 1/9  Check prerequisites"

# Docker — used for frida-server build and CA cert generation
command -v docker &>/dev/null || die "Docker not found. Install Docker Desktop and try again."
docker info &>/dev/null || die "Docker daemon not running. Start Docker Desktop and try again."
ok "Docker available"

# Native mitmproxy app — required for capture sessions (Docker NAT blocks emulator traffic)
if [[ -x "${MITMPROXY_NATIVE_APP}/Contents/MacOS/mitmweb" ]]; then
  ok "mitmproxy native app found at ${MITMPROXY_NATIVE_APP}"
else
  warn "mitmproxy native macOS app not found at ${MITMPROXY_NATIVE_APP}"
  warn "Download and install it before running capture sessions:"
  warn "  curl -L '${MITMPROXY_NATIVE_URL}' -o /tmp/mitmproxy.tar.gz"
  warn "  tar -xzf /tmp/mitmproxy.tar.gz -C /Applications"
fi

# ── Step 2: Generate mitmproxy CA cert (via Docker) ───────────────────────────
echo ""
echo "Step 2/9  Generate mitmproxy CA certificate"
mkdir -p "${HOME}/.mitmproxy"
if [[ -f "${MITM_CA}" ]]; then
  ok "CA cert already present at ${MITM_CA}"
else
  info "Starting mitmproxy container briefly to generate CA..."
  MITM_CID=$(docker run -d \
    -v "${HOME}/.mitmproxy:/home/mitmproxy/.mitmproxy" \
    "${MITMPROXY_IMAGE}" \
    mitmdump --listen-host 0.0.0.0 --listen-port 8080)
  sleep 4
  docker stop "${MITM_CID}" > /dev/null
  docker rm "${MITM_CID}" > /dev/null

  if [[ -f "${HOME}/.mitmproxy/mitmproxy-ca.pem" ]]; then
    openssl x509 -in "${HOME}/.mitmproxy/mitmproxy-ca.pem" -out "${MITM_CA}"
    ok "CA cert extracted to ${MITM_CA}"
  else
    die "CA generation failed — ~/.mitmproxy/mitmproxy-ca.pem not found"
  fi
fi

# Verify issuer
ISSUER=$(openssl x509 -in "${MITM_CA}" -noout -issuer 2>/dev/null)
if echo "${ISSUER}" | grep -qi "mitmproxy"; then
  ok "Cert issuer: ${ISSUER}"
else
  warn "Unexpected cert issuer: ${ISSUER} — verify ${MITM_CA} is the mitmproxy CA"
fi

# ── Step 3: Obtain Bose APK ───────────────────────────────────────────────────
echo ""
echo "Step 3/9  Obtain Bose SoundTouch APK"
if [[ -f "${BOSE_APK}" ]]; then
  ok "APK already present at ${BOSE_APK}"
else
  echo ""
  echo "  The Bose APK is needed but not found at ${BOSE_APK}."
  echo "  Two options:"
  echo "    a) Pull from a real Android device connected via USB"
  echo "    b) Download from a URL you provide (e.g. from APKMirror or APKPure)"
  echo ""
  echo "  APKMirror: https://www.apkmirror.com/apk/bose-corporation/bose-soundtouch/"
  echo "  APKPure:   https://apkpure.com/bose-soundtouch/com.bose.soundtouch"
  echo ""

  PS3="  Choose: "
  select METHOD in "Pull from connected device (adb)" "Download from URL" "Skip (place APK manually later)"; do
    case "${REPLY}" in
      1)
        info "Listing connected devices..."
        "${ADB}" devices
        read -r -p "  Enter device serial (leave blank for default): " DEVICE_SERIAL
        ADB_ARGS=()
        [[ -n "${DEVICE_SERIAL}" ]] && ADB_ARGS=(-s "${DEVICE_SERIAL}")

        APK_PATH=$("${ADB}" "${ADB_ARGS[@]}" shell pm path com.bose.soundtouch \
          | tr -d '\r' | sed 's/package://')
        [[ -n "${APK_PATH}" ]] || die "Bose app not found on device. Is it installed?"

        info "Pulling ${APK_PATH}..."
        "${ADB}" "${ADB_ARGS[@]}" pull "${APK_PATH}" "${BOSE_APK}"
        ok "APK saved to ${BOSE_APK}"
        break
        ;;
      2)
        read -r -p "  Paste the direct APK download URL: " APK_URL
        echo ""
        echo "  URL: ${APK_URL}"
        echo "  Destination: ${BOSE_APK}"
        if confirm "Download from this URL?"; then
          info "Downloading..."
          curl -L --progress-bar "${APK_URL}" -o "${BOSE_APK}"
          ok "APK saved to ${BOSE_APK}"
        else
          warn "Download skipped — place APK at ${BOSE_APK} before continuing"
        fi
        break
        ;;
      3)
        warn "Skipped — place APK at ${BOSE_APK} and re-run this script"
        break
        ;;
      *)
        echo "  Please enter 1, 2, or 3"
        ;;
    esac
  done
fi

[[ -f "${BOSE_APK}" ]] || die "APK not found at ${BOSE_APK} — cannot continue"

# ── Step 4: Install system image ─────────────────────────────────────────────
echo ""
echo "Step 4/9  Install Android system image"
if "${SDKMANAGER}" --list_installed 2>/dev/null | grep -q "${SYSTEM_IMAGE}"; then
  ok "Already installed: ${SYSTEM_IMAGE}"
else
  info "Installing ${SYSTEM_IMAGE} ..."
  "${SDKMANAGER}" "${SYSTEM_IMAGE}"
  ok "Installed"
fi

# ── Step 5: Create AVD ────────────────────────────────────────────────────────
echo ""
echo "Step 5/9  Create AVD '${AVD_NAME}'"
if "${AVDMANAGER}" list avd 2>/dev/null | grep -q "Name: ${AVD_NAME}"; then
  ok "AVD '${AVD_NAME}' already exists — skipping creation"
else
  echo no | "${AVDMANAGER}" create avd \
    -n "${AVD_NAME}" \
    -k "${SYSTEM_IMAGE}" \
    -d "${AVD_DEVICE}"
  ok "Created AVD '${AVD_NAME}'"
fi

# ── Step 6: Build frida image and extract frida-server ────────────────────────
echo ""
echo "Step 6/9  Frida server v${FRIDA_VERSION} (via Docker)"
if [[ -f "${FRIDA_SERVER}" ]]; then
  ok "Already present at ${FRIDA_SERVER}"
else
  info "Building frida Docker image (FRIDA_VERSION=${FRIDA_VERSION})..."
  docker build -q \
    --build-arg "FRIDA_VERSION=${FRIDA_VERSION}" \
    -t "bose-frida:${FRIDA_VERSION}" \
    "${ANDROID_DIR}"

  info "Extracting frida-server binary and SSL scripts from image..."
  CONTAINER_ID=$(docker create "bose-frida:${FRIDA_VERSION}")
  docker cp "${CONTAINER_ID}:/usr/local/bin/frida-server-android" "${FRIDA_SERVER}"
  docker cp "${CONTAINER_ID}:/usr/local/share/frida-scripts/." "${ANDROID_DIR}/frida/"
  docker rm "${CONTAINER_ID}" > /dev/null
  ok "Extracted to ${FRIDA_SERVER} and ${ANDROID_DIR}/frida/"
fi

# ── Step 7: Start emulator ────────────────────────────────────────────────────
echo ""
echo "Step 7/9  Start emulator with writable system"
if "${ADB}" devices | grep -q "${EMULATOR_SERIAL}"; then
  info "Emulator already running — will reuse"
else
  "${EMULATOR}" -avd "${AVD_NAME}" -writable-system -no-snapshot-load &
  EMULATOR_PID=$!
  info "Emulator PID: ${EMULATOR_PID}"
fi
wait_for_boot

# ── Step 8: Root + cert + APK + frida-server ─────────────────────────────────
echo ""
echo "Step 8/9  Configure emulator (root, cert, APK, frida-server)"

"${ADB}" -s "${EMULATOR_SERIAL}" root
"${ADB}" -s "${EMULATOR_SERIAL}" shell avbctl disable-verification
"${ADB}" -s "${EMULATOR_SERIAL}" reboot
wait_for_boot
"${ADB}" -s "${EMULATOR_SERIAL}" root

# Install mitmproxy CA cert
HASH=$(openssl x509 -inform PEM -subject_hash_old -in "${MITM_CA}" | head -1)
"${ADB}" -s "${EMULATOR_SERIAL}" push "${MITM_CA}" /data/local/tmp/mitmproxy.pem
"${ADB}" -s "${EMULATOR_SERIAL}" shell su 0 mkdir -p /data/misc/user/0/cacerts-added
"${ADB}" -s "${EMULATOR_SERIAL}" shell su 0 \
  cp /data/local/tmp/mitmproxy.pem "/data/misc/user/0/cacerts-added/${HASH}.0"
"${ADB}" -s "${EMULATOR_SERIAL}" shell su 0 \
  chmod 644 "/data/misc/user/0/cacerts-added/${HASH}.0"
ok "Certificate installed (hash: ${HASH})"

# Install Bose APK
if "${ADB}" -s "${EMULATOR_SERIAL}" shell pm list packages 2>/dev/null | grep -q "com.bose.soundtouch"; then
  ok "Bose app already installed"
else
  "${ADB}" -s "${EMULATOR_SERIAL}" install "${BOSE_APK}"
  ok "Bose app installed"
fi

# Push frida-server
"${ADB}" -s "${EMULATOR_SERIAL}" push "${FRIDA_SERVER}" /data/local/tmp/frida-server
"${ADB}" -s "${EMULATOR_SERIAL}" shell su 0 chmod 755 /data/local/tmp/frida-server
ok "frida-server pushed"

# ── Step 9: Save snapshot ─────────────────────────────────────────────────────
echo ""
echo "Step 9/9  Save snapshot '${SNAPSHOT_NAME}'"
"${ADB}" -s "${EMULATOR_SERIAL}" emu avd snapshot save "${SNAPSHOT_NAME}"
ok "Snapshot '${SNAPSHOT_NAME}' saved"

# ── Done ──────────────────────────────────────────────────────────────────────
echo ""
echo "Setup complete."
echo ""
echo "  AVD name : ${AVD_NAME}"
echo "  Snapshot : ${SNAPSHOT_NAME}"
echo ""
echo "Start a capture session with:"
echo "  scripts/android/start-mitm-session.sh"
echo ""
echo "Shut down the emulator when done:"
echo "  ${ADB} -s ${EMULATOR_SERIAL} emu kill"
