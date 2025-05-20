FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY main.go .
RUN go install github.com/duosecurity/duo_api_golang/authapi@latest
RUN go build -o ssh-knock .

FROM alpine
RUN apk add --no-cache fwknop-client
COPY --from=builder /app/ssh-knock /ssh-knock
ENV KNOCK_SECRET=supersecrettoken
CMD ["/ssh-knock"]
