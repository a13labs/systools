package main

import (
	"bufio"
	"flag"
	"fmt"
	"net"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"log"
)

// Change consts to vars so they can be set by flags
var (
	socketPath      = "/var/run/ssh_locker.sock"
	autoLockTimeout = 5 * time.Minute
	timer           *time.Timer
	timerLock       sync.Mutex
)

const (
	lockFileName = "authorized_keys.lock"
	authKeysName = "authorized_keys"
)

func setupSignalHandler(ln net.Listener) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Printf("Received shutdown signal, shutting down...")
		ln.Close()
		os.Remove(socketPath)
		lockFile()
		os.Exit(0)
	}()
}

func getSSHDir() (string, error) {
	usr, err := user.Current()
	if err != nil {
		return "", err
	}
	return filepath.Join(usr.HomeDir, ".ssh"), nil
}

func lockFile() error {
	sshDir, err := getSSHDir()
	if err != nil {
		return err
	}
	authKeys := filepath.Join(sshDir, authKeysName)
	lockFile := filepath.Join(sshDir, lockFileName)
	if _, err := os.Stat(authKeys); err == nil {
		return os.Rename(authKeys, lockFile)
	}
	return nil // Already locked
}

func unlockFile() error {
	sshDir, err := getSSHDir()
	if err != nil {
		return err
	}
	authKeys := filepath.Join(sshDir, authKeysName)
	lockFile := filepath.Join(sshDir, lockFileName)
	if _, err := os.Stat(lockFile); err == nil {
		return os.Rename(lockFile, authKeys)
	}
	return nil // Already unlocked
}

func startAutoLock() {
	timerLock.Lock()
	defer timerLock.Unlock()
	if timer != nil {
		timer.Stop()
	}
	timer = time.AfterFunc(autoLockTimeout, func() {
		log.Printf("Auto-locking after %v", autoLockTimeout)
		lockFile()
	})
}

func handleCommand(cmd string) string {
	switch strings.TrimSpace(strings.ToLower(cmd)) {
	case "lock":
		log.Printf("Received lock command")
		if err := lockFile(); err != nil {
			return "Lock failed: " + err.Error()
		}
		return "Locked"
	case "unlock":
		log.Printf("Received unlock command")
		if err := unlockFile(); err != nil {
			return "Unlock failed: " + err.Error()
		}
		startAutoLock()
		return fmt.Sprintf("Unlocked. Will auto-lock in %v", autoLockTimeout)
	default:
		return "Unknown command"
	}
}

func handleConn(conn net.Conn) {
	defer conn.Close()
	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		resp := handleCommand(scanner.Text())
		conn.Write([]byte(resp + "\n"))
	}
}

func main() {
	var (
		socket  string
		timeout string
	)
	flag.StringVar(&socket, "s", socketPath, "Path to unix socket")
	flag.StringVar(&timeout, "t", autoLockTimeout.String(), "Auto-lock timeout (e.g. 5m, 30s)")
	flag.Parse()

	if socket != "" {
		socketPath = socket
	}
	if timeout != "" {
		if d, err := time.ParseDuration(timeout); err == nil {
			autoLockTimeout = d
		}
	}
	// configure logging to include timestamp
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	os.Remove(socketPath)
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		log.Printf("Listen error: %v", err)
		return
	}
	defer ln.Close()
	os.Chmod(socketPath, 0666)

	log.Printf("Listening on %s", socketPath)
	log.Printf("Commands: lock, unlock")
	log.Printf("Auto-lock timeout: %v", autoLockTimeout)
	lockFile()
	setupSignalHandler(ln)
	for {
		conn, err := ln.Accept()
		if err != nil {
			continue
		}
		go handleConn(conn)
	}
}
