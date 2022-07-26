#!/bin/bash
set -xe

cd ~/go/src/github.com/codingeasygo/util
util_sha=`git rev-parse HEAD`

cd ~/go/src/github.com/codingeasygo/web
web_sha=`git rev-parse HEAD`

cd ~/go/src/sxbastudio.com/emall/emservice
go get github.com/codingeasygo/util@$util_sha
go get github.com/codingeasygo/web@$web_sha
go mod tidy
