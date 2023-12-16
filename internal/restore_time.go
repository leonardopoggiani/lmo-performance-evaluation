package internal

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	controllers "github.com/leonardopoggiani/live-migration-operator/controllers"
	types "github.com/leonardopoggiani/live-migration-operator/controllers/types"
	utils "github.com/leonardopoggiani/live-migration-operator/controllers/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func GetRestoreTime(ctx context.Context, clientset *kubernetes.Clientset, numContainers int, db *pgx.Conn) {
	// Get the start time of the restore

	reconciler := controllers.LiveMigrationReconciler{}
	pod := CreateTestContainers(ctx, numContainers, clientset, reconciler)

	err := utils.WaitForContainerReady(pod.Name, "default", fmt.Sprintf("container-%d", numContainers-1), clientset)
	if err != nil {
		CleanUp(ctx, clientset, pod)
		fmt.Println(err.Error())
		return
	}

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
		return
	}

	path := "/tmp/checkpoints/checkpoints/"
	// create dummy file
	_, err = os.Create(path + "dummy")
	if err != nil {
		fmt.Println(err.Error())
		return
	}

	files, err := CountFilesInFolder(path)
	if err != nil {
		fmt.Println(err.Error())
		return
	}

	fmt.Printf("Files count => %d", files)

	start := time.Now()

	pod, err = reconciler.BuildahRestore(ctx, "/tmp/checkpoints/checkpoints", clientset)
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	// Calculate the time taken for the restore
	elapsed := time.Since(start)
	fmt.Println("Elapsed sequential: ", elapsed)

	SaveToDB(ctx, db, int64(numContainers), float64(elapsed.Milliseconds()), "sequential", "restore_times")

	// eliminate docker image
	for i := 0; i < numContainers; i++ {
		BuildahDeleteImage("localhost/leonardopoggiani/checkpoint-images:container-" + strconv.Itoa(i))
	}

	CleanUp(ctx, clientset, pod)
	start = time.Now()

	pod, err = reconciler.BuildahRestorePipelined(ctx, "/tmp/checkpoints/checkpoints", clientset)
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	// Calculate the time taken for the restore
	elapsed = time.Since(start)
	fmt.Println("Elapsed pipelined: ", elapsed)

	SaveToDB(ctx, db, int64(numContainers), float64(elapsed.Milliseconds()), "pipelined", "restore_times")

	// eliminate docker image
	for i := 0; i < numContainers; i++ {
		BuildahDeleteImage("localhost/leonardopoggiani/checkpoint-images:container-" + strconv.Itoa(i))
	}

	CleanUp(ctx, clientset, pod)

	start = time.Now()

	pod, err = reconciler.BuildahRestoreParallelized(ctx, "/tmp/checkpoints/checkpoints", clientset)
	if err != nil {
		fmt.Println(err.Error())
		return
	}

	// Calculate the time taken for the restore
	elapsed = time.Since(start)
	fmt.Println("Elapsed parallel: ", elapsed)

	SaveToDB(ctx, db, int64(numContainers), float64(elapsed.Milliseconds()), "parallelized", "restore_times")

	// eliminate docker image
	for i := 0; i < numContainers; i++ {
		BuildahDeleteImage("localhost/leonardopoggiani/checkpoint-images:container-" + strconv.Itoa(i))
	}

	CleanUp(ctx, clientset, pod)
}
