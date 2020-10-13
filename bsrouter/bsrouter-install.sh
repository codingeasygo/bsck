#!/bin/bash
cd `dirname ${0}`

install(){
  if [ ! -d /home/bsrouter ];then
    useradd bsrouter
    mkdir -p /home/bsrouter
    chown -R bsrouter:bsrouter /home/bsrouter
  fi
  cp -f bsrouter /usr/local/bin/bsrouter
  cp -f bsconsole /usr/local/bin/bsconsole
  /usr/local/bin/bsconsole -uninstall
  /usr/local/bin/bsconsole -install
  if [ ! -f /etc/systemd/system/bsrouter.service ];then
    cp -f bsrouter.service /etc/systemd/system/
  fi
  mkdir -p /etc/bsrouter
  if [ ! -f /etc/bsrouter/bsrouter.env ];then
    cp -f default-bsrouter.env /etc/bsrouter/bsrouter.env
  fi
  if [ ! -f /etc/bsrouter/bsrouter.json ];then
    cp -f default-bsrouter.json /etc/bsrouter/bsrouter.json
  fi
  systemctl enable bsrouter.service
  systemctl restart bsrouter.service
}

case "$1" in
-i)
  install
  ;;
*)
  echo "Usage: ./bsrouter-install.sh -i"
  ;;
esac