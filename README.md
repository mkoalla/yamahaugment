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
