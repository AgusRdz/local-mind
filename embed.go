package main

import _ "embed"

// publicKeyPEM is the Ed25519 release-signing public key, embedded at build
// time so `local-mind update` can verify release signatures without needing
// openssl or a local key file.
//
//go:embed public_key.pem
var publicKeyPEM []byte
