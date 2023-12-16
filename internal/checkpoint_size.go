package internal

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5"
	controllers "github.com/leonardopoggiani/live-migration-operator/controllers"
	types "github.com/leonardopoggiani/live-migration-operator/controllers/types"
	utils "github.com/leonardopoggiani/live-migration-operator/controllers/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func GetCheckpointSizePipelined(ctx context.Context, clientset *kubernetes.Clientset, numContainers int, db *pgx.Conn) {

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

	err = controllers.CheckpointPodPipelined(containers, "default", pod.Name)
	if err != nil {
		CleanUp(ctx, clientset, pod)
		fmt.Println(err.Error())
		return
	}

	directory := "/tmp/checkpoints/checkpoints"
	var size int64 = 0

	err = filepath.Walk(directory, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			CleanUp(ctx, clientset, pod)
			fmt.Println(err.Error())
			return err
		}
		if !info.Mode().IsRegular() {
			fmt.Println("Not a regular file")
			return nil
		}
		size += info.Size()
		return nil
	})
	if err != nil {
		CleanUp(ctx, clientset, pod)
		fmt.Println(err.Error())
		return
	}

	sizeInMB := float64(size) / (1024 * 1024)
	fmt.Printf("The size of %s is %.2f MB.\n", directory, sizeInMB)
	SaveToDB(ctx, db, int64(numContainers), sizeInMB, "pipelined", "checkpoint_sizes")

	// delete checkpoints folder
	if _, err := exec.Command("sudo", "rm", "-f", directory+"/").Output(); err != nil {
		CleanUp(ctx, clientset, pod)
		fmt.Println(err.Error())
		return
	}

	if _, err = exec.Command("sudo", "mkdir", "/tmp/checkpoints/checkpoints/").Output(); err != nil {
		CleanUp(ctx, clientset, pod)
		fmt.Println(err.Error())
		return
	}

	// check that checkpoints folder is empty
	if output, err := exec.Command("sudo", "ls", directory).Output(); err != nil {
		CleanUp(ctx, clientset, pod)
		fmt.Println(err.Error())
		return
	} else {
		fmt.Printf("Output: %s\n", output)
	}

	CleanUp(ctx, clientset, pod)
}

func GetCheckpointSizeSequential(ctx context.Context, clientset *kubernetes.Clientset, numContainers int, db *pgx.Conn) {

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

	err = reconciler.CheckpointPodCrio(containers, "default", pod.Name)
	if err != nil {
		fmt.Println(err.Error())
		CleanUp(ctx, clientset, pod)
		return
	}

	directory := "/tmp/checkpoints/checkpoints"
	var size int64 = 0

	err = filepath.Walk(directory, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			fmt.Println(err.Error())
			CleanUp(ctx, clientset, pod)
			return err
		}
		if !info.Mode().IsRegular() {
			fmt.Println("Not a regular file")
			return nil
		}
		size += info.Size()
		return nil
	})
	if err != nil {
		CleanUp(ctx, clientset, pod)
		fmt.Println(err.Error())
		return
	}

	sizeInMB := float64(size) / (1024 * 1024)
	fmt.Printf("The size of %s is %.2f MB.\n", directory, sizeInMB)
	SaveToDB(ctx, db, int64(numContainers), sizeInMB, "sequential", "checkpoint_sizes")

	// delete checkpoints folder
	if _, err := exec.Command("sudo", "rm", "-f", directory+"/").Output(); err != nil {
		CleanUp(ctx, clientset, pod)
		fmt.Println(err.Error())
		return
	}

	if _, err = exec.Command("sudo", "mkdir", "/tmp/checkpoints/checkpoints/").Output(); err != nil {
		CleanUp(ctx, clientset, pod)
		fmt.Println(err.Error())
		return
	}

	// check that checkpoints folder is empty
	if output, err := exec.Command("sudo", "ls", directory).Output(); err != nil {
		CleanUp(ctx, clientset, pod)
		fmt.Println(err.Error())
		return
	} else {
		fmt.Printf("Output: %s\n", output)
	}

	CleanUp(ctx, clientset, pod)
}
