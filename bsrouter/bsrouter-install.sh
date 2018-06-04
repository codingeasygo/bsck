#!/bin/bash
case "$1" in
  -i)
    useradd bsrouter
    mkdir -p /home/bsrouter
    chown -R bsrouter:bsrouter /home/bsrouter
    cp -f bsrouter /usr/local/bin/bsrouter
    cp -f bsconsole /usr/local/bin/bsconsole
    cp -f bsrouter.service /etc/systemd/system/
    mkdir -p /etc/bsrouter
    cp -f bsrouter.json /etc/bsrouter
    systemctl enable bsrouter.service
    ;;
  *)
    echo "Usage: ./bsrouter-install.sh -i"
    ;;
esac