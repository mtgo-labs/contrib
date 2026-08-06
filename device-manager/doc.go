// Package device generates realistic Telegram client device profiles
// for MTProto initConnection, matching the mtcute TypeScript device
// profile generation patterns.
//
// Usage:
//
//	import devicemanager "github.com/mtgo-labs/contrib/device-manager"
//
//	// Deterministic profile from session ID
//	profile := devicemanager.GenerateAndroid("my-session")
//
//	// Copy fields into a raw.Config
//	cfg.InitConnection.DeviceModel = profile.DeviceModel
//	cfg.InitConnection.SystemVersion = profile.SystemVersion
//	// ...
//
//	// Or use the convenience helper
//	dev := profile.ToInitConnection()
//	cfg.InitConnection.DeviceModel = dev.DeviceModel
//	cfg.InitConnection.SystemVersion = dev.SystemVersion
//	// ...
//
// Features:
//   - 11 device types (Android, AndroidX, Plus, iOS, Desktop, etc.)
//   - Deterministic generation from unique IDs
//   - Official Telegram client presets
//   - Thread-safe lazy initialization
//   - Zero external dependencies
package device
