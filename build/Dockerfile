FROM golang:1.22 as builder

RUN mkdir /app
ADD . /app/
WORKDIR /app

# Enable static linking
ENV CGO_ENABLED=0

RUN go mod tidy
RUN go build -o main ./cmd/

FROM alpine:3.14

RUN apk --no-cache add curl bind-tools
WORKDIR /app
COPY --from=builder /app/main .
CMD ["./main"]