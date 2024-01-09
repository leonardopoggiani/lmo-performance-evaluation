package pkg

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/withmandala/go-log"
)

func BuildahDeleteImage(imageName string) {
	logger := log.New(os.Stderr).WithColor()

	buildahCmd := exec.Command("sudo", "buildah", "rmi", imageName)
	err := buildahCmd.Run()
	if err != nil {
		logger.Errorf("Error removing image: %s", err)
		return
	}
	fmt.Println("Image", imageName, "removed successfully.")
}
