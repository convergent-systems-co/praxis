//go:build !darwin

package bootstrapv4

const imageBindings = "non-darwin: file bytes read through the start-time descriptor plus pathname identity; in-place rewrite of a running executable relies on the operating system refusing writes to executing images (not qualified here)"

// bindExecutingCode has no kernel-recorded code identity to consult outside
// darwin. The remaining guarantee is the descriptor read and pathname identity
// in verifyProcessImage, plus the OS refusing to open a running executable for
// writing where it does (Linux returns ETXTBSY). That is a platform property
// stated in imageBindings, not something this package proves.
func bindExecutingCode(data []byte) error { return nil }
