// Package updater implements `local-mind update`: self-replace the running
// binary with the latest GitHub release, verifying SHA256 and (when available)
// the Ed25519 signature over checksums.txt using an embedded public key.
package updater

import (
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const repo = "AgusRdz/local-mind"

var httpClient = &http.Client{Timeout: 60 * time.Second}

// Run checks for a newer release and, if found, replaces the running binary.
// currentVersion is the ldflags-injected version (e.g. "v0.1.1" or "dev").
func Run(currentVersion string, pubKeyPEM []byte) error {
	latest, err := latestTag()
	if err != nil {
		return fmt.Errorf("check latest version: %w", err)
	}

	if currentVersion == latest {
		fmt.Printf("already up to date (%s)\n", currentVersion)
		return nil
	}

	assetBase := fmt.Sprintf("local-mind-%s-%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		assetBase += ".exe"
	}
	base := fmt.Sprintf("https://github.com/%s/releases/download/%s", repo, latest)

	fmt.Printf("updating %s -> %s ...\n", currentVersion, latest)

	binData, err := download(base + "/" + assetBase)
	if err != nil {
		return fmt.Errorf("download %s: %w", assetBase, err)
	}
	sums, err := download(base + "/checksums.txt")
	if err != nil {
		return fmt.Errorf("download checksums: %w", err)
	}

	// --- SHA256 ---
	if err := verifyChecksum(binData, sums, assetBase); err != nil {
		return err
	}

	// --- Ed25519 signature over the exact checksums bytes (best-effort) ---
	if sig, err := download(base + "/checksums.txt.sig"); err == nil {
		if err := verifySignature(pubKeyPEM, sums, sig); err != nil {
			return fmt.Errorf("signature verification failed: %w", err)
		}
		fmt.Println("signature verified")
	} else {
		fmt.Fprintln(os.Stderr, "note: release is unsigned — verified by checksum only")
	}

	if err := replaceSelf(binData); err != nil {
		return err
	}
	fmt.Printf("updated to %s\n", latest)
	return nil
}

func latestTag() (string, error) {
	resp, err := httpClient.Get(fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo))
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API returned %s", resp.Status)
	}
	var rel struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return "", err
	}
	if rel.TagName == "" {
		return "", fmt.Errorf("no tag_name in latest release")
	}
	return rel.TagName, nil
}

func download(url string) ([]byte, error) {
	resp, err := httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	return io.ReadAll(resp.Body)
}

func verifyChecksum(binData, sums []byte, assetName string) error {
	sum := sha256.Sum256(binData)
	actual := hex.EncodeToString(sum[:])
	for _, line := range strings.Split(string(sums), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[1] == assetName {
			if !strings.EqualFold(fields[0], actual) {
				return fmt.Errorf("checksum mismatch for %s: expected %s, got %s", assetName, fields[0], actual)
			}
			return nil
		}
	}
	return fmt.Errorf("checksum not found for %s", assetName)
}

func verifySignature(pubKeyPEM, message, sigHex []byte) error {
	block, _ := pem.Decode(pubKeyPEM)
	if block == nil {
		return fmt.Errorf("invalid embedded public key")
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return err
	}
	edPub, ok := pub.(ed25519.PublicKey)
	if !ok {
		return fmt.Errorf("embedded key is not Ed25519")
	}
	sig, err := hex.DecodeString(strings.TrimSpace(string(sigHex)))
	if err != nil {
		return fmt.Errorf("decode signature: %w", err)
	}
	if !ed25519.Verify(edPub, message, sig) {
		return fmt.Errorf("bad signature")
	}
	return nil
}

// replaceSelf writes binData over the running executable. On Windows a running
// .exe cannot be overwritten in place, but it can be renamed, so we move the
// old one aside first.
func replaceSelf(binData []byte) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	dir := filepath.Dir(exe)

	tmp, err := os.CreateTemp(dir, ".local-mind-update-*")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(binData); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	tmp.Close()
	if err := os.Chmod(tmpName, 0o755); err != nil {
		os.Remove(tmpName)
		return err
	}

	if runtime.GOOS == "windows" {
		old := exe + ".old"
		os.Remove(old)
		if err := os.Rename(exe, old); err != nil {
			os.Remove(tmpName)
			return fmt.Errorf("move current binary aside: %w", err)
		}
		if err := os.Rename(tmpName, exe); err != nil {
			os.Rename(old, exe) // roll back
			return fmt.Errorf("install new binary: %w", err)
		}
		os.Remove(old) // may be locked while running; cleaned on next update
		return nil
	}

	if err := os.Rename(tmpName, exe); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("install new binary: %w", err)
	}
	return nil
}
