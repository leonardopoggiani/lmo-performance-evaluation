package pkg

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	controllers "github.com/leonardopoggiani/live-migration-operator/controllers"
	types "github.com/leonardopoggiani/live-migration-operator/controllers/types"
	utils "github.com/leonardopoggiani/live-migration-operator/controllers/utils"
	"github.com/withmandala/go-log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func GetRestoreTimePipelined(ctx context.Context, clientset *kubernetes.Clientset, numContainers int, db *pgx.Conn, namespace string) {
	// Get the start time of the restore
	logger := log.New(os.Stderr).WithColor()

	reconciler := controllers.LiveMigrationReconciler{}
	pod := CreateTestContainers(ctx, numContainers, clientset, reconciler, namespace)
	if pod == nil {
		logger.Error("Pod not correctly created")
		logger.Info("Retrying creation..")
		pod = CreateTestContainers(ctx, numContainers, clientset, reconciler, namespace)
		if pod == nil {
			logger.Error("Pod not correctly created for the second time, exiting..")
			return
		}
	}

	err := utils.WaitForContainerReady(pod.Name, namespace, fmt.Sprintf("container-%d", numContainers-1), clientset)
	if err != nil {
		CleanUp(ctx, clientset, pod, namespace)
		logger.Error(err.Error())
		return
	}

	// Create a slice of Container structs
	var containers []types.Container

	// Append the container ID and name for each container in each pod
	pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		logger.Errorf(err.Error())
		CleanUp(ctx, clientset, pod, namespace)
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

	logger.Info("Checkpointing...")

	err = controllers.CheckpointPodPipelined(containers, namespace, pod.Name)
	if err != nil {
		return
	}

	path := "/tmp/checkpoints/checkpoints/"

	// create dummy file
	_, err = os.Create(path + "dummy")
	if err != nil {
		logger.Errorf(err.Error())
		return
	}

	files, err := CountFilesInFolder(path)
	if err != nil {
		logger.Errorf(err.Error())
		return
	}

	fmt.Printf("Files count => %d \n", files)

	start := time.Now()

	pod, err = reconciler.BuildahRestorePipelined(ctx, "/tmp/checkpoints/checkpoints", clientset, namespace)
	if err != nil {
		logger.Errorf(err.Error())
		return
	}
	// Calculate the time taken for the restore
	elapsed := time.Since(start)
	logger.Info("Elapsed sequential: ", elapsed)

	SaveTimeToDB(ctx, db, numContainers, elapsed, "sequential", "restore_times", "containers", "elapsed")

	// eliminate docker image
	for i := 0; i < numContainers; i++ {
		BuildahDeleteImage("localhost/leonardopoggiani/checkpoint-images:container-" + strconv.Itoa(i))
	}

	CleanUp(ctx, clientset, pod, namespace)
}

func GetRestoreTimeSequential(ctx context.Context, clientset *kubernetes.Clientset, numContainers int, db *pgx.Conn, namespace string) {
	// Get the start time of the restore
	logger := log.New(os.Stderr).WithColor()

	reconciler := controllers.LiveMigrationReconciler{}
	pod := CreateTestContainers(ctx, numContainers, clientset, reconciler, namespace)
	if pod == nil {
		logger.Error("Pod not correctly created")
		logger.Info("Retrying creation..")
		pod = CreateTestContainers(ctx, numContainers, clientset, reconciler, namespace)
		if pod == nil {
			logger.Error("Pod not correctly created for the second time, exiting..")
			return
		}
	}

	err := utils.WaitForContainerReady(pod.Name, namespace, fmt.Sprintf("container-%d", numContainers-1), clientset)
	if err != nil {
		CleanUp(ctx, clientset, pod, namespace)
		logger.Error(err.Error())
		return
	}

	// Create a slice of Container structs
	var containers []types.Container

	// Append the container ID and name for each container in each pod
	pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		logger.Errorf(err.Error())
		CleanUp(ctx, clientset, pod, namespace)
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

	logger.Info("Checkpointing...")

	err = controllers.CheckpointPodPipelined(containers, namespace, pod.Name)
	if err != nil {
		return
	}

	path := "/tmp/checkpoints/checkpoints/"
	// create dummy file
	_, err = os.Create(path + "dummy")
	if err != nil {
		logger.Errorf(err.Error())
		return
	}

	files, err := CountFilesInFolder(path)
	if err != nil {
		logger.Errorf(err.Error())
		return
	}

	fmt.Printf("Files count => %d \n", files)

	CleanUp(ctx, clientset, pod, namespace)
	err = utils.WaitForPodDeletion(ctx, pod.Name, namespace, clientset)
	if err != nil {
		logger.Errorf(err.Error())
		return
	}

	start := time.Now()

	pod, err = reconciler.BuildahRestore(ctx, "/tmp/checkpoints/checkpoints", clientset, namespace)
	if err != nil {
		logger.Errorf(err.Error())
		return
	}

	err = utils.WaitForContainerReady(pod.Name, namespace, fmt.Sprintf("container-%d", numContainers-1), clientset)
	if err != nil {
		CleanUp(ctx, clientset, pod, namespace)
		logger.Errorf(err.Error())
		return
	}

	// Calculate the time taken for the restore
	elapsed := time.Since(start)
	fmt.Println("Elapsed sequential: ", elapsed)

	SaveTimeToDB(ctx, db, numContainers, elapsed, "sequential", "restore_times", "containers", "elapsed")

	// eliminate docker image
	for i := 0; i < numContainers; i++ {
		BuildahDeleteImage("localhost/leonardopoggiani/checkpoint-images:container-" + strconv.Itoa(i))
	}

	if _, err := exec.Command("sudo", "rm", "-rf", "/tmp/checkpoints/checkpoints/").Output(); err != nil {
		logger.Error(err.Error())
		return
	}

	if _, err := exec.Command("sudo", "mkdir", "/tmp/checkpoints/checkpoints/").Output(); err != nil {
		logger.Error(err.Error())
		return
	}

	CleanUp(ctx, clientset, pod, namespace)
}

