#!/usr/bin/env bash
# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT
#
# Launch Codex in this checkout with project-local Lark skills and a dev
# lark-cli shim. The global ~/.agents/skills install is left untouched.

set -euo pipefail

usage() {
	cat <<'USAGE'
Usage:
  env/codex-dev-lark.sh [options] [--] [codex args or initial prompt]

Options:
  --lane <lane>       Lane injected through LARK_LANE; ignored by --use-pre.
                      Default: boe_bitable_bk11
  --env <env>         larkenv target: boe, pre, ppe, use-pre, or online.
                      Default: boe
  --ppe, --use-ppe    Use PPE for business APIs; auth/config remain on production.
  --use-pre           Use pre for business APIs without x-use-ppe or X-TT-ENV;
                      auth/config remain on production.
  --skill <name>      Link only one local skill, e.g. lark-base.
                      Default: all lark-* skills under ./skills
  --cx                Launch Codex with cx-style permissions:
                      codex --dangerously-bypass-approvals-and-sandbox
                      Adds --profile proxy when CODEX_PROXY_API_KEY is exported.
  --no-cx             Disable --cx when CODEX_DEV_LARK_CX=1 is set.
  --no-build          Reuse the current ./lark-cli binary instead of rebuilding.
  -h, --help          Show this help.

Examples:
  env/codex-dev-lark.sh
  CODEX_DEV_LARK_CX=1 env/codex-dev-lark.sh
  env/codex-dev-lark.sh --cx
  env/codex-dev-lark.sh --lane boe_larkcli_baseapp --use-ppe
  env/codex-dev-lark.sh --use-pre
  env/codex-dev-lark.sh --skill lark-base
  env/codex-dev-lark.sh -- "用 Base Skill 查一下这个 workspace"
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
use_pre=0
use_cx="${CODEX_DEV_LARK_CX:-0}"
codex_args=()

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
	--skill)
		[ $# -ge 2 ] || die "--skill requires a value"
		skill_filter="$2"
		shift 2
		;;
	--ppe | --use-ppe)
		use_ppe=1
		shift
		;;
	--use-pre)
		use_pre=1
		shift
		;;
	--cx)
		use_cx=1
		shift
		;;
	--no-cx)
		use_cx=0
		shift
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
		codex_args=("$@")
		break
		;;
	*)
		codex_args+=("$1")
		shift
		;;
	esac
done

if [ "$use_ppe" -eq 1 ] && [ "$use_pre" -eq 1 ]; then
	die "--use-ppe and --use-pre are mutually exclusive"
fi

if [ "$use_ppe" -eq 1 ] && [ "$target_env_explicit" -eq 0 ]; then
	target_env="ppe"
fi
if [ "$use_ppe" -eq 1 ]; then
	case "$target_env" in
	ppe) ;;
	*) die "--use-ppe uses the pre endpoint; remove --env $target_env or pass --env ppe" ;;
	esac
fi
if [ "$use_pre" -eq 1 ] && [ "$target_env_explicit" -eq 0 ]; then
	target_env="use-pre"
fi
if [ "$use_pre" -eq 1 ]; then
	case "$target_env" in
	use-pre) ;;
	*) die "--use-pre uses the pre endpoint without PPE/lane headers; remove --env $target_env or pass --env use-pre" ;;
	esac
fi

case "$target_env" in
boe | pre | ppe | use-pre | online) ;;
*) die "--env must be one of: boe, pre, ppe, use-pre, online" ;;
esac

use_pre_effective=0
if [ "$target_env" = "use-pre" ]; then
	use_pre_effective=1
fi

case "$use_cx" in
0 | 1) ;;
*) die "CODEX_DEV_LARK_CX must be 0 or 1" ;;
esac

codex_launch_args=(-C "$repo_root" -c shell_environment_policy.inherit=all)
if [ "$use_cx" -eq 1 ]; then
	if [ -n "${CODEX_PROXY_API_KEY:-}" ]; then
		codex_launch_args=(--profile proxy --dangerously-bypass-approvals-and-sandbox "${codex_launch_args[@]}")
	else
		codex_launch_args=(--dangerously-bypass-approvals-and-sandbox "${codex_launch_args[@]}")
	fi
fi

command -v codex >/dev/null 2>&1 || die "codex not found in PATH"
[ -x "$repo_root/env/larkenv" ] || die "missing executable env/larkenv"

bin_dir="$repo_root/.codex-dev/bin"
skills_dir="$repo_root/.agents/skills"
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

