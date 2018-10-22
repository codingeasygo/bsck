for i in $(seq 0 100);
do
  echo testing-$i
  go test -v github.com/sutils/bsck -count=1 > test.log 
done