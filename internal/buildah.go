package internal

import (
	"fmt"
	"os/exec"
)

func BuildahDeleteImage(imageName string) {
	buildahCmd := exec.Command("sudo", "buildah", "rmi", imageName)
	err := buildahCmd.Run()
	if err != nil {
		fmt.Println("Error removing image:", err)
		return
	}
	fmt.Println("Image", imageName, "removed successfully.")
}
