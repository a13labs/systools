package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"

	duoapi "github.com/duosecurity/duo_api_golang"
	"github.com/duosecurity/duo_api_golang/authapi"
)

type KnockRequest struct {
	IP string `json:"ip"`
}

var secretToken string
var socketPath = "/var/run/ssh_locker.sock"

func main() {
	secretToken = os.Getenv("KNOCK_SECRET")
	socket := os.Getenv("SSH_LOCKER_SOCKET")
	if socket != "" {
		socketPath = socket
	}

	http.HandleFunc("/knock", knockHandler)
	log.Println("Starting knock server on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func knockHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST allowed", http.StatusMethodNotAllowed)
		return
	}

	if r.Header.Get("X-Auth-Token") != secretToken {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req KnockRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.IP == "" {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if !duoAuth(req.IP) {
		http.Error(w, "Duo auth failed", http.StatusUnauthorized)
		return
	}

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		http.Error(w, "Dial error", http.StatusInternalServerError)
		return
	}
	defer conn.Close()

	_, err = fmt.Fprintf(conn, "%s\n", "unlock")
	if err != nil {
		http.Error(w, "Write error", http.StatusInternalServerError)
		return
	}

	resp, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		http.Error(w, "Read error", http.StatusInternalServerError)
		return
	}
	fmt.Print(resp)

	w.Write([]byte(`{"status":"ok","ip":"` + req.IP + `"}`))
}

func duoAuth(ip string) bool {
	ikey := os.Getenv("DUO_IKEY")
	skey := os.Getenv("DUO_SKEY")
	host := os.Getenv("DUO_HOST")
	user := os.Getenv("DUO_USER")

	duo := authapi.NewAuthApi(*duoapi.NewDuoApi(ikey, skey, host, "ssh_locker"))
	result, err := duo.Auth("auto", authapi.AuthUserId(user), authapi.AuthIpAddr(ip))
	if err != nil || result.Response.Status != "allow" {
		log.Printf("Duo auth failed: %v, %v", result.Response.Status_Msg, err)
		return false
	}
	return true
}
