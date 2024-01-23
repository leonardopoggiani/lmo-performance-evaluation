#!/bin/bash

set -e

while true; do 
  printf "Receiver starting..\n"
  DATABASE_URL=postgresql://postgres:secret-password@127.0.0.1:25432/performance \
  CHECKPOINTS_FOLDER=/tmp/checkpoints/checkpoints \
  NUM_CONTAINERS=1 \
  NAMESPACE=test-offloading \
  KUBECONFIG=~/.kube/config sudo -E go run main.go receiver
sleep 2; done