remove_extra_header_name() {
	local target_name="$1"
	local raw="${LARKSUITE_CLI_EXTRA_HEADERS:-}"
	local item result="" name
	local IFS=';'

	for item in $raw; do
		item="${item#"${item%%[![:space:]]*}"}"
		item="${item%"${item##*[![:space:]]}"}"
		[ -n "$item" ] || continue
		name="${item%%:*}"
		name="$(printf '%s' "$name" | tr '[:upper:]' '[:lower:]')"
		[ "$name" = "$target_name" ] && continue
		if [ -n "$result" ]; then
			result="$result; $item"
		else
			result="$item"
		fi
	done

	if [ -n "$result" ]; then
		export LARKSUITE_CLI_EXTRA_HEADERS="$result"
	else
		unset LARKSUITE_CLI_EXTRA_HEADERS
	fi
}

shim_path="$bin_dir/lark-cli"
shim_tmp="$bin_dir/.lark-cli.$$.tmp"
cat >"$shim_tmp" <<SHIM
#!/usr/bin/env bash
set -euo pipefail

if [ "$use_ppe" -eq 1 ]; then
  case "; \${LARKSUITE_CLI_EXTRA_HEADERS:-};" in
    *"; x-use-ppe:1;"*) ;;
    *)
      if [ -n "\${LARKSUITE_CLI_EXTRA_HEADERS:-}" ]; then
        export LARKSUITE_CLI_EXTRA_HEADERS="\${LARKSUITE_CLI_EXTRA_HEADERS}; x-use-ppe:1"
      else
        export LARKSUITE_CLI_EXTRA_HEADERS="x-use-ppe:1"
      fi
      ;;
  esac
  case "; \${LARKSUITE_CLI_EXTRA_HEADERS:-};" in
    *"; env:pre_release;"*) ;;
    *)
      if [ -n "\${LARKSUITE_CLI_EXTRA_HEADERS:-}" ]; then
        export LARKSUITE_CLI_EXTRA_HEADERS="\${LARKSUITE_CLI_EXTRA_HEADERS}; env:pre_release"
      else
        export LARKSUITE_CLI_EXTRA_HEADERS="env:pre_release"
      fi
      ;;
  esac
elif [ "$use_pre_effective" -eq 1 ]; then
  case "; \${LARKSUITE_CLI_EXTRA_HEADERS:-};" in
    *"; env:pre_release;"*) ;;
    *)
      if [ -n "\${LARKSUITE_CLI_EXTRA_HEADERS:-}" ]; then
        export LARKSUITE_CLI_EXTRA_HEADERS="\${LARKSUITE_CLI_EXTRA_HEADERS}; env:pre_release"
      else
        export LARKSUITE_CLI_EXTRA_HEADERS="env:pre_release"
      fi
      ;;
  esac
fi

if [ "$use_pre_effective" -eq 1 ]; then
  unset LARK_LANE
  exec env \\
    LARK_CLI_ENV_BIN="$bin_dir" \\
    LARKSUITE_CLI_NO_UPDATE_NOTIFIER="\${LARKSUITE_CLI_NO_UPDATE_NOTIFIER:-1}" \\
    LARKSUITE_CLI_NO_SKILLS_NOTIFIER="\${LARKSUITE_CLI_NO_SKILLS_NOTIFIER:-1}" \\
    "$bin_dir/larkenv" "$target_env" "\$@"
else
  exec env \\
    LARK_CLI_ENV_BIN="$bin_dir" \\
    LARK_LANE="$lane" \\
    LARKSUITE_CLI_NO_UPDATE_NOTIFIER="\${LARKSUITE_CLI_NO_UPDATE_NOTIFIER:-1}" \\
    LARKSUITE_CLI_NO_SKILLS_NOTIFIER="\${LARKSUITE_CLI_NO_SKILLS_NOTIFIER:-1}" \\
    "$bin_dir/larkenv" "$target_env" "\$@"
fi
SHIM
chmod +x "$shim_tmp"
mv -f "$shim_tmp" "$shim_path"

echo "==> Starting Codex with dev lark-cli and project-local skills" >&2
echo "    PATH prefix: $bin_dir" >&2
echo "    Skill root:   $skills_dir" >&2
if [ "$use_pre_effective" -eq 1 ]; then
	echo "    lark-cli ->   larkenv $target_env" >&2
else
	echo "    lark-cli ->   LARK_LANE=$lane larkenv $target_env" >&2
fi
if [ "$use_ppe" -eq 1 ] || [ "$target_env" = "ppe" ]; then
	echo "    extra headers: x-use-ppe:1; env:pre_release" >&2
fi
if [ "$use_pre_effective" -eq 1 ]; then
	echo "    extra headers: env:pre_release" >&2
	echo "    omitted:       x-use-ppe; X-TT-ENV" >&2
fi
if [ "$use_cx" -eq 1 ]; then
	if [ -n "${CODEX_PROXY_API_KEY:-}" ]; then
		echo "    codex mode:   cx (--profile proxy --dangerously-bypass-approvals-and-sandbox)" >&2
	else
		echo "    codex mode:   cx (--dangerously-bypass-approvals-and-sandbox; proxy profile skipped)" >&2
	fi
fi

export PATH="$bin_dir:$PATH"
export LARK_CLI_ENV_BIN="$bin_dir"
if [ "$use_pre_effective" -eq 1 ]; then
	unset LARK_LANE
	remove_extra_header_name "x-use-ppe"
	remove_extra_header_name "x-tt-env"
else
	export LARK_LANE="$lane"
fi
if [ "$use_ppe" -eq 1 ]; then
	append_extra_header "x-use-ppe:1"
	append_extra_header "env:pre_release"
elif [ "$use_pre_effective" -eq 1 ]; then
	append_extra_header "env:pre_release"
fi
export LARKSUITE_CLI_NO_UPDATE_NOTIFIER="${LARKSUITE_CLI_NO_UPDATE_NOTIFIER:-1}"
export LARKSUITE_CLI_NO_SKILLS_NOTIFIER="${LARKSUITE_CLI_NO_SKILLS_NOTIFIER:-1}"

exec codex "${codex_launch_args[@]}" "${codex_args[@]}"
