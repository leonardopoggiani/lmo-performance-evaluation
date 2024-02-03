package cmd

import (
	"context"
	"os"
	"os/exec"
	"strings"

	"github.com/leonardopoggiani/live-migration-operator/controllers"
	"github.com/leonardopoggiani/live-migration-operator/controllers/types"
	"github.com/spf13/cobra"
	"github.com/withmandala/go-log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

var checkpointAllCmd = &cobra.Command{
	Use:   "checkpoint-all",
	Short: "Checkpoint all the pods in the selected namespace",
	Run: func(cmd *cobra.Command, args []string) {
		logger := log.New(os.Stderr).WithColor()
		logger.Info("checkpoint-all command called")

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		// Load Kubernetes config
		kubeconfigPath := os.Getenv("KUBECONFIG")
		if kubeconfigPath == "" {
			kubeconfigPath = "~/.kube/config"
		}

		kubeconfigPath = os.ExpandEnv(kubeconfigPath)
		if _, err := os.Stat(kubeconfigPath); os.IsNotExist(err) {
			logger.Info("Kubeconfig file not found")
			return
		}

		kubeconfig, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			logger.Errorf("Error loading kubeconfig")
			return
		}

		// Create Kubernetes API client
		clientset, err := kubernetes.NewForConfig(kubeconfig)
		if err != nil {
			logger.Errorf("Error creating kubernetes client")
			return
		}

		namespace := os.Getenv("NAMESPACE")

		var containers []types.Container

		pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			logger.Error(err.Error())
			return
		}

		for _, pod := range pods.Items {
			for _, containerStatus := range pod.Status.ContainerStatuses {
				idParts := strings.Split(containerStatus.ContainerID, "//")

				logger.Info("containerStatus.ContainerID: " + containerStatus.ContainerID)
				logger.Info("containerStatus.Name: " + containerStatus.Name)

				if len(idParts) < 2 {
					logger.Error("Malformed container ID")
					return
				}
				containerID := idParts[1]

				container := types.Container{
					ID:   containerID,
					Name: containerStatus.Name,
				}
				containers = append(containers, container)
			}
		}

		controllers.CheckpointPodPipelined(containers, namespace, pods.Items[0].Name)
	},
}

func init() {
	exec.Command("sudo", "rm", "-rf", "/tmp/checkpoints/checkpoints")
	exec.Command("sudo", "mkdir", "/tmp/checkpoints/checkpoints")
	rootCmd.AddCommand(checkpointAllCmd)
}
