package settings

import (
	"fmt"

	"github.com/BurntSushi/toml"
)

type MainConfig struct {
	Driver       string `toml:"driver"`
	DefaultTrack string `toml:"default-track"`
}
type NodeConfig struct {
	Port uint `toml:"port"`
}
type Config struct {
	Main     MainConfig `toml:"main"`
	HTTPNode NodeConfig `toml:"listener"`
}

func Decode(path string, conf *Config) (toml.MetaData, error) {
	// in case validation code is added
	md, err := toml.DecodeFile(path, &conf)
	if err != nil {
		return md, fmt.Errorf("while decoding %s: %w", path, err)
	}
	return md, nil
}
