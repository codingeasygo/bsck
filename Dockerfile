FROM golang:1.17
WORKDIR /src
ADD . /src/
RUN GOPROXY=https://goproxy.cn,direct CGO_ENABLED=0 GOOS=linux go build -o /usr/bin/bsrouter ./bsrouter
RUN GOPROXY=https://goproxy.cn,direct CGO_ENABLED=0 GOOS=linux go build -o /usr/bin/bsconsole ./bsconsole

FROM alpine:latest  
RUN apk --no-cache add ca-certificates
WORKDIR /
COPY --from=0 /usr/bin/bsrouter /usr/bin
COPY --from=0 /usr/bin/bsconsole /usr/bin
RUN /usr/bin/bsconsole install
