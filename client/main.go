package main

import (
	"errors"
	"fmt"
	"log"
	"yamaclient/lib"
	"yamaclient/lib/channel"
	"yamaclient/lib/node"
	"yamaclient/lib/settings"

	"gitlab.com/gomidi/midi/v2"

	"gitlab.com/gomidi/midi/v2/drivers"
	_ "gitlab.com/gomidi/midi/v2/drivers/midicatdrv"
)

var config settings.Config
var driver *drivers.Out
var stopRuntime func() error

func init() {
	log.Println("Load Settings")
	if _, err := settings.Decode("settings.toml", &config); err != nil {
		log.Fatal(err)
	}
	log.Println("Load Runtime")
	out, stop, err := config.LaunchDriverRuntime()
	if err != nil {
		log.Fatal(err)
	}
	stopRuntime = stop
	driver = &out
	printPorts()
	log.Println("Ready")
}

func NewPlayer(content *channel.Content) (*channel.Channel, error) {
	file := config.Main.DefaultTrack

	if content == nil {
		content = &channel.Content{Name: file, Source: lib.ContentFile, Data: file}
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
	fmt.Println(midi.GetOutPorts())
}
func RunDefaultTrack() error {
	player, err := NewPlayer(nil)
	if err != nil {
		return err
	}

	log.Printf("DRIVER: %s", config.Main.Runtime)
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
	defer stopRuntime() //nolint

	node := node.New(driver, config.HttpConfig)
	node.Start()
}
