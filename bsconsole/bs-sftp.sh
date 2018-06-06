#!/bin/bash
if [ "$#" -lt 2 ]; then
  echo "Usage: $0 URI path"
  exit 1
fi
connect=`echo $1| sed 's/->/_/g;s/:\\/\\//_/g;s/\\./_/g;s/:/_/g'`
args=()
for a in ${@:2}
do
    a=`echo $a| sed "s/bshost/$connect/g;"`
    args+=($a)
done
sftp -o ProxyCommand="bsconsole --proxy \"$1\"" ${args[@]}