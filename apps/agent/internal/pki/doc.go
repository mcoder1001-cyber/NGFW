// Package pki stages operational PKI material under an agent-owned root. The layout uses
// x509, x509ca, private and x509crl subdirectories, without enabling a system daemon.
// Certificate and CRL fingerprints are SHA-256; private-key fingerprints are agent-local
// HMAC. Keys are 0600 inside 0700 directories, validated against their certificate, and
// excluded from scheduler values and responses. Atomic apply rolls back file changes on
// failure; a persisted manifest limits cleanup to owned files and supports restart recovery.
package pki
