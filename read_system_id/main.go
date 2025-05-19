package main

import (
	"fmt"
	"os"

	"github.com/a13labs/systools/internal/system"
)

func main() {
	uuid, err := system.GetUniqueID()
	if err != nil {
		fmt.Println("Error:", err)
		os.Exit(1)
	}
	fmt.Println(uuid)
}
