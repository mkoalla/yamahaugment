//go:build cgo && !linux

package drivers

// rtmidi on CoreMIDI/WinMM may care which thread calls it
// ALSA and midicat don't (disabled on linux)
const LockThread = true
