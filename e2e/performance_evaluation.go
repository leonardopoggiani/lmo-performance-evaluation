package e2e

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	migrationoperator "github.com/leonardopoggiani/live-migration-operator/controllers"
	_ "github.com/mattn/go-sqlite3"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func CreateContainers(ctx context.Context, numContainers int, clientset *kubernetes.Clientset) *v1.Pod {
	// Generate a random string
	randStr := fmt.Sprintf("%d", rand.Intn(4000)+1000)

	createContainers := []v1.Container{}
	// Add the specified number of containers to the Pod manifest
	for i := 0; i < numContainers; i++ {
		container := v1.Container{
			Name:            fmt.Sprintf("container-%d", i),
			Image:           "docker.io/library/nginx:latest",
			ImagePullPolicy: v1.PullPolicy("IfNotPresent"),
		}

		createContainers = append(createContainers, container)
	}

	// Create the Pod with the random string appended to the name
	pod, err := clientset.CoreV1().Pods("default").Create(ctx, &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("test-pod-%d-containers-%s", numContainers, randStr),
			Labels: map[string]string{
				"app": "test",
			},
		},
		Spec: v1.PodSpec{
			Containers: createContainers,
		},
	}, metav1.CreateOptions{})
	if err != nil {
		fmt.Println(err.Error())
		return nil
	}

	return pod
}

func getCheckpointSizePipelined(ctx context.Context, clientset *kubernetes.Clientset, numContainers int, db *sql.DB) {

	pod := CreateContainers(ctx, numContainers, clientset)

	LiveMigrationReconciler := migrationoperator.LiveMigrationReconciler{}

	err := LiveMigrationReconciler.WaitForContainerReady(pod.Name, "default", fmt.Sprintf("container-%d", numContainers-1), clientset)
	if err != nil {
		cleanUp(ctx, clientset, pod)
		fmt.Println(err.Error())
		return
	}

	fmt.Printf("Pod %s is ready\n", pod.Name)

	// Create a slice of Container structs
	var containers []migrationoperator.Container

	// Append the container ID and name for each container in each pod
	pods, err := clientset.CoreV1().Pods("default").List(ctx, metav1.ListOptions{})
	if err != nil {
		fmt.Println(err.Error())
		cleanUp(ctx, clientset, pod)
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

			container := migrationoperator.Container{
				ID:   containerID,
				Name: containerStatus.Name,
			}
			containers = append(containers, container)
		}
	}

	err = LiveMigrationReconciler.CheckpointPodPipelined(containers, "default", pod.Name)
	if err != nil {
		cleanUp(ctx, clientset, pod)
		fmt.Println(err.Error())
		return
	}

	directory := "/tmp/checkpoints/checkpoints"
	var size int64 = 0

	err = filepath.Walk(directory, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			cleanUp(ctx, clientset, pod)
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
		cleanUp(ctx, clientset, pod)
		fmt.Println(err.Error())
		return
	}

	sizeInMB := float64(size) / (1024 * 1024)
	fmt.Printf("The size of %s is %.2f MB.\n", directory, sizeInMB)
	saveToDB(db, int64(numContainers), sizeInMB, "pipelined", "checkpoint_sizes")

	// delete checkpoints folder
	if _, err := exec.Command("sudo", "rm", "-f", directory+"/").Output(); err != nil {
		cleanUp(ctx, clientset, pod)
		fmt.Println(err.Error())
		return
	}

	if _, err = exec.Command("sudo", "mkdir", "/tmp/checkpoints/checkpoints/").Output(); err != nil {
		cleanUp(ctx, clientset, pod)
		fmt.Println(err.Error())
		return
	}

	// check that checkpoints folder is empty
	if output, err := exec.Command("sudo", "ls", directory).Output(); err != nil {
		cleanUp(ctx, clientset, pod)
		fmt.Println(err.Error())
		return
	} else {
		fmt.Printf("Output: %s\n", output)
	}

	cleanUp(ctx, clientset, pod)
}

