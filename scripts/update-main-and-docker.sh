#!/usr/bin/env bash

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"

TARGET_BRANCH="${TARGET_BRANCH:-main}"
LOCAL_IMAGE="${CLI_PROXY_IMAGE:-cli-proxy-api:local}"
MANAGEMENT_SECRET_DEFAULT="${MANAGEMENT_SECRET:-local-admin-20260420}"
FORCE_MANAGEMENT_SECRET="${FORCE_MANAGEMENT_SECRET:-false}"
MANAGEMENT_URL="${MANAGEMENT_URL:-http://127.0.0.1:8317}"
ALLOW_DIRTY_WORKTREE="${ALLOW_DIRTY_WORKTREE:-false}"

cd "${REPO_ROOT}"

ACTIVE_MANAGEMENT_SECRET=""
MANAGEMENT_SECRET_WAS_SET_BY_SCRIPT="false"

log() {
  printf '[update] %s\n' "$*"
}

ensure_clean_worktree() {
  if [[ "${ALLOW_DIRTY_WORKTREE}" == "true" ]]; then
    log "已启用 ALLOW_DIRTY_WORKTREE=true，跳过工作区干净检查"
    return
  fi

  if ! git diff --quiet || ! git diff --cached --quiet; then
    log "检测到未提交改动，请先提交或暂存后再执行。"
    exit 1
  fi
}

ensure_target_branch() {
  local current_branch
  current_branch="$(git rev-parse --abbrev-ref HEAD)"

  if [[ "${current_branch}" != "${TARGET_BRANCH}" ]]; then
    log "切换到分支 ${TARGET_BRANCH}"
    git checkout "${TARGET_BRANCH}"
  fi
}

ensure_config_file() {
  if [[ -d config.yaml ]]; then
    local backup_dir
    backup_dir="config.yaml.bak-$(date '+%Y%m%d%H%M%S')"
    log "发现 config.yaml 是目录，备份为 ${backup_dir}"
    mv config.yaml "${backup_dir}"
  fi

  if [[ ! -f config.yaml ]]; then
    log "创建默认配置文件 config.yaml"
    cp config.example.yaml config.yaml
  fi
}

ensure_management_config() {
  local current_secret

  log "确保管理端配置可用"

  perl -0pi -e 's/^(\s*allow-remote:\s*).*$/${1}true/m' config.yaml

  current_secret="$(sed -n 's/^[[:space:]]*secret-key:[[:space:]]*"\(.*\)"[[:space:]]*$/\1/p' config.yaml | head -n 1)"

  if [[ "${FORCE_MANAGEMENT_SECRET}" == "true" ]]; then
    perl -0pi -e 's/^(\s*secret-key:\s*).*$/${1}"'"${MANAGEMENT_SECRET_DEFAULT}"'"/m' config.yaml
    ACTIVE_MANAGEMENT_SECRET="${MANAGEMENT_SECRET_DEFAULT}"
    MANAGEMENT_SECRET_WAS_SET_BY_SCRIPT="true"
    log "已强制重置管理密码"
    return
  fi

  if [[ -z "${current_secret}" ]]; then
    perl -0pi -e 's/^(\s*secret-key:\s*).*$/${1}"'"${MANAGEMENT_SECRET_DEFAULT}"'"/m' config.yaml
    ACTIVE_MANAGEMENT_SECRET="${MANAGEMENT_SECRET_DEFAULT}"
    MANAGEMENT_SECRET_WAS_SET_BY_SCRIPT="true"
    log "检测到管理密码为空，已写入默认管理密码"
    return
  fi

  ACTIVE_MANAGEMENT_SECRET="${current_secret}"

  if [[ "${current_secret}" == '$2a$'* || "${current_secret}" == '$2b$'* || "${current_secret}" == '$2y$'* ]]; then
    log "检测到已存在哈希后的管理密码，脚本保持原值不覆盖"
  else
    log "检测到已存在明文管理密码，脚本保持原值不覆盖"
  fi
}

sync_origin_main() {
  log "拉取 origin/${TARGET_BRANCH} 最新代码"
  git fetch origin

  if git merge-base --is-ancestor "origin/${TARGET_BRANCH}" HEAD; then
    log "本地已经包含 origin/${TARGET_BRANCH} 的最新提交"
    return
  fi

  if git merge-base --is-ancestor HEAD "origin/${TARGET_BRANCH}"; then
    log "执行 fast-forward 更新"
    git merge --ff-only "origin/${TARGET_BRANCH}"
    return
  fi

  log "检测到本地与 origin/${TARGET_BRANCH} 已分叉，执行自动 merge"
  git merge --no-edit "origin/${TARGET_BRANCH}"
}

rebuild_and_restart() {
  local version commit build_date

  version="$(git describe --tags --always --dirty)"
  commit="$(git rev-parse --short HEAD)"
  build_date="$(date -u '+%Y-%m-%dT%H:%M:%SZ')"

  export VERSION="${version}"
  export COMMIT="${commit}"
  export BUILD_DATE="${build_date}"
  export CLI_PROXY_IMAGE="${LOCAL_IMAGE}"

  log "开始重建 Docker 镜像"
  log "VERSION=${VERSION}"
  log "COMMIT=${COMMIT}"
  log "BUILD_DATE=${BUILD_DATE}"
  log "CLI_PROXY_IMAGE=${CLI_PROXY_IMAGE}"

  docker compose build
  docker compose up -d --force-recreate --remove-orphans --pull never
}

verify_service() {
  local status

  status="$(docker inspect cli-proxy-api --format '{{.State.Status}}')"
  log "当前容器状态: ${status}"

  if [[ "${status}" != "running" ]]; then
    log "容器未正常运行，输出最近日志帮助排查"
    docker compose logs --tail=120 cli-proxy-api
    exit 1
  fi

  docker compose ps
}

print_summary() {
  log "完成：代码已同步，Docker 已更新到当前本地代码版本"
  log "管理地址: ${MANAGEMENT_URL}"

  if [[ "${MANAGEMENT_SECRET_WAS_SET_BY_SCRIPT}" == "true" ]]; then
    log "管理密码: ${ACTIVE_MANAGEMENT_SECRET}"
  elif [[ "${ACTIVE_MANAGEMENT_SECRET}" == '$2a$'* || "${ACTIVE_MANAGEMENT_SECRET}" == '$2b$'* || "${ACTIVE_MANAGEMENT_SECRET}" == '$2y$'* ]]; then
    log "管理密码: 保留了原有哈希值，无法直接回显；如需重置，使用 FORCE_MANAGEMENT_SECRET=true MANAGEMENT_SECRET=你的密码"
  else
    log "管理密码: ${ACTIVE_MANAGEMENT_SECRET}"
  fi

  log "后续如需重置管理密码，可执行：FORCE_MANAGEMENT_SECRET=true MANAGEMENT_SECRET=你的密码 ./scripts/update-main-and-docker.sh"
}

main() {
  ensure_clean_worktree
  ensure_target_branch
  ensure_config_file
  ensure_management_config
  sync_origin_main
  rebuild_and_restart
  verify_service
  print_summary
}

main "$@"
