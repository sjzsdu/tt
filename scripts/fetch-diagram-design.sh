#!/usr/bin/env bash
# Fetch the diagram-design skill from upstream, pinned to a commit and verified
# by a content hash. The skill data lands in internal/docsvisual/skilldata/
# (gitignored) and is embedded into the tt binary via go:embed.
# Re-running is idempotent: the archive is cached and the hash is verified.
set -euo pipefail

# Pin: commit of cathrynlavery/diagram-design whose SKILL.md metadata says version 2.6.
COMMIT="f4547ee95f88e5b28a52517feff6b6c11cc657f9"
REPO="cathrynlavery/diagram-design"
ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
DEST_DIR="${ROOT_DIR}/internal/docsvisual/skilldata/diagram-design"
ARCHIVE="${TMPDIR:-/tmp}/diagram-design-${COMMIT}.tar.gz"
TMP_EXTRACT="$(mktemp -d)"
trap 'rm -rf "$TMP_EXTRACT"' EXIT

SUBTREE_IN_ARCHIVE="diagram-design-${COMMIT}/skills/diagram-design"
EXPECTED_SHA256="1d9d5898225b8afd824b3c87b9a49443e2493248a3b68d12cfc8c6ccf248fc2c"

hash_of() {
  # Deterministic hash: sorted relative paths + file digests, hashed again.
  (cd "$1" && find . -type f -print0 | sort -z | xargs -0 shasum -a 256 | shasum -a 256 | cut -d' ' -f1)
}

if [[ ! -f "$ARCHIVE" ]]; then
  echo "downloading ${REPO}@${COMMIT:0:7}..."
  curl -fsSL -o "$ARCHIVE" "https://github.com/${REPO}/archive/${COMMIT}.tar.gz"
fi

tar -xzf "$ARCHIVE" -C "$TMP_EXTRACT"
SRC_DIR="${TMP_EXTRACT}/${SUBTREE_IN_ARCHIVE}"
if [[ ! -f "${SRC_DIR}/SKILL.md" ]]; then
  echo "skill subtree missing in archive: ${SUBTREE_IN_ARCHIVE}" >&2
  exit 1
fi

ACTUAL="$(hash_of "$SRC_DIR")"

if [[ "$EXPECTED_SHA256" == "REPLACE_ME" ]]; then
  echo "no expected hash pinned yet; actual hash is:"
  echo "$ACTUAL"
  echo "Pin it into EXPECTED_SHA256 in this script, then re-run."
  exit 2
fi

if [[ "$ACTUAL" != "$EXPECTED_SHA256" ]]; then
  echo "sha256 mismatch for ${REPO}@${COMMIT:0:7} skills/diagram-design" >&2
  echo "  expected: $EXPECTED_SHA256" >&2
  echo "  actual:   $ACTUAL" >&2
  echo "If this is an intentional bump: change COMMIT, re-run, pin the new hash." >&2
  exit 1
fi

rm -rf "$DEST_DIR"
mkdir -p "$(dirname "$DEST_DIR")"
cp -R "$SRC_DIR" "$DEST_DIR"
echo "diagram-design @ ${COMMIT:0:7} verified (sha256 ok) and staged at internal/docsvisual/skilldata/diagram-design"
