package main

import (
	"os"

	"github.com/leonardopoggiani/lmo-performance-evaluation/pkg"
	"github.com/withmandala/go-log"
)

func main() {
	logger := log.New(os.Stderr).WithColor()

	logger.Infof("Sender program, sending migration request")
	pkg.Sender(logger)
}
