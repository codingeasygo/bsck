#!/bin/bash
if [ "$#" -lt 1 ]; then
  echo "Usage: $0 URI"
  exit 1
fi
connect=`echo $1| sed 's/->/_/g;s/:\\/\\//_/g;s/\\./_/g;s/:/_/g'`
ssh -o ProxyCommand="bsconsole --proxy \"$1\"" $connect ${@:2}