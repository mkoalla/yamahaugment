package settings

import (
	"errors"
	"fmt"

	"github.com/BurntSushi/toml"
)

type MainConfig struct {
	Driver       string `toml:"driver"`
	DefaultTrack string `toml:"default-track"`
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
	Main       MainConfig `toml:"main"`
	HttpConfig HttpConfig `toml:"http"`
}

var (
	ErrConfigValidation = errors.New("bad config")
)

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
