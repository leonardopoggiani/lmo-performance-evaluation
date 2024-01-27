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
	serviceAddress := "10.99.16.67"
	logger.Info("Starting latency test")

	for {
		startTime := time.Now()
		statusCode := CurlServiceAddress(serviceAddress)
		elapsed := time.Since(startTime)

		if statusCode == "200" {
			logger.Infof("Successfully reached service at %s with latency: %v\n", serviceAddress, elapsed)
			pkg.SaveTimeToDB(ctx, db, numContainers, elapsed, "service", "latency", "containers", "elapsed")
			time.Sleep(time.Second * 1)
			continue
		} else {
			logger.Infof("Unexpected status code from service at %s: %d\n", serviceAddress, statusCode)
		}

		time.Sleep(time.Second * 3)
	}
}

func CurlServiceAddress(serviceAddress string) string {

	cmd := exec.Command("curl", "--head", "--silent", "--connect-timeout", "50", "--max-time", "5", "--parallel", "--retry", "100", "--retry-delay", "1", "--retry-all-errors", serviceAddress)
	output, err := cmd.CombinedOutput()

	if err != nil {
		return "500"
	}

	statusCode, err := extractStatusCode(output)
	if err != nil {
		return "500"
	}

	return statusCode
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
