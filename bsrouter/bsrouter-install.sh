#!/bin/bash
case "$1" in
  -i)
    if [ ! -d /home/bsrouter ];then
      useradd bsrouter
      mkdir -p /home/bsrouter
      chown -R bsrouter:bsrouter /home/bsrouter
    fi
    cp -f bsrouter /usr/local/bin/bsrouter
    cp -f bsconsole /usr/local/bin/bsconsole
    if [ ! -f /etc/systemd/system/bsrouter.service ];then
      cp -f bsrouter.service /etc/systemd/system/
    fi
    mkdir -p /etc/bsrouter
    if [ ! -f /etc/bsrouter/bsrouter.json ];then
      cp -f bsrouter.json /etc/bsrouter
    fi
    systemctl enable bsrouter.service
    ;;
  *)
    echo "Usage: ./bsrouter-install.sh -i"
    ;;
esac