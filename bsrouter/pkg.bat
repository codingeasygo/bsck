@echo off
set srv_name=bsrouter
set srv_ver=1.0.0
del /s /a /q build\%srv_name%
mkdir build
mkdir build\%srv_name%
go build -o build\%srv_name%\bsrouter.exe github.com/sutils/bsck/bsrouter
if NOT %ERRORLEVEL% EQU 0 goto :efail
reg Query "HKLM\Hardware\Description\System\CentralProcessor\0" | find /i "x86" > NUL && set OS=x86||set OS=x64
xcopy win-%OS%\nssm.exe build\%srv_name%
xcopy bsrouter-conf.bat build\%srv_name%
xcopy bsrouter-install.bat build\%srv_name%
xcopy bsrouter-uninstall.bat build\%srv_name%
xcopy create-cert.bat build\%srv_name%
xcopy bsrouter.json /F build\%srv_name%

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