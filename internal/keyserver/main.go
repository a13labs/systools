package keyserver

import (
	"fmt"
	"io"
	"net/http"
)

func ServerAvailable(url string) bool {
	resp, err := http.Head(url)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return true
}

func GetKey(server, uuid string) ([]byte, error) {
	keyurl := fmt.Sprintf("%s/%s", server, uuid)
	resp, err := http.Get(keyurl)
	if err != nil || resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to download key from key server: %v", err)
	}
	defer resp.Body.Close()
	key, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read key from key server: %v", err)
	}
	if len(key) == 0 {
		return nil, fmt.Errorf("key is empty")
	}
	return key, nil
}
