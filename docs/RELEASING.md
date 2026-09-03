# Releasing local-mind

## Prerequisites (one-time)

1. **Give `gh` the `workflow` scope** (needed to push files under `.github/workflows/`):
   ```
   gh auth refresh -h github.com -s workflow
   ```

2. **(Optional) Set up release signing.** The pipeline signs the checksums file
   with an Ed25519 key; the installers verify it if `public_key.pem` is present.
   Signing is skipped gracefully if the `SIGNING_KEY` secret is not set.

   ```bash
   # generate an Ed25519 keypair (run outside the repo, or in a dir git ignores)
   openssl genpkey -algorithm ed25519 -out signing.pem
   openssl pkey -in signing.pem -pubout -out public_key.pem

   # commit ONLY the public key (signing.pem is gitignored — never commit it)
   cp public_key.pem /path/to/local-mind/public_key.pem
   cd /path/to/local-mind && git add public_key.pem && git commit -m "chore: add release signing public key"

   # store the private key as a repo secret (base64), then delete the local copy
   base64 -w0 signing.pem | gh secret set SIGNING_KEY --repo AgusRdz/local-mind
   shred -u signing.pem 2>/dev/null || rm -f signing.pem
   ```

   > On macOS `base64 -w0` is just `base64`. On Windows PowerShell:
   > `[Convert]::ToBase64String([IO.File]::ReadAllBytes("signing.pem")) | gh secret set SIGNING_KEY --repo AgusRdz/local-mind`

## Cutting a release

Releases are tag-driven. The `Release` workflow runs on any `v*` tag: it tests,
validates the semver tag, regenerates the changelog, cross-builds five binaries,
writes `checksums.txt`, signs it (if `SIGNING_KEY` is set), creates the GitHub
Release with generated notes, and attests build provenance.

```bash
# auto-detect the bump from conventional commits since the last tag
make release           # feat! / BREAKING -> major, feat -> minor, else patch

# or force a specific bump
make release-patch     # vX.Y.Z -> vX.Y.(Z+1)
make release-minor
make release-major
```

`make release*` requires [git-cliff](https://git-cliff.org/docs/installation)
locally to regenerate `CHANGELOG.md`, then it commits, tags, and pushes — which
triggers the workflow. To release without the Make targets, just push a tag:

```bash
git tag v0.1.0 && git push origin v0.1.0
```

## Installing a published release

```bash
# Linux / macOS / Git-Bash (uses your gh login for the private repo)
curl -fsSL https://raw.githubusercontent.com/AgusRdz/local-mind/main/install.sh | sh

# Windows PowerShell
irm https://raw.githubusercontent.com/AgusRdz/local-mind/main/install.ps1 | iex
```

Both prefer the `gh` CLI (using your existing login, required for a private
repo) and fall back to `GITHUB_TOKEN`/`GH_TOKEN`. They verify the SHA256
checksum always, and the Ed25519 signature when `public_key.pem` is alongside
the script.
