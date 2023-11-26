package internal

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/leonardopoggiani/live-migration-operator/controllers"
	utils "github.com/leonardopoggiani/live-migration-operator/controllers/utils"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func CreateTestContainers(ctx context.Context, numContainers int, clientset *kubernetes.Clientset, reconciler controllers.LiveMigrationReconciler) *v1.Pod {
	// Generate a random string
	randStr := fmt.Sprintf("%d", rand.Intn(4000)+1000)

	createContainers := []v1.Container{}
	fmt.Println("numContainers: " + fmt.Sprintf("%d", numContainers))
	// Add the specified number of containers to the Pod manifest
	for i := 0; i < numContainers; i++ {
		fmt.Println("Creating container: " + fmt.Sprintf("container-%d", i))

		container := v1.Container{
			Name:            fmt.Sprintf("container-%d", i),
			Image:           "docker.io/library/tomcat:latest",
			ImagePullPolicy: v1.PullPolicy("IfNotPresent"),
			Ports: []v1.ContainerPort{
				{
					ContainerPort: 8080,
					Protocol:      v1.Protocol("TCP"),
				},
			},
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
			Containers:            createContainers,
			ShareProcessNamespace: &[]bool{true}[0],
		},
	}, metav1.CreateOptions{})

	if err != nil {
		fmt.Println(err.Error())
		return nil
	} else {
		fmt.Printf("Pod created %s", pod.Name)
		fmt.Println(createContainers[0].Name)
	}

	err = utils.WaitForContainerReady(pod.Name, "default", createContainers[0].Name, clientset)
	if err != nil {
		fmt.Println(err.Error())
		return nil
	} else {
		fmt.Println("Container ready")
	}

	return pod
}

func DeletePodsStartingWithTest(ctx context.Context, clientset *kubernetes.Clientset) error {
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

func CleanUp(ctx context.Context, clientset *kubernetes.Clientset, pod *v1.Pod) {
	fmt.Println("Garbage collecting => " + pod.Name)
	err := clientset.CoreV1().Pods("default").Delete(ctx, pod.Name, metav1.DeleteOptions{})
	if err != nil {
		fmt.Println(err.Error())
		return
	}
}

func SaveToDB(db *sql.DB, numContainers int64, size float64, checkpointType string, db_name string) {
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

func CountFilesInFolder(folderPath string) (int, error) {
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

func GetImageSize(imageName string) (float64, error) {
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
