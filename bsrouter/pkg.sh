#!/bin/bash
##############################
#####Setting Environments#####
echo "Setting Environments"
set -e
export cpwd=`pwd`
export LD_LIBRARY_PATH=/usr/local/lib:/usr/lib
export PATH=$PATH:$GOPATH/bin:$HOME/bin:$GOROOT/bin
output=build


#### Package ####
srv_name=bsrouter
srv_ver=2.1.0
srv_out=$output/$srv_name
rm -rf $srv_out
mkdir -p $srv_out
##build normal
echo "Build $srv_name normal executor..."
go build -o $srv_out/bsrouter github.com/codingeasygo/bsck/bsrouter
go build -o $srv_out/bsconsole github.com/codingeasygo/bsck/bsconsole
cp -f bsrouter-install.sh $srv_out
cp -f bsrouter.service $srv_out
cp -f create-cert.sh $srv_out
cp -f default-bsrouter.json $srv_out
cp -f default-bsrouter.env $srv_out

###
cd $output
rm -f $srv_name-$srv_ver-`uname`.zip
zip -r $srv_name-$srv_ver-`uname`.zip $srv_name
cd ../
echo "Package $srv_name done..."