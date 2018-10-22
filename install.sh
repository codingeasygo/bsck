sys=`uname`
case $sys in
Linux)
    go build -o /usr/local/bin/bsrouter github.com/sutils/bsck/bsrouter 
    go build -o /usr/local/bin/bsconsole github.com/sutils/bsck/bsconsole 
;;
Darwin)
    go install github.com/sutils/bsck/bsrouter 
    go install github.com/sutils/bsck/bsconsole 
    ln -sf `pwd`/bsconsole/bs-scp.sh $GOPATH/bin/bs-scp
    ln -sf `pwd`/bsconsole/bs-sftp.sh $GOPATH/bin/bs-sftp
    ln -sf `pwd`/bsconsole/bs-ssh.sh $GOPATH/bin/bs-ssh
;;
esac