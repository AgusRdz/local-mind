#!/bin/sh
set -e

# local-mind installer (Linux/macOS/Git-Bash).
#
# local-mind is a PRIVATE repo, so release assets require authentication.
# This script prefers the GitHub CLI (`gh`, using your existing login) and
# falls back to curl with a token from $GITHUB_TOKEN / $GH_TOKEN.

REPO="AgusRdz/local-mind"

# --- OS / arch detection ---
OS="$(uname -s)"
case "$OS" in
  Linux*)  OS="linux" ;;
  Darwin*) OS="darwin" ;;
  MINGW*|MSYS*|CYGWIN*) OS="windows" ;;
  *) echo "unsupported OS: $OS" >&2; exit 1 ;;
esac

ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) echo "unsupported architecture: $ARCH" >&2; exit 1 ;;
esac

EXT=""
[ "$OS" = "windows" ] && EXT=".exe"
BINARY="local-mind-${OS}-${ARCH}${EXT}"

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

have_gh() { command -v gh >/dev/null 2>&1 && gh auth status >/dev/null 2>&1; }

# --- resolve version ---
VERSION="$LOCAL_MIND_VERSION"
if [ -z "$VERSION" ]; then
  if have_gh; then
    VERSION=$(gh release view --repo "$REPO" --json tagName -q .tagName 2>/dev/null || true)
  else
    TOKEN="${GITHUB_TOKEN:-$GH_TOKEN}"
    [ -z "$TOKEN" ] && { echo "need gh CLI logged in, or GITHUB_TOKEN set (private repo)" >&2; exit 1; }
    VERSION=$(curl -fsSL -H "Authorization: Bearer $TOKEN" \
      "https://api.github.com/repos/${REPO}/releases/latest" \
      | grep '"tag_name"' | sed 's/.*"tag_name": *"//;s/".*//')
  fi
fi
[ -z "$VERSION" ] && { echo "failed to determine latest version" >&2; exit 1; }

echo "installing local-mind ${VERSION} (${OS}/${ARCH})..."
mkdir -p "$INSTALL_DIR"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

# --- download binary + checksums (+ optional signature) ---
if have_gh; then
  gh release download "$VERSION" --repo "$REPO" --dir "$WORK" \
    --pattern "$BINARY" --pattern "checksums.txt" --pattern "checksums.txt.sig" 2>/dev/null || {
      # signature is optional; retry without it
      gh release download "$VERSION" --repo "$REPO" --dir "$WORK" \
        --pattern "$BINARY" --pattern "checksums.txt"
    }
else
  TOKEN="${GITHUB_TOKEN:-$GH_TOKEN}"
  base="https://api.github.com/repos/${REPO}/releases"
  # Resolve asset download via the API (works for private repos).
  dl() { # $1=asset name
    aid=$(curl -fsSL -H "Authorization: Bearer $TOKEN" "${base}/tags/${VERSION}" \
      | grep -B3 "\"name\": \"$1\"" | grep '"id"' | head -1 | grep -o '[0-9]\+')
    [ -z "$aid" ] && return 1
    curl -fsSL -H "Authorization: Bearer $TOKEN" -H "Accept: application/octet-stream" \
      "${base}/assets/${aid}" -o "$WORK/$1"
  }
  dl "$BINARY" || { echo "failed to download $BINARY" >&2; exit 1; }
  dl "checksums.txt" || { echo "failed to download checksums.txt" >&2; exit 1; }
  dl "checksums.txt.sig" || true
fi

[ -f "$WORK/$BINARY" ] || { echo "binary not found after download" >&2; exit 1; }

# --- verify SHA256 ---
EXPECTED=$(grep " ${BINARY}\$" "$WORK/checksums.txt" | awk '{print $1}')
[ -z "$EXPECTED" ] && { echo "checksum not found for ${BINARY}" >&2; exit 1; }
if command -v sha256sum >/dev/null 2>&1; then
  ACTUAL=$(sha256sum "$WORK/$BINARY" | awk '{print $1}')
elif command -v shasum >/dev/null 2>&1; then
  ACTUAL=$(shasum -a 256 "$WORK/$BINARY" | awk '{print $1}')
else
  echo "warning: no sha256 tool, skipping checksum verification" >&2
  ACTUAL="$EXPECTED"
fi
[ "$ACTUAL" != "$EXPECTED" ] && { echo "checksum mismatch: expected $EXPECTED, got $ACTUAL" >&2; exit 1; }

# --- optional signature verification (only if published + public_key present) ---
PUBKEY="$(dirname "$0")/public_key.pem"
if [ -f "$WORK/checksums.txt.sig" ] && [ -f "$PUBKEY" ] && command -v openssl >/dev/null 2>&1; then
  SIG_BIN="$WORK/checksums.txt.sig.bin"
  if xxd -r -p "$WORK/checksums.txt.sig" > "$SIG_BIN" 2>/dev/null &&
     openssl pkeyutl -verify -pubin -inkey "$PUBKEY" -rawin -in "$WORK/checksums.txt" -sigfile "$SIG_BIN" >/dev/null 2>&1; then
    echo "signature verified"
  else
    echo "WARNING: signature verification failed" >&2
  fi
fi

# --- install ---
mv "$WORK/$BINARY" "${INSTALL_DIR}/local-mind${EXT}"
chmod +x "${INSTALL_DIR}/local-mind${EXT}"
echo "installed local-mind to ${INSTALL_DIR}/local-mind${EXT}"
echo ""

# --- PATH hint ---
case ":$PATH:" in
  *":${INSTALL_DIR}:"*) ;;
  *)
    if [ "$OS" = "windows" ]; then
      WIN_DIR=$(cygpath -w "$INSTALL_DIR" 2>/dev/null || echo "$INSTALL_DIR")
      powershell.exe -NoProfile -Command "\$p=[Environment]::GetEnvironmentVariable('Path','User'); \$d='${WIN_DIR}'.TrimEnd('\\'); if ((\$p -split ';' | ForEach-Object { \$_.TrimEnd('\\') }) -notcontains \$d) { [Environment]::SetEnvironmentVariable('Path', \"\$d;\$p\", 'User'); Write-Host \"added \$d to User PATH\" }"
    else
      echo "NOTE: add ${INSTALL_DIR} to your PATH:"
      echo "  export PATH=\"${INSTALL_DIR}:\$PATH\""
    fi
    ;;
esac

echo ""
echo "next steps:"
echo "  local-mind init       # register the UserPromptSubmit hook"
echo "  local-mind rebuild    # build the index from your notes"
