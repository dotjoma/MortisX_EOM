@echo off
set "PATH=D:\go\bin;D:\GOPATH\bin;%PATH%"
set "GOROOT=D:\go"
set "GOPATH=D:\GOPATH"
set "GOCACHE=D:\GOCACHE"
set "GOTMPDIR=D:\GOTMP"
cd /d "%~dp0"

call npm run build
