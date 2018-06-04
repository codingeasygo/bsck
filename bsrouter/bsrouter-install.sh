#!/bin/bash
case "$1" in
  -i)
    useradd bsrouter
    cp -f bsrouter /usr/local/bin/bsrouter
    cp -f bsrouter.service /etc/systemd/system/
    mkdir -p /etc/bsrouter
    cp -f bsrouter.json /etc/bsrouter
    systemctl enable bsrouter.service
    ;;
  *)
    echo "Usage: ./bsrouter-install.sh -i"
    ;;
esac