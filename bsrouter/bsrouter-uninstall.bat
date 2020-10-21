@echo off
cd /d %~dp0
nssm stop "bsrouter"
nssm remove "bsrouter" confirm
bsconsole uninstall
pause