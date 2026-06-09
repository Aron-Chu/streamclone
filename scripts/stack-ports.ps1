# Shows which process holds Streamclone-related host ports.
$ports = @(8090, 5174, 8081, 8082, 8083, 8084, 8086, 8095, 1935, 8888, 5432, 6379, 9000, 9001, 3001, 9090)

Write-Host "Streamclone port check:" -ForegroundColor Cyan
foreach ($port in $ports) {
    $listeners = Get-NetTCPConnection -LocalPort $port -State Listen -ErrorAction SilentlyContinue
    if (-not $listeners) {
        continue
    }
    foreach ($conn in $listeners) {
        $proc = Get-Process -Id $conn.OwningProcess -ErrorAction SilentlyContinue
        $name = if ($proc) { $proc.ProcessName } else { "pid $($conn.OwningProcess)" }
        Write-Host ("  :{0,-5} -> {1} (pid {2})" -f $port, $name, $conn.OwningProcess)
    }
}

Write-Host ""
Write-Host "Docker streamclone containers:" -ForegroundColor Cyan
docker ps -a --filter "name=streamclone" --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"
