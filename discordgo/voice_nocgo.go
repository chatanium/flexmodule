//go:build voice && !cgo

package discordgo

// Intentionally a compile-time configuration error, not an unavailable backend.
var _ = voice_build_requires_CGO_ENABLED_1_and_pinned_libdave
