package pkg

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/jackc/pgx/v5"
	"github.com/joho/godotenv"
	controllers "github.com/leonardopoggiani/live-migration-operator/controllers"
	"github.com/leonardopoggiani/live-migration-operator/controllers/dummy"
	utils "github.com/leonardopoggiani/live-migration-operator/controllers/utils"
	"github.com/withmandala/go-log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func waitForFile(timeout time.Duration, path string) bool {
	logger := log.New(os.Stderr).WithColor()

	filePath := filepath.Join(path, "dummy")

	for {
		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			logger.Errorf("Failed to create watcher: %v", err)
			return false
		}
		defer watcher.Close()

		err = watcher.Add(path)
		if err != nil {
			logger.Errorf("Failed to add watcher: %v", err)
			return false
		}

		done := make(chan bool)

		for {
			select {
			case event, ok := <-watcher.Events:
				if event.Op.Has(fsnotify.Create) && filepath.Clean(event.Name) == filePath {
					logger.Info("File 'dummy' detected.")
					close(done)
					return true
				}

				if !ok {
					logger.Errorf("Error occurred in watcher: %v", err)
					return false
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					logger.Errorf("Error occurred in watcher: %v", err)
					return false
				}
			}
		}
	}
}

func Receive(logger *log.Logger) {
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
	if err != nil {
		logger.Errorf(err.Error())
		return
	}

	_, err = clientset.CoreV1().Pods(namespace).Get(ctx, "dummy-pod", metav1.GetOptions{})
	if err == nil {
		_ = DeleteDummyPodAndService(ctx, clientset, namespace, "dummy-pod", "dummy-service")
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

	logger.Info("Starting receiver")
	i := 0

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
				end := time.Now()
				logger.Infof("[MEASURE] Restoring the pod took %d\n", elapsed)

				SaveTimeToDB(ctx, db, len(pod.Spec.Containers), elapsed, "restore", "total_times", "containers", "elapsed")
				if err != nil {
					logger.Error(err.Error())
				}

				if i%2 == 0 {
					SaveAbsoluteTimeToDB(ctx, db, len(pod.Spec.Containers), end, "restore", "back_and_forth_times", "containers", "elapsed")
					if err != nil {
						logger.Error(err.Error())
					}
				}

				i++

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

			_ = DeleteDummyPodAndService(ctx, clientset, namespace, "dummy-pod", "dummy-service")
			_ = utils.WaitForPodDeletion(ctx, "dummy-pod", namespace, clientset)

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

		} else {
			logger.Error("Timeout: File not detected.")
			os.Exit(1)
		}
	}
}
