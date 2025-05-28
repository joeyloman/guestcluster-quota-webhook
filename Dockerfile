FROM docker.io/golang:1.24.3-alpine3.21 AS builder
RUN mkdir /src
RUN apk update && apk add git build-base binutils-gold
ADD / /src
WORKDIR /src
RUN go build -mod vendor -o guestcluster-quota-webhook .
FROM docker.io/alpine:3.21
RUN adduser -S -D -h /app guestcluster-quota-webhook
USER guestcluster-quota-webhook
COPY --from=builder /src/guestcluster-quota-webhook /app/
WORKDIR /app
ENTRYPOINT ["./guestcluster-quota-webhook"]
