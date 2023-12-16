package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"

	"github.com/jackc/pgx/v5"
	"github.com/joho/godotenv"
	internal "github.com/leonardopoggiani/performance-evaluation/internal"
	_ "github.com/leonardopoggiani/performance-evaluation/pkg"
	_ "github.com/mattn/go-sqlite3"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func main() {
	// Use a context to cancel the loop that checks for sourcePod
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	godotenv.Load(".env")

	db, err := pgx.Connect(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to connect to database: %v\n", err)
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
		fmt.Println("kubeconfig file not existing")
	}

	kubeconfig, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		fmt.Println("Failed to retrieve kubeconfig")
		return
	}

	// Create Kubernetes API client
	clientset, err := kubernetes.NewForConfig(kubeconfig)
	if err != nil {
		fmt.Println("failed to create Kubernetes client")
		return
	}

	containerCounts := []int{1}
	// 	containerCounts := []int{1, 2, 3, 5, 10}
	repetitions := 2
	//  repetitions := 20

	fmt.Printf("############### SIZE ###############\n")
	for i := 0; i < repetitions; i++ {
		fmt.Printf("Repetition %d\n", i)

		for _, numContainers := range containerCounts {
			fmt.Printf("Size for %d containers\n", numContainers)
			internal.GetCheckpointSizePipelined(ctx, clientset, numContainers, db)
		}

		internal.DeletePodsStartingWithTest(ctx, clientset)

		fmt.Printf("############### CHECKPOINT TIME ###############\n")

		for _, numContainers := range containerCounts {
			fmt.Printf("Time for %d containers\n", numContainers)
			internal.GetCheckpointTimePipelined(ctx, clientset, numContainers, db)
		}

		internal.DeletePodsStartingWithTest(ctx, clientset)

	}

	for i := 0; i < repetitions; i++ {
		fmt.Printf("############### SIZE ###############\n")
		fmt.Printf("Repetition %d\n", i)

		for _, numContainers := range containerCounts {
			fmt.Printf("Size for %d containers\n", numContainers)
			internal.GetCheckpointSizeSequential(ctx, clientset, numContainers, db)
		}

		internal.DeletePodsStartingWithTest(ctx, clientset)

		fmt.Printf("############### CHECKPOINT TIME ###############\n")

		for _, numContainers := range containerCounts {
			fmt.Printf("Time for %d containers\n", numContainers)
			internal.GetCheckpointTimeSequential(ctx, clientset, numContainers, db)
		}

		internal.DeletePodsStartingWithTest(ctx, clientset)

	}

	fmt.Printf("############### RESTORE TIME ###############\n")

	for i := 0; i < repetitions; i++ {
		fmt.Printf("Repetition %d\n", i)

		for _, numContainers := range containerCounts {
			fmt.Printf("Restore time for %d containers\n", numContainers)
			internal.GetRestoreTime(ctx, clientset, numContainers, db)
		}

		internal.DeletePodsStartingWithTest(ctx, clientset)
	}

	fmt.Printf("############### DOCKER IMAGE SIZE ###############\n")

	for i := 0; i < repetitions; i++ {
		fmt.Printf("Repetition %d\n", i)

		for _, numContainers := range containerCounts {
			fmt.Printf("Docker image size for %d containers\n", numContainers)
			internal.GetCheckpointImageRestoreSize(ctx, clientset, numContainers, db)
		}

		internal.DeletePodsStartingWithTest(ctx, clientset)
	}

	fmt.Printf("############### TOTAL TIME ###############\n")

	for i := 0; i < repetitions; i++ {
		fmt.Printf("Repetition %d\n", i)
		for _, numContainers := range containerCounts {
			fmt.Printf("Total times for %d containers\n", numContainers)
			internal.GetTimeDirectVsTriangularized(ctx, clientset, numContainers, db, "triangularized")
		}

		internal.DeletePodsStartingWithTest(ctx, clientset)
	}

	fmt.Printf("############### DIFFERENCE TIME ###############\n")

	for i := 0; i < repetitions; i++ {
		fmt.Printf("Repetition %d\n", i)
		for _, numContainers := range containerCounts {
			fmt.Printf("Difference times for %d containers\n", numContainers)
			internal.GetTimeDirectVsTriangularized(ctx, clientset, numContainers, db, "direct")
		}

		internal.DeletePodsStartingWithTest(ctx, clientset)

	}

	internal.DeletePodsStartingWithTest(ctx, clientset)

	// Command to call the Python program
	cmd := exec.Command("python", "graphs.py")

	// Run the command and capture the output
	output, err := cmd.Output()
	if err != nil {
		fmt.Println("Error:", err)
		return
	}

	// Print the output of the Python program
	fmt.Println(string(output))
}
