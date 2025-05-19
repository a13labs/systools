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
	namespace := os.Args[1]
	prefix := os.Args[2]

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

	cmd := []string{
		"env", "SSH_ORIGINAL_COMMAND=" + os.Getenv("SSH_ORIGINAL_COMMAND"),
		"su", "-s", "/bin/bash", "git", "--",
	}

	// Append any extra arguments passed to this program
	if len(os.Args) > 3 {
		cmd = append(cmd, os.Args[3:]...)
	}

	err = k8sutil.ExecCommand(clientset, config, namespace, podName, cmd)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Exec stream failed: %v\n", err)
		os.Exit(1)
	}
}
