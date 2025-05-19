package k8sutil

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/remotecommand"
	"k8s.io/client-go/util/homedir"
)

// GetKubeConfig returns a Kubernetes rest.Config using KUBECONFIG, ~/.kube/config, or in-cluster config.
func GetKubeConfig() (*rest.Config, error) {
	if kubeconfig := os.Getenv("KUBECONFIG"); kubeconfig != "" {
		return clientcmd.BuildConfigFromFlags("", kubeconfig)
	}
	if home := homedir.HomeDir(); home != "" {
		kubeconfigPath := filepath.Join(home, ".kube", "config")
		return clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	}
	return rest.InClusterConfig()
}

// GetClientset returns a Kubernetes clientset using the provided config.
func GetClientset(config *rest.Config) (*kubernetes.Clientset, error) {
	return kubernetes.NewForConfig(config)
}

// FindRunningPod returns the name of the first running pod in the namespace with the given prefix.
func FindRunningPod(clientset *kubernetes.Clientset, namespace, prefix string) (string, error) {
	pods, err := clientset.CoreV1().Pods(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return "", fmt.Errorf("failed to list pods: %w", err)
	}
	for _, pod := range pods.Items {
		if strings.HasPrefix(pod.Name, prefix) && pod.Status.Phase == corev1.PodRunning {
			return pod.Name, nil
		}
	}
	return "", fmt.Errorf("no running pod found matching prefix")
}

func ExecCommand(clientset *kubernetes.Clientset, config *rest.Config, namespace, podName string, cmd []string) error {
	req := clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(podName).
		Namespace(namespace).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: "", // default container
			Command:   cmd,
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

	err = exec.StreamWithContext(context.TODO(), remotecommand.StreamOptions{
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Tty:    false,
	})

	return err
}
