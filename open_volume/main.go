package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"

	"github.com/a13labs/systools/internal/system"
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
	return true
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

	// Use key from stdin instead of writing temp file
	log.Printf("open_volume:'%s' Key downloaded successfully, opening volume", mapperDevice)
	cmd := exec.Command("/usr/sbin/cryptsetup", "luksOpen", encrypedDevice, mapperDevice, "-d", "-")
	cmd.Stdin = resp.Body
	out, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("open_volume: (%s) Failed to open volume: %s", mapperDevice, string(out))
		os.Exit(1)
	}

	log.Printf("open_volume: (%s) Volume opened successfully", mapperDevice)
	os.Exit(0)
}
