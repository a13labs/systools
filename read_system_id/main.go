package main

import (
	"fmt"

	"github.com/a13labs/systools/internal/system"
)

func main() {
	uuid, err := system.GetUniqueID()
	if err != nil {
		fmt.Println("Error:", err)
		return
	}
	fmt.Println(uuid)
}
