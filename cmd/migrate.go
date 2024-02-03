package cmd

import (
	"context"
	"os"

	"github.com/leonardopoggiani/live-migration-operator/controllers"
	"github.com/spf13/cobra"
	"github.com/withmandala/go-log"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Migrate the given pod",
	Run: func(cmd *cobra.Command, args []string) {
		logger := log.New(os.Stderr).WithColor()
		logger.Info("migrate command called")

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

		reconciler := controllers.LiveMigrationReconciler{}
		reconciler.MigrateCheckpoint(ctx, "/tmp/checkpoints/checkpoints", clientset, namespace)
	},
}

func init() {
	rootCmd.AddCommand(migrateCmd)
}
