package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"yamaclient/lib/channel"
	"yamaclient/lib/node"
	"yamaclient/lib/settings"

	"gitlab.com/gomidi/midi/v2"

	_ "gitlab.com/gomidi/midi/v2/drivers/midicatdrv"
)

var config settings.Config

func init() {
	if _, err := settings.Decode("settings.toml", &config); err != nil {
		log.Fatal(err)
	}

}
func NewPlayer(content *channel.Content) (*channel.Channel, error) {
	driver, file := config.Main.Driver, config.Main.DefaultTrack
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
	node := node.New(config.HTTPNode)

	go node.Start()
	defer node.Stop(context.Background()) //nolint

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
	err := run()

	if err != nil {
		log.Println(err)
	}
}
