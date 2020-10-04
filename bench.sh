#!/bin/bash
for i in {1..10}
do
   go test -v -run='^$' -bench=BenchmarkProxy
   if [ "$?" != "0" ];then
        echo "test error"
        exit 1
   fi
done
