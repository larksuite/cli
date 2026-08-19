#!/usr/bin/env bash
# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT
#
# Launch Claude Code in this checkout with project-local Lark skills and a dev
# lark-cli shim. The global ~/.claude/skills install is left untouched.

set -euo pipefail

usage() {
	cat <<'USAGE'
Usage:
  env/claude-dev-lark.sh [options] [--] [claude args or initial prompt]

Options:
  --lane <lane>       Lane injected through LARK_LANE.
                      Default: boe_bitable_bk11
  --env <env>         larkenv target: boe, pre, ppe, or online.
                      Default: boe
  --ppe, --use-ppe    Use PPE: pre endpoint plus x-use-ppe:1 and env:pre_release headers.
  --skill <name>      Link only one local skill, e.g. lark-base.
                      Default: all lark-* skills under ./skills
  --no-build          Reuse the current ./lark-cli binary instead of rebuilding.
  -h, --help          Show this help.

Examples:
  env/claude-dev-lark.sh
  env/claude-dev-lark.sh --lane boe_larkcli_baseapp --use-ppe
  env/claude-dev-lark.sh --skill lark-base
  env/claude-dev-lark.sh -- "用 Base Skill 查一下这个 workspace"
USAGE
}

die() {
	echo "error: $*" >&2
	exit 1
}

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$script_dir/.." && pwd)"

lane="${LARK_LANE:-boe_bitable_bk11}"
target_env="${LARKENV_TARGET:-boe}"
target_env_explicit=0
skill_filter="all"
do_build=1
use_ppe=0
claude_args=()
claude_arg_count=0

while [ $# -gt 0 ]; do
	case "$1" in
	--lane)
		[ $# -ge 2 ] || die "--lane requires a value"
		lane="$2"
		shift 2
		;;
	--env)
		[ $# -ge 2 ] || die "--env requires a value"
		target_env="$2"
		target_env_explicit=1
		shift 2
		;;
	--ppe | --use-ppe)
		use_ppe=1
		shift
		;;
	--skill)
		[ $# -ge 2 ] || die "--skill requires a value"
		skill_filter="$2"
		shift 2
		;;
	--no-build)
		do_build=0
		shift
		;;
	-h | --help)
		usage
		exit 0
		;;
	--)
		shift
		claude_args=("$@")
		claude_arg_count=$#
		break
		;;
	*)
		claude_args+=("$1")
		claude_arg_count=$((claude_arg_count + 1))
		shift
		;;
	esac
done

if [ "$use_ppe" -eq 1 ] && [ "$target_env_explicit" -eq 0 ]; then
	target_env="ppe"
fi
if [ "$use_ppe" -eq 1 ]; then
	case "$target_env" in
	ppe) ;;
	*) die "--use-ppe uses the pre endpoint; remove --env $target_env or pass --env ppe" ;;
	esac
fi

case "$target_env" in
boe | pre | ppe | online) ;;
*) die "--env must be one of: boe, pre, ppe, online" ;;
esac

command -v claude >/dev/null 2>&1 || die "claude not found in PATH"
[ -x "$repo_root/env/larkenv" ] || die "missing executable env/larkenv"

append_extra_header() {
	local header="$1"
	case "; ${LARKSUITE_CLI_EXTRA_HEADERS:-};" in
	*"; $header;"*) return ;;
	esac
	if [ -n "${LARKSUITE_CLI_EXTRA_HEADERS:-}" ]; then
		export LARKSUITE_CLI_EXTRA_HEADERS="${LARKSUITE_CLI_EXTRA_HEADERS}; $header"
	else
		export LARKSUITE_CLI_EXTRA_HEADERS="$header"
	fi
}

# launch_claude starts Claude Code with the caller's environment inherited as
# is, including any proxy variables already exported by the shell. Network setup
# is left to the caller so this script stays portable.
#
# If your proxy is configured through a shell function (rather than exported
# variables), invoke this script through it, e.g. `my_proxy_wrapper
# env/claude-dev-lark.sh`. A bash script cannot exec a shell function.
#
# lark-cli traffic is unaffected either way for boe: larkenv boe sets
# LARK_CLI_NO_PROXY=1 and dials the internal endpoint directly. For pre/ppe
# larkenv does not set it, so if you run behind a proxy make sure feishu-pre.cn
# is in your own no_proxy — ".feishu.cn" does not cover "feishu-pre.cn".
launch_claude() {
	exec claude --allow-dangerously-skip-permissions "$@"
}

bin_dir="$repo_root/.claude-dev/bin"
skills_dir="$repo_root/.claude/skills"
mkdir -p "$bin_dir" "$skills_dir"

cd "$repo_root"

if [ "$do_build" -eq 1 ]; then
	echo "==> Building dev lark-cli from $repo_root" >&2
	./build.sh
