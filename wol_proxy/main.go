package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sabhiram/go-wol/wol"
)

const (
	// maxWait is the maximum time to wait for the upstream server to become available.
	maxWait = 60 * time.Second
	// pollInterval is the interval at which to poll the upstream server.
	pollInterval = 2 * time.Second
	// defaultBroadcastAddr is the default broadcast address for WOL packets.
	defaultBroadcastAddr = "255.255.255.255:9"
)

// Config holds the application configuration.
type Config struct {
	UpstreamMAC    string
	UpstreamIP     string
	UpstreamPort   string
	UpstreamScheme string
	ProxyPort      string
}

// loadConfig loads configuration from command-line flags and environment variables.
func loadConfig() *Config {
	cfg := &Config{}

	flag.StringVar(&cfg.UpstreamMAC, "mac", os.Getenv("UPSTREAM_MAC"), "Upstream MAC address")
	flag.StringVar(&cfg.UpstreamIP, "ip", os.Getenv("UPSTREAM_IP"), "Upstream IP address")
	flag.StringVar(&cfg.UpstreamPort, "port", os.Getenv("UPSTREAM_PORT"), "Upstream port")
	flag.StringVar(&cfg.UpstreamScheme, "scheme", os.Getenv("UPSTREAM_SCHEME"), "Upstream scheme (http or https)")
	flag.StringVar(&cfg.ProxyPort, "proxy-port", os.Getenv("PROXY_PORT"), "Proxy port")
	flag.Parse()

	if cfg.UpstreamMAC == "" {
		log.Fatal("Upstream MAC address must be set via --mac flag or UPSTREAM_MAC environment variable")
	}
	if cfg.UpstreamIP == "" {
		log.Fatal("Upstream IP address must be set via --ip flag or UPSTREAM_IP environment variable")
	}
	if cfg.UpstreamPort == "" {
		cfg.UpstreamPort = "80"
	}
	if cfg.UpstreamScheme == "" {
		cfg.UpstreamScheme = "http"
	}
	if cfg.ProxyPort == "" {
		cfg.ProxyPort = "5000"
	}

	return cfg
}

// wakeUpstream sends a Wake-on-LAN (WOL) magic packet to the configured MAC address.
func wakeUpstream(macAddress string) error {
	log.Println("Sending Wake-on-LAN packet...")

	udpAddr, err := net.ResolveUDPAddr("udp", defaultBroadcastAddr)
	if err != nil {
		return fmt.Errorf("failed to resolve UDP address: %w", err)
	}

	// Build the magic packet.
	mp, err := wol.New(macAddress)
	if err != nil {
		return fmt.Errorf("failed to create magic packet: %w", err)
	}

	// Get the byte representation of the magic packet.
	bs, err := mp.Marshal()
	if err != nil {
		return fmt.Errorf("failed to marshal magic packet: %w", err)
	}

	// Create a UDP connection to send the packet.
	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		return fmt.Errorf("failed to dial UDP: %w", err)
	}
	defer conn.Close()

	log.Printf("Attempting to send a magic packet to MAC %s", macAddress)
	log.Printf("... Broadcasting to: %s", defaultBroadcastAddr)
	n, err := conn.Write(bs)
	if err != nil {
		return fmt.Errorf("failed to write magic packet: %w", err)
	}
	if n != 102 {
		return fmt.Errorf("magic packet sent was %d bytes (expected 102 bytes)", n)
	}

	log.Printf("Magic packet sent successfully to %s", macAddress)
	return nil
}

// waitForUpstream waits for the upstream server to become available.
// It polls a health check URL until it gets a successful response or times out.
func waitForUpstream(scheme, ip, port string) bool {
	start := time.Now()
	checkURL := fmt.Sprintf("%s://%s:%s", scheme, ip, port)
	log.Printf("Waiting for upstream server at %s", checkURL)

	for {
		// Create a new client with a short timeout for each check
		client := http.Client{
			Timeout: pollInterval,
		}
		resp, err := client.Get(checkURL)
		if err == nil {
			// We consider any 2xx status code as success.
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				resp.Body.Close()
				log.Println("Upstream server is online.")
				return true
			}
			resp.Body.Close()
		}

		if time.Since(start) > maxWait {
			log.Println("Timed out waiting for upstream server.")
			return false
		}
		time.Sleep(pollInterval)
	}
}

// wakingTransport is a custom http.RoundTripper that wakes the upstream server
// if it's not available.
type wakingTransport struct {
	transport http.RoundTripper
	cfg       *Config
}

// RoundTrip implements the http.RoundTripper interface.
func (t *wakingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// First, attempt to proxy the request.
	resp, err := t.transport.RoundTrip(req)
	if err != nil {
		// If the error is a network dialing error, it suggests the server is down.
		if e, ok := err.(*net.OpError); ok && e.Op == "dial" {
			log.Printf("Upstream not available: %v. Attempting to wake...", err)

			// Send the Wake-on-LAN packet.
			if wakeErr := wakeUpstream(t.cfg.UpstreamMAC); wakeErr != nil {
				log.Printf("Failed to send WOL packet: %v", wakeErr)
				return nil, fmt.Errorf("failed to send WOL packet: %w", wakeErr)
			}

			// Wait for the server to come online.
			log.Println("Waiting for upstream server to become available...")
			if !waitForUpstream(t.cfg.UpstreamScheme, t.cfg.UpstreamIP, t.cfg.UpstreamPort) {
				log.Println("Upstream server did not become available in time")
				return nil, fmt.Errorf("upstream server did not become available in time")
			}

			// Retry the request now that the server should be up.
			log.Println("Server is online, retrying the request.")
			return t.transport.RoundTrip(req)
		}
		// For other types of errors, return them directly.
		return nil, err
	}
	// If the first attempt was successful, return the response.
	return resp, nil
}

// newProxy creates a new reverse proxy with the waking transport.
func newProxy(cfg *Config) *httputil.ReverseProxy {
	director := func(req *http.Request) {
		target, _ := url.Parse(fmt.Sprintf("%s://%s:%s", cfg.UpstreamScheme, cfg.UpstreamIP, cfg.UpstreamPort))
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		// We let the gin router handle the path.
		// req.URL.Path is already set by gin's c.Param("proxyPath")
		req.Host = target.Host
	}

	// Create a custom transport that handles waking the server.
	transport := &wakingTransport{
		transport: http.DefaultTransport,
		cfg:       cfg,
	}

	return &httputil.ReverseProxy{
		Director:  director,
		Transport: transport,
	}
}

func main() {
	// Load configuration from flags and environment variables.
	cfg := loadConfig()

	// Set up the Gin router.
	r := gin.Default()

	// Create the reverse proxy.
	proxy := newProxy(cfg)

	// Handle all requests with the proxy.
	r.Any("/*proxyPath", func(c *gin.Context) {
		proxy.ServeHTTP(c.Writer, c.Request)
	})

	log.Printf("WOL proxy listening on :%s", cfg.ProxyPort)
	if err := r.Run(":" + cfg.ProxyPort); err != nil {
		log.Fatalf("Failed to run server: %v", err)
	}
}
