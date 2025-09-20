package main

import (
	"fmt"
	"os"

	"github.com/a13labs/systools/internal/k8sutil"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintf(os.Stderr, "Usage: %s <namespace> <pod-prefix> [<extra args>...]\n", os.Args[0])
		os.Exit(1)
	}
	rootless := false
	args := os.Args[1:]
	// Check for -r flag
	filteredArgs := []string{}
	for _, arg := range args {
		if arg == "-r" {
			rootless = true
		} else {
			filteredArgs = append(filteredArgs, arg)
		}
	}
	if len(filteredArgs) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <namespace> <pod-prefix> [-r] [<extra args>...]\n", os.Args[0])
		os.Exit(1)
	}
	namespace := filteredArgs[0]
	prefix := filteredArgs[1]

	config, err := k8sutil.GetKubeConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to build kubeconfig: %v\n", err)
		os.Exit(1)
	}

	clientset, err := k8sutil.GetClientset(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create clientset: %v\n", err)
		os.Exit(1)
	}

	podName, err := k8sutil.FindRunningPod(clientset, namespace, prefix)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	cmd := make([]string, 0, 5)
	cmd = append(cmd, "env", "SSH_ORIGINAL_COMMAND="+os.Getenv("SSH_ORIGINAL_COMMAND"))

	if !rootless {
		cmd = append(cmd, "su", "-s", "/bin/bash", "git", "--")
	} else {
		cmd = append(cmd, "/bin/bash", "--")
	}

	// Append any extra arguments passed to this program (excluding -r)
	if len(filteredArgs) > 2 {
		cmd = append(cmd, filteredArgs[2:]...)
	}

	err = k8sutil.ExecCommand(clientset, config, namespace, podName, cmd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Exec stream failed: %v\n", err)
		os.Exit(1)
	}
}
