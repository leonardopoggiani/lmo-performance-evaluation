package cmd

import (
	"context"
	"os"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/joho/godotenv"
	pkg "github.com/leonardopoggiani/lmo-performance-evaluation/pkg"
	"github.com/spf13/cobra"
	"github.com/withmandala/go-log"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// serveCmd represents the serve command
var forthCmd = &cobra.Command{
	Use:   "forth",
	Short: "Execute the back-and-forth migration test",
	Long:  `Make HTTP request to the pod during multiple back-and-forth migration and record the latency.`,
	Run: func(cmd *cobra.Command, args []string) {
		logger := log.New(os.Stderr).WithColor()
		logger.Info("backd-and-forth command called")

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

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

		clientset, err := kubernetes.NewForConfig(kubeconfig)
		if err != nil {
			logger.Errorf("Error creating kubernetes client")
			return
		}
		defer cancel()

		namespace := os.Getenv("NAMESPACE")

		containers := os.Getenv("NUM_CONTAINERS")
		numContainers, err := strconv.Atoi(containers)
		if err != nil {
			logger.Error("Error converting with Atoi")
			return
		}

		godotenv.Load(".env")

		db, err := pgx.Connect(ctx, os.Getenv("DATABASE_URL"))
		if err != nil {
			logger.Errorf("Unable to connect to database: %v\n", err)
			os.Exit(1)
		}
		defer db.Close(ctx)

		pkg.GetForthLatency(ctx, clientset, namespace, db, numContainers, logger)
	},
}

func init() {
	rootCmd.AddCommand(forthCmd)
}
