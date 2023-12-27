package pkg

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/fsnotify/fsnotify"
	controllers "github.com/leonardopoggiani/live-migration-operator/controllers"
	"github.com/leonardopoggiani/live-migration-operator/controllers/dummy"
	"github.com/leonardopoggiani/live-migration-operator/controllers/utils"
	internal "github.com/leonardopoggiani/performance-evaluation/internal"
	_ "github.com/mattn/go-sqlite3"
	"github.com/withmandala/go-log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func waitForFile(timeout time.Duration) bool {
	logger := log.New(os.Stderr).WithColor()

	filePath := "/tmp/checkpoints/checkpoints/dummy"

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		logger.Errorf("Failed to create watcher: %v", err)
	}
	defer watcher.Close()

	done := make(chan bool)

	go func() {
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Op&fsnotify.Create == fsnotify.Create {
					if filepath.Clean(event.Name) == filePath {
						fmt.Println("File 'dummy' detected.")
						done <- true
					}
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				logger.Errorf("Error occurred in watcher: %v", err)
			default:
				// Continue executing other tasks or operations
			}
		}
	}()

	err = watcher.Add(filepath.Dir(filePath))
	if err != nil {
		logger.Errorf("Failed to add watcher: %v", err)
	}

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

func receiver(namespace string) {
	logger := log.New(os.Stderr).WithColor()

	fmt.Println("Receiver program, waiting for migration request")
	runtime.GOMAXPROCS(runtime.NumCPU())

	// Connect to the SQLite database
	db, err := sql.Open("sqlite3", "performance.db")
	if err != nil {
		logger.Errorf(err.Error())
	}
	defer db.Close()

	// Create the time_measurements table if it doesn't exist
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS time_measurements (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			mode TEXT,
			start_time TIMESTAMP,
			end_time TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			elapsed_time INTEGER
		)
	`)
	if err != nil {
		logger.Errorf(err.Error())
	}

	// Load Kubernetes config
	kubeconfigPath := os.Getenv("KUBECONFIG")
	if kubeconfigPath == "" {
		kubeconfigPath = "~/.kube/config"
	}

	kubeconfigPath = os.ExpandEnv(kubeconfigPath)
	if _, err := os.Stat(kubeconfigPath); os.IsNotExist(err) {
		fmt.Println("Kubeconfig file not found")
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

	ctx := context.Background()

	reconciler := controllers.LiveMigrationReconciler{}

	// Check if the pod exists
	_, err = clientset.CoreV1().Pods("liqo-demo").Get(ctx, "dummy=pod", metav1.GetOptions{})
	if err == nil {
		_ = deleteDummyPodAndService(ctx, clientset)
		_ = utils.WaitForPodDeletion(ctx, "dummy-pod", "liqo-demo", clientset)
	}

	err = dummy.CreateDummyPod(clientset, ctx)
	if err != nil {
		logger.Errorf(err.Error())
		return
	}

	err = dummy.CreateDummyService(clientset, ctx)
	if err != nil {
		logger.Errorf(err.Error())
		return
	}

	directory := "/tmp/checkpoints/checkpoints/"

	for {
		if waitForFile(21000 * time.Second) {
			fmt.Println("File detected, restoring pod")

			start := time.Now()

			pod, err := reconciler.BuildahRestore(ctx, directory, clientset)
			if err != nil {
				logger.Error(err.Error())
				os.Exit(1) // Terminate the process with a non-zero exit code
			} else {
				fmt.Println("Pod restored")

				utils.WaitForContainerReady(pod.Name, namespace, pod.Spec.Containers[0].Name, clientset)

				elapsed := time.Since(start)
				fmt.Printf("[MEASURE] Checkpointing took %d\n", elapsed.Milliseconds())

				// Insert the time measurement into the database
				_, err = db.Exec("INSERT INTO time_measurements (mode, start_time, end_time, elapsed_time) VALUES (?, ?, ?, ?)", "sequential_restore", start, time.Now(), elapsed.Milliseconds())
				if err != nil {
					logger.Error(err.Error())
				}

				internal.DeletePodsStartingWithTest(ctx, clientset, pod.Namespace)
			}

			/// delete checkpoints folder
			if _, err := exec.Command("sudo", "rm", "-rf", directory).Output(); err != nil {
				fmt.Println("Delete checkpoints failed")
				logger.Error(err.Error())
				return
			}

			if _, err = exec.Command("sudo", "mkdir", "/tmp/checkpoints/checkpoints/").Output(); err != nil {
				logger.Error(err.Error())
				return
			}

		} else {
			fmt.Println("Timeout: File not detected.")
			os.Exit(1)
		}
	}
}
