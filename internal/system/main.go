package system

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"

	"github.com/zcalusic/sysinfo"
)

// GetUniqueID generates a unique ID for the system by hashing various system information.
func GetUniqueID() (string, error) {
	var si sysinfo.SysInfo

	si.GetSysInfo()

	nodeData, _ := json.MarshalIndent(&si.Node, "", "  ")
	productData, _ := json.MarshalIndent(&si.Product, "", "  ")
	boardData, _ := json.MarshalIndent(&si.Board, "", "  ")
	chassisData, _ := json.MarshalIndent(&si.Chassis, "", "  ")
	biosData, _ := json.MarshalIndent(&si.BIOS, "", "  ")
	cpuData, _ := json.MarshalIndent(&si.CPU, "", "  ")
	memData, _ := json.MarshalIndent(&si.Memory, "", "  ")
	storageData, _ := json.MarshalIndent(&si.Storage, "", "  ")
	networkData, _ := json.MarshalIndent(&si.Network, "", "  ")

	uuid := string(nodeData) + string(productData) + string(boardData) + string(chassisData) + string(biosData) + string(cpuData) + string(memData) + string(storageData) + string(networkData)
	if uuid == "" {
		return "", fmt.Errorf("failed to get unique ID")
	}

	h := md5.New()
	io.WriteString(h, uuid)
	return hex.EncodeToString(h.Sum(nil)), nil
}
