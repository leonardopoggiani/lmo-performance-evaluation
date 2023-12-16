package internal

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	controllers "github.com/leonardopoggiani/live-migration-operator/controllers"
	types "github.com/leonardopoggiani/live-migration-operator/controllers/types"
	utils "github.com/leonardopoggiani/live-migration-operator/controllers/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func GetCheckpointImageRestoreSize(ctx context.Context, clientset *kubernetes.Clientset, numContainers int, db *pgx.Conn) {

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

	for i := 0; i < numContainers; i++ {
		// Get the image name
		imageName := "localhost/leonardopoggiani/checkpoint-images:container-" + strconv.Itoa(i)

		// Get the image size
		sizeInMB, err := GetImageSize(imageName)
		if err != nil {
			log.Fatal(err)
		}

		fmt.Printf("The size of %s is %.2f MB.\n", imageName, sizeInMB)
	}

	// delete checkpoints folder
	if _, err := exec.Command("sudo", "rm", "-f", directory+"/").Output(); err != nil {
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
