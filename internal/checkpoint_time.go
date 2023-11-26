package internal

import (
	"context"
	"database/sql"
	"fmt"
	"os/exec"
	"strings"
	"time"

	controllers "github.com/leonardopoggiani/live-migration-operator/controllers"
	types "github.com/leonardopoggiani/live-migration-operator/controllers/types"
	utils "github.com/leonardopoggiani/live-migration-operator/controllers/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func GetCheckpointTimeSequential(ctx context.Context, clientset *kubernetes.Clientset, numContainers int, db *sql.DB) {

	reconciler := controllers.LiveMigrationReconciler{}
	pod := CreateTestContainers(ctx, numContainers, clientset, reconciler)

	err := utils.WaitForContainerReady(pod.Name, "default", fmt.Sprintf("container-%d", numContainers-1), clientset)
	if err != nil {
		CleanUp(ctx, clientset, pod)
		fmt.Println(err.Error())
		return
	}

	fmt.Printf("Pod %s is ready\n", pod.Name)

	// Create a slice of Container structs
	var containers []types.Container

	// Append the container ID and name for each container in each pod
	pods, err := clientset.CoreV1().Pods("default").List(ctx, metav1.ListOptions{})
	if err != nil {
		fmt.Println(err.Error())
		CleanUp(ctx, clientset, pod)
		return
	}

	for _, pod := range pods.Items {
		for _, containerStatus := range pod.Status.ContainerStatuses {
			idParts := strings.Split(containerStatus.ContainerID, "//")
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

	// Get the start time of the checkpoint
	start := time.Now()

	err = reconciler.CheckpointPodCrio(containers, "default", pod.Name)
	if err != nil {
		return
	}

	// Calculate the time taken for the checkpoint
	elapsed := time.Since(start)
	fmt.Println("Elapsed sequential: ", elapsed)

	SaveToDB(db, int64(numContainers), float64(elapsed.Milliseconds()), "sequential", "checkpoint_times")

	// delete checkpoint folder
	directory := "/tmp/checkpoints/checkpoints"
	if _, err := exec.Command("sudo", "rm", "-rf", directory+"/").Output(); err != nil {
		CleanUp(ctx, clientset, pod)
		fmt.Println(err.Error())
		return
	}

	if _, err = exec.Command("sudo", "mkdir", "/tmp/checkpoints/checkpoints/").Output(); err != nil {
		CleanUp(ctx, clientset, pod)
		fmt.Println(err.Error())
		return
	}

	CleanUp(ctx, clientset, pod)
}

func GetCheckpointTimePipelined(ctx context.Context, clientset *kubernetes.Clientset, numContainers int, db *sql.DB) {

	reconciler := controllers.LiveMigrationReconciler{}
	pod := CreateTestContainers(ctx, numContainers, clientset, reconciler)

	err := utils.WaitForContainerReady(pod.Name, "default", fmt.Sprintf("container-%d", numContainers-1), clientset)
	if err != nil {
		CleanUp(ctx, clientset, pod)
		fmt.Println(err.Error())
		return
	}

	fmt.Printf("Pod %s is ready\n", pod.Name)

	// Create a slice of Container structs
	var containers []types.Container

	// Append the container ID and name for each container in each pod
	pods, err := clientset.CoreV1().Pods("default").List(ctx, metav1.ListOptions{})
	if err != nil {
		fmt.Println(err.Error())
		CleanUp(ctx, clientset, pod)
		return
	}

	for _, pod := range pods.Items {
		for _, containerStatus := range pod.Status.ContainerStatuses {
			idParts := strings.Split(containerStatus.ContainerID, "//")
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

	start := time.Now()

	err = controllers.CheckpointPodPipelined(containers, "default", pod.Name)
	if err != nil {
		return
	}

	// Calculate the time taken for the checkpoint
	elapsed := time.Since(start)
	fmt.Println("Elapsed pipelined: ", elapsed)

	SaveToDB(db, int64(numContainers), float64(elapsed.Milliseconds()), "pipelined", "checkpoint_times")

	// delete checkpoint folder
	directory := "/tmp/checkpoints/checkpoints"
	if _, err := exec.Command("sudo", "rm", "-rf", directory+"/").Output(); err != nil {
		CleanUp(ctx, clientset, pod)
		fmt.Println(err.Error())
		return
	}

	if _, err = exec.Command("sudo", "mkdir", "/tmp/checkpoints/checkpoints/").Output(); err != nil {
		CleanUp(ctx, clientset, pod)
		fmt.Println(err.Error())
		return
	}

	CleanUp(ctx, clientset, pod)
}
