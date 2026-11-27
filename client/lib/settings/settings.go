package settings

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/BurntSushi/toml"
	"gitlab.com/gomidi/midi/v2"
	"gitlab.com/gomidi/midi/v2/drivers"
)

type Runtime = string

const (
	FluidSynth = "fluidsynth"
)

const (
	runtimePortTimeout = 5 * time.Second
	runtimeStopTimeout = 2 * time.Second
)

var (
	ErrConfigValidation = errors.New("bad config")
	ErrUnknownRuntime   = errors.New("unknown runtime")
	ErrLoadingRuntime   = errors.New("while loading runtime")
)

type MainConfig struct {
	Runtime      Runtime `toml:"runtime"`
	DefaultTrack string  `toml:"default-track"`
}
type DriverConfig struct {
	Audio      string   `toml:"audio-driver"`
	MIDI       string   `toml:"midi-driver"`
	SoundFonts []string `toml:"sound-fonts"`
}
type NodeConfig struct {
	Port      uint `toml:"port"`
	QueueSize uint `toml:"queue-size"`
	SongCache uint `toml:"song-cache"`
}
type LimitsConfig struct {
	BodyPreferKB       uint `toml:"body-prefer-kb"`
	CacheExpirySeconds uint `toml:"cache-expiry-sec"`
}
type HttpConfig struct {
	NodeConfig `toml:"node"`
	Limits     LimitsConfig `toml:"limits"`
}
type Config struct {
	Main       MainConfig   `toml:"main"`
	Driver     DriverConfig `toml:"driver"`
	HttpConfig HttpConfig   `toml:"http"`
}

// starts the runtime and waits for its out port; call stop() on shutdown
func (c *Config) LaunchDriverRuntime() (out drivers.Out, stop func() error, err error) {
	dc := c.Driver
	var cmd *exec.Cmd
	switch c.Main.Runtime {
	case FluidSynth:
		args := append(
			[]string{"-q", "-p", "fluidsynth", "-a", dc.Audio, "-m", dc.MIDI},
			dc.SoundFonts...,
		)
		cmd = exec.Command("fluidsynth", args...)
	default:
		return nil, nil, ErrUnknownRuntime
	}
	cmd.Stderr = os.Stderr
	// fluidsynth's shell quits on stdin EOF (and with -i it has nothing to wait on),
	// so hold stdin open for its lifetime; closing it is a graceful shutdown
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %w", ErrLoadingRuntime, err)
	}
	if err := cmd.Start(); err != nil { // Start, not Run: the runtime is long-lived
		return nil, nil, fmt.Errorf("%w: %w", ErrLoadingRuntime, err)
	}

	// reap as soon as it exits, so it never lingers as a zombie
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()

	stop = sync.OnceValue(func() error {
		_ = stdin.Close()
		select {
		case err := <-exited:
			return err
		case <-time.After(runtimeStopTimeout):
			_ = cmd.Process.Kill()
			return <-exited
		}
	})

	// the runtime registers its port asynchronously after launch
	deadline := time.Now().Add(runtimePortTimeout)
	for {
		if out, err = midi.FindOutPort(c.Main.Runtime); err == nil {
			return out, stop, nil
		}
		select {
		case werr := <-exited:
			return nil, nil, fmt.Errorf("%w: exited before opening port: %v", ErrLoadingRuntime, werr)
		default:
		}
		if time.Now().After(deadline) {
			_ = stop()
			return nil, nil, fmt.Errorf("%w: no out port '%s': %w", ErrLoadingRuntime, c.Main.Runtime, err)
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func Decode(path string, conf *Config) (toml.MetaData, error) {
	// in case validation code is added
	md, err := toml.DecodeFile(path, &conf)
	if err != nil {
		return md, fmt.Errorf("%w: %w", ErrConfigValidation, err)
	}
	if conf.HttpConfig.QueueSize == 0 || conf.HttpConfig.SongCache == 0 {
		return md, fmt.Errorf("%w: queue and cache must be at least 1", ErrConfigValidation)
	}
	return md, nil
}
