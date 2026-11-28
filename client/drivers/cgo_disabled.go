//go:build !cgo

package drivers

// talks to a spawned midicat helper (which must be on PATH)
import _ "gitlab.com/gomidi/midi/v2/drivers/midicatdrv"

const MIDI = "midicatdrv"
