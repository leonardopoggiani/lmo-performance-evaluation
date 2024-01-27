package pkg

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/leonardopoggiani/live-migration-operator/controllers"
	"github.com/leonardopoggiani/live-migration-operator/controllers/dummy"
	"github.com/leonardopoggiani/live-migration-operator/controllers/types"
	utils "github.com/leonardopoggiani/live-migration-operator/controllers/utils"
	"github.com/withmandala/go-log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

func GetBackLatency(ctx context.Context, clientset *kubernetes.Clientset, namespace string, db *pgx.Conn, numContainers int, logger *log.Logger) {
	logger.Info("Starting back-and-forth test")

	err := dummy.CreateDummyPod(clientset, ctx, namespace)
	if err != nil {
		logger.Errorf(err.Error())
		return
	}

	err = dummy.CreateDummyService(clientset, ctx, namespace)
	if err != nil {
		logger.Errorf(err.Error())
		return
	}

	_ = DeleteDummyPodAndService(ctx, clientset, "back-offloading", "dummy-pod", "dummy-service")
	_ = utils.WaitForPodDeletion(ctx, "dummy-pod", "back-offloading", clientset)

	err = dummy.CreateDummyPod(clientset, ctx, namespace)
	if err != nil {
		logger.Errorf(err.Error())
		return
	}

	err = dummy.CreateDummyService(clientset, ctx, namespace)
	if err != nil {
		logger.Errorf(err.Error())
		return
	}

	err = DeletePodsStartingWithTest(ctx, clientset, namespace)
	if err != nil {
		logger.Error("Error deleting pods starting with test-")
		return
	}

	logger.Info("Creating test pod..")
	reconciler := controllers.LiveMigrationReconciler{}
	directory := "/tmp/checkpoints/checkpoints"

	pod := CreateTestContainers(ctx, numContainers, clientset, reconciler, namespace)

	logger.Infof("Checkpointing pod %s", pod.Name)

	var containers []types.Container

	pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		logger.Error(err.Error())
		return
	}

	for _, pod := range pods.Items {
		for _, containerStatus := range pod.Status.ContainerStatuses {
			idParts := strings.Split(containerStatus.ContainerID, "//")

			logger.Info("containerStatus.ContainerID: " + containerStatus.ContainerID)
			logger.Info("containerStatus.Name: " + containerStatus.Name)

			if len(idParts) < 2 {
				logger.Error("Malformed container ID")
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

	err = controllers.CheckpointPodPipelined(containers, namespace, pod.Name)
	if err != nil {
		logger.Error(err.Error())
		return
	} else {
		logger.Info("Checkpointing completed")
	}

	if _, err := exec.Command("sudo", "touch", directory+"/dummy").Output(); err != nil {
		logger.Error(err.Error())
		return
	} else {
		logger.Info("Dummy file created")
	}

	for {
		files, err := os.ReadDir(directory)
		if err != nil {
			logger.Errorf("Error reading directory: %v\n", err)
			return
		}

		for _, file := range files {
			if file.IsDir() {
				logger.Infof("Directory: %s\n", file.Name())
			} else {
				logger.Infof("File: %s\n", file.Name())
			}
		}

		err = reconciler.MigrateCheckpoint(ctx, directory, clientset, "forth-offloading")
		if err != nil {
			logger.Error(err.Error())
			return
		} else {
			logger.Info("Migration completed")
		}

		err = DeletePodsStartingWithTest(ctx, clientset, namespace)
		if err != nil {
			logger.Error("Error deleting pods starting with test-")
			return
		}

		if _, err := exec.Command("sudo", "rm", "-f", directory+"/*").Output(); err != nil {
			CleanUp(ctx, clientset, pod, namespace)
			logger.Error(err.Error())
			logger.Error("Failed to delete checkpoints folder, command: " + "sudo rm -f " + directory + "/*")
			return
		}

		for {
			if waitForFile(21000*time.Second, directory) {
				logger.Info("File detected, restoring pod")

				start := time.Now()

				pod, err := reconciler.BuildahRestore(ctx, directory, clientset, "back-offloading")
				if err != nil {
					logger.Error(err.Error())
					os.Exit(1)
				} else {
					logger.Infof("Pod restored %s", pod.Name)

					utils.WaitForContainerReady(pod.Name, namespace, pod.Spec.Containers[0].Name, clientset)

					elapsed := time.Since(start)
					logger.Infof("[MEASURE] Restoring the pod took %d\n", elapsed)

					SaveTimeToDB(ctx, db, len(pod.Spec.Containers), elapsed, "restore", "back_and_forth_times", "containers", "elapsed")
					if err != nil {
						logger.Error(err.Error())
					}

					_ = exec.Command("sudo", "rm", "/tmp/checkpoints/checkpoints/dummy")

					_ = DeleteDummyPodAndService(ctx, clientset, namespace, "dummy-pod", "dummy-service")
					_ = utils.WaitForPodDeletion(ctx, "dummy-pod", namespace, clientset)

					err = dummy.CreateDummyPod(clientset, ctx, namespace)
					if err != nil {
						logger.Errorf(err.Error())
						return
					}

					err = dummy.CreateDummyService(clientset, ctx, namespace)
					if err != nil {
						logger.Errorf(err.Error())
						return
					}

					err = DeletePodsStartingWithTest(ctx, clientset, namespace)
					if err != nil {
						logger.Error("Error deleting pods starting with test-")
						return
					}

					pod := CreateTestContainers(ctx, numContainers, clientset, reconciler, namespace)

					logger.Infof("Checkpointing pod %s", pod.Name)

					var containers []types.Container

					pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{})
					if err != nil {
						logger.Error(err.Error())
						return
					}

					for _, pod := range pods.Items {
						for _, containerStatus := range pod.Status.ContainerStatuses {
							idParts := strings.Split(containerStatus.ContainerID, "//")

							logger.Info("containerStatus.ContainerID: " + containerStatus.ContainerID)
							logger.Info("containerStatus.Name: " + containerStatus.Name)

							if len(idParts) < 2 {
								logger.Error("Malformed container ID")
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

					err = controllers.CheckpointPodPipelined(containers, namespace, pod.Name)
					if err != nil {
						logger.Error(err.Error())
						return
					} else {
						logger.Info("Checkpointing completed")
					}

					err = reconciler.MigrateCheckpoint(ctx, directory, clientset, "forth-offloading")
					if err != nil {
						logger.Error(err.Error())
						return
					} else {
						logger.Info("Migration completed")
					}

					err = DeletePodsStartingWithTest(ctx, clientset, namespace)
					if err != nil {
						logger.Error("Error deleting pods starting with test-")
						return
					}
				}
			} else {
				logger.Error("Timeout: File not detected.")
				os.Exit(1)
			}
		}
	}
}
