@echo off

cls

@REM File to be deleted
SET Settings="settings.json"
SET Database="vertigo.db"

IF EXIST %Settings% del /F %Settings%
IF EXIST %Database% del /F %Database%

go test -cover