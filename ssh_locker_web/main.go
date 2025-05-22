package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"

	"github.com/duosecurity/duo_universal_golang/duouniversal"
)

const duoUnavailable = "Duo unavailable"
const defaultConfig = "config.json"

type Config struct {
	ClientId     string `json:"clientId"`
	ClientSecret string `json:"clientSecret"`
	ApiHost      string `json:"apiHost"`
	RedirectUri  string `json:"redirectUri"`
	AccessToken  string `json:"accessToken"`
	SocketPath   string `json:"socketPath,omitempty"`
	Port         string `json:"port,omitempty"`
}

type ActionRequest struct {
	User   string `json:"user"`
	Action string `json:"action"`
}

type Session struct {
	duoState    string
	duoUsername string
	request     ActionRequest
}

var currentSessions map[string]Session
var socketPath = "/var/run/ssh_locker.sock"

func main() {

	var configFile string

	flag.StringVar(&configFile, "c", defaultConfig, "Path to the config file")
	flag.Parse()

	file, err := os.Open(configFile)
	config := Config{}
	if err != nil {
		log.Fatal("can't open config file: ", err)
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	err = decoder.Decode(&config)
	if err != nil {
		log.Fatal("can't decode config JSON: ", err)
	}
	// Step 1: Create a Duo client
	duoClient, err := duouniversal.NewClient(config.ClientId, config.ClientSecret, config.ApiHost, config.RedirectUri)
	if err != nil {
		log.Fatal("Error parsing config: ", err)
	}

	currentSessions = make(map[string]Session)
	if config.SocketPath != "" {
		socketPath = config.SocketPath
	}
	if config.Port == "" {
		config.Port = "8080"
	}

	http.HandleFunc("/action", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Only POST allowed", http.StatusMethodNotAllowed)
			return
		}

		if r.Header.Get("X-Auth-Token") != config.AccessToken {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		var req ActionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.User == "" {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}

		session := Session{}
		session.duoUsername = req.User
		session.request = req

		if req.Action != "lock" && req.Action != "unlock" {
			http.Error(w, "Invalid action", http.StatusBadRequest)
			return
		}

		// Step 2: Call the healthCheck to make sure Duo is accessable
		_, err := duoClient.HealthCheck()

		// Step 3: If Duo is not available to authenticate then either allow user
		// to bypass Duo (failopen) or prevent user from authenticating (failclosed)
		if err != nil {
			log.Println("Duo unavailable, fail closed")
			http.Error(w, duoUnavailable, http.StatusInternalServerError)
			return
		}

		// Step 4: Generate and save a state variable
		session.duoState, err = duoClient.GenerateState()
		if err != nil {
			log.Fatal("Error generating state: ", err)
		}

		// Step 5: Create a URL to redirect to inorder to reach the Duo prompt
		redirectToDuoUrl, err := duoClient.CreateAuthURL(session.duoUsername, session.duoState)
		if err != nil {
			log.Fatal("Error creating the auth URL: ", err)
		}

		// Save the session in a map or database
		currentSessions[session.duoState] = session

		// Step 6: Redirect to that prompt
		http.Redirect(w, r, redirectToDuoUrl, http.StatusFound)
	})

	http.HandleFunc("/duo-callback", func(w http.ResponseWriter, r *http.Request) {
		// Step 7: Grab the state and duo_code variables from the URL parameters
		urlState := r.URL.Query().Get("state")
		duoCode := r.URL.Query().Get("duo_code")

		// Step 8: Verify that the state in the URL matches the state saved previously
		session, ok := currentSessions[urlState]
		if !ok {
			log.Println("Session not found")
			http.Error(w, "Session not found", http.StatusBadRequest)
			return
		}

		// remove the session from the map
		delete(currentSessions, urlState)

		if urlState != session.duoState {
			log.Println("State mismatch")
			http.Error(w, "State mismatch", http.StatusBadRequest)
			return
		}

		// Step 9: Exchange the duoCode from the URL parameters and the username of the user trying to authenticate
		// for an authentication token containing information about the auth
		authToken, err := duoClient.ExchangeAuthorizationCodeFor2faResult(duoCode, session.duoUsername)
		if err != nil {
			log.Fatal("Error exchanging authToken: ", err)
		}
		// Step 10: Check if the authentication was successful
		if authToken.AuthResult.Status != "allow" {
			log.Println("Authentication failed")
			http.Error(w, "Authentication failed", http.StatusUnauthorized)
			return
		}

		// Step 11: If the authentication was successful, then perform the action
		doAction(w, r, session.request)
	})

	fmt.Printf("Running on port %s\n", config.Port)
	fmt.Printf("Listening on %s\n", socketPath)
	log.Fatal(http.ListenAndServe(":"+config.Port, nil))
}

// Renders HTML page with message

func doAction(w http.ResponseWriter, r *http.Request, req ActionRequest) {

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		http.Error(w, "Dial error", http.StatusInternalServerError)
		return
	}
	defer conn.Close()

	ip := r.RemoteAddr
	if ip == "" {
		ip = r.Header.Get("X-Forwarded-For")
		if ip == "" {
			ip = r.Header.Get("X-Real-IP")
		}
	}
	if ip == "" {
		http.Error(w, "IP not found", http.StatusBadRequest)
		return
	}

	_, err = fmt.Fprintf(conn, "%s\n", req.Action)
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

	w.Write([]byte(`{"status":"ok","message":"` + req.Action + ` successful"}`))
	w.WriteHeader(http.StatusOK)
	log.Printf("Action %s for user %s from IP %s", req.Action, req.User, ip)
}
