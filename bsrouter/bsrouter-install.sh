#!/bin/bash
cd `dirname ${0}`

installService(){
  if [ ! -d /home/bsrouter ];then
    useradd bsrouter
    mkdir -p /home/bsrouter
    chown -R bsrouter:bsrouter /home/bsrouter
  fi
  cp -f bsrouter /usr/local/bin/bsrouter
  cp -f bsconsole /usr/local/bin/bsconsole
  /usr/local/bin/bsconsole uninstall
  /usr/local/bin/bsconsole install
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


uninstallService(){
  systemctl stop bsrouter.service
  systemctl disable bsrouter.service
  rm -f /etc/systemd/system/bsrouter.service
  /usr/local/bin/bsconsole uninstall
  rm -f /usr/local/bin/bsconsole
  rm -f /usr/local/bin/bsrouter
  userdel -R bsrouter
}

installClient(){
  cp -f bsrouter /usr/local/bin/bsrouter
  cp -f bsconsole /usr/local/bin/bsconsole
  /usr/local/bin/bsconsole uninstall
  /usr/local/bin/bsconsole install
  mkdir -p ~/.bsrouter
  if [ ! -f ~/.bsrouter/bsrouter.json ];then
    cp -f default-bsrouter.json ~/.bsrouter/bsrouter.json
  fi
}

uninstallClient(){
  /usr/local/bin/bsconsole uninstall
  rm -f /usr/local/bin/bsrouter
  rm -f /usr/local/bin/bsconsole
}

case "$1" in
install)
  case "$2" in
  service)
    installService
  ;;
  client)
    installClient
  ;;
  *)
    echo "Usage: ./bsrouter-install.sh -i service|client"
  ;;
  esac
;;
uninstall)
  case "$2" in
  service)
    uninstallService
  ;;
  client)
    uninstallClient
  ;;
  *)
    echo "Usage: ./bsrouter-install.sh install|uninstall service|client"
  ;;
  esac
;;
*)
  echo "Usage: ./bsrouter-install.sh install|uninstall service|client"
;;
esac