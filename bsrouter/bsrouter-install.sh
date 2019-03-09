#!/bin/bash

installServer(){
  if [ ! -d /home/bsrouter ];then
    useradd bsrouter
    mkdir -p /home/bsrouter
    chown -R bsrouter:bsrouter /home/bsrouter
  fi
  cp -f bsrouter /usr/local/bin/bsrouter
  cp -f bsconsole /usr/local/bin/bsconsole
  cp -f bs-ssh.sh /usr/local/bin/bs-ssh
  cp -f bs-scp.sh /usr/local/bin/bs-scp
  cp -f bs-sftp.sh /usr/local/bin/bs-sftp
  ln -sf /usr/local/bin/bsconsole /usr/local/bin/bs-ping
  ln -sf /usr/local/bin/bsconsole /usr/local/bin/bs-state
  ln -sf /usr/local/bin/bsconsole /usr/local/bin/bs-bash
  if [ ! -f /etc/systemd/system/bsrouter.service ];then
    cp -f bsrouter.service /etc/systemd/system/
  fi
  mkdir -p /etc/bsrouter
  if [ ! -f /etc/bsrouter/bsrouter.json ];then
    cp -f default-bsrouter.json /etc/bsrouter/bsrouter.json
  fi
  systemctl enable bsrouter.service
}

installClient(){
  cd `dirname ${0}`
  ln -sf bs-ssh.sh bs-ssh
  ln -sf bs-scp.sh bs-scp
  ln -sf bs-sftp.sh bs-sftp
  ln -sf bsconsole bs-ping
  ln -sf bsconsole bs-state
  ln -sf bsconsole bs-bash
}

case "$1" in
  -i)
    case "$2" in
    -s)
      installServer
      ;;
    -c)
      installClient
      ;;
    esac
    ;;
  *)
    echo "Usage: ./bsrouter-install.sh -i"
    ;;
esac