package cmd

import (
	"context"
	"os"
	"strconv"

	"github.com/leonardopoggiani/live-migration-operator/controllers"
	pkg "github.com/leonardopoggiani/lmo-performance-evaluation/pkg"
	"github.com/spf13/cobra"
	"github.com/withmandala/go-log"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a test pod with a random name",
	Run: func(cmd *cobra.Command, args []string) {
		logger := log.New(os.Stderr).WithColor()
		logger.Info("create command called")

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
		containersNumber, err := strconv.Atoi(args[0])
		if err != nil {
			logger.Errorf("Error converting containers number")
			return
		}

		pkg.CreateTestContainers(ctx, containersNumber, clientset, controllers.LiveMigrationReconciler{}, namespace)
	},
}

func init() {
	rootCmd.AddCommand(createCmd)
}
