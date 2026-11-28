//go:build !cgo || linux

package drivers

// ALSA and midicat don't (disabled on linux)
const LockThread = false
