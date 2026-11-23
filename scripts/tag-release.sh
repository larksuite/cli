#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${REPO_ROOT}"

VERSION=$(node -p "require('./package.json').version")
TAG="v${VERSION}"

node "${SCRIPT_DIR}/release-preflight.js" --tag "${TAG}"

echo "Version: ${VERSION}"
echo "Tag: ${TAG}"

CURRENT_BRANCH=$(git branch --show-current)
if [ "${CURRENT_BRANCH}" != "main" ]; then
  echo "Error: releases must be tagged from main; current branch is '${CURRENT_BRANCH}'." >&2
  exit 1
fi

if ! git diff --quiet HEAD -- package.json package-lock.json; then
  echo "Error: package.json or package-lock.json has uncommitted changes. Please commit them before tagging." >&2
  exit 1
fi

git fetch origin main

HEAD_SHA=$(git rev-parse HEAD)
FETCHED_MAIN_SHA=$(git rev-parse "FETCH_HEAD^{commit}")
if [ "${HEAD_SHA}" != "${FETCHED_MAIN_SHA}" ]; then
  echo "Error: HEAD must exactly match origin/main before tagging." >&2
  exit 1
fi

if git rev-parse -q --verify "refs/tags/${TAG}" >/dev/null; then
  echo "Error: local tag ${TAG} already exists." >&2
  exit 1
fi

REMOTE_TAG=$(git ls-remote --tags origin "refs/tags/${TAG}")
if [ -n "${REMOTE_TAG}" ]; then
  echo "Error: remote tag ${TAG} already exists." >&2
  exit 1
fi

git tag "${TAG}" "${HEAD_SHA}"
git push origin "refs/tags/${TAG}"

echo "Successfully pushed tag ${TAG}"
