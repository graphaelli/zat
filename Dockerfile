FROM golang:1.19

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY . .

RUN go build -v -o /usr/local/bin/ ./...

EXPOSE 8060

CMD ["/usr/local/bin/zat", "-addr=0.0.0.0:8060", "-no-server"]
