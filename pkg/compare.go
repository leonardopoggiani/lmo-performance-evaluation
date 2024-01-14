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
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func GetTriangularizedTime(ctx context.Context, clientset *kubernetes.Clientset, numContainers int, db *pgx.Conn, namespace string) {
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

	err = reconciler.CheckpointPodCrio(containers, namespace, pod.Name)
	if err != nil {
		logger.Errorf(err.Error())
		CleanUp(ctx, clientset, pod, namespace)
		return
	}

	CleanUp(ctx, clientset, pod, namespace)

	start := time.Now()

	pod, err = reconciler.BuildahRestore(ctx, "/tmp/checkpoints/checkpoints", clientset, namespace)
	if err != nil {
		logger.Errorf(err.Error())
		return
	}

	CleanUp(ctx, clientset, pod, namespace)

	for i := 0; i < numContainers; i++ {
		utils.PushDockerImage("localhost/leonardopoggiani/checkpoint-images:container-"+strconv.Itoa(i), "container-"+strconv.Itoa(i), pod.Name)
	}

	createContainers := []v1.Container{}

	for i := 0; i < numContainers; i++ {
		container := v1.Container{
			Name:            fmt.Sprintf("container-%d", i),
			Image:           "172.16.3.75:5000/checkpoint-images:container-" + strconv.Itoa(i),
			ImagePullPolicy: v1.PullPolicy("Always"),
			SecurityContext: &v1.SecurityContext{
				SELinuxOptions: &v1.SELinuxOptions{
					Level: "s0:c525,c600",
				},
			},
		}

		createContainers = append(createContainers, container)
	}

	// Create the Pod
	pod, err = clientset.CoreV1().Pods(namespace).Create(ctx, &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("test-pod-%d-containers", numContainers),
			Labels: map[string]string{
				"app": "restored",
			},
			Namespace: namespace,
			Annotations: map[string]string{
				"io.kubernetes.cri-o.TrySkipVolumeSELinuxLabel": "true",
			},
		},
		Spec: v1.PodSpec{
			Containers:            createContainers,
			ShareProcessNamespace: &[]bool{true}[0],
		},
	}, metav1.CreateOptions{})
	if err != nil {
		logger.Errorf(err.Error())
		return
	}

	utils.WaitForContainerReady(pod.Name, namespace, "container-"+strconv.Itoa(numContainers-1), clientset)

	elapsed := time.Since(start)
	logger.Infof("Time to checkpoint and restore %d containers: %s\n", numContainers, elapsed)

	SaveTimeToDB(ctx, db, numContainers, elapsed, "triangularized", "triangularized_times", "containers", "elapsed")

	directory := "/tmp/checkpoints/checkpoints"
	if _, err := exec.Command("sudo", "rm", "-rf", directory+"/").Output(); err != nil {
		CleanUp(ctx, clientset, pod, namespace)
		logger.Error(err.Error())
		return
	}

	if _, err = exec.Command("sudo", "mkdir", "/tmp/checkpoints/checkpoints/").Output(); err != nil {
		CleanUp(ctx, clientset, pod, namespace)
		logger.Error(err.Error())
		return
	}

	CleanUp(ctx, clientset, pod, namespace)
}
