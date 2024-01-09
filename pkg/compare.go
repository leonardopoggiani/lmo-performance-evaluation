package pkg

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
	"github.com/withmandala/go-log"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func GetTimeDirectVsTriangularized(ctx context.Context, clientset *kubernetes.Clientset, numContainers int, db *pgx.Conn, exchange string, namespace string) {
	logger := log.New(os.Stderr).WithColor()

	reconciler := controllers.LiveMigrationReconciler{}
	pod := CreateTestContainers(ctx, numContainers, clientset, reconciler, namespace)

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

	start := time.Now()

	err = reconciler.CheckpointPodCrio(containers, namespace, pod.Name)
	if err != nil {
		logger.Errorf(err.Error())
		CleanUp(ctx, clientset, pod, namespace)
		return
	}

	CleanUp(ctx, clientset, pod, namespace)

	pod, err = reconciler.BuildahRestore(ctx, "/tmp/checkpoints/checkpoints", clientset, namespace)
	if err != nil {
		logger.Errorf(err.Error())
		return
	}

	utils.PushDockerImage("localhost/leonardopoggiani/checkpoint-images:container-"+strconv.Itoa(numContainers-1), "container-"+strconv.Itoa(numContainers-1), pod.Name)

	createContainers := []v1.Container{}

	if exchange == "direct" {
		// TODO: send to the other node
		for i := 0; i < numContainers; i++ {
			container := v1.Container{
				Name:            fmt.Sprintf("container-%d", i),
				Image:           "localhost/leonardopoggiani/checkpoint-images:container-" + strconv.Itoa(i),
				ImagePullPolicy: v1.PullPolicy("IfNotPresent"),
			}

			createContainers = append(createContainers, container)
		}
	} else if exchange == "triangularized" {
		for i := 0; i < numContainers; i++ {
			container := v1.Container{
				Name:            fmt.Sprintf("container-%d", i),
				Image:           "docker.io/leonardopoggiaini/checkpoint-images:container-" + strconv.Itoa(i),
				ImagePullPolicy: v1.PullPolicy("IfNotPresent"),
			}

			createContainers = append(createContainers, container)
		}
	}

	// Create the Pod
	pod, err = clientset.CoreV1().Pods(namespace).Create(ctx, &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("test-pod-%d-containers", numContainers),
			Labels: map[string]string{
				"app": "test",
			},
		},
		Spec: v1.PodSpec{
			Containers: createContainers,
		},
	}, metav1.CreateOptions{})
	if err != nil {
		logger.Errorf(err.Error())
		return
	}

	utils.WaitForContainerReady(pod.Name, namespace, "container-"+strconv.Itoa(numContainers-1), clientset)

	elapsed := time.Since(start)
	fmt.Printf("Time to checkpoint and restore %d containers: %s\n", numContainers, elapsed)

	SaveTimeToDB(ctx, db, numContainers, elapsed.Seconds(), exchange, "total_times", "containers", "elapsed")
}
