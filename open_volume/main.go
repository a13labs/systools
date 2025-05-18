package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/a13labs/systools/internal/system"
	"github.com/anatol/luks.go"
)

func usage() {
	fmt.Printf("Usage: %s <keyserver> <encrypted device> <mapper device>\n", os.Args[0])
	fmt.Printf("Example: %s https://keyserver.example.com /dev/sda1 /dev/mapper/crypt1\n", os.Args[0])
	fmt.Printf("Example: %s https://keyserver.example.com /path/to/image.img /dev/mapper/crypt1\n", os.Args[0])
	os.Exit(1)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func deviceExists(device string) bool {
	_, err := os.Stat("/dev/mapper/" + device)
	return err == nil
}

func keyserverReachable(url string) bool {
	resp, err := http.Head(url)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode < 400
}

func main() {
	log.SetFlags(0)
	if len(os.Args) != 4 {
		usage()
	}
	keyserver := os.Args[1]
	encrypedDevice := os.Args[2]
	mapperDevice := os.Args[3]

	if !fileExists(encrypedDevice) {
		log.Printf("open_volume: (%s) Image file not found", mapperDevice)
		os.Exit(1)
	}

	if !keyserverReachable(keyserver) {
		log.Printf("open_volume: (%s) Key server not reachable, exiting.", mapperDevice)
		os.Exit(1)
	}

	if deviceExists(mapperDevice) {
		fmt.Printf("open_volume: (%s) Device already open, exiting.\n", mapperDevice)
		os.Exit(1)
	}

	uuid, err := system.GetUniqueID()
	if err != nil {
		log.Printf("open_volume: (%s) Failed to get unique ID", mapperDevice)
		os.Exit(1)
	}

	log.Printf("open_volume: (%s) Downloading key from key server,", mapperDevice)
	keyurl := fmt.Sprintf("%s/%s", keyserver, uuid)
	resp, err := http.Get(keyurl)
	if err != nil || resp.StatusCode != 200 {
		log.Printf("open_volume: (%s) Failed to download key from key server", mapperDevice)
		os.Exit(1)
	}
	defer resp.Body.Close()

	// Read the key from the response to a []byte
	key, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("open_volume: (%s) Failed to read key from response", mapperDevice)
		os.Exit(1)
	}

	log.Printf("open_volume:'%s' Key downloaded successfully, opening volume", mapperDevice)

	dev, err := luks.Open(encrypedDevice)
	if err != nil {
		log.Printf("open_volume: (%s) Failed to open LUKS device: %s", mapperDevice, err)
		os.Exit(1)
	}

	volume, err := dev.UnsealVolume(0, key)
	if err == luks.ErrPassphraseDoesNotMatch {
		log.Printf("open_volume: (%s) Key does not match", mapperDevice)
		os.Exit(1)
	} else if err != nil {
		log.Printf("open_volume: (%s) Failed to unseal volume: %s", mapperDevice, err)
		os.Exit(1)
	} else {
		err := volume.SetupMapper(mapperDevice)
		if err != nil {
			log.Printf("open_volume: (%s) Failed to setup mapper: %s", mapperDevice, err)
			os.Exit(1)
		}
	}

	log.Printf("open_volume: (%s) Volume opened successfully", mapperDevice)
	os.Exit(0)
}
