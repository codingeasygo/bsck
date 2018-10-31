#!/bin/bash
set -e
pkg_ver=1.4.2
pkg_osx(){
    rm -rf dist/
    rm -rf out/BSRouter-darwin-x64-$pkg_ver
    rm -rf bsrouter
    7z x ../build/bsrouter-$pkg_ver-Darwin.zip
    npm run pack-osx
    cd out
    plutil -insert LSUIElement -bool true BSRouter-darwin-x64/BSRouter.app/Contents/Info.plist
    mv BSRouter-darwin-x64 BSRouter-darwin-x64-$pkg_ver
    7z a -r BSRouter-darwin-x64-$pkg_ver.zip BSRouter-darwin-x64-$pkg_ver
    cd ../
}
pkg_linux(){
    rm -rf dist/
    rm -rf out/BSRouter-linux-x64-$pkg_ver
    rm -rf bsrouter
    7z x ../build/bsrouter-$pkg_ver-Linux.zip
    npm run pack-linux
    cd out
    mv BSRouter-linux-x64 BSRouter-linux-x64-$pkg_ver
    7z a -r BSRouter-linux-x64-$pkg_ver.zip BSRouter-linux-x64-$pkg_ver
    cd ../
}
pkg_win(){
    rm -rf dist/
    rm -rf out/BSRouter-win32-ia32-$pkg_ver
    rm -rf bsrouter
    7z x ../build/bsrouter-$pkg_ver-Win-386.zip
    npm run pack-win
    cd out
    mv BSRouter-win32-ia32 BSRouter-win32-ia32-$pkg_ver
    7z a -r BSRouter-win32-ia32-$pkg_ver.zip BSRouter-win32-ia32-$pkg_ver
    cd ../
}

case $1 in
osx)
    pkg_osx
;;
linux)
    pkg_linux
;;
win)
    pkg_win
;;
all)
    rm -rf out
    pkg_osx
    pkg_win
;;
esac