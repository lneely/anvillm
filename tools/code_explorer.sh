#!/bin/bash
# capabilities: code-exploration
# description: Token-efficient code exploration with preview/fetch phases
set -euo pipefail

CACHE_BASE="${XDG_CACHE_HOME:-$HOME/.cache}/anvillm/code-explorer"

slugify() {
  echo "$1" | sed 's/-/--/g; s|/|-|g'
}

get_cache_dir() {
  local dir="${1:-$PWD}"
  local slug=$(slugify "$dir")
  echo "$CACHE_BASE/$slug"
}

cmd_init() {
  local dir="${1:-$PWD}"
  local cache=$(get_cache_dir "$dir")
  mkdir -p "$cache"
  
  # Build file index
  find "$dir" -type f \
    ! -path '*/.git/*' \
    ! -path '*/node_modules/*' \
    ! -path '*/vendor/*' \
    ! -path '*/__pycache__/*' \
    ! -path '*/target/*' \
    ! -path '*/dist/*' \
    ! -path '*/build/*' \
    -printf '%P\t%s\t%T@\n' 2>/dev/null > "$cache/index.tsv"
  
  # Build symbol index if ctags available
  if command -v ctags &>/dev/null; then
    ctags -R --fields=+n -f "$cache/tags" "$dir" 2>/dev/null || true
  fi
  
  echo "Indexed $(wc -l < "$cache/index.tsv") files in $cache"
}

cmd_preview() {
  local pattern="$1"
  local dir="${2:-$PWD}"
  local lang="${3:-}"
  
  local include=""
  [ -n "$lang" ] && include="--include=*.$lang"
  
  rg -c $include \
    --glob '!.git' \
    --glob '!node_modules' \
    --glob '!vendor' \
    --glob '!__pycache__' \
    --glob '!target' \
    --glob '!dist' \
    --glob '!build' \
    "$pattern" "$dir" 2>/dev/null | head -50
}

cmd_fetch() {
  local pattern="$1"
  local file="$2"
  local context="${3:-2}"
  
  rg -n -C "$context" "$pattern" "$file" 2>/dev/null | head -100
}

cmd_symbols() {
  local pattern="$1"
  local dir="${2:-$PWD}"
  local cache=$(get_cache_dir "$dir")
  
  if [ -f "$cache/tags" ]; then
    grep -i "$pattern" "$cache/tags" | head -30
  else
    echo "No symbol index. Run: code_explorer init" >&2
    exit 1
  fi
}

cmd_tree() {
  local dir="${1:-$PWD}"
  local depth="${2:-2}"
  local lang="${3:-}"
  
  local name_filter=""
  [ -n "$lang" ] && name_filter="-name '*.$lang'"
  
  find "$dir" -maxdepth "$depth" -type f \
    ! -path '*/.git/*' \
    ! -path '*/node_modules/*' \
    $name_filter \
    2>/dev/null | head -100
}

case "${1:-help}" in
  init)    cmd_init "${2:-}" ;;
  preview) cmd_preview "$2" "${3:-}" "${4:-}" ;;
  fetch)   cmd_fetch "$2" "$3" "${4:-2}" ;;
  symbols) cmd_symbols "$2" "${3:-}" ;;
  tree)    cmd_tree "${2:-}" "${3:-2}" "${4:-}" ;;
  cache-dir) get_cache_dir "${2:-$PWD}" ;;
  *)
    cat <<EOF
Usage: code_explorer <command> [args]

Commands:
  init [dir]                    Build index for directory (default: \$PWD)
  preview <pattern> [dir] [ext] Count matches per file (cheap)
  fetch <pattern> <file> [ctx]  Get matches with context (default: 2 lines)
  symbols <pattern> [dir]       Search symbol index
  tree [dir] [depth] [ext]      List files (default depth: 2)
  cache-dir [dir]               Show cache location
EOF
    ;;
esac
