#!/bin/sh
set -e

REPO="AgusRdz/local-mind"

# --- OS detection ---
OS="$(uname -s)"
case "$OS" in
  Linux*)  OS="linux" ;;
  Darwin*) OS="darwin" ;;
  MINGW*|MSYS*|CYGWIN*) OS="windows" ;;
  *) echo "unsupported OS: $OS" >&2; exit 1 ;;
esac

# --- install dir ---
if [ -z "$LOCAL_MIND_INSTALL_DIR" ]; then
  if [ "$OS" = "windows" ]; then
    INSTALL_DIR="$(cygpath "$LOCALAPPDATA/Programs/local-mind" 2>/dev/null || echo "$HOME/AppData/Local/Programs/local-mind")"
  else
    INSTALL_DIR="$HOME/.local/bin"
  fi
else
  INSTALL_DIR="$LOCAL_MIND_INSTALL_DIR"
fi

# --- arch detection ---
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) echo "unsupported architecture: $ARCH" >&2; exit 1 ;;
esac

EXT=""
[ "$OS" = "windows" ] && EXT=".exe"
BINARY="local-mind-${OS}-${ARCH}${EXT}"

# --- resolve version ---
if [ -z "$LOCAL_MIND_VERSION" ]; then
  LOCAL_MIND_VERSION=$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
    | grep '"tag_name"' | sed 's/.*"tag_name": *"//;s/".*//')
fi
[ -z "$LOCAL_MIND_VERSION" ] && { echo "failed to determine latest version" >&2; exit 1; }

URL="https://github.com/${REPO}/releases/download/${LOCAL_MIND_VERSION}/${BINARY}"
CHECKSUMS_URL="https://github.com/${REPO}/releases/download/${LOCAL_MIND_VERSION}/checksums.txt"
SIG_URL="https://github.com/${REPO}/releases/download/${LOCAL_MIND_VERSION}/checksums.txt.sig"

echo "installing local-mind ${LOCAL_MIND_VERSION} (${OS}/${ARCH})..."
mkdir -p "$INSTALL_DIR"
TMP="${INSTALL_DIR}/local-mind${EXT}.tmp"
curl -fsSL "$URL" -o "$TMP"

# --- verify SHA256 ---
CHECKSUMS=$(curl -fsSL "$CHECKSUMS_URL") || { echo "failed to download checksums.txt" >&2; rm -f "$TMP"; exit 1; }
EXPECTED=$(printf '%s' "$CHECKSUMS" | grep " ${BINARY}$" | awk '{print $1}')
[ -z "$EXPECTED" ] && { echo "checksum not found for ${BINARY}" >&2; rm -f "$TMP"; exit 1; }
if command -v sha256sum >/dev/null 2>&1; then
  ACTUAL=$(sha256sum "$TMP" | awk '{print $1}')
elif command -v shasum >/dev/null 2>&1; then
  ACTUAL=$(shasum -a 256 "$TMP" | awk '{print $1}')
else
  echo "warning: no sha256 tool, skipping checksum verification" >&2
  ACTUAL="$EXPECTED"
fi
[ "$ACTUAL" != "$EXPECTED" ] && { echo "checksum mismatch: expected $EXPECTED, got $ACTUAL" >&2; rm -f "$TMP"; exit 1; }

# --- optional Ed25519 signature verification (if signed + public_key.pem present) ---
PUBKEY="$(dirname "$0")/public_key.pem"
if [ -f "$PUBKEY" ] && command -v openssl >/dev/null 2>&1; then
  if SIG=$(curl -fsSL "$SIG_URL" 2>/dev/null); then
    printf '%s' "$CHECKSUMS" > "${TMP}.sums"
    printf '%s' "$SIG" | xxd -r -p > "${TMP}.sig" 2>/dev/null || true
    if [ -s "${TMP}.sig" ] && openssl pkeyutl -verify -pubin -inkey "$PUBKEY" -rawin -in "${TMP}.sums" -sigfile "${TMP}.sig" >/dev/null 2>&1; then
      echo "signature verified"
    else
      echo "WARNING: signature verification failed" >&2
    fi
    rm -f "${TMP}.sums" "${TMP}.sig"
  fi
fi

mv "$TMP" "${INSTALL_DIR}/local-mind${EXT}"
chmod +x "${INSTALL_DIR}/local-mind${EXT}"
echo "installed local-mind to ${INSTALL_DIR}/local-mind${EXT}"
echo ""

# --- PATH wiring ---
case ":$PATH:" in
  *":${INSTALL_DIR}:"*) ;;
  *)
    if [ "$OS" = "windows" ]; then
      WIN_DIR=$(cygpath -w "$INSTALL_DIR" 2>/dev/null || echo "$INSTALL_DIR")
      powershell.exe -NoProfile -Command "\$p=[Environment]::GetEnvironmentVariable('Path','User'); \$d='${WIN_DIR}'.TrimEnd('\\'); if ((\$p -split ';' | ForEach-Object { \$_.TrimEnd('\\') }) -notcontains \$d) { [Environment]::SetEnvironmentVariable('Path', \"\$d;\$p\", 'User'); Write-Host \"added \$d to User PATH\" }"
      export PATH="${INSTALL_DIR}:$PATH"
    else
      SHELL_NAME="$(basename "${SHELL:-}")"
      case "$SHELL_NAME" in
        zsh)  SHELL_RC="$HOME/.zshrc" ;;
        bash) SHELL_RC="$HOME/.bashrc" ;;
        *)    SHELL_RC="" ;;
      esac
      PATH_LINE="export PATH=\"${INSTALL_DIR}:\$PATH\""
      if [ -n "$SHELL_RC" ] && ! grep -qF "$INSTALL_DIR" "$SHELL_RC" 2>/dev/null; then
        printf '\n# local-mind\n%s\n' "$PATH_LINE" >> "$SHELL_RC"
        echo "added ${INSTALL_DIR} to PATH in $SHELL_RC (reload: source $SHELL_RC)"
      else
        echo "NOTE: add ${INSTALL_DIR} to your PATH: $PATH_LINE"
      fi
    fi
    ;;
esac

echo ""
echo "next steps:"
echo "  local-mind init       # register the UserPromptSubmit hook"
echo "  local-mind rebuild    # build the index from your notes"
