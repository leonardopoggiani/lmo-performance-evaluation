FROM docker.io/golang:1.21-bullseye AS builder
WORKDIR /app

COPY go.mod ./go.mod
COPY main.go ./main.go
COPY internal ./internal
COPY pkg ./pkg

RUN go mod tidy 
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -tags containers_image_openpgp -a -installsuffix cgo -o ./bin/performance-evaluation .

FROM gcr.io/distroless/static-debian12:latest
WORKDIR /app

COPY --from=builder /app/bin/performance-evaluation /app/performance-evaluation

CMD ["./performance-evaluation"]