func getCheckpointSizeSequential(ctx context.Context, clientset *kubernetes.Clientset, numContainers int, db *sql.DB) {

	pod := CreateContainers(ctx, numContainers, clientset)

	LiveMigrationReconciler := migrationoperator.LiveMigrationReconciler{}

	err := LiveMigrationReconciler.WaitForContainerReady(pod.Name, "default", fmt.Sprintf("container-%d", numContainers-1), clientset)
	if err != nil {
		cleanUp(ctx, clientset, pod)
		fmt.Println(err.Error())
		return
	}

	fmt.Printf("Pod %s is ready\n", pod.Name)

	// Create a slice of Container structs
	var containers []migrationoperator.Container

	// Append the container ID and name for each container in each pod
	pods, err := clientset.CoreV1().Pods("default").List(ctx, metav1.ListOptions{})
	if err != nil {
		fmt.Println(err.Error())
		cleanUp(ctx, clientset, pod)
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

			container := migrationoperator.Container{
				ID:   containerID,
				Name: containerStatus.Name,
			}
			containers = append(containers, container)
		}
	}

	err = LiveMigrationReconciler.CheckpointPodCrio(containers, "default", pod.Name)
	if err != nil {
		fmt.Println(err.Error())
		cleanUp(ctx, clientset, pod)
		return
	}

	directory := "/tmp/checkpoints/checkpoints"
	var size int64 = 0

	err = filepath.Walk(directory, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			fmt.Println(err.Error())
			cleanUp(ctx, clientset, pod)
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
		cleanUp(ctx, clientset, pod)
		fmt.Println(err.Error())
		return
	}

	sizeInMB := float64(size) / (1024 * 1024)
	fmt.Printf("The size of %s is %.2f MB.\n", directory, sizeInMB)
	saveToDB(db, int64(numContainers), sizeInMB, "sequential", "checkpoint_sizes")

	// delete checkpoints folder
	if _, err := exec.Command("sudo", "rm", "-f", directory+"/").Output(); err != nil {
		cleanUp(ctx, clientset, pod)
		fmt.Println(err.Error())
		return
	}

	if _, err = exec.Command("sudo", "mkdir", "/tmp/checkpoints/checkpoints/").Output(); err != nil {
		cleanUp(ctx, clientset, pod)
		fmt.Println(err.Error())
		return
	}

	// check that checkpoints folder is empty
	if output, err := exec.Command("sudo", "ls", directory).Output(); err != nil {
		cleanUp(ctx, clientset, pod)
		fmt.Println(err.Error())
		return
	} else {
		fmt.Printf("Output: %s\n", output)
	}

	cleanUp(ctx, clientset, pod)
}

func cleanUp(ctx context.Context, clientset *kubernetes.Clientset, pod *v1.Pod) {
	fmt.Println("Garbage collecting => " + pod.Name)
	err := clientset.CoreV1().Pods("default").Delete(ctx, pod.Name, metav1.DeleteOptions{})
	if err != nil {
		fmt.Println(err.Error())
		return
	}
}

func getCheckpointTimeSequential(ctx context.Context, clientset *kubernetes.Clientset, numContainers int, db *sql.DB) {

	pod := CreateContainers(ctx, numContainers, clientset)

	LiveMigrationReconciler := migrationoperator.LiveMigrationReconciler{}

	err := LiveMigrationReconciler.WaitForContainerReady(pod.Name, "default", fmt.Sprintf("container-%d", numContainers-1), clientset)
	if err != nil {
		cleanUp(ctx, clientset, pod)
		fmt.Println(err.Error())
		return
	}

	fmt.Printf("Pod %s is ready\n", pod.Name)

	// Create a slice of Container structs
	var containers []migrationoperator.Container

	// Append the container ID and name for each container in each pod
	pods, err := clientset.CoreV1().Pods("default").List(ctx, metav1.ListOptions{})
	if err != nil {
		fmt.Println(err.Error())
		cleanUp(ctx, clientset, pod)
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

			container := migrationoperator.Container{
				ID:   containerID,
				Name: containerStatus.Name,
			}
			containers = append(containers, container)
		}
	}

	// Get the start time of the checkpoint
	start := time.Now()

	err = LiveMigrationReconciler.CheckpointPodCrio(containers, "default", pod.Name)
	if err != nil {
		return
	}

	// Calculate the time taken for the checkpoint
	elapsed := time.Since(start)
	fmt.Println("Elapsed sequential: ", elapsed)

	saveToDB(db, int64(numContainers), float64(elapsed.Milliseconds()), "sequential", "checkpoint_times")

	// delete checkpoint folder
	directory := "/tmp/checkpoints/checkpoints"
	if _, err := exec.Command("sudo", "rm", "-rf", directory+"/").Output(); err != nil {
		cleanUp(ctx, clientset, pod)
		fmt.Println(err.Error())
		return
	}

	if _, err = exec.Command("sudo", "mkdir", "/tmp/checkpoints/checkpoints/").Output(); err != nil {
		cleanUp(ctx, clientset, pod)
		fmt.Println(err.Error())
		return
	}

	cleanUp(ctx, clientset, pod)
}

func countFilesInFolder(folderPath string) (int, error) {
	fileCount := 0

	err := filepath.Walk(folderPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			fmt.Println(err.Error())
			return err
		}
		if !info.Mode().IsRegular() {
			fmt.Println("Not a regular file")
			return nil
		}
		fileCount += 1
		return nil
	})
	if err != nil {
		fmt.Println(err.Error())
		return 0, err
	}

	return fileCount, nil
}

func getCheckpointTimePipelined(ctx context.Context, clientset *kubernetes.Clientset, numContainers int, db *sql.DB) {

	pod := CreateContainers(ctx, numContainers, clientset)

	LiveMigrationReconciler := migrationoperator.LiveMigrationReconciler{}

	err := LiveMigrationReconciler.WaitForContainerReady(pod.Name, "default", fmt.Sprintf("container-%d", numContainers-1), clientset)
	if err != nil {
		cleanUp(ctx, clientset, pod)
		fmt.Println(err.Error())
		return
	}

	fmt.Printf("Pod %s is ready\n", pod.Name)

	// Create a slice of Container structs
	var containers []migrationoperator.Container

	// Append the container ID and name for each container in each pod
	pods, err := clientset.CoreV1().Pods("default").List(ctx, metav1.ListOptions{})
	if err != nil {
		fmt.Println(err.Error())
		cleanUp(ctx, clientset, pod)
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

			container := migrationoperator.Container{
				ID:   containerID,
				Name: containerStatus.Name,
			}
			containers = append(containers, container)
		}
	}

	start := time.Now()

	err = LiveMigrationReconciler.CheckpointPodPipelined(containers, "default", pod.Name)
	if err != nil {
		return
	}

	// Calculate the time taken for the checkpoint
	elapsed := time.Since(start)
	fmt.Println("Elapsed pipelined: ", elapsed)

	saveToDB(db, int64(numContainers), float64(elapsed.Milliseconds()), "pipelined", "checkpoint_times")

	// delete checkpoint folder
	directory := "/tmp/checkpoints/checkpoints"
	if _, err := exec.Command("sudo", "rm", "-rf", directory+"/").Output(); err != nil {
		cleanUp(ctx, clientset, pod)
		fmt.Println(err.Error())
		return
	}

	if _, err = exec.Command("sudo", "mkdir", "/tmp/checkpoints/checkpoints/").Output(); err != nil {
		cleanUp(ctx, clientset, pod)
		fmt.Println(err.Error())
		return
	}

	cleanUp(ctx, clientset, pod)
}

func getImageSize(imageName string) (float64, error) {
	// Build the command to get the image size
	cmd := exec.Command("sudo", "buildah", "inspect", "-f", "{{.Size}}", imageName)

	// Run the command and capture the output
	output, err := cmd.Output()
	if err != nil {
		return 0, err
	}

	// Parse the output as a float64
	sizeInBytes, err := strconv.ParseFloat(strings.TrimSpace(string(output)), 64)
	if err != nil {
		return 0, err
	}

	// Convert the size to MB
	sizeInMB := sizeInBytes / (1024 * 1024)

	return sizeInMB, nil
}

func saveToDB(db *sql.DB, numContainers int64, size float64, checkpointType string, db_name string) {
	// Prepare SQL statement
	stmt, err := db.Prepare("INSERT INTO " + db_name + " (containers, size, checkpoint_type) VALUES (?, ?, ?)")
	if err != nil {
		return
	}
	defer stmt.Close()

	// Execute statement
	_, err = stmt.Exec(numContainers, size, checkpointType)
	if err != nil {
		return
	}
}

func deletePodsStartingWithTest(ctx context.Context, clientset *kubernetes.Clientset) error {
	podList, err := clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return err
	}

	for _, pod := range podList.Items {
		if pod.ObjectMeta.Name[:5] == "test-" {
			err := clientset.CoreV1().Pods(pod.Namespace).Delete(ctx, pod.Name, metav1.DeleteOptions{})
			if err != nil {
				return err
			}
			fmt.Printf("Deleted pod %s\n", pod.Name)
		}
	}

	return nil
}
