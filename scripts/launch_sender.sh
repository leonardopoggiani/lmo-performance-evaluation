#!/bin/bash

set -e

printf "1 container..\n"
DATABASE_URL=postgresql://postgres:secret-password@127.0.0.1:25432/performance \
CHECKPOINTS_FOLDER=/tmp/checkpoints/checkpoints \
NUM_CONTAINERS=1 \
REPETITIONS=50 \
NAMESPACE=test-offloading \
KUBECONFIG=~/.kube/config sudo -E go run main.go sender

printf "2 container..\n"
DATABASE_URL=postgresql://postgres:secret-password@127.0.0.1:25432/performance \
CHECKPOINTS_FOLDER=/tmp/checkpoints/checkpoints \
NUM_CONTAINERS=2 \
REPETITIONS=50 \
NAMESPACE=test-offloading \
KUBECONFIG=~/.kube/config sudo -E go run main.go sender

printf "3 container..\n"
DATABASE_URL=postgresql://postgres:secret-password@127.0.0.1:25432/performance \
CHECKPOINTS_FOLDER=/tmp/checkpoints/checkpoints \
NUM_CONTAINERS=3 \
REPETITIONS=50 \
NAMESPACE=test-offloading \
KUBECONFIG=~/.kube/config sudo -E go run main.go sender

printf "5 container..\n"
DATABASE_URL=postgresql://postgres:secret-password@127.0.0.1:25432/performance \
CHECKPOINTS_FOLDER=/tmp/checkpoints/checkpoints \
NUM_CONTAINERS=5 \
REPETITIONS=50 \
NAMESPACE=test-offloading \
KUBECONFIG=~/.kube/config sudo -E go run main.go sender

printf "10 container..\n"
DATABASE_URL=postgresql://postgres:secret-password@127.0.0.1:25432/performance \
CHECKPOINTS_FOLDER=/tmp/checkpoints/checkpoints \
NUM_CONTAINERS=10 \
REPETITIONS=50 \
NAMESPACE=test-offloading \
KUBECONFIG=~/.kube/config sudo -E go run main.go sender