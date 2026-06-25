@echo off
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0laptopworker-remote.ps1" %*
