#!/usr/bin/env bash
# =============================================================================
# GitHub Wiki Publisher for thunderfinder/flashsystem-collector
# =============================================================================
# Automates syncing and publishing the documentation in docs/wiki/
# directly to the root of the GitHub Wiki git repository:
# https://github.com/thunderfinder/flashsystem-collector.wiki.git
# =============================================================================
set -euo pipefail

WIKI_REPO_URL="https://github.com/thunderfinder/flashsystem-collector.wiki.git"
WORK_DIR="/tmp/flashsystem-collector.wiki"
SOURCE_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../docs/wiki" && pwd)"

echo "=== IBM FlashSystem Collector - GitHub Wiki Publisher ==="
echo "Source directory: ${SOURCE_DIR}"
echo "Wiki Git URL:     ${WIKI_REPO_URL}"

if [[ ! -d "${SOURCE_DIR}" ]]; then
    echo "ERROR: Source directory '${SOURCE_DIR}' not found!" >&2
    exit 1
fi

rm -rf "${WORK_DIR}"
echo "Cloning Wiki repository..."
git clone "${WIKI_REPO_URL}" "${WORK_DIR}"

echo "Syncing Markdown pages to Wiki root..."
cp -v "${SOURCE_DIR}"/*.md "${WORK_DIR}/"

cd "${WORK_DIR}"
git add .
if git diff-index --quiet HEAD --; then
    echo "Wiki is already up to date. Nothing to commit."
else
    git commit -m "docs: publish complete GitHub Wiki architecture"
    echo "Pushing changes to GitHub Wiki..."
    git push origin master
    echo "SUCCESS: GitHub Wiki successfully published!"
    echo "Visit: https://github.com/thunderfinder/flashsystem-collector/wiki"
fi
