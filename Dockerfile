FROM docker.io/golang:1.24-alpine3.20 as builder
RUN mkdir /src /deps
RUN apk update && apk add git build-base binutils-gold
WORKDIR /deps
ADD go.mod /deps
RUN go mod download
ADD / /src
WORKDIR /src
RUN go build -o guestcluster-quota-webhook .
FROM docker.io/alpine:3.20
RUN adduser -S -D -h /app guestcluster-quota-webhook
USER guestcluster-quota-webhook
COPY --from=builder /src/guestcluster-quota-webhook /app/
WORKDIR /app
ENTRYPOINT ["./guestcluster-quota-webhook"]
