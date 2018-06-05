sys=`uname`
case $sys in
Linux)
    go build -o /usr/local/bin/bsrouter github.com/sutils/bsck/bsrouter 
    go build -o /usr/local/bin/bsconsole github.com/sutils/bsck/bsconsole 
;;
Darwin)
    go install github.com/sutils/bsck/bsrouter 
    go install github.com/sutils/bsck/bsconsole 
;;
esac