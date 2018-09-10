#!/bin/bash
rm -rf out
npm run pack-$1
cd out
if [ "$1" == "osx" ];then
    plutil -insert LSUIElement -bool true BSRouter-darwin-x64/BSRouter.app/Contents/Info.plist
    7za a -r BSRouter-darwin-x64.zip BSRouter-darwin-x64
fi
