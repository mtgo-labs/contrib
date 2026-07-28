// Package compression provides gzip compression backed by
// github.com/klauspost/compress/gzip.
//
// Gunzip accepts an optional decompressed-size limit for safely processing
// untrusted payloads. Pass zero only when the input size is already trusted.
package compression
