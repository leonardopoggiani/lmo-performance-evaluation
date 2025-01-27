package cmd

import (
	"context"
	"os"

	"github.com/leonardopoggiani/live-migration-operator/controllers/dummy"
	"github.com/leonardopoggiani/live-migration-operator/controllers/utils"
	pkg "github.com/leonardopoggiani/lmo-performance-evaluation/pkg"
	"github.com/spf13/cobra"
	"github.com/withmandala/go-log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// serveCmd represents the serve command
var dummyCmd = &cobra.Command{
	Use:   "dummy",
	Short: "Create dummy pod and service",
	Long: `Create the dummy pod and service, needed for the Live Migration Operator.
The dummy pod and service will be created in the test namespace.`,
	Run: func(cmd *cobra.Command, args []string) {
		logger := log.New(os.Stderr).WithColor()
		logger.Info("dummy command called")

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

		// Check if the pod exists
		_, err = clientset.CoreV1().Pods(namespace).Get(ctx, "dummy-pod", metav1.GetOptions{})
		if err == nil {
			_ = pkg.DeleteDummyPodAndService(ctx, clientset, namespace, "dummy-pod", "dummy-service")
			_ = utils.WaitForPodDeletion(ctx, "dummy-pod", namespace, clientset)
		}

		err = dummy.CreateDummyPod(clientset, ctx, namespace)
		if err != nil {
			logger.Errorf(err.Error())
			return
		}

		err = dummy.CreateDummyService(clientset, ctx, namespace)
		if err != nil {
			logger.Errorf(err.Error())
			return
		}
	},
}

func init() {
	rootCmd.AddCommand(dummyCmd)
}
