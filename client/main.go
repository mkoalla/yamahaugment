package main

import (
	"errors"
	"fmt"
	"log"
	"yamaclient/lib/channel"

	"github.com/BurntSushi/toml"
	"gitlab.com/gomidi/midi/v2"

	_ "gitlab.com/gomidi/midi/v2/drivers/midicatdrv"
)

type mainConfig struct {
	Driver       string `toml:"driver"`
	DefaultTrack string `toml:"default-track"`
}
type config struct {
	Main mainConfig `toml:"main"`
}

var settings config

func init() {
	if _, err := toml.DecodeFile("settings.toml", &settings); err != nil {
		log.Fatalf("Couldn't decode settings: %v", err)
	}
}
func NewPlayer(content *channel.Content) (*channel.Channel, error) {
	driver, file := settings.Main.Driver, settings.Main.DefaultTrack
	out, err := midi.FindOutPort(driver)
	if err != nil {
		return nil, fmt.Errorf("can't find '%s'", driver)
	}
	if content == nil {
		content = &channel.Content{Source: channel.ContentFile, Data: file}
	} else if file == "" {
		return nil, errors.New("no file provided")
	}
	player := channel.New(&out)
	if err := player.Load(*content, true); err != nil {
		return nil, err
	}
	return player, err
}

const HELP = "args: [file]"

func printPorts() {
	log.Println(midi.GetOutPorts())
}
func run() error {
	player, err := NewPlayer(nil)
	if err != nil {
		return err
	}
	log.Printf("DRIVER: %s", settings.Main.Driver)
	log.Printf("Now Playing: %s", settings.Main.DefaultTrack)
	start, err := player.GetPlayer()
	if err != nil {
		return err
	}
	go start() //nolint
	return player.Await()
}

func main() {
	defer midi.CloseDriver()
	printPorts()
	err := run()

	if err != nil {
		log.Println(err)
	}
}