else
	echo "==> Reusing existing ./lark-cli" >&2
fi

[ -x "$repo_root/lark-cli" ] || die "missing ./lark-cli; run without --no-build first"

cp "$repo_root/lark-cli" "$bin_dir/lark-cli-env"
cp "$repo_root/env/larkenv" "$bin_dir/larkenv"
chmod +x "$bin_dir/lark-cli-env" "$bin_dir/larkenv"

link_skill() {
	local name="$1"
	local src="$repo_root/skills/$name"
	local dst="$skills_dir/$name"
	local rel="../../skills/$name"
	local current backup

	[ -f "$src/SKILL.md" ] || die "missing skill: skills/$name/SKILL.md"

	current="$(readlink "$dst" 2>/dev/null || true)"
	if [ "$current" = "$rel" ]; then
		return
	fi

	if [ -L "$dst" ]; then
		backup="$dst.bak.$(date +%Y%m%d%H%M%S)"
		mv "$dst" "$backup"
		echo "==> Backed up existing local skill symlink: $backup" >&2
	elif [ -e "$dst" ]; then
		backup="$dst.bak.$(date +%Y%m%d%H%M%S)"
		mv "$dst" "$backup"
		echo "==> Backed up existing local skill directory: $backup" >&2
	fi

	ln -s "$rel" "$dst"
}

if [ "$skill_filter" = "all" ]; then
	found=0
	for skill_path in "$repo_root"/skills/lark-*; do
		[ -d "$skill_path" ] || continue
		link_skill "${skill_path##*/}"
		found=1
	done
	[ "$found" -eq 1 ] || die "no local lark-* skills found under ./skills"
else
	link_skill "$skill_filter"
fi

> "$bin_dir/lark-cli" echo '#!/usr/bin/env bash'

# Only PPE needs the extra headers; every other target keeps the plain shim so
# the non-PPE path stays byte-identical to what it was before PPE support.
if [ "$use_ppe" -eq 1 ]; then
	cat >>"$bin_dir/lark-cli" <<'PPE_SHIM'

for h in "x-use-ppe:1" "env:pre_release"; do
  case "; ${LARKSUITE_CLI_EXTRA_HEADERS:-};" in
    *"; $h;"*) ;;
    *)
      if [ -n "${LARKSUITE_CLI_EXTRA_HEADERS:-}" ]; then
        export LARKSUITE_CLI_EXTRA_HEADERS="${LARKSUITE_CLI_EXTRA_HEADERS}; $h"
      else
        export LARKSUITE_CLI_EXTRA_HEADERS="$h"
      fi
      ;;
  esac
done
PPE_SHIM
fi

cat >>"$bin_dir/lark-cli" <<SHIM
exec env \\
  LARK_CLI_ENV_BIN="$bin_dir" \\
  LARK_LANE="\${LARK_LANE:-$lane}" \\
  LARKSUITE_CLI_NO_UPDATE_NOTIFIER="\${LARKSUITE_CLI_NO_UPDATE_NOTIFIER:-1}" \\
  LARKSUITE_CLI_NO_SKILLS_NOTIFIER="\${LARKSUITE_CLI_NO_SKILLS_NOTIFIER:-1}" \\
  "$bin_dir/larkenv" "$target_env" "\$@"
SHIM
chmod +x "$bin_dir/lark-cli"

echo "==> Starting Claude Code with dev lark-cli and project-local skills" >&2
echo "    PATH prefix: $bin_dir" >&2
echo "    Skill root:   $skills_dir" >&2
echo "    lark-cli ->   LARK_LANE=$lane larkenv $target_env" >&2
if [ "$use_ppe" -eq 1 ] || [ "$target_env" = "ppe" ]; then
	echo "    extra headers: x-use-ppe:1; env:pre_release" >&2
	echo "    identity:      reuses the online app config and user login state" >&2
fi

export PATH="$bin_dir:$PATH"
export LARK_CLI_ENV_BIN="$bin_dir"
export LARK_LANE="$lane"
if [ "$use_ppe" -eq 1 ]; then
	append_extra_header "x-use-ppe:1"
	append_extra_header "env:pre_release"
fi
export LARKSUITE_CLI_NO_UPDATE_NOTIFIER="${LARKSUITE_CLI_NO_UPDATE_NOTIFIER:-1}"
export LARKSUITE_CLI_NO_SKILLS_NOTIFIER="${LARKSUITE_CLI_NO_SKILLS_NOTIFIER:-1}"

# Claude has no "-C <dir>" flag like Codex; it uses the current working
# directory, which is $repo_root here (see the cd above).
if [ "$claude_arg_count" -eq 0 ]; then
	launch_claude
fi

launch_claude "${claude_args[@]}"
