# yamahaugment
- building with midicat (`CGO_ENABLED=0`) results in the program's input ports busywaiting, so compile wisely

## Process priority (single core systems)
Playback timing depends on how fast Linux schedules the client when a note is due. If you're having issues, raise the client's priority with one of these in its systemd unit. Keep `WorkingDirectory` set, since file access is relative.

```ini
[Service]
WorkingDirectory=/path/to/client
ExecStart=/path/to/client/yamaclient
```

1. **Nice (recommended):** the client gets the CPU ahead of normal processes, and nothing can lock up.
   ```ini
   Nice=-10
   ```
2. **round robin:** the client always runs before every normal process. Only use this if measurements show `Nice` isn't enough. midicat build = death sentence
   ```ini
   CPUSchedulingPolicy=rr
   CPUSchedulingPriority=10
   CPUSchedulingResetOnFork=yes
   ```

## Dependencies
### Running the client

- **ALSA sequencer**: the `snd-seq` kernel module (loaded by default), and the user must be in the `audio` group to open `/dev/snd/seq`.
- **MIDI driver**: depends on how the binary was built (see `client/drivers/`).
  - cgo build (`CGO_ENABLED=1`, rtmidi; the default for native builds): `libasound2t64`, `libstdc++6`
  - non-cgo build (`CGO_ENABLED=0`, midicat; what CI releases): [`midicat`](https://gitlab.com/gomidi/tools) on `PATH`
- **Dev Runtime (opt)**: (software synth stand-in for the piano): `fluidsynth` is implemented + plus a soundfont, e.g. `fluid-soundfont-gm`.

### Building
- `go`
- `gcc`, `g++`, `libasound2` / `libasound2-dev` (ALSA headers for rtmidi)
- `make`, `git`
