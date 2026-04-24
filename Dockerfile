FROM docker.io/golang:1.25.9-alpine3.23 AS builder
RUN mkdir /src
RUN apk update && apk add git build-base binutils-gold
ADD / /src
WORKDIR /src
RUN go build -mod vendor -o guestcluster-quota-webhook .
FROM docker.io/alpine:3.23
RUN adduser -S -D -h /app guestcluster-quota-webhook
USER guestcluster-quota-webhook
COPY --from=builder /src/guestcluster-quota-webhook /app/
WORKDIR /app
ENTRYPOINT ["./guestcluster-quota-webhook"]
