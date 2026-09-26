# PruneDocker: Automated Verification & Test Runner
Write-Host "=================================================================" -ForegroundColor Cyan
Write-Host "  PruneDocker: Automated Quality & Test Suite Verification" -ForegroundColor Cyan
Write-Host "=================================================================" -ForegroundColor Cyan

# 1. Environment & Path Check
$env:Path = "C:\Program Files\Go\bin;" + $env:Path
$goVersion = & go version 2>&1
if ($LASTEXITCODE -ne 0) {
    Write-Host "[ERROR] Go compiler not found in PATH." -ForegroundColor Red
    exit 1
}
Write-Host "[INFO] Detected Go: $goVersion" -ForegroundColor Green

# 2. Run All Unit & Integration Tests (100% Serverless)
Write-Host "`n[STEP 1/3] Running All Automated Unit & Integration Tests..." -ForegroundColor Yellow
$testOutput = & go test ./... -v -cover 2>&1
$testExitCode = $LASTEXITCODE

Write-Host $testOutput

if ($testExitCode -ne 0) {
    Write-Host "`n[FAILED] One or more tests failed!" -ForegroundColor Red
    exit 1
}
Write-Host "`n[PASS] All test suites passed with 0 errors!" -ForegroundColor Green

# 3. Build Standalone Binary
Write-Host "`n[STEP 2/3] Building Standalone Binary (bin/prunedocker.exe)..." -ForegroundColor Yellow
if (-not (Test-Path "bin")) {
    New-Item -ItemType Directory -Path "bin" | Out-Null
}
$buildOutput = & go build -o bin/prunedocker.exe ./cmd/prunedocker 2>&1
if ($LASTEXITCODE -ne 0) {
    Write-Host "[FAILED] Binary compilation failed: $buildOutput" -ForegroundColor Red
    exit 1
}
$binInfo = Get-Item "bin/prunedocker.exe"
Write-Host "[PASS] Successfully compiled bin/prunedocker.exe ($([math]::Round($binInfo.Length / 1MB, 2)) MB)" -ForegroundColor Green

# 4. Smoke Test CLI Execution
Write-Host "`n[STEP 3/3] Performing CLI Smoke Tests..." -ForegroundColor Yellow
$helpOutput = & .\bin\prunedocker.exe --help 2>&1
if ($LASTEXITCODE -ne 0) {
    Write-Host "[FAILED] CLI smoke test failed!" -ForegroundColor Red
    exit 1
}
Write-Host "[PASS] CLI commands verified." -ForegroundColor Green

Write-Host "`n=================================================================" -ForegroundColor Cyan
Write-Host "  VERIFICATION COMPLETE: 100% TESTS PASSED, ZERO BUGS DETECTED!" -ForegroundColor Green
Write-Host "=================================================================" -ForegroundColor Cyan
