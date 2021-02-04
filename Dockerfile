FROM golang:alpine AS builder
MAINTAINER vk@mikrotik.me s
RUN apk update && apk add --no-cache git build-base
RUN git clone https://github.com/vasilevkirill/pupirka.git

RUN cp -R pupirka $GOPATH/src/app
WORKDIR $GOPATH/src/app
RUN go get -d -v
RUN go mod init
RUN GO111MODULE=on CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -o $GOPATH/src/app/pupirka.bin

FROM alpine:3.13.0
RUN apk update && apk add --no-cache tzdata
ENV TZ Europe/Moscow
COPY --from=builder /go/src/app/pupirka.bin /pupirka.bin
RUN  echo '*/1 * * * * cd / && /pupirka.bin' > /etc/crontabs/root
COPY device /device
COPY keys /keys
COPY pupirka.config.json /pupirka.config.json
CMD ["crond", "-f"]
