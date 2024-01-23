package pkg

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/joho/godotenv"
	controllers "github.com/leonardopoggiani/live-migration-operator/controllers"
	types "github.com/leonardopoggiani/live-migration-operator/controllers/types"
	"github.com/withmandala/go-log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func Sender(logger *log.Logger) {
	ctx := context.Background()
	godotenv.Load(".env")

	db, err := pgx.Connect(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		logger.Errorf("Unable to connect to database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close(ctx)

	kubeconfigPath := os.Getenv("KUBECONFIG")
	if kubeconfigPath == "" {
		kubeconfigPath = "~/.kube/config"
	}

	kubeconfigPath = os.ExpandEnv(kubeconfigPath)
	if _, err := os.Stat(kubeconfigPath); os.IsNotExist(err) {
		logger.Error("Kubeconfig file not found")
		return
	}

	kubeconfig, err := clientcmd.BuildConfigFromFlags("", kubeconfigPath)
	if err != nil {
		logger.Error("Error loading kubeconfig")
		return
	}

	clientset, err := kubernetes.NewForConfig(kubeconfig)
	if err != nil {
		logger.Error("Error creating kubernetes client")
		return
	}

	namespace := os.Getenv("NAMESPACE")

	err = DeletePodsStartingWithTest(ctx, clientset, namespace)
	if err != nil {
		logger.Error("Error deleting pods starting with test-")
		return
	}

	logger.Info("Sending checkpoints..")
	reconciler := controllers.LiveMigrationReconciler{}

	repetitions := os.Getenv("REPETITIONS")
	containers := os.Getenv("NUM_CONTAINERS")

	numRepetitions, err := strconv.Atoi(repetitions)
	if err != nil {
		logger.Error("Error converting with Atoi")
		return
	}

	numContainers, err := strconv.Atoi(containers)
	if err != nil {
		logger.Error("Error converting with Atoi")
		return
	}

	for j := 0; j <= numRepetitions-1; j++ {
		time.Sleep(30 * time.Second)
		logger.Infof("Repetitions %d \n", j)
		pod := CreateTestContainers(ctx, numContainers, clientset, reconciler, namespace)

		var containers []types.Container

		pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			logger.Error(err.Error())
			return
		}

		for _, pod := range pods.Items {
			for _, containerStatus := range pod.Status.ContainerStatuses {
				idParts := strings.Split(containerStatus.ContainerID, "//")

				logger.Info("containerStatus.ContainerID: " + containerStatus.ContainerID)
				logger.Info("containerStatus.Name: " + containerStatus.Name)

				if len(idParts) < 2 {
					logger.Error("Malformed container ID")
					return
				}
				containerID := idParts[1]

				container := types.Container{
					ID:   containerID,
					Name: containerStatus.Name,
				}
				containers = append(containers, container)
			}
		}

		logger.Infof("Checkpointing pod %s", pod.Name)
		start := time.Now()

		err = reconciler.CheckpointPodCrio(containers, namespace, pod.Name)
		if err != nil {
			logger.Error(err.Error())
			return
		} else {
			logger.Info("Checkpointing completed")
		}

		elapsed := time.Since(start)
		logger.Infof("[MEASURE] Checkpoint the pod took %d\n", elapsed)

		SaveTimeToDB(ctx, db, numContainers, elapsed, "restore", "total_times", "containers", "elapsed")
		if err != nil {
			logger.Error(err.Error())
		}

		logger.Infof("[MEASURE] Start time %d\n", start.UnixMilli())
		SaveAbsoluteTimeToDB(ctx, db, numContainers, start, "restore", "start_times", "containers", "elapsed")
		if err != nil {
			logger.Error(err.Error())
		}

		err = reconciler.TerminateCheckpointedPod(ctx, pod.Name, clientset, namespace)
		if err != nil {
			logger.Error(err.Error())
			return
		} else {
			logger.Info("Pod terminated")
		}

		directory := os.Getenv("CHECKPOINTS_FOLDER")

		if _, err := exec.Command("sudo", "touch", directory+"/dummy").Output(); err != nil {
			logger.Error(err.Error())
			return
		} else {
			logger.Info("Dummy file created")
		}

		files, err := os.ReadDir(directory)
		if err != nil {
			logger.Errorf("Error reading directory: %v\n", err)
			return
		}

		for _, file := range files {
			if file.IsDir() {
				logger.Infof("Directory: %s\n", file.Name())
			} else {
				logger.Infof("File: %s\n", file.Name())
			}
		}

		err = reconciler.MigrateCheckpoint(ctx, directory, clientset, namespace)
		if err != nil {
			logger.Error(err.Error())
			return
		} else {
			logger.Info("Migration completed")
		}

		if _, err := exec.Command("sudo", "rm", "-rf", directory+"/").Output(); err != nil {
			logger.Error(err.Error())
			return
		} else {
			logger.Info("Checkpoints folder deleted")
		}

		if _, err = exec.Command("sudo", "mkdir", "/tmp/checkpoints/checkpoints/").Output(); err != nil {
			logger.Error(err.Error())
			return
		} else {
			logger.Info("Checkpoints folder created")
		}

		DeletePodsStartingWithTest(ctx, clientset, namespace)
	}
}
