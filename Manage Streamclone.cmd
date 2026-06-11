@echo off
title Streamclone Manager
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0scripts\streamclone-manager.ps1" -Action menu
if errorlevel 1 pause
