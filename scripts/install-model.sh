#!/usr/bin/env bash
# 从公开 GitHub Release 下载并校验模型，或导入已有 ZIP。只写用户指定的推荐实例，
# 不直接改运行中的 Registry，激活始终经过服务端现有 Bundle 校验。
set -euo pipefail

version=latest
repository="${MODEL_RELEASE_REPOSITORY:-starcat-app/starcat-recsys-trainer}"
base_url="${RECOMMEND_BASE_URL:-http://127.0.0.1:5005}"
archive=""
checksum=""
while [[ $# -gt 0 ]]; do
  case "$1" in
    --version) version="${2:?missing version}"; shift 2 ;;
    --file) archive="${2:?missing file}"; shift 2 ;;
    --sha256) checksum="${2:?missing sha256}"; shift 2 ;;
    --server) base_url="${2:?missing server}"; shift 2 ;;
    *) echo "Usage: bash scripts/install-model.sh [--version MODEL|latest] [--file ZIP --sha256 SHA256] [--server URL]" >&2; exit 2 ;;
  esac
done
# 版本同时用于 URL 和临时文件名，必须在下载前确定是稳定标识。
valid_version() {
  [[ "$1" =~ ^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$ && "$1" != *..* && "$1" != latest ]]
}
[[ "${version}" == latest ]] || valid_version "${version}" || {
  echo "Invalid model version." >&2; exit 2;
}
: "${MODEL_PUBLISH_KEY:?Set MODEL_PUBLISH_KEY to one server MODEL_PUBLISH_KEYS value}"
command -v curl >/dev/null
command -v jq >/dev/null
temporary="$(mktemp -d)"
trap 'rm -rf "${temporary}"' EXIT

if [[ -z "${archive}" ]]; then
  [[ "${repository}" =~ ^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$ ]] || exit 2
  release_path="tags/model-${version}"
  if [[ "${version}" == latest ]]; then release_path=latest; fi
  curl --fail --silent --show-error \
    "https://api.github.com/repos/${repository}/releases/${release_path}" >"${temporary}/release.json"
  version="$(jq -er '.tag_name | select(startswith("model-")) | ltrimstr("model-")' "${temporary}/release.json")"
  valid_version "${version}" || { echo "Invalid model release tag." >&2; exit 2; }
  asset="${version}.zip"
  archive="${temporary}/${asset}"
  archive_url="$(jq -er --arg name "${asset}" '.assets[] | select(.name == $name) | .browser_download_url' "${temporary}/release.json")"
  sums_url="$(jq -er '.assets[] | select(.name == "SHA256SUMS") | .browser_download_url' "${temporary}/release.json")"
  curl --fail --location --silent --show-error "${sums_url}" >"${temporary}/SHA256SUMS"
  checksum="$(awk -v name="${asset}" '$2 == name {print $1}' "${temporary}/SHA256SUMS")"
  curl --fail --location --silent --show-error "${archive_url}" >"${archive}"
fi
valid_version "${version}" || {
  echo "Use the exact model version for a local ZIP." >&2; exit 2;
}
[[ "${checksum}" =~ ^[0-9a-f]{64}$ ]] || { echo "A SHA-256 checksum is required." >&2; exit 2; }
if command -v sha256sum >/dev/null; then
  actual="$(sha256sum "${archive}" | awk '{print $1}')"
else
  actual="$(shasum -a 256 "${archive}" | awk '{print $1}')"
fi
[[ "${actual}" == "${checksum}" ]] || { echo "Model checksum mismatch." >&2; exit 1; }
# 用权限受限的 header 文件避免发布凭据进入 curl 的进程参数。
umask 077
printf 'Authorization: Bearer %s\n' "${MODEL_PUBLISH_KEY}" >"${temporary}/headers"
printf 'Content-Type: application/zip\nX-SC-Svc: recommend\n' >>"${temporary}/headers"
curl --fail --silent --show-error \
  --header @"${temporary}/headers" --data-binary @"${archive}" \
  "${base_url%/}/internal/v1/model-bundles/${version}?activate=true" >"${temporary}/receipt.json"
jq -e --arg version "${version}" '.data.model_version == $version and .data.active == true' \
  "${temporary}/receipt.json" >/dev/null
curl --fail --silent --show-error --header @"${temporary}/headers" \
  "${base_url%/}/internal/v1/model-bundles/active" >"${temporary}/active.json"
jq -e --arg version "${version}" '.data.model_version == $version' "${temporary}/active.json" >/dev/null
printf 'Activated model: %s\n' "${version}"
