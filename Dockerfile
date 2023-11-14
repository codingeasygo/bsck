FROM golang:1.19
WORKDIR /src
ADD . /src/
RUN cd /src/bsrouter/ && GOOS=linux go build -o .
RUN cd /src/bsconsole/ && GOOS=linux go build -o .

FROM ubuntu:22.04  
RUN apt update && apt-get install -y ca-certificates ssh && apt clean all
WORKDIR /
COPY --from=0 /src/bsrouter/bsrouter /usr/bin
COPY --from=0 /src/bsconsole/bsconsole /usr/bin
RUN /usr/bin/bsconsole install
