@echo off
cd /d %~dp0
mkdir logs
nssm install "bsrouter" %CD%\bsrouter.exe %CD%\bsrouter.json
nssm set "bsrouter" AppStdout %CD%\logs\out.log
nssm set "bsrouter" AppStderr %CD%\logs\err.log
nssm start "bsrouter"
bsconsole install
pause