param(
    [Parameter(Mandatory = $true)]
    [ValidateSet('Preflight','Windows','Final')]
    [string]$Phase,
    [Parameter(Mandatory = $true)]
    [string]$BaseUrl,
    [Parameter(Mandatory = $true)]
    [string]$SessionDir,
    [string]$ObservedTrueNASVersion,
    [string]$ObservedControlImage,
    [string]$ObservedCupsImage,
    [string]$QueueName,
    [string]$AppleOSVersion,
    [ValidateSet('iPhone','iPad')]
    [string]$AppleClientType = 'iPhone'
)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$ExpectedTrueNASVersion = '25.04.1'
$ExpectedControlDigest = 'sha256:c8d5787162db919f84e9607d13f368995138861355f3fa269cbb10561f24d80d'
$ExpectedCupsDigest = 'sha256:0997ad2054ca5e57f34291372aed55f549eee9ff201f171b430436b0655c0814'
$ExpectedPort = 8634
$ExpectedResourcePath = '/printers/FolioRelay'

New-Item -ItemType Directory -Force -Path $SessionDir | Out-Null
$SessionDir = (Resolve-Path $SessionDir).Path
$BaseUrl = $BaseUrl.TrimEnd('/')

function ConvertFrom-SecureStringPlain([Security.SecureString]$Secure) {
    $ptr = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($Secure)
    try { return [Runtime.InteropServices.Marshal]::PtrToStringBSTR($ptr) }
    finally { [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($ptr) }
}

function Write-JsonFile([string]$Path, $Value) {
    $Value | ConvertTo-Json -Depth 16 | Set-Content -Encoding utf8 $Path
}

function Assert-IdentityEqual($Expected, $Observed) {
    if ($Expected.printer_uuid -ne $Observed.printer_uuid) {
        throw "printer UUID changed: expected=$($Expected.printer_uuid) observed=$($Observed.printer_uuid)"
    }
    if ($Expected.host -ne $Observed.host -or
        [int]$Expected.port -ne [int]$Observed.port -or
        $Expected.resource_path -ne $Observed.resource_path -or
        $Expected.scheme -ne $Observed.scheme) {
        throw 'public printer URI identity changed'
    }
}

function Get-ClientOS {
    $os = Get-CimInstance Win32_OperatingSystem
    [ordered]@{
        caption = $os.Caption
        version = $os.Version
        build_number = $os.BuildNumber
        architecture = $os.OSArchitecture
    }
}

$secureToken = Read-Host 'FolioRelay management token (used in memory only)' -AsSecureString
$token = ConvertFrom-SecureStringPlain $secureToken
if ([string]::IsNullOrWhiteSpace($token)) { throw 'management token is required' }

$handler = [System.Net.Http.HttpClientHandler]::new()
$handler.UseProxy = $false
$client = [System.Net.Http.HttpClient]::new($handler)
$client.Timeout = [TimeSpan]::FromSeconds(8)
$client.DefaultRequestHeaders.Authorization = [System.Net.Http.Headers.AuthenticationHeaderValue]::new('Bearer', $token)

function Resolve-Uri([string]$Path) {
    return [Uri]::new("$BaseUrl/" + $Path.TrimStart('/'))
}

function Get-Json([string]$Path) {
    $response = $client.GetAsync((Resolve-Uri $Path)).GetAwaiter().GetResult()
    $body = $response.Content.ReadAsStringAsync().GetAwaiter().GetResult()
    if (-not $response.IsSuccessStatusCode) {
        throw "GET $Path failed: HTTP $([int]$response.StatusCode) $body"
    }
    return $body | ConvertFrom-Json
}

function Get-HttpStatus([string]$Path) {
    $response = $client.GetAsync((Resolve-Uri $Path)).GetAwaiter().GetResult()
    return [int]$response.StatusCode
}

function Read-LiveState {
    $health = Get-HttpStatus '/healthz'
    $ready = Get-HttpStatus '/readyz'
    if ($health -ne 204 -or $ready -ne 204) {
        throw "FolioRelay is not healthy/ready (health=$health ready=$ready)"
    }
    $capabilities = Get-Json '/.well-known/foliorelay'
    $status = Get-Json '/api/v1/status'
    $printer = Get-Json '/api/v1/printer'
    $jobs = Get-Json '/api/v1/jobs?limit=200'
    [ordered]@{
        health_http = $health
        ready_http = $ready
        capabilities = $capabilities
        status = $status
        printer = $printer
        job_ids = @($jobs.items | ForEach-Object { $_.job_id })
        recent_jobs = @($jobs.items)
    }
}

function Assert-PublicCandidate($Live) {
    if ([int]$Live.printer.identity.port -ne $ExpectedPort) {
        throw "unexpected IPP port: $($Live.printer.identity.port)"
    }
    if ($Live.printer.identity.resource_path -ne $ExpectedResourcePath) {
        throw "unexpected resource path: $($Live.printer.identity.resource_path)"
    }
    if (-not $Live.printer.profiles.windows_ipp -or -not $Live.printer.profiles.airprint) {
        throw 'Windows IPP and AirPrint profiles must both be enabled'
    }
}

function Assert-NoTokenLeak {
    foreach ($file in Get-ChildItem -Path $SessionDir -File | Where-Object { $_.Extension -in @('.json','.txt','.log') }) {
        $content = Get-Content -Raw -ErrorAction SilentlyContinue $file.FullName
        if ($null -ne $content -and $content.Contains($token)) {
            throw "management token leaked into evidence file $($file.Name)"
        }
    }
}

try {
    switch ($Phase) {
        'Preflight' {
            if ($ObservedTrueNASVersion -ne $ExpectedTrueNASVersion) {
                throw "H0 requires observed TrueNAS $ExpectedTrueNASVersion; got '$ObservedTrueNASVersion'"
            }
            if ([string]::IsNullOrWhiteSpace($ObservedControlImage) -or -not $ObservedControlImage.Contains($ExpectedControlDigest)) {
                throw 'H0 control image does not match the qualified immutable digest'
            }
            if ([string]::IsNullOrWhiteSpace($ObservedCupsImage) -or -not $ObservedCupsImage.Contains($ExpectedCupsDigest)) {
                throw 'H0 CUPS image does not match the qualified immutable digest'
            }

            $live = Read-LiveState
            Assert-PublicCandidate $live
            $repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..\..')).Path
            $go = Get-Command go -ErrorAction SilentlyContinue
            if ($null -eq $go) {
                throw 'Go is required for the H1 client-side observer; do not substitute a server-side mDNS probe'
            }

            $observerBin = Join-Path $SessionDir 'physical-hil-observer.exe'
            Push-Location $repoRoot
            try {
                & $go.Source build -trimpath -o $observerBin ./tools/physical-hil-observer
                if ($LASTEXITCODE -ne 0) { throw "unable to build physical HIL observer rc=$LASTEXITCODE" }
            }
            finally { Pop-Location }

            $identity = $live.printer.identity
            $discoveryPath = Join-Path $SessionDir 'discovery.json'
            $discoveryErr = Join-Path $SessionDir 'discovery.stderr.log'
            $out = & $observerBin -uuid $identity.printer_uuid -expected-host $identity.host -expected-ipp-port ([int]$identity.port) -resource-path $identity.resource_path -seconds 30 2> $discoveryErr
            $observerRC = $LASTEXITCODE
            ($out -join [Environment]::NewLine) | Set-Content -Encoding utf8 $discoveryPath
            Remove-Item -Force $observerBin -ErrorAction SilentlyContinue
            if ($observerRC -ne 0) {
                throw "H1 DNS-SD observer failed; evidence retained in $discoveryPath and $discoveryErr"
            }
            $discovery = Get-Content -Raw $discoveryPath | ConvertFrom-Json
            if ($discovery.status -ne 'success') { throw 'H1 observer did not return success' }

            $preflight = [ordered]@{
                schema = 'semper-supra.foliorelay-physical-hil/1'
                phase = 'H0-H1'
                captured_at_utc = [DateTime]::UtcNow.ToString('o')
                expected_candidate = [ordered]@{
                    truenas_version = $ExpectedTrueNASVersion
                    control_digest = $ExpectedControlDigest
                    cups_digest = $ExpectedCupsDigest
                }
                observed_server = [ordered]@{
                    truenas_version = $ObservedTrueNASVersion
                    control_image = $ObservedControlImage
                    cups_image = $ObservedCupsImage
                }
                client_os = Get-ClientOS
                identity = $live.printer.identity
                public_uri = $live.printer.public_uri
                profiles = $live.printer.profiles
                inbox_jobs = [int]$live.status.inbox_jobs
                baseline_job_ids = @($live.job_ids)
                discovery = $discovery
                health_http = $live.health_http
                ready_http = $live.ready_http
            }
            Write-JsonFile (Join-Path $SessionDir 'preflight.json') $preflight
            Assert-NoTokenLeak
            Write-Host "H0/H1 PASS. Evidence: $SessionDir"
        }

        'Windows' {
            $preflightPath = Join-Path $SessionDir 'preflight.json'
            if (-not (Test-Path $preflightPath)) { throw 'run Preflight first using the same SessionDir' }
            $preflight = Get-Content -Raw $preflightPath | ConvertFrom-Json
            $before = Read-LiveState
            Assert-PublicCandidate $before
            Assert-IdentityEqual $preflight.identity $before.printer.identity
            if ([int]$before.status.inbox_jobs -ne [int]$preflight.inbox_jobs) {
                throw 'Inbox changed since H0; stop HIL and investigate as a separate campaign'
            }

            if ([string]::IsNullOrWhiteSpace($QueueName)) {
                $QueueName = 'FolioRelay-HIL-' + [DateTime]::UtcNow.ToString('yyyyMMddHHmmss')
            }
            if (Get-Printer -Name $QueueName -ErrorAction SilentlyContinue) {
                throw "temporary HIL queue already exists: $QueueName"
            }

            $ippUrl = [string]$before.printer.public_uri
            $ippUrl = $ippUrl -replace '^ipp://','http://'
            $ippUrl = $ippUrl -replace '^ipps://','https://'
            $created = $false
            try {
                Add-Printer -Name $QueueName -IppURL $ippUrl
                $created = $true
                $printer = Get-Printer -Name $QueueName
                $driver = Get-PrinterDriver -Name $printer.DriverName
                $port = Get-PrinterPort -Name $printer.PortName
                if ($printer.DriverName -notmatch '(?i)IPP') {
                    throw "Windows did not bind a native IPP class driver: $($printer.DriverName)"
                }

                $marker = 'FolioRelay physical HIL Windows ' + [Guid]::NewGuid().ToString('D')
                $marker | Out-Printer -Name $QueueName
                $targetCount = [int]$before.status.inbox_jobs + 1
                $after = $null
                for ($i = 0; $i -lt 60; $i++) {
                    Start-Sleep -Seconds 2
                    $after = Read-LiveState
                    if ([int]$after.status.inbox_jobs -gt $targetCount) {
                        throw 'Windows submission created more than one durable Inbox job'
                    }
                    if ([int]$after.status.inbox_jobs -eq $targetCount) { break }
                }
                if ($null -eq $after -or [int]$after.status.inbox_jobs -ne $targetCount) {
                    throw 'Windows native IPP submission did not create exactly one durable Inbox job'
                }
                Assert-IdentityEqual $preflight.identity $after.printer.identity

                $baseline = @{}
                foreach ($id in @($preflight.baseline_job_ids)) { $baseline[[string]$id] = $true }
                $newJobs = @($after.recent_jobs | Where-Object { -not $baseline.ContainsKey([string]$_.job_id) })
                if ($newJobs.Count -ne 1) {
                    throw "expected exactly one new durable job after Windows print; observed $($newJobs.Count)"
                }

                $windows = [ordered]@{
                    schema = 'semper-supra.foliorelay-physical-hil/1'
                    phase = 'H2'
                    captured_at_utc = [DateTime]::UtcNow.ToString('o')
                    client_os = Get-ClientOS
                    queue = [ordered]@{
                        name = $printer.Name
                        driver_name = $printer.DriverName
                        port_name = $printer.PortName
                        printer_status = [string]$printer.PrinterStatus
                        ipp_url = $ippUrl
                    }
                    driver = [ordered]@{
                        name = $driver.Name
                        manufacturer = $driver.Manufacturer
                    }
                    port = [ordered]@{
                        name = $port.Name
                        description = $port.Description
                    }
                    inbox_before = [int]$before.status.inbox_jobs
                    inbox_after = [int]$after.status.inbox_jobs
                    new_jobs = $newJobs
                    identity = $after.printer.identity
                }
                Write-JsonFile (Join-Path $SessionDir 'windows.json') $windows
            }
            finally {
                if ($created) {
                    Get-Printer -Name $QueueName -ErrorAction SilentlyContinue | Remove-Printer -ErrorAction SilentlyContinue
                }
            }
            Assert-NoTokenLeak
            Write-Host 'H2 PASS. Perform H3 from the physical iPhone/iPad, then run Final with the same SessionDir.'
        }

        'Final' {
            if ([string]::IsNullOrWhiteSpace($AppleOSVersion)) {
                throw 'Final requires -AppleOSVersion after the physical AirPrint submission'
            }
            $preflightPath = Join-Path $SessionDir 'preflight.json'
            $windowsPath = Join-Path $SessionDir 'windows.json'
            if (-not (Test-Path $preflightPath) -or -not (Test-Path $windowsPath)) {
                throw 'Preflight and Windows phases must pass before Final'
            }
            $preflight = Get-Content -Raw $preflightPath | ConvertFrom-Json
            $windows = Get-Content -Raw $windowsPath | ConvertFrom-Json
            $finalLive = Read-LiveState
            Assert-PublicCandidate $finalLive
            Assert-IdentityEqual $preflight.identity $finalLive.printer.identity

            $expected = [int]$preflight.inbox_jobs + 2
            if ([int]$finalLive.status.inbox_jobs -ne $expected) {
                throw "H4 expected Inbox=$expected after exactly one Windows and one Apple job; observed $($finalLive.status.inbox_jobs)"
            }
            $baseline = @{}
            foreach ($id in @($preflight.baseline_job_ids)) { $baseline[[string]$id] = $true }
            $newJobs = @($finalLive.recent_jobs | Where-Object { -not $baseline.ContainsKey([string]$_.job_id) })
            if ($newJobs.Count -ne 2) {
                throw "H4 expected exactly two new durable jobs; observed $($newJobs.Count)"
            }
            $windowsIds = @($windows.new_jobs | ForEach-Object { [string]$_.job_id })
            $newIds = @($newJobs | ForEach-Object { [string]$_.job_id })
            if ($windowsIds.Count -ne 1 -or $windowsIds[0] -notin $newIds) {
                throw 'H4 cannot correlate the Windows durable job into the final two-job set'
            }

            $final = [ordered]@{
                schema = 'semper-supra.foliorelay-physical-hil/1'
                phase = 'H3-H4'
                captured_at_utc = [DateTime]::UtcNow.ToString('o')
                apple_client = [ordered]@{
                    type = $AppleClientType
                    os_version = $AppleOSVersion
                    operator_assertion = 'FolioRelay was selected from the native AirPrint picker and one print was submitted successfully.'
                }
                identity = $finalLive.printer.identity
                public_uri = $finalLive.printer.public_uri
                inbox_before = [int]$preflight.inbox_jobs
                inbox_after = [int]$finalLive.status.inbox_jobs
                new_jobs = $newJobs
                health_http = $finalLive.health_http
                ready_http = $finalLive.ready_http
            }
            Write-JsonFile (Join-Path $SessionDir 'final.json') $final

            $receipt = [ordered]@{
                schema = 'semper-supra.foliorelay-physical-hil/1'
                status = 'PASS'
                completed_at_utc = [DateTime]::UtcNow.ToString('o')
                candidate = $preflight.expected_candidate
                observed_server = $preflight.observed_server
                printer_uuid = $preflight.identity.printer_uuid
                public_uri = $preflight.public_uri
                discovery = $preflight.discovery
                windows = [ordered]@{
                    os = $windows.client_os
                    driver = $windows.queue.driver_name
                    queue = $windows.queue.name
                    durable_jobs = $windows.new_jobs
                }
                apple = $final.apple_client
                inbox_before = [int]$preflight.inbox_jobs
                inbox_after = [int]$finalLive.status.inbox_jobs
                inbox_delta = [int]$finalLive.status.inbox_jobs - [int]$preflight.inbox_jobs
                new_jobs = $newJobs
                identity_unchanged = $true
                services_healthy = $true
                server_mutation_performed = $false
            }
            Write-JsonFile (Join-Path $SessionDir 'receipt.json') $receipt
            Assert-NoTokenLeak
            Write-Host "H0-H4 PASS. Sanitized receipt: $(Join-Path $SessionDir 'receipt.json')"
        }
    }
}
finally {
    if ($null -ne $client) { $client.Dispose() }
    if ($null -ne $handler) { $handler.Dispose() }
    $token = $null
    $secureToken = $null
}
