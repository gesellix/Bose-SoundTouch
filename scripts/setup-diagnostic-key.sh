#!/usr/bin/env bash
# setup-diagnostic-key.sh — one-time key generation for the encrypted
# diagnostic export feature. Run this once as the project maintainer.
#
# Output:
#   keys/private/diagnostic      SSH ed25519 private key  (gitignored)
#   keys/private/diagnostic.pub  Matching public key      (gitignored — copy in keys/public/)
#   keys/public/diagnostic.pub   Public key in version control
#
# After running this script:
#   1. Add the public key to your GitHub account SSH keys so it appears
#      at https://github.com/<you>.keys — this lets users verify the key.
#   2. Update the DiagnosticPublicKey constant in
#      pkg/service/export/encrypt.go to match keys/public/diagnostic.pub.
#   3. Commit keys/public/diagnostic.pub and the updated constant.
#   4. Keep keys/private/diagnostic somewhere safe (the .gitignore protects
#      it from accidental commits, but it is NOT backed up by git).

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
KEY_DIR="$REPO_ROOT/keys"
PRIVATE_DIR="$KEY_DIR/private"
PUBLIC_DIR="$KEY_DIR/public"

mkdir -p "$PRIVATE_DIR" "$PUBLIC_DIR"

KEY_FILE="$PRIVATE_DIR/diagnostic"

if [[ -f "$KEY_FILE" ]]; then
    echo "Key already exists at $KEY_FILE — delete it first to regenerate."
    exit 1
fi

ssh-keygen -t ed25519 \
    -C "aftertouch-diagnostic@gesellix" \
    -N "" \
    -f "$KEY_FILE"

cp "$KEY_FILE.pub" "$PUBLIC_DIR/diagnostic.pub"
cp "$KEY_FILE.pub" "$REPO_ROOT/pkg/service/export/diagnostic.pub"

echo
echo "Keys generated:"
echo "  Private : $KEY_FILE                              (gitignored — keep safe)"
echo "  Public  : $PUBLIC_DIR/diagnostic.pub             (canonical — add to GitHub)"
echo "  Embed   : pkg/service/export/diagnostic.pub      (compiled into binary)"
echo
echo "Public key:"
cat "$PUBLIC_DIR/diagnostic.pub"
echo
echo "Next steps:"
echo "  1. Add the public key to your GitHub account:"
echo "     https://github.com/settings/ssh/new"
echo "  2. Commit keys/public/diagnostic.pub and pkg/service/export/diagnostic.pub."
