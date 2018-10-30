#!/bin/bash
set -e
pkg_ver=1.4.2
rm -rf dist/
rm -rf out/BSRouter-darwin-x64-$pkg_ver
go build -o dist/bsrouter github.com/sutils/bsck/bsrouter
npm run pack-$1
cd out
case $1 in
osx)
    plutil -insert LSUIElement -bool true BSRouter-darwin-x64/BSRouter.app/Contents/Info.plist
    mv BSRouter-darwin-x64 BSRouter-darwin-x64-$pkg_ver
    7za a -r BSRouter-darwin-x64-$pkg_ver.zip BSRouter-darwin-x64-$pkg_ver
;;
linux)
    mv BSRouter-linux-x64 BSRouter-linux-x64-$pkg_ver
    7za a -r BSRouter-linux-x64-$pkg_ver.zip BSRouter-linux-x64-$pkg_ver
;;
win)
    mv BSRouter-win32-ia32 BSRouter-win32-ia32-$pkg_ver
    7za a -r BSRouter-win32-ia32-$pkg_ver.zip BSRouter-win32-ia32-$pkg_ver
;;
esac