package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5"
	pkg "github.com/leonardopoggiani/lmo-performance-evaluation/pkg"
	"github.com/spf13/cobra"
	"github.com/withmandala/go-log"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// serveCmd represents the serve command
var performanceCmd = &cobra.Command{
	Use:   "performance",
	Short: "Start a performance test",
	Run: func(cmd *cobra.Command, args []string) {
		logger := log.New(os.Stderr).WithColor()
		logger.Info("dummy command called")

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		db, err := pgx.Connect(ctx, os.Getenv("DATABASE_URL"))
		if err != nil {
			logger.Errorf("Unable to connect to database: %v\n", err)
			os.Exit(1)
		}
		defer db.Close(ctx)

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

		for i := 0; i < 100; i++ {
			logger.Info("Repetition: " + fmt.Sprint(i))
			pkg.GetCheckpointTimePipelined(ctx, clientset, 1, db, namespace)
		}

		for i := 0; i < 100; i++ {
			logger.Info("Repetition: " + fmt.Sprint(i))
			pkg.GetCheckpointTimeSequential(ctx, clientset, 1, db, namespace)
		}

		for i := 0; i < 100; i++ {
			logger.Info("Repetition: " + fmt.Sprint(i))
			pkg.GetCheckpointTimePipelined(ctx, clientset, 3, db, namespace)
		}

		for i := 0; i < 100; i++ {
			logger.Info("Repetition: " + fmt.Sprint(i))
			pkg.GetCheckpointTimeSequential(ctx, clientset, 3, db, namespace)
		}

		for i := 0; i < 100; i++ {
			logger.Info("Repetition: " + fmt.Sprint(i))
			pkg.GetCheckpointTimePipelined(ctx, clientset, 5, db, namespace)
		}

		for i := 0; i < 100; i++ {
			logger.Info("Repetition: " + fmt.Sprint(i))
			pkg.GetCheckpointTimeSequential(ctx, clientset, 5, db, namespace)
		}

		for i := 0; i < 100; i++ {
			logger.Info("Repetition: " + fmt.Sprint(i))
			pkg.GetCheckpointTimePipelined(ctx, clientset, 10, db, namespace)
		}

		for i := 0; i < 100; i++ {
			logger.Info("Repetition: " + fmt.Sprint(i))
			pkg.GetCheckpointTimeSequential(ctx, clientset, 10, db, namespace)
		}

		// for i := 0; i < 100; i++ {
		// 	logger.Info("Repetition: " + fmt.Sprint(i))
		// 	pkg.GetRestoreTimeParallelized(ctx, clientset, 1, db, namespace)
		// }

		// for i := 0; i < 100; i++ {
		// 	logger.Info("Repetition: " + fmt.Sprint(i))
		// 	pkg.GetRestoreTimeSequential(ctx, clientset, 1, db, namespace)
		// }

		// for i := 0; i < 100; i++ {
		// 	logger.Info("Repetition: " + fmt.Sprint(i))
		// 	pkg.GetRestoreTimeSequential(ctx, clientset, 2, db, namespace)
		// }

		// for i := 0; i < 100; i++ {
		// 	logger.Info("Repetition: " + fmt.Sprint(i))
		// 	pkg.GetRestoreTimeSequential(ctx, clientset, 3, db, namespace)
		// }

		// for i := 0; i < 100; i++ {
		// 	logger.Info("Repetition: " + fmt.Sprint(i))
		// 	pkg.GetRestoreTimeSequential(ctx, clientset, 5, db, namespace)
		// }

		// for i := 0; i < 10; i++ {
		// 	logger.Info("Repetition: " + fmt.Sprint(i))
		// 	pkg.GetRestoreTimeParallelized(ctx, clientset, 10, db, namespace)
		// }

		// for i := 0; i < 10; i++ {
		// 	logger.Info("Repetition: " + fmt.Sprint(i))
		// 	pkg.GetRestoreTimeParallelized(ctx, clientset, 20, db, namespace)
		// }

		// for i := 0; i < 100; i++ {
		// 	logger.Info("Repetition: " + fmt.Sprint(i))
		// 	pkg.GetTriangularizedTime(ctx, clientset, 1, db, namespace)
		// }

		// for i := 0; i < 100; i++ {
		// 	logger.Info("Repetition: " + fmt.Sprint(i))
		// 	pkg.GetTriangularizedTime(ctx, clientset, 2, db, namespace)
		// }

		// for i := 0; i < 100; i++ {
		// 	logger.Info("Repetition: " + fmt.Sprint(i))
		// 	pkg.GetTriangularizedTime(ctx, clientset, 3, db, namespace)
		// }

		// for i := 0; i < 100; i++ {
		// 	logger.Info("Repetition: " + fmt.Sprint(i))
		// 	pkg.GetTriangularizedTime(ctx, clientset, 5, db, namespace)
		// }

		// for i := 0; i < 100; i++ {
		// 	logger.Info("Repetition: " + fmt.Sprint(i))
		// 	pkg.GetTriangularizedTime(ctx, clientset, 10, db, namespace)
		// }

		// for i := 0; i < 100; i++ {
		// 	logger.Info("Repetition: " + fmt.Sprint(i))
		// 	pkg.GetTriangularizedTime(ctx, clientset, 20, db, namespace)
		// }
	},
}

func init() {
	rootCmd.AddCommand(dummyCmd)
}
