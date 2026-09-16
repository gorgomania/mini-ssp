FROM golang:1.27-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o /bin/ssp ./cmd/ssp/
RUN go build -o /bin/dsp ./cmd/dsp/
RUN go build -o /bin/consumer ./cmd/consumer/

FROM alpine:latest AS ssp
COPY --from=builder /bin/ssp /ssp
EXPOSE 8080
ENTRYPOINT ["/ssp"]

FROM alpine:latest AS dsp
COPY --from=builder /bin/dsp /dsp
EXPOSE 50051
ENTRYPOINT ["/dsp"]

FROM alpine:latest AS consumer
COPY --from=builder /bin/consumer /consumer
ENTRYPOINT ["/consumer"]
