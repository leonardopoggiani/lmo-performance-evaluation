package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/fsnotify/fsnotify"
	migrationoperator "github.com/leonardopoggiani/live-migration-operator/controllers"
	_ "github.com/mattn/go-sqlite3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func deletePodsStartingWithTest(ctx context.Context, clientset *kubernetes.Clientset) error {
	podList, err := clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}

	for _, pod := range podList.Items {
		if pod.ObjectMeta.Name[:5] == "test-" {
			err := clientset.CoreV1().Pods(pod.Namespace).Delete(ctx, pod.Name, metav1.DeleteOptions{})
			if err != nil {
				return err
			}
			fmt.Printf("Deleted pod %s\n", pod.Name)
		}
	}

	return nil
}

func waitForFile(timeout time.Duration) bool {
	filePath := "/tmp/checkpoints/checkpoints/dummy"

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatalf("Failed to create watcher: %v", err)
	}
	defer watcher.Close()

	done := make(chan bool)

	go monitorEvents(watcher, done, filePath)

	err = watcher.Add(filepath.Dir(filePath))
	if err != nil {
		log.Fatalf("Failed to add watcher: %v", err)
	}

	return waitForCompletionOrTimeout(done, timeout)
}

func monitorEvents(watcher *fsnotify.Watcher, done chan bool, filePath string) {
	for {
		select {
		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			handleFileCreationEvent(event, done, filePath)
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			log.Printf("Error occurred in watcher: %v", err)
		default:
			// Continue executing other tasks or operations
		}
	}
}

func handleFileCreationEvent(event fsnotify.Event, done chan bool, filePath string) {
	if event.Op&fsnotify.Create == fsnotify.Create && filepath.Clean(event.Name) == filePath {
		fmt.Println("File 'dummy' detected.")
		done <- true
	}
}

func waitForCompletionOrTimeout(done chan bool, timeout time.Duration) bool {
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}

func deleteDummyPodAndService(ctx context.Context, clientset *kubernetes.Clientset) error {

	// Delete Pod
	err := clientset.CoreV1().Pods("liqo-demo").Delete(ctx, "dummy-pod", metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("error deleting pod: %v", err)
	}

	// Delete Service
	err = clientset.CoreV1().Services("liqo-demo").Delete(ctx, "dummy-service", metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("error deleting service: %v", err)
	}

	return nil
}

func connectToDatabase() *sql.DB {
	db, err := sql.Open("sqlite3", "performance.db")
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
	return db
}

func createTimeMeasurementsTable(db *sql.DB) {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS time_measurements (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			mode TEXT,
			start_time TIMESTAMP,
			end_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			elapsed_time INTEGER
		)
	`)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
}

func getKubeconfigPath() string {
	kubeconfigPath := os.Getenv("KUBECONFIG")
	if kubeconfigPath == "" {
		kubeconfigPath = "~/.kube/config"
	}
	kubeconfigPath = os.ExpandEnv(kubeconfigPath)
	if _, err := os.Stat(kubeconfigPath); os.IsNotExist(err) {
		fmt.Println("Kubeconfig file not found")
		os.Exit(1)
	}
	return kubeconfigPath
}

func loadKubeconfig(kubeconfigPath string) *rest.Config {
	kubeconfig, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		fmt.Println("Error loading kubeconfig")
		os.Exit(1)
	}
	return kubeconfig
}

func createKubernetesClient(kubeconfig *rest.Config) *kubernetes.Clientset {
	clientset, err := kubernetes.NewForConfig(kubeconfig)
	if err != nil {
		fmt.Println("Error creating kubernetes client")
		os.Exit(1)
	}
	return clientset
}

func ensureDummyPodDeleted(ctx context.Context, clientset *kubernetes.Clientset, reconciler migrationoperator.LiveMigrationReconciler) {
	_, err := clientset.CoreV1().Pods("liqo-demo").Get(ctx, "dummy=pod", metav1.GetOptions{})
	if err == nil {
		_ = deleteDummyPodAndService(ctx, clientset)
		_ = reconciler.WaitForPodDeletion(ctx, "dummy-pod", "liqo-demo", clientset)
	}
}

func createDummyPodAndService(ctx context.Context, clientset *kubernetes.Clientset, reconciler migrationoperator.LiveMigrationReconciler) {
	if err := reconciler.CreateDummyPod(clientset, ctx); err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}

	if err := reconciler.CreateDummyService(clientset, ctx); err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
}

func restoreAndMeasure(ctx context.Context, directory string, clientset *kubernetes.Clientset, reconciler migrationoperator.LiveMigrationReconciler, db *sql.DB) {
	start := time.Now()
	pod, err := reconciler.BuildahRestore(ctx, directory, clientset)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}

	fmt.Println("Pod restored")
	reconciler.WaitForContainerReady(pod.Name, "default", pod.Spec.Containers[0].Name, clientset)

	elapsed := time.Since(start)
	fmt.Printf("[MEASURE] Checkpointing took %d\n", elapsed.Milliseconds())

	_, err = db.Exec("INSERT INTO time_measurements (mode, start_time, end_time, elapsed_time) VALUES (?, ?, ?, ?)", "sequential_restore", start, time.Now(), elapsed.Milliseconds())
	if err != nil {
		fmt.Println(err.Error())
	}

	deletePodsStartingWithTest(ctx, clientset)

	if _, err := exec.Command("sudo", "rm", "-rf", directory).Output(); err != nil {
		fmt.Println("Delete checkpoints failed")
		fmt.Println(err.Error())
		os.Exit(1)
	}

	if _, err = exec.Command("sudo", "mkdir", "/tmp/checkpoints/checkpoints/").Output(); err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
}

func waitForCheckpointFile(timeout time.Duration) bool {
	filePath := "/path/to/your/checkpoint/file" // Replace with the actual file path

	// Get the current time
	startTime := time.Now()

	for {
		// Check if the file exists
		if _, err := os.Stat(filePath); err == nil {
			return true // File found
		}

		// If the file is not found and the timeout has elapsed, return false
		if time.Since(startTime) >= timeout {
			return false
		}

		// Sleep for a short interval before checking again
		time.Sleep(1 * time.Second) // Adjust the sleep duration as needed
	}
}

func main() {
	fmt.Println("Receiver program, waiting for migration request")
	runtime.GOMAXPROCS(runtime.NumCPU())

	db := connectToDatabase()
	defer db.Close()

	createTimeMeasurementsTable(db)

	kubeconfigPath := getKubeconfigPath()
	kubeconfig := loadKubeconfig(kubeconfigPath)

	clientset := createKubernetesClient(kubeconfig)
	ctx := context.Background()

	reconciler := migrationoperator.LiveMigrationReconciler{}

	ensureDummyPodDeleted(ctx, clientset, reconciler)
	createDummyPodAndService(ctx, clientset, reconciler)

	directory := "/tmp/checkpoints/checkpoints/"

	for {
		if waitForCheckpointFile(21000 * time.Second) {
			fmt.Println("File detected, restoring pod")
			restoreAndMeasure(ctx, directory, clientset, reconciler, db)
		} else {
			fmt.Println("Timeout: File not detected.")
			os.Exit(1)
		}
	}
}
