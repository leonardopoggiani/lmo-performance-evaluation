package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/joho/godotenv"
	internal "github.com/leonardopoggiani/performance-evaluation/internal"
	_ "github.com/leonardopoggiani/performance-evaluation/pkg"
	_ "github.com/mattn/go-sqlite3"
	"github.com/withmandala/go-log"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func main() {
	logger := log.New(os.Stderr).WithColor()
	logger.Info("Performance evaluation for live-migration controller")

	// Use a context to cancel the loop that checks for sourcePod
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	godotenv.Load(".env")

	db, err := pgx.Connect(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		logger.Errorf("Unable to connect to database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close(context.Background())

	internal.CreateTable(ctx, db, "checkpoint_sizes", "timestamp TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP, containers INTEGER, size FLOAT, checkpoint_type TEXT")
	internal.CreateTable(ctx, db, "checkpoint_times", "timestamp TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP, containers INTEGER, elapsed FLOAT, checkpoint_type TEXT")
	internal.CreateTable(ctx, db, "restore_times", "timestamp TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP, containers INTEGER, elapsed FLOAT, checkpoint_type TEXT")
	internal.CreateTable(ctx, db, "docker_sizes", "timestamp TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP, containers INTEGER, elapsed FLOAT")
	internal.CreateTable(ctx, db, "total_times", "timestamp TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP, containers INTEGER, elapsed FLOAT, checkpoint_type TEXT")

	// Load Kubernetes config
	kubeconfigPath := os.Getenv("KUBECONFIG")
	if kubeconfigPath == "" {
		kubeconfigPath = "~/.kube/config"
	}

	kubeconfigPath = os.ExpandEnv(kubeconfigPath)
	if _, err := os.Stat(kubeconfigPath); os.IsNotExist(err) {
		fmt.Printf("Kubeconfig file %s not existing", kubeconfigPath)
		return
	}

	kubeconfig, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		logger.Errorf("Failed to retrieve kubeconfig")
		return
	}

	// Create Kubernetes API client
	clientset, err := kubernetes.NewForConfig(kubeconfig)
	if err != nil {
		logger.Errorf("Failed to create Kubernetes client")
		return
	}

	fmt.Print("Insert y value here: ")
	input := bufio.NewScanner(os.Stdin)
	input.Scan()
	fmt.Println(input.Text())

	if input.Text() != "y" {
		fmt.Println("Exiting..")
		return
	}

	namespace := os.Getenv("NAMESPACE")
	if kubeconfigPath == "" {
		kubeconfigPath = "test"
	}

	nsName := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}

	_, err = clientset.CoreV1().Namespaces().Create(context.Background(), nsName, metav1.CreateOptions{})
	if err != nil {
		fmt.Printf("Failed to create namespace %s for error: %s \n", namespace, err)
	}

	containerCounts := []int{1}
	// 	containerCounts := []int{1, 2, 3, 5, 10}
	repetitions_count := os.Getenv("REPETITIONS")
	if repetitions_count == "" {
		logger.Error("Error on retrieving repetitions number")
	}
	repetitions, err := strconv.Atoi(repetitions_count)
	if err != nil {
		logger.Error("Failed to convert repetitions string to number")
	}

	//  repetitions := 20

	fmt.Printf("############### SIZE ###############\n")
	for i := 0; i < repetitions; i++ {
		fmt.Printf("Repetition %d\n", i)

		for _, numContainers := range containerCounts {
			fmt.Printf("Size for %d containers\n", numContainers)
			internal.GetCheckpointSizePipelined(ctx, clientset, numContainers, db, namespace)
		}

		internal.DeletePodsStartingWithTest(ctx, clientset, namespace)

		fmt.Printf("############### CHECKPOINT TIME ###############\n")

		for _, numContainers := range containerCounts {
			fmt.Printf("Time for %d containers\n", numContainers)
			internal.GetCheckpointTimePipelined(ctx, clientset, numContainers, db, namespace)
		}

		internal.DeletePodsStartingWithTest(ctx, clientset, namespace)

	}

	for i := 0; i < repetitions; i++ {
		fmt.Printf("############### SIZE ###############\n")
		fmt.Printf("Repetition %d\n", i)

		for _, numContainers := range containerCounts {
			fmt.Printf("Size for %d containers\n", numContainers)
			internal.GetCheckpointSizeSequential(ctx, clientset, numContainers, db, namespace)
		}

		internal.DeletePodsStartingWithTest(ctx, clientset, namespace)

		fmt.Printf("############### CHECKPOINT TIME ###############\n")

		for _, numContainers := range containerCounts {
			fmt.Printf("Time for %d containers\n", numContainers)
			internal.GetCheckpointTimeSequential(ctx, clientset, numContainers, db, namespace)
		}

		internal.DeletePodsStartingWithTest(ctx, clientset, namespace)

	}

	fmt.Printf("############### RESTORE TIME ###############\n")

	for i := 0; i < repetitions; i++ {
		fmt.Printf("Repetition %d\n", i)

		for _, numContainers := range containerCounts {
			fmt.Printf("Restore time for %d containers\n", numContainers)
			internal.GetRestoreTime(ctx, clientset, numContainers, db, namespace)
		}

		internal.DeletePodsStartingWithTest(ctx, clientset, namespace)
	}

	fmt.Printf("############### DOCKER IMAGE SIZE ###############\n")

	for i := 0; i < repetitions; i++ {
		fmt.Printf("Repetition %d\n", i)

		for _, numContainers := range containerCounts {
			fmt.Printf("Docker image size for %d containers\n", numContainers)
			internal.GetCheckpointImageRestoreSize(ctx, clientset, numContainers, db, namespace)
		}

		internal.DeletePodsStartingWithTest(ctx, clientset, namespace)
	}

	fmt.Printf("############### TOTAL TIME ###############\n")

	for i := 0; i < repetitions; i++ {
		fmt.Printf("Repetition %d\n", i)
		for _, numContainers := range containerCounts {
			fmt.Printf("Total times for %d containers\n", numContainers)
			internal.GetTimeDirectVsTriangularized(ctx, clientset, numContainers, db, "triangularized", namespace)
		}

		internal.DeletePodsStartingWithTest(ctx, clientset, namespace)
	}

	fmt.Printf("############### DIFFERENCE TIME ###############\n")

	for i := 0; i < repetitions; i++ {
		fmt.Printf("Repetition %d\n", i)
		for _, numContainers := range containerCounts {
			fmt.Printf("Difference times for %d containers\n", numContainers)
			internal.GetTimeDirectVsTriangularized(ctx, clientset, numContainers, db, "direct", namespace)
		}

		internal.DeletePodsStartingWithTest(ctx, clientset, namespace)

	}

	internal.DeletePodsStartingWithTest(ctx, clientset, namespace)

	// Command to call the Python program
	cmd := exec.Command("python", "graphs.py")

	// Run the command and capture the output
	output, err := cmd.Output()
	if err != nil {
		logger.Errorf("Error: %s", err)
		return
	}

	// Print the output of the Python program
	fmt.Println(string(output))
}
