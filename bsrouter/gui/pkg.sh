#!/bin/bash
rm -rf out
pkg_ver=1.4.2
npm run pack-$1
cd out
if [ "$1" == "osx" ];then
    plutil -insert LSUIElement -bool true BSRouter-darwin-x64/BSRouter.app/Contents/Info.plist
    mv BSRouter-darwin-x64 BSRouter-darwin-x64-$pkg_ver
    7za a -r BSRouter-darwin-x64-$pkg_ver.zip BSRouter-darwin-x64-$pkg_ver
fi
