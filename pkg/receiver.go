package pkg

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/jackc/pgx/v5"
	"github.com/joho/godotenv"
	controllers "github.com/leonardopoggiani/live-migration-operator/controllers"
	utils "github.com/leonardopoggiani/live-migration-operator/controllers/utils"
	"github.com/withmandala/go-log"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func waitForFile(timeout time.Duration, path string) bool {
	logger := log.New(os.Stderr).WithColor()

	filePath := path

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
						logger.Info("File 'dummy' detected.")
						done <- true
					}
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				logger.Errorf("Error occurred in watcher: %v", err)
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

func Receive(logger *log.Logger) {
	// Use a context to cancel the loop that checks for sourcePod
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	godotenv.Load(".env")

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

	reconciler := controllers.LiveMigrationReconciler{}
	namespace := os.Getenv("NAMESPACE")

	directory := os.Getenv("CHECKPOINTS_FOLDER")
	containers := os.Getenv("NUM_CONTAINERS")
	numContainers, err := strconv.Atoi(containers)
	if err != nil {
		logger.Errorf(err.Error())
		return
	}

	logger.Info("Starting receiver")

	for {
		if waitForFile(21000*time.Second, directory) {
			logger.Info("File detected, restoring pod")

			start := time.Now()

			pod, err := reconciler.BuildahRestore(ctx, directory, clientset, namespace)
			if err != nil {
				logger.Error(err.Error())
				os.Exit(1)
			} else {
				logger.Infof("Pod restored %s", pod.Name)

				utils.WaitForContainerReady(pod.Name, namespace, pod.Spec.Containers[0].Name, clientset)

				elapsed := time.Since(start)
				logger.Infof("[MEASURE] Checkpointing took %d\n", elapsed)

				SaveTimeToDB(ctx, db, numContainers, elapsed, "restore", "restore_times", "containers", "elapsed")
				if err != nil {
					logger.Error(err.Error())
				}

				DeletePodsStartingWithTest(ctx, clientset, pod.Namespace)
			}

			if _, err := exec.Command("sudo", "rm", "-rf", directory).Output(); err != nil {
				logger.Error("Delete checkpoints failed")
				logger.Error(err.Error())
				return
			}

			if _, err = exec.Command("sudo", "mkdir", "/tmp/checkpoints/checkpoints/").Output(); err != nil {
				logger.Error(err.Error())
				return
			}

		} else {
			logger.Error("Timeout: File not detected.")
			os.Exit(1)
		}
	}
}
