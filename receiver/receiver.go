package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/jackc/pgx/v5"
	"github.com/joho/godotenv"
	controllers "github.com/leonardopoggiani/live-migration-operator/controllers"
	"github.com/leonardopoggiani/live-migration-operator/controllers/dummy"
	"github.com/leonardopoggiani/live-migration-operator/controllers/utils"
	pkg "github.com/leonardopoggiani/performance-evaluation/pkg"
	_ "github.com/mattn/go-sqlite3"
	"github.com/withmandala/go-log"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

func deleteDummyPodAndService(ctx context.Context, clientset *kubernetes.Clientset, namespace string, podName string, serviceName string) error {
	logger := log.New(os.Stderr).WithColor()

	// Delete Pod
	err := clientset.CoreV1().Pods(namespace).Delete(ctx, podName, metav1.DeleteOptions{})
	if err != nil {
		logger.Errorf("error deleting pod: %v", err)
		return err
	}

	// Delete Service
	err = clientset.CoreV1().Services(namespace).Delete(ctx, serviceName, metav1.DeleteOptions{})
	if err != nil {
		logger.Errorf("error deleting service: %v", err)
		return err
	}

	return nil
}

func main() {
	logger := log.New(os.Stderr).WithColor()
	runtime.GOMAXPROCS(runtime.NumCPU())

	logger.Info("Receiver program started, waiting for migration request")

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

	pkg.CreateTable(ctx, db, "checkpoint_sizes", "timestamp TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP, containers INTEGER, size FLOAT, checkpoint_type TEXT")
	pkg.CreateTable(ctx, db, "checkpoint_times", "timestamp TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP, containers INTEGER, elapsed FLOAT, checkpoint_type TEXT")
	pkg.CreateTable(ctx, db, "restore_times", "timestamp TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP, containers INTEGER, elapsed FLOAT, checkpoint_type TEXT")
	pkg.CreateTable(ctx, db, "docker_sizes", "timestamp TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP, containers INTEGER, elapsed FLOAT, checkpoint_type TEXT")
	pkg.CreateTable(ctx, db, "total_times", "timestamp TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP, containers INTEGER, elapsed FLOAT, checkpoint_type TEXT")

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

	reconciler := controllers.LiveMigrationReconciler{}
	namespace := os.Getenv("NAMESPACE")

	nsName := &v1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespace,
		},
	}

	_, err = clientset.CoreV1().Namespaces().Create(context.Background(), nsName, metav1.CreateOptions{})
	if err != nil {
		fmt.Printf("Failed to create namespace %s for error: %s \n", namespace, err)
	}

	// Check if the pod exists
	_, err = clientset.CoreV1().Pods(namespace).Get(ctx, "dummy=pod", metav1.GetOptions{})
	if err == nil {
		_ = deleteDummyPodAndService(ctx, clientset, namespace, "dummy-pod", "dummy-service")
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

	directory := os.Getenv("CHECKPOINTS_FOLDER")
	containers := os.Getenv("NUM_CONTAINERS")
	numContainers, err := strconv.Atoi(containers)
	if err != nil {
		logger.Errorf(err.Error())
		return
	}

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
				logger.Infof("[MEASURE] Checkpointing took %d\n", elapsed.Milliseconds())

				pkg.SaveTimeToDB(ctx, db, numContainers, float64(elapsed.Milliseconds()), "restore", "restore_times", "containers", "elapsed")
				if err != nil {
					logger.Error(err.Error())
				}

				pkg.DeletePodsStartingWithTest(ctx, clientset, pod.Namespace)
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
