package main

import (
	"errors"
	"log"
	"yamaclient/lib/api"
	"yamaclient/lib/channel"
	"yamaclient/lib/node"
	"yamaclient/lib/settings"

	"gitlab.com/gomidi/midi/v2"

	"gitlab.com/gomidi/midi/v2/drivers"
	_ "gitlab.com/gomidi/midi/v2/drivers/midicatdrv"
)

var config settings.Config
var driver *drivers.Out

func init() {
	var err error
	if _, err := settings.Decode("settings.toml", &config); err != nil {
		log.Fatal(err)
	}
	out, err := midi.FindOutPort(config.Main.Driver)
	if err != nil {
		log.Fatalf("can't find driver '%s'", config.Main.Driver)
	}
	driver = &out
}

func NewPlayer(content *channel.Content) (*channel.Channel, error) {
	file := config.Main.DefaultTrack

	if content == nil {
		content = &channel.Content{Name: file, Source: api.ContentFile, Data: file}
	} else if file == "" {
		return nil, errors.New("no file provided")
	}
	player := channel.New(driver)
	if err := player.Load(*content, true); err != nil {
		return nil, err
	}
	return player, nil
}

const HELP = "args: [file]"

func printPorts() {
	log.Println(midi.GetOutPorts())
}
func RunDefaultTrack() error {
	player, err := NewPlayer(nil)
	if err != nil {
		return err
	}

	log.Printf("DRIVER: %s", config.Main.Driver)
	log.Printf("Now Playing: %s", config.Main.DefaultTrack)
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

	node := node.New(driver, config.HttpConfig)
	node.Start()
}
