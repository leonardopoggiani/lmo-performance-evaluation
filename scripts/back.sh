#!/bin/bash

set -e

echo "Starting back.."

DATABASE_URL=postgresql://postgres:secret-password@127.0.0.1:25432/performance \
CHECKPOINTS_FOLDER=/tmp/checkpoints/checkpoints \
NAMESPACE=offloading \
KUBECONFIG=~/.kube/config sudo -E go run main.go create 1

DATABASE_URL=postgresql://postgres:secret-password@127.0.0.1:25432/performance \
CHECKPOINTS_FOLDER=/tmp/checkpoints/checkpoints \
NAMESPACE=offloading \
KUBECONFIG=~/.kube/config sudo -E go run main.go dummy

sudo rm -rf /tmp/checkpoints/checkpoints
sudo mkdir /tmp/checkpoints/checkpoints

while :
  do  
  DATABASE_URL=postgresql://postgres:secret-password@127.0.0.1:25432/performance \
  CHECKPOINTS_FOLDER=/tmp/checkpoints/checkpoints \
  NAMESPACE=offloading \
  KUBECONFIG=~/.kube/config sudo -E go run main.go checkpoint-all

  DATABASE_URL=postgresql://postgres:secret-password@127.0.0.1:25432/performance \
  CHECKPOINTS_FOLDER=/tmp/checkpoints/checkpoints \
  NAMESPACE=offloading \
  KUBECONFIG=~/.kube/config sudo -E go run main.go dummy

  DATABASE_URL=postgresql://postgres:secret-password@127.0.0.1:25432/performance \
  CHECKPOINTS_FOLDER=/tmp/checkpoints/checkpoints \
  NAMESPACE=pod-offloading \
  KUBECONFIG=~/.kube/config sudo -E go run main.go migrate

  sudo rm -rf /tmp/checkpoints/checkpoints
  sudo mkdir /tmp/checkpoints/checkpoints
  
  sleep 120

  DATABASE_URL=postgresql://postgres:secret-password@127.0.0.1:25432/performance \
  CHECKPOINTS_FOLDER=/tmp/checkpoints/checkpoints \
  NAMESPACE=offloading \
  KUBECONFIG=~/.kube/config sudo -E go run main.go delete
done

