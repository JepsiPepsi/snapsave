FROM golang:1.21.5
WORKDIR /app
RUN mkdir -p /data
COPY go.mod go.sum ./
COPY main.go ./
RUN CGO_ENABLED=0 GOARCH=amd64 GOOS=linux go build -o ./main
CMD ["./main"]
