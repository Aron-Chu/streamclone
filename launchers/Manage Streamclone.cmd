@echo off
title Streamclone Manager
powershell -NoProfile -ExecutionPolicy Bypass -File "%~dp0install-streamclone-launcher.ps1" -Action manage -LauncherRoot "%~dp0.."
if errorlevel 1 pause
