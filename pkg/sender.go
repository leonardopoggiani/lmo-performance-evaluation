package pkg

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/leonardopoggiani/live-migration-operator/controllers"
	"github.com/leonardopoggiani/live-migration-operator/controllers/types"
	internal "github.com/leonardopoggiani/performance-evaluation/internal"
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
				fmt.Println("Watcher channel closed")
				return
			}
			if event.Type == watch.Added {
				service, ok := event.Object.(*v1.Service)
				if !ok {
					fmt.Println("Error casting service object")
					return
				}
				if service.Name == "dummy-service" {
					fmt.Println("Service dummy-service created")
					return
				}
			}
		case <-ctx.Done():
			fmt.Println("Context done")
			return
		}
	}
}

func sender(namespace string) {
	logger := log.New(os.Stderr).WithColor()

	logger.Infof("Sender program, sending migration request")

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

	fmt.Println("Kubeconfig file not found")

	internal.DeletePodsStartingWithTest(ctx, clientset, namespace)
	waitForServiceCreation(clientset, ctx)

	reconciler := controllers.LiveMigrationReconciler{}

	// once created the dummy pod and correctly offloaded, i can create a pod to migrate
	repetitions := 1
	numContainers := 1

	for j := 0; j <= repetitions-1; j++ {
		fmt.Printf("Repetitions %d \n", j)
		pod := internal.CreateTestContainers(ctx, numContainers, clientset, reconciler, namespace)

		// Create a slice of Container structs
		var containers []types.Container

		// Append the container ID and name for each container in each pod
		pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			fmt.Println(err.Error())
			return
		}

		for _, pod := range pods.Items {
			for _, containerStatus := range pod.Status.ContainerStatuses {
				idParts := strings.Split(containerStatus.ContainerID, "//")

				fmt.Println("containerStatus.ContainerID: " + containerStatus.ContainerID)
				fmt.Println("containerStatus.Name: " + containerStatus.Name)

				if len(idParts) < 2 {
					fmt.Println("Malformed container ID")
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

		fmt.Println("Checkpointing pod")
		fmt.Println("pod.Name: " + pod.Name)

		err = reconciler.CheckpointPodCrio(containers, namespace, pod.Name)
		if err != nil {
			fmt.Println(err.Error())
			return
		} else {
			fmt.Println("Checkpointing completed")
		}

		err = reconciler.TerminateCheckpointedPod(ctx, pod.Name, clientset)
		if err != nil {
			fmt.Println(err.Error())
			return
		} else {
			fmt.Println("Pod terminated")
		}

		directory := "/tmp/checkpoints/checkpoints/"

		files, err := os.ReadDir(directory)
		if err != nil {
			fmt.Printf("Error reading directory: %v\n", err)
			return
		}

		for _, file := range files {
			if file.IsDir() {
				fmt.Printf("Directory: %s\n", file.Name())
			} else {
				fmt.Printf("File: %s\n", file.Name())
			}
		}

		err = reconciler.MigrateCheckpoint(ctx, directory, clientset)
		if err != nil {
			fmt.Println(err.Error())
			return
		} else {
			fmt.Println("Migration completed")
		}

		// delete checkpoints folder
		if _, err := exec.Command("sudo", "rm", "-rf", directory+"/").Output(); err != nil {
			fmt.Println(err.Error())
			return
		} else {
			fmt.Println("Checkpoints folder deleted")
		}

		if _, err = exec.Command("sudo", "mkdir", "/tmp/checkpoints/checkpoints/").Output(); err != nil {
			fmt.Println(err.Error())
			return
		} else {
			fmt.Println("Checkpoints folder created")
		}

		internal.DeletePodsStartingWithTest(ctx, clientset, namespace)
	}
}