func GetRestoreTimeParallelized(ctx context.Context, clientset *kubernetes.Clientset, numContainers int, db *pgx.Conn, namespace string) {
	// Get the start time of the restore
	logger := log.New(os.Stderr).WithColor()

	reconciler := controllers.LiveMigrationReconciler{}
	pod := CreateTestContainers(ctx, numContainers, clientset, reconciler, namespace)
	if pod == nil {
		logger.Error("Pod not correctly created")
		logger.Info("Retrying creation..")
		pod = CreateTestContainers(ctx, numContainers, clientset, reconciler, namespace)
		if pod == nil {
			logger.Error("Pod not correctly created for the second time, exiting..")
			return
		}
	}

	err := utils.WaitForContainerReady(pod.Name, namespace, fmt.Sprintf("container-%d", numContainers-1), clientset)
	if err != nil {
		CleanUp(ctx, clientset, pod, namespace)
		logger.Error(err.Error())
		return
	}

	// Create a slice of Container structs
	var containers []types.Container

	// Append the container ID and name for each container in each pod
	pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		logger.Errorf(err.Error())
		CleanUp(ctx, clientset, pod, namespace)
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

	logger.Info("Checkpointing...")

	err = controllers.CheckpointPodPipelined(containers, namespace, pod.Name)
	if err != nil {
		return
	}

	path := "/tmp/checkpoints/checkpoints/"
	// create dummy file
	_, err = os.Create(path + "dummy")
	if err != nil {
		logger.Errorf(err.Error())
		return
	}

	files, err := CountFilesInFolder(path)
	if err != nil {
		logger.Errorf(err.Error())
		return
	}

	fmt.Printf("Files count => %d \n", files)

	CleanUp(ctx, clientset, pod, namespace)
	err = utils.WaitForPodDeletion(ctx, pod.Name, namespace, clientset)
	if err != nil {
		logger.Errorf(err.Error())
		return
	}

	start := time.Now()

	pod, err = reconciler.BuildahRestoreParallelized(ctx, "/tmp/checkpoints/checkpoints", clientset, namespace)
	if err != nil {
		logger.Errorf(err.Error())
		return
	}

	err = utils.WaitForContainerReady(pod.Name, namespace, fmt.Sprintf("container-%d", numContainers-1), clientset)
	if err != nil {
		CleanUp(ctx, clientset, pod, namespace)
		logger.Errorf(err.Error())
		return
	}

	elapsed := time.Since(start)
	fmt.Println("Elapsed parallelized: ", elapsed)

	SaveTimeToDB(ctx, db, numContainers, elapsed, "parallelized", "restore_times", "containers", "elapsed")

	// eliminate docker image
	for i := 0; i < numContainers; i++ {
		BuildahDeleteImage("localhost/leonardopoggiani/checkpoint-images:container-" + strconv.Itoa(i))
	}

	if _, err := exec.Command("sudo", "rm", "-rf", "/tmp/checkpoints/checkpoints/").Output(); err != nil {
		logger.Error(err.Error())
		return
	}

	if _, err := exec.Command("sudo", "mkdir", "/tmp/checkpoints/checkpoints/").Output(); err != nil {
		logger.Error(err.Error())
		return
	}

	CleanUp(ctx, clientset, pod, namespace)
}
