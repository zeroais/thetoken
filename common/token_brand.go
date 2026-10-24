package common

import "mime"

// TOKEN fork: default user-visible system name. Kept in its own file so the
// upstream default in constants.go stays untouched for rebases. An admin can
// still override it at runtime via the SystemName option.
func init() {
	SystemName = "TOKEN"
	// Serve the PWA manifest with its registered type; Go has no built-in
	// mapping and the slim runtime image has no /etc/mime.types.
	_ = mime.AddExtensionType(".webmanifest", "application/manifest+json")
}
