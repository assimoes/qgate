#!/usr/bin/env bash
# Captures real analyzer, linter, coverage and test-runner output for the
# Seam C golden tests. Each tool runs in a pinned Docker image against the
# sample project in testdata/capture/ and writes straight into the
# ingestors' testdata/ directories. Then run:
#
#   go test ./internal/ingest/... -update
#
# and review the golden diffs. Needs Docker, gh (to download CodeQL) and
# network access. CodeQL runs as linux/amd64, emulated on arm64 hosts.
set -euo pipefail
cd "$(dirname "$0")/.."
root=$PWD
sample=$root/testdata/capture
out=$root/internal/ingest

GO_IMAGE=golang:1.26
PYTHON_IMAGE=python:3.13-slim
NODE_IMAGE=node:24-slim
GOLANGCI_IMAGE=golangci/golangci-lint:v2.13.2
SEMGREP_IMAGE=semgrep/semgrep:1.95.0
TRIVY_IMAGE=aquasec/trivy:0.56.2
CODEQL_BUNDLE=codeql-bundle-v2.27.2

step() { printf '\n== %s\n' "$1"; }

mkdir -p "$out"/{sarif,checkstyle,cobertura,lcov,junit}/testdata

step "Go coverage (gocover-cobertura) and test results (go-junit-report)"
docker run --rm -v "$sample/go":/src -v "$out":/out -w /src "$GO_IMAGE" sh -c '
  set -e
  go install github.com/boumenot/gocover-cobertura@v1.5.0
  go install github.com/jstemmer/go-junit-report/v2@v2.1.0
  go test -v -coverprofile=/tmp/cover.out ./... > /tmp/test.txt 2>&1 || true
  gocover-cobertura < /tmp/cover.out > /out/cobertura/testdata/gocover.xml
  go-junit-report -in /tmp/test.txt > /out/junit/testdata/gojunitreport.xml'

step "Python coverage (coverage.py) and test results (pytest)"
docker run --rm -v "$sample/py":/src -v "$out":/out -w /src "$PYTHON_IMAGE" sh -c '
  set -e
  pip install --quiet --disable-pip-version-check --root-user-action=ignore pytest==8.3.3 coverage==7.6.1
  coverage run --branch -m pytest --junitxml=/out/junit/testdata/pytest.xml -q > /dev/null 2>&1 || true
  coverage xml -o /out/cobertura/testdata/coveragepy.xml > /dev/null'

step "JavaScript lint (ESLint checkstyle) and coverage (c8 lcov)"
docker run --rm -v "$sample/js":/src:ro -v "$out":/out "$NODE_IMAGE" sh -c '
  set -e
  cp -r /src /work && cd /work
  npm init -y > /dev/null
  npm install --silent --no-audit --no-fund eslint@9.13.0 eslint-formatter-checkstyle@8.40.0 c8@10.1.2 > /dev/null
  npx eslint -f checkstyle . > /out/checkstyle/testdata/eslint.xml || true
  npx c8 --reporter=lcovonly --report-dir=/tmp/cov node --test > /dev/null 2>&1 || true
  cp /tmp/cov/lcov.info /out/lcov/testdata/c8.info'

step "Go lint (golangci-lint checkstyle)"
docker run --rm -v "$sample/go":/src -v "$out":/out -w /src "$GOLANGCI_IMAGE" sh -c '
  golangci-lint run --output.checkstyle.path=/out/checkstyle/testdata/golangci.xml ./... > /dev/null || true'

step "Semgrep (SARIF)"
docker run --rm -v "$sample":/src -v "$out":/out -w /src "$SEMGREP_IMAGE" \
  semgrep scan --metrics=off --quiet --config semgrep-rules.yml --sarif --output /out/sarif/testdata/semgrep.sarif go js

step "Trivy (SARIF)"
docker run --rm -v "$sample/go":/src:ro -v "$out":/out -v qgate-trivy-cache:/root/.cache "$TRIVY_IMAGE" \
  fs --quiet --scanners vuln --format sarif --output /out/sarif/testdata/trivy.sarif /src

step "CodeQL (SARIF)"
bundle_dir=${TMPDIR:-/tmp}/qgate-codeql
mkdir -p "$bundle_dir"
if [ ! -f "$bundle_dir/$CODEQL_BUNDLE-go.tar.zst" ]; then
  gh release download "$CODEQL_BUNDLE" --repo github/codeql-action --pattern codeql-bundle-go-linux64.tar.zst --dir "$bundle_dir"
  mv "$bundle_dir/codeql-bundle-go-linux64.tar.zst" "$bundle_dir/$CODEQL_BUNDLE-go.tar.zst"
fi
docker run --rm --platform linux/amd64 -v "$sample/go":/src:ro -v "$out":/out -v "$bundle_dir":/bundle \
  -e BUNDLE="$CODEQL_BUNDLE-go.tar.zst" "$GO_IMAGE" sh -c '
  set -e
  apt-get update -qq > /dev/null && apt-get install -y -qq zstd > /dev/null
  tar --zstd -xf "/bundle/$BUNDLE" -C /opt
  cp -r /src /work
  /opt/codeql/codeql database create /tmp/db --language=go --source-root=/work --quiet > /dev/null
  /opt/codeql/codeql database analyze /tmp/db codeql/go-queries:codeql-suites/go-security-extended.qls \
    --format=sarif-latest --output=/out/sarif/testdata/codeql.sarif --quiet > /dev/null'

printf '\ncaptured; now run: go test ./internal/ingest/... -update\n'
