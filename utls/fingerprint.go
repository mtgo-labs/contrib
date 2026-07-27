package utls

import utls "github.com/refraction-networking/utls"

// Fingerprint is a ClientHello fingerprint identifier.
// It maps directly to uTLS's ClientHelloID so additional fingerprints
// (Firefox, Safari, randomized, etc.) can be used without API changes.
type Fingerprint = utls.ClientHelloID

// Well-known fingerprints.
//
// The list is non-exhaustive; any utls.ClientHelloID constant can be used
// as a Fingerprint.
var (
	// FingerprintChrome selects the latest Chrome fingerprint.
	FingerprintChrome = utls.HelloChrome_Auto

	// FingerprintFirefox selects the latest Firefox fingerprint.
	FingerprintFirefox = utls.HelloFirefox_Auto

	// FingerprintSafari selects the latest Safari fingerprint.
	FingerprintSafari = utls.HelloSafari_Auto

	// FingerprintiOS selects the latest iOS fingerprint.
	FingerprintiOS = utls.HelloIOS_Auto

	// FingerprintRandomized selects a randomized fingerprint.
	FingerprintRandomized = utls.HelloRandomized

	// FingerprintGolang selects Go's default crypto/tls handshake.
	FingerprintGolang = utls.HelloGolang
)
