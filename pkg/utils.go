package pkg

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/leonardopoggiani/live-migration-operator/controllers"
	utils "github.com/leonardopoggiani/live-migration-operator/controllers/utils"
	"github.com/withmandala/go-log"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func CreateTestContainers(ctx context.Context, numContainers int, clientset *kubernetes.Clientset, reconciler controllers.LiveMigrationReconciler, namespace string) *v1.Pod {
	logger := log.New(os.Stderr).WithColor()

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
	pod, err := clientset.CoreV1().Pods(namespace).Create(ctx, &v1.Pod{
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
		logger.Errorf(err.Error())
		return nil
	} else {
		fmt.Printf("Pod %s created, container name: %s\n", pod.Name, createContainers[0].Name)
	}

	err = utils.WaitForContainerReady(pod.Name, namespace, createContainers[0].Name, clientset)
	if err != nil {
		logger.Errorf(err.Error())
		return nil
	} else {
		fmt.Println("Container started and ready")
	}

	return pod
}

func DeletePodsStartingWithTest(ctx context.Context, clientset *kubernetes.Clientset, namespace string) error {
	podList, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
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

func CleanUp(ctx context.Context, clientset *kubernetes.Clientset, pod *v1.Pod, namespace string) {
	logger := log.New(os.Stderr).WithColor()

	fmt.Println("Garbage collecting => " + pod.Name)
	err := clientset.CoreV1().Pods(namespace).Delete(ctx, pod.Name, metav1.DeleteOptions{})
	if err != nil {
		logger.Errorf(err.Error())
		return
	}
}

func CreateTable(ctx context.Context, conn *pgx.Conn, tableName string, columns string) {
	logger := log.New(os.Stderr).WithColor()

	_, err := conn.Exec(ctx, fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS %s (%s)`, tableName, columns))
	if err != nil {
		logger.Error(err)
	}
	fmt.Printf("Table %s created or already exists.\n", tableName)
}

func SaveSizeToDB(
	ctx context.Context,
	conn *pgx.Conn,
	numContainers int,
	size float64,
	checkpointType string,
	tableName string,
	column1 string,
	column2 string) {

	logger := log.New(os.Stderr).WithColor()

	normalizedSize := strconv.FormatFloat(size, 'f', -1, 32)

	sql := fmt.Sprintf("INSERT INTO %s (%s, %s, %s) VALUES ($1, $2, $3)", tableName, column1, column2, "checkpoint_type")
	// Prepare the SQL statement
	statement_name := fmt.Sprintf("statement-%d", rand.Intn(4000)+1000)

	stmt, err := conn.Prepare(ctx, statement_name, sql)
	if err != nil {
		logger.Error(err)
		logger.Error(stmt.SQL)
		return
	}

	logger.Info("Inserting data, query: " + stmt.SQL)

	// Execute the prepared statement
	_, err = conn.Exec(ctx, stmt.SQL, numContainers, normalizedSize, checkpointType)
	if err != nil {
		logger.Error(err)
		return
	}

	fmt.Println("Data inserted successfully.")
}

func SaveTimeToDB(
	ctx context.Context,
	conn *pgx.Conn,
	numContainers int,
	time float64,
	checkpointType string,
	tableName string,
	column1 string,
	column2 string) {

	logger := log.New(os.Stderr).WithColor()

	normalizedTime := strconv.FormatFloat(time, 'f', -1, 32)

	sql := fmt.Sprintf("INSERT INTO %s (%s, %s, %s) VALUES ($1, $2, $3)", tableName, column1, column2, "checkpoint_type")
	// Prepare the SQL statement
	statement_name := fmt.Sprintf("statement-%d", rand.Intn(4000)+1000)

	stmt, err := conn.Prepare(ctx, statement_name, sql)
	if err != nil {
		logger.Error(err)
		logger.Error(stmt.SQL)
		return
	}

	logger.Info("Inserting data, query: " + stmt.SQL)

	// Execute the prepared statement
	_, err = conn.Exec(ctx, stmt.SQL, numContainers, normalizedTime, checkpointType)
	if err != nil {
		logger.Error(err)
		return
	}

	logger.Info("Data inserted successfully.")
}

func CountFilesInFolder(folderPath string) (int, error) {
	logger := log.New(os.Stderr).WithColor()

	fileCount := 0

	err := filepath.Walk(folderPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			logger.Error(err.Error())
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
		logger.Errorf(err.Error())
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
