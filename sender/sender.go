package main

import (
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/leonardopoggiani/live-migration-operator/controllers"
	"github.com/leonardopoggiani/live-migration-operator/controllers/types"
	pkg "github.com/leonardopoggiani/performance-evaluation/pkg"
	"github.com/withmandala/go-log"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func waitForServiceCreation(clientset *kubernetes.Clientset, ctx context.Context) {
	logger := log.New(os.Stderr).WithColor()

	watcher, err := clientset.CoreV1().Services("liqo-demo").Watch(ctx, metav1.ListOptions{})
	if err != nil {
		logger.Errorf("Error watching services")
		return
	}
	defer watcher.Stop()

	for {
		select {
		case event, ok := <-watcher.ResultChan():
			if !ok {
				logger.Info("Watcher channel closed")
				return
			}
			if event.Type == watch.Added {
				service, ok := event.Object.(*v1.Service)
				if !ok {
					logger.Error("Error casting service object")
					return
				}
				if service.Name == "dummy-service" {
					logger.Info("Service dummy-service created")
					return
				}
			}
		case <-ctx.Done():
			logger.Info("Context done")
			return
		}
	}
}

func main() {
	logger := log.New(os.Stderr).WithColor()

	logger.Infof("Sender program, sending migration request")

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

	ctx := context.Background()
	namespace := os.Getenv("NAMESPACE")

	err = pkg.DeletePodsStartingWithTest(ctx, clientset, namespace)
	if err != nil {
		logger.Error("Error deleting pods starting with test-")
		return
	}

	waitForServiceCreation(clientset, ctx)

	reconciler := controllers.LiveMigrationReconciler{}

	repetitions := os.Getenv("REPETITIONS")
	containers := os.Getenv("NUM_CONTAINERS")

	numRepetitions, err := strconv.Atoi(repetitions)
	if err != nil {
		logger.Error("Error covnerting with Atoi")
		return
	}

	numContainers, err := strconv.Atoi(containers)
	if err != nil {
		logger.Error("Error covnerting with Atoi")
		return
	}

	for j := 0; j <= numRepetitions-1; j++ {
		logger.Infof("Repetitions %d \n", j)
		pod := pkg.CreateTestContainers(ctx, numContainers, clientset, reconciler, namespace)

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

		logger.Info("Checkpointing pod %s", pod.Name)

		err = reconciler.CheckpointPodCrio(containers, namespace, pod.Name)
		if err != nil {
			logger.Error(err.Error())
			return
		} else {
			logger.Info("Checkpointing completed")
		}

		err = reconciler.TerminateCheckpointedPod(ctx, pod.Name, clientset, namespace)
		if err != nil {
			logger.Error(err.Error())
			return
		} else {
			logger.Info("Pod terminated")
		}

		directory := os.Getenv("CHECKPOINTS_FOLDER")

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

		err = reconciler.MigrateCheckpoint(ctx, directory, clientset)
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

		pkg.DeletePodsStartingWithTest(ctx, clientset, namespace)
	}
}
