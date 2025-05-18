package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"encoding/csv"
	"os/signal"
	"syscall"

	"github.com/a13labs/systools/internal/system"
	"github.com/aws/aws-sdk-go-v2/config"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"sigs.k8s.io/aws-encryption-provider/pkg/cloud"
	"sigs.k8s.io/aws-encryption-provider/pkg/logging"
	"sigs.k8s.io/aws-encryption-provider/pkg/plugin"
	"sigs.k8s.io/aws-encryption-provider/pkg/server"
)

type Credentials struct {
	AccessKey string `json:"access_key"`
	SecretKey string `json:"secret_key"`
	KeyArn    string `json:"key_arn"`
	Region    string `json:"region,omitempty"`
}

func main() {
	if len(os.Args) < 2 {
		fmt.Printf("Usage: %s <keyserver> [socket]\n", os.Args[0])
		fmt.Printf("Example: %s https://keyserver.example.com\n", os.Args[0])
		fmt.Printf("Example: %s https://keyserver.example.com /var/run/kmsplugin/socket.sock\n", os.Args[0])
		os.Exit(1)
	}

	keyserver := os.Args[1]
	socket := "/var/run/kmsplugin/socket.sock"
	if len(os.Args) > 2 {
		socket = os.Args[2]
	}

	// Check if keyserver is reachable
	resp, err := http.Get(keyserver)
	if err != nil {
		log.Printf("Keyserver is not reachable: %v, %d", err, resp.StatusCode)
		os.Exit(1)
	}

	// Generate UUID
	uuid, err := system.GetUniqueID()
	if err != nil {
		log.Printf("Failed to generate UUID: %v", err)
		os.Exit(1)
	}

	log.Println("Downloading key from key server")
	keyURL := fmt.Sprintf("%s/%s-encryption-service-credentials.json", keyserver, uuid)
	resp, err = http.Get(keyURL)
	if err != nil || resp.StatusCode != 200 {
		log.Printf("Failed to download key file: %v", err)
		os.Exit(1)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body) // Changed from ioutil.ReadAll
	if err != nil {
		log.Println("Failed to read key file")
		os.Exit(1)
	}

	var creds Credentials
	if err := json.Unmarshal(body, &creds); err != nil {
		log.Println("Failed to extract credentials from key file")
		os.Exit(1)
	}

	if creds.Region == "" {
		creds.Region = "eu-west-1"
	}

	os.Setenv("AWS_ACCESS_KEY_ID", creds.AccessKey)
	os.Setenv("AWS_SECRET_ACCESS_KEY", creds.SecretKey)

	// Check if AWS credentials are valid
	var optFns []func(*config.LoadOptions) error
	if creds.Region != "" {
		optFns = append(optFns, config.WithRegion(creds.Region))
	}

	cfg, err := config.LoadDefaultConfig(context.Background(), optFns...)
	if err != nil {
		fmt.Printf("failed to create AWS config: %v", err)
		os.Exit(1)
	}

	_, err = cfg.Credentials.Retrieve(context.Background())
	if err != nil {
		fmt.Printf("failed to retrieve AWS credentials")
		os.Exit(1)
	}

	// Prepare socket dir
	socketDir := filepath.Dir(socket)
	os.MkdirAll(socketDir, 0700)
	os.Chmod(socketDir, 0700)

	runServer([]string{creds.KeyArn}, []string{socket}, creds.Region, "", 0, 0, 0, []string{}, false)
}

func runServer(keys []string, addrs []string, region string, kmsEndpoint string, qpsLimit int, burstLimit int, retryTokenCapacity int, encryptionCtxsArr []string, debug bool) {

	encryptionCtxs := []map[string]string{}
	for _, encryptionCtxStr := range encryptionCtxsArr {
		encryptionCtx, err := stringToStringConv(encryptionCtxStr)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to parse encryption-context: %v", err)
			os.Exit(1)
		}
		encryptionCtxs = append(encryptionCtxs, encryptionCtx.(map[string]string))
	}

	if len(keys) != len(addrs) {
		fmt.Fprintf(os.Stderr, "key and listen lists must have the same number of elements")
		os.Exit(1)
	}

	logLevel := zapcore.InfoLevel
	if debug {
		logLevel = zapcore.DebugLevel
	}

	l, err := logging.NewStandardLogger(logLevel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to configure logging")
		os.Exit(1)
	}

	zap.ReplaceGlobals(l)

	zap.L().Info("creating kms server",
		zap.String("region", region),
		zap.Strings("listen-address", addrs),
		zap.String("kms-endpoint", kmsEndpoint),
		zap.Int("qps-limit", qpsLimit),
		zap.Int("burst-limit", burstLimit),
		zap.Int("retry-token-capacity", retryTokenCapacity),
	)
	c, err := cloud.New(region, kmsEndpoint, qpsLimit, burstLimit, retryTokenCapacity)
	if err != nil {
		zap.L().Fatal("Failed to create new KMS service", zap.Error(err))
	}

	for i, encryptionCtx := range encryptionCtxs {
		for k, v := range encryptionCtx {
			zap.L().Info("encryption-context", zap.Int("index", i), zap.String("key", k), zap.String(
				"value", v))
		}
	}

	sharedHealthCheck := plugin.NewSharedHealthCheck(plugin.DefaultHealthCheckPeriod, plugin.DefaultErrcBufSize)
	go sharedHealthCheck.Start()
	defer sharedHealthCheck.Stop()

	servers := []*server.Server{}

	for i, key := range keys {
		s := server.New()
		servers = append(servers, s)
		encryptionCtx := getOrDefault(encryptionCtxs, i, map[string]string{})

		p := plugin.New(key, c, encryptionCtx, sharedHealthCheck)
		p.Register(s.Server)
		p2 := plugin.NewV2(key, c, encryptionCtx, sharedHealthCheck)
		p2.Register(s.Server)
	}

	for i, addr := range addrs {
		s := servers[i]

		go func() {
			if err := s.ListenAndServe(addr); err != nil {
				zap.L().Fatal("Failed to start server", zap.Error(err))
			}
		}()

		zap.L().Info("Plugin server started", zap.String("port", addr))
	}

	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)

	signal := <-signals

	zap.L().Info("Received signal", zap.Stringer("signal", signal))
	zap.L().Info("Shutting down server")
	for _, s := range servers {
		s.GracefulStop()
	}
	zap.L().Info("Exiting...")
	os.Exit(0)
}

// get index in array or return default value if out of index
func getOrDefault[T any](arr []T, index int, defaultVal T) T {
	if index >= len(arr) || index < 0 {
		return defaultVal
	}
	return arr[index]
}

// parses string into map[string]string
// based on plog's stringToString implementation
func stringToStringConv(val string) (interface{}, error) {
	val = strings.Trim(val, "[]")
	// An empty string would cause an empty map
	if len(val) == 0 {
		return map[string]string{}, nil
	}
	r := csv.NewReader(strings.NewReader(val))
	ss, err := r.Read()
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(ss))
	for _, pair := range ss {
		kv := strings.SplitN(pair, "=", 2)
		if len(kv) != 2 {
			return nil, fmt.Errorf("%s must be formatted as key=value", pair)
		}
		out[kv[0]] = kv[1]
	}
	return out, nil
}
