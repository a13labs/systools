package main

import (
	"bufio"
	"flag"
	"fmt"
	"net"
	"os"
)

var socketPath = "/var/run/ssh_locker.sock"

func main() {
	var socket string
	flag.StringVar(&socket, "s", socketPath, "Path to unix socket")
	flag.Parse()
	if len(flag.Args()) != 1 {
		fmt.Println("Usage: client [lock|unlock]")
		os.Exit(1)
	}
	if socket != "" {
		socketPath = socket
	}
	cmd := flag.Arg(0)

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		fmt.Println("Dial error:", err)
		return
	}
	defer conn.Close()

	_, err = fmt.Fprintf(conn, "%s\n", cmd)
	if err != nil {
		fmt.Println("Write error:", err)
		return
	}

	resp, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		fmt.Println("Read error:", err)
		return
	}
	fmt.Print(resp)
}
