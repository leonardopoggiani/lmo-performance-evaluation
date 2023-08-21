# Performance evaluation

This repository contains the performance evaluation for the Live-Migration-Controller project. It consists of three main components: e2e, sender, and receiver.

## End-to-End (e2e)

The e2e component is responsible for end-to-end testing of the application. It contains the performance_evaluation.go file which includes functions for creating containers, getting checkpoint time, getting restore time, and getting checkpoint image restore size among others.

To run the end-to-end tests, navigate to the e2e directory and run the performance_evaluation.go file with the go run command:

```Bash
cd e2e
go run performance_evaluation.go
```

## Sender

The sender component is responsible for sending migration requests. It contains the sender.go file which includes functions for creating containers and deleting pods that start with "test".

To run the sender, navigate to the sender directory and run the sender.go file with the go run command:

```Bash
cd sender
go run sender.go
```

## Receiver

The receiver component is responsible for receiving migration requests. It contains the receiver.go file which includes functions for waiting for file creation and getting all files in a directory.

To run the receiver, navigate to the receiver directory and run the receiver.go file with the go run command:

```Bash
cd receiver
go run receiver.go
```

## Dependencies

Each component has its own set of dependencies, which are listed in their respective go.sum files.

## Summary

This repository provides the necessary components to evaluate the performance of the Live-Migration-Controller project. By running the e2e, sender, and receiver components, you can test the application end-to-end, send migration requests, and receive migration requests, respectively.