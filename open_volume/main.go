package main

import (
	"fmt"
	"log"
	"os"

	"github.com/a13labs/systools/internal/keyserver"
	"github.com/a13labs/systools/internal/system"
)

func main() {
	log.SetFlags(0)
	if len(os.Args) != 4 {
		fmt.Printf("Usage: %s <server> <encrypted device> <mapper device>\n", os.Args[0])
		fmt.Printf("Example: %s https://server.example.com /dev/sda1 /dev/mapper/crypt1\n", os.Args[0])
		fmt.Printf("Example: %s https://server.example.com /path/to/image.img /dev/mapper/crypt1\n", os.Args[0])
		os.Exit(1)
	}
	server := os.Args[1]
	encrypedDevice := os.Args[2]
	mapperDevice := os.Args[3]

	if !system.FileExists(encrypedDevice) {
		log.Printf("open_volume: (%s) Image file not found", mapperDevice)
		os.Exit(1)
	}

	if !keyserver.ServerAvailable(server) {
		log.Printf("open_volume: (%s) Key server not reachable, exiting.", mapperDevice)
		os.Exit(1)
	}

	if system.DeviceMapperExists(mapperDevice) {
		fmt.Printf("open_volume: (%s) Device already open, exiting.\n", mapperDevice)
		os.Exit(1)
	}

	uuid, err := system.GetUniqueID()
	if err != nil {
		log.Printf("open_volume: (%s) Failed to get unique ID", mapperDevice)
		os.Exit(1)
	}

	key, err := keyserver.GetKey(server, uuid)
	if err != nil {
		log.Printf("open_volume: (%s) Failed to get key from server: %s", mapperDevice, err)
		os.Exit(1)
	}

	// Use key from stdin instead of writing temp file
	err = system.OpenVolume(encrypedDevice, mapperDevice, key)
	if err != nil {
		log.Printf("open_volume: (%s) Failed to open volume: %s", mapperDevice, err)
		os.Exit(1)
	}

	log.Printf("open_volume: (%s) Volume opened successfully", mapperDevice)
	os.Exit(0)
}
