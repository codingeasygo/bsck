@echo off
set srv_name=bsrouter
set srv_ver=1.4.2
set OS=%1
del /s /a /q build\%srv_name%
mkdir build
mkdir build\%srv_name%
set GOOS=windows
set GOARCH=%1
go build -o build\%srv_name%\bsrouter.exe github.com/codingeasygo/bsck/bsrouter
go build -o build\%srv_name%\bsconsole.exe github.com/codingeasygo/bsck/bsconsole
if NOT %ERRORLEVEL% EQU 0 goto :efail
xcopy win-%OS%\nssm.exe build\%srv_name%
xcopy bsrouter-conf.bat build\%srv_name%
xcopy bsrouter-install.bat build\%srv_name%
xcopy bsrouter-uninstall.bat build\%srv_name%
xcopy create-cert.bat build\%srv_name%
xcopy default-bsrouter.json /F build\%srv_name%

if NOT %ERRORLEVEL% EQU 0 goto :efail

cd build
del /s /a /q %srv_name%-%srv_ver%-Win-%OS%.zip
7z a -r %srv_name%-%srv_ver%-Win-%OS%.zip %srv_name%
if NOT %ERRORLEVEL% EQU 0 goto :efail
cd ..\
goto :esuccess

:efail
echo "Build fail"
pause
exit 1

:esuccess
echo "Build success"
pause