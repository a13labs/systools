package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"path/filepath"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/client-go/util/homedir"
)

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintf(os.Stderr, "Usage: %s <namespace> <pod-prefix> [<extra args>...]\n", os.Args[0])
		os.Exit(1)
	}
	namespace := os.Args[1]
	prefix := os.Args[2]

	// Build Kubernetes client config
	var config *rest.Config
	var err error
	if kubeconfig := os.Getenv("KUBECONFIG"); kubeconfig != "" {
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	} else if home := homedir.HomeDir(); home != "" {
		kubeconfigPath := filepath.Join(home, ".kube", "config")
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	} else {
		config, err = rest.InClusterConfig()
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to build kubeconfig: %v\n", err)
		os.Exit(1)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create clientset: %v\n", err)
		os.Exit(1)
	}

	// List pods in the namespace
	pods, err := clientset.CoreV1().Pods(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to list pods: %v\n", err)
		os.Exit(1)
	}

	var podName string
	for _, pod := range pods.Items {
		if strings.HasPrefix(pod.Name, prefix) && pod.Status.Phase == corev1.PodRunning {
			podName = pod.Name
			break
		}
	}

	// Remove "-c g" from the command line arguments
	args := os.Args[3:]
	var filteredArgs []string
	for i := 0; i < len(args); i++ {
		if args[i] == "-c" {
			i++
			continue
		}
		filteredArgs = append(filteredArgs, args[i])
	}
	command := strings.Join(filteredArgs, " ")

	if podName == "" {
		fmt.Fprintln(os.Stderr, "No running pod found matching prefix")
		os.Exit(1)
	}

	// Prepare the exec request
	req := clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: "", // default container
			Command:   []string{"env", "SSH_ORIGINAL_COMMAND=" + os.Getenv("SSH_ORIGINAL_COMMAND"), "su", "-s", "/bin/bash", "git", "--", command},
			Stdin:     true,
			Stdout:    true,
			Stderr:    true,
			TTY:       false,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(config, "POST", req.URL())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create executor: %v\n", err)
		os.Exit(1)
	}

	err = exec.StreamWithContext(context.Background(), remotecommand.StreamOptions{
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Tty:    false,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Exec stream failed: %v\n", err)
		os.Exit(1)
	}
}
