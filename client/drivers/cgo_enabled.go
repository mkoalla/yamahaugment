//go:build cgo

package drivers

// in-process ALSA/CoreMIDI/WinMM via rtmidi; needs a C toolchain + ALSA headers
import _ "gitlab.com/gomidi/midi/v2/drivers/rtmididrv"

const MIDI = "rtmididrv"
