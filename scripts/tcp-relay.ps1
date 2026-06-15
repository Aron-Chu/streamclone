param(
    [string]$ListenHost = '127.0.0.1',
    [Parameter(Mandatory = $true)][int]$ListenPort,
    [Parameter(Mandatory = $true)][string]$ConnectHost,
    [Parameter(Mandatory = $true)][int]$ConnectPort
)

$ErrorActionPreference = 'Stop'

$address = [System.Net.IPAddress]::Parse($ListenHost)
$listener = [System.Net.Sockets.TcpListener]::new($address, $ListenPort)
$listener.Server.SetSocketOption(
    [System.Net.Sockets.SocketOptionLevel]::Socket,
    [System.Net.Sockets.SocketOptionName]::ReuseAddress,
    $true
)
$listener.Start()

while ($true) {
    $client = $listener.AcceptTcpClient()
    try {
        $upstream = [System.Net.Sockets.TcpClient]::new()
        $upstream.Connect($ConnectHost, $ConnectPort)

        $clientStream = $client.GetStream()
        $upstreamStream = $upstream.GetStream()

        # CopyToAsync is implemented by .NET and avoids runspace/thread lifetime issues.
        $clientStream.CopyToAsync($upstreamStream) | Out-Null
        $upstreamStream.CopyToAsync($clientStream) | Out-Null
    } catch {
        try { $client.Close() } catch {}
        try { if ($upstream) { $upstream.Close() } } catch {}
    }
}
