package main

import (
	"os"
	"runtime"

	"github.com/leonardopoggiani/lmo-performance-evaluation/pkg"
	"github.com/withmandala/go-log"
)

func main() {
	logger := log.New(os.Stderr).WithColor()
	runtime.GOMAXPROCS(runtime.NumCPU())

	logger.Info("Receiver program started, waiting for migration request")

	pkg.Receive(logger)
}
