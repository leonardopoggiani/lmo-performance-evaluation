package latency

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/leonardopoggiani/lmo-performance-evaluation/pkg"
	"github.com/withmandala/go-log"
	"k8s.io/client-go/kubernetes"
)

func GetLatency(ctx context.Context, clientset *kubernetes.Clientset, namespace string, db *pgx.Conn, numContainers int, logger *log.Logger) {
	serviceAddress := "10.106.175.108"
	logger.Info("Starting latency test")

	for {
		statusCode, latency, err := CurlServiceAddress(serviceAddress)
		if err != nil {
			logger.Info("Error during service address curl request:", err)
			pkg.SaveTimeToDB(ctx, db, numContainers, 0, "unreachable", "latency", "containers", "elapsed")
		} else if statusCode == "200" {
			logger.Infof("Successfully reached service at %s with latency: %v\n", serviceAddress, latency)
			pkg.SaveTimeToDB(ctx, db, numContainers, latency, "service", "latency", "containers", "elapsed")
			time.Sleep(time.Second * 1)
			continue
		} else {
			logger.Infof("Unexpected status code from service at %s: %d\n", serviceAddress, statusCode)
		}

		time.Sleep(time.Second * 3)
	}
}

func CurlServiceAddress(serviceAddress string) (string, time.Duration, error) {
	startTime := time.Now()

	cmd := exec.Command("curl", "--head", "--silent", "--connect-timeout", "2", serviceAddress)
	output, err := cmd.CombinedOutput()

	elapsed := time.Since(startTime)

	if err != nil {
		return "500", elapsed, errors.New("failed to curl service address: " + err.Error())
	}

	statusCode, err := extractStatusCode(output)
	if err != nil {
		return "500", elapsed, errors.New("failed to extract status code: " + err.Error())
	}

	return statusCode, elapsed, nil
}

func extractStatusCode(response []byte) (string, error) {
	lines := strings.Split(string(response), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "HTTP/") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				return strings.Split(fields[1], "/")[0], nil
			}
		}
	}

	return "500", errors.New("failed to extract status code from response")
}
