#!/usr/bin/env bash
# tag-release.sh: create and push a semver release tag for one release stream.
#
# Purpose:
#   Each GitHub Actions release workflow enforces strict semver in-job, so
#   tags must be "<prefix><semver>" (e.g. flux-apps-v1.2.3). This script
#   picks the prefix, finds the previous semver tag, bumps it, and pushes
#   only that ref. It never edits files, creates releases, or signs images.
#
# Usage:
#   scripts/tag-release.sh [options]
#
# Examples:
#   scripts/tag-release.sh --list
#   scripts/tag-release.sh --prefix flux-apps --bump patch --dry-run
#   scripts/tag-release.sh --prefix apprise-go-api-v --bump minor
#   scripts/tag-release.sh -p eso-proton-pass -b major --yes
#   scripts/tag-release.sh   # fully interactive
set -euo pipefail

# One entry per release stream: "<tag-prefix>|<owning-workflow>".
# To add a stream, append one line here matching .github/workflows on.push.tags.
PREFIX_ENTRIES=(
  "apprise-go-api-v|apprise-go-api.yml"
  "eso-proton-pass-v|eso-proton-pass.yml"
  "external-dns-netbird-v|external-dns-netbird.yml"
  "flux-apps-v|flux-apps-release.yaml"
  "flux-fleet-v|flux-fleet-release.yaml"
  "flux-infra-v|flux-infra-release.yaml"
  "helm-rclone-sync-v|helm-rclone-sync.yml"
)

SEMVER_RE='^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$'

PREFIX=""
BUMP=""
ASSUME_YES=false
DRY_RUN=false
DO_FETCH=true
ALLOW_DIRTY=false

log() { printf '%s\n' "$*"; }
warn() { printf 'warning: %s\n' "$*" >&2; }
die() { printf 'error: %s\n' "$*" >&2; exit 1; }

usage() {
  cat <<'EOF'
Usage: tag-release.sh [options]

Create and push a "<prefix><semver>" release tag for one release stream.

Options:
  -p, --prefix NAME   Release prefix, with or without trailing -v
                      (e.g. flux-apps or flux-apps-v). Omit for a menu.
  -b, --bump KIND     Bump kind: major, minor, or patch. Omit for a menu.
  -y, --yes           Skip the confirmation prompt.
  -n, --dry-run       Print previous->next and the exact git commands
                      without executing them.
  --no-fetch          Skip 'git fetch --tags --prune' before lookup.
  --allow-dirty       Allow tagging with uncommitted worktree changes.
  --list              Print all known prefixes and exit.
  -h, --help          Show this help and exit.

Examples:
  scripts/tag-release.sh --list
  scripts/tag-release.sh --prefix flux-apps --bump patch --dry-run
  scripts/tag-release.sh --prefix apprise-go-api-v --bump minor  # first tag from 0.0.0 when none exists
EOF
}

list_prefixes() {
  local entry prefix workflow
  for entry in "${PREFIX_ENTRIES[@]}"; do
    prefix="${entry%%|*}"
    workflow="${entry##*|}"
    printf '%s (%s)\n' "$prefix" "$workflow"
  done
}

known_prefix() {
  local want="$1" entry prefix
  for entry in "${PREFIX_ENTRIES[@]}"; do
    prefix="${entry%%|*}"
    [[ "$prefix" == "$want" ]] && return 0
  done
  return 1
}

workflow_for() {
  local want="$1" entry prefix
  for entry in "${PREFIX_ENTRIES[@]}"; do
    prefix="${entry%%|*}"
    if [[ "$prefix" == "$want" ]]; then
      printf '%s' "${entry##*|}"
      return 0
    fi
  done
  return 1
}

normalize_prefix() {
  local p="$1"
  if [[ "$p" == *-v ]]; then
    printf '%s' "$p"
  else
    printf '%s-v' "${p%-}"
  fi
}

select_prefix_interactive() {
  local i entry prefix workflow choice
  printf 'Select a release prefix:\n' >&2
  i=0
  for entry in "${PREFIX_ENTRIES[@]}"; do
    i=$((i + 1))
    prefix="${entry%%|*}"
    workflow="${entry##*|}"
    printf '  %d) %s (%s)\n' "$i" "$prefix" "$workflow" >&2
  done
  printf 'Enter number [1-%d]: ' "$i" >&2
  read -r choice || die "no prefix selected."
  [[ "$choice" =~ ^[0-9]+$ ]] || die "invalid selection '$choice'; expected 1-$i."
  if ! ((choice >= 1 && choice <= i)); then die "selection '$choice' out of range 1-$i."; fi
  i=0
  for entry in "${PREFIX_ENTRIES[@]}"; do
    i=$((i + 1))
    if ((i == choice)); then
      printf '%s' "${entry%%|*}"
      return 0
    fi
  done
}

select_bump_interactive() {
  local choice
  printf 'Select a bump kind:\n  1) major (X+1.0.0)\n  2) minor (X.Y+1.0)\n  3) patch (X.Y.Z+1)\n' >&2
  printf 'Enter number [1-3]: ' >&2
  read -r choice || die "no bump kind selected."
  case "$choice" in
    1|major) printf 'major' ;;
    2|minor) printf 'minor' ;;
    3|patch) printf 'patch' ;;
    *) die "invalid selection '$choice'; expected 1-3 or major/minor/patch." ;;
  esac
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    -p|--prefix) PREFIX="${2:-}"; shift 2 ;;
    -b|--bump) BUMP="${2:-}"; shift 2 ;;
    -y|--yes) ASSUME_YES=true; shift ;;
    -n|--dry-run) DRY_RUN=true; shift ;;
    --no-fetch) DO_FETCH=false; shift ;;
    --allow-dirty) ALLOW_DIRTY=true; shift ;;
    --list) list_prefixes; exit 0 ;;
    -h|--help) usage; exit 0 ;;
    --) shift; break ;;
    -*) die "unknown flag '$1'. See --help." ;;
    *) die "unexpected argument '$1'. See --help." ;;
  esac
done

# Resolve prefix (flag wins, else interactive menu).
if [[ -z "$PREFIX" ]]; then
  PREFIX="$(select_prefix_interactive)"
else
  PREFIX="$(normalize_prefix "$PREFIX")"
fi
if ! known_prefix "$PREFIX"; then
  printf "error: unknown prefix '%s'. Valid prefixes:\n" "$PREFIX" >&2
  list_prefixes >&2
  exit 1
fi

# Preflight: inside a work tree, clean tree, optional fetch, branch notice.
git rev-parse --is-inside-work-tree >/dev/null \
  || die "not inside a git work tree."
if [[ -n "$(git status --porcelain)" ]]; then
  if [[ "$ALLOW_DIRTY" == true ]]; then
    warn "worktree has uncommitted changes (--allow-dirty given); tagging anyway."
  else
    die "worktree has uncommitted changes. Commit, stash, or pass --allow-dirty."
  fi
fi
BRANCH="$(git branch --show-current 2>/dev/null || true)"
if [[ -z "$BRANCH" ]]; then
  warn "detached HEAD; tagging from a detached HEAD."
elif [[ "$BRANCH" != "main" ]]; then
  warn "on branch '$BRANCH' (not main); tags usually cut from main."
fi
if [[ "$DO_FETCH" == true ]]; then
  git fetch --tags --prune || warn "git fetch failed; continuing with local tags."
fi

# Previous-semver lookup: strip prefix, keep semver, sort -V, take newest.
PREV_VERSION=""
while IFS= read -r tag; do
  [[ -n "$tag" ]] || continue
  suffix="${tag#"${PREFIX}"}"
  if [[ "$suffix" =~ $SEMVER_RE ]]; then
    PREV_VERSION+="${suffix}"$'\n'
  else
    warn "ignoring non-semver tag '$tag'."
  fi
done < <(git tag -l "${PREFIX}*")
if [[ -n "$PREV_VERSION" ]]; then
  PREV_VERSION="$(printf '%s' "$PREV_VERSION" | sort -V | tail -n 1)"
  FIRST_TAG=false
else
  PREV_VERSION="0.0.0"
  FIRST_TAG=true
  log "No previous tag for prefix '$PREFIX'; starting from base 0.0.0."
fi
log "Previous version: ${PREFIX}${PREV_VERSION}"

# Resolve bump kind (flag wins, else interactive menu).
if [[ -z "$BUMP" ]]; then
  BUMP="$(select_bump_interactive)"
fi
case "$BUMP" in
  major|minor|patch) ;;
  *) die "invalid --bump '$BUMP'; expected major, minor, or patch." ;;
esac

# Bump math on the numeric core; pre-release/build metadata is dropped.
CORE="${PREV_VERSION%%[-+]*}"
IFS=. read -r MAJOR MINOR PATCH <<<"$CORE"
[[ "$MAJOR" =~ ^[0-9]+$ && "$MINOR" =~ ^[0-9]+$ && "$PATCH" =~ ^[0-9]+$ ]] \
  || die "cannot parse numeric core '$CORE'."
case "$BUMP" in
  major) MAJOR=$((MAJOR + 1)); MINOR=0; PATCH=0 ;;
  minor) MINOR=$((MINOR + 1)); PATCH=0 ;;
  patch) PATCH=$((PATCH + 1)) ;;
esac
NEXT_VERSION="${MAJOR}.${MINOR}.${PATCH}"
NEXT_TAG="${PREFIX}${NEXT_VERSION}"
[[ "$NEXT_VERSION" =~ $SEMVER_RE ]] \
  || die "computed version '$NEXT_VERSION' failed semver validation."

# Refuse duplicates locally or on origin.
if git rev-parse -q --verify "refs/tags/${NEXT_TAG}" >/dev/null; then
  die "tag '$NEXT_TAG' already exists locally."
fi
if git remote get-url origin >/dev/null 2>&1; then
  if [[ -n "$(git ls-remote --tags origin "refs/tags/${NEXT_TAG}" 2>/dev/null)" ]]; then
    die "tag '$NEXT_TAG' already exists on origin."
  fi
fi

log "Bump: ${PREFIX}${PREV_VERSION} -> ${NEXT_TAG} (${BUMP})"

# Dry run: show exact commands, change nothing.
if [[ "$DRY_RUN" == true ]]; then
  log "Dry run; would execute:"
  log "  git tag -a ${NEXT_TAG} -m ${NEXT_TAG}"
  log "  git push origin ${NEXT_TAG}"
  exit 0
fi

# Confirm unless --yes.
if [[ "$ASSUME_YES" != true ]]; then
  REPLY=""
  printf 'Create and push %s? [y/N] ' "$NEXT_TAG"
  read -r REPLY || REPLY=""
  case "$REPLY" in
    [yY]|[yY][eE][sS]) ;;
    *) die "aborted; tag not created." ;;
  esac
fi

git tag -a "$NEXT_TAG" -m "$NEXT_TAG"
git push origin "$NEXT_TAG"
log "Pushed tag '$NEXT_TAG'; the '$(workflow_for "$PREFIX")' release workflow will now run."
