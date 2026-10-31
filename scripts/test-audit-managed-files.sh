#!/bin/sh
# Deterministic tests for the read-only managed-file audit script.

set -eu

SCRIPT_DIR=$(CDPATH= cd -- "$(dirname "$0")" && pwd)
AUDIT_SCRIPT=$SCRIPT_DIR/audit-managed-files.sh
TMP=$(mktemp -d "${TMPDIR:-/tmp}/portal-managed-file-audit-test.XXXXXX")
trap 'rm -rf "$TMP"' EXIT HUP INT TERM

mkdir -p "$TMP/bin" "$TMP/uploads" "$TMP/resources"
cat > "$TMP/bin/psql" <<'MOCK_PSQL'
#!/bin/sh
case "$*" in
	*portal.uploaded_images*)
		[ "${MOCK_PSQL_FAIL:-0}" = 0 ] || exit 7
		printf 'image-present.png\n'
		[ "${MOCK_MISSING:-0}" = 0 ] || printf 'image-missing.png\n'
		;;
	*portal.teaching_resources*)
		[ "${MOCK_PSQL_FAIL:-0}" = 0 ] || exit 7
		printf 'resource-present.pdf\n'
		;;
	*) exit 8 ;;
esac
MOCK_PSQL
chmod 0755 "$TMP/bin/psql"

: > "$TMP/uploads/image-present.png"
: > "$TMP/uploads/image-orphan.png"
: > "$TMP/resources/resource-present.pdf"
: > "$TMP/resources/resource-orphan.pdf"

run_audit() {
	PATH="$TMP/bin:$PATH" PSQL_BIN=psql \
		PORTAL_UPLOAD_DIR="$TMP/uploads" \
		PORTAL_RESOURCE_DIR="$TMP/resources" \
		"$AUDIT_SCRIPT"
}

# Orphan files are integrity warnings, but are report-only and never deleted.
if output=$(run_audit); then
	printf '%s\n' 'FAIL: unregistered disk files did not fail the integrity audit' >&2
	exit 1
else
	status=$?
	[ "$status" -eq 2 ] || {
		printf 'FAIL: orphan-file audit status=%s, want 2\n' "$status" >&2
		exit 1
	}
fi
printf '%s\n' "$output" | grep -q 'missing_registered_files=0 unregistered_disk_files=1'
printf '%s\n' "$output" | grep -q 'image-orphan.png'
printf '%s\n' "$output" | grep -q 'resource-orphan.pdf'
[ -f "$TMP/uploads/image-orphan.png" ]
[ -f "$TMP/resources/resource-orphan.pdf" ]

# A registered file missing from disk is a non-zero integrity warning.
export MOCK_MISSING=1
if output=$(run_audit); then
	printf '%s\n' 'FAIL: missing registered file did not fail audit' >&2
	exit 1
else
	status=$?
	[ "$status" -eq 2 ] || {
		printf 'FAIL: missing-file audit status=%s, want 2\n' "$status" >&2
		exit 1
	}
fi
unset MOCK_MISSING
printf '%s\n' "$output" | grep -q 'image-missing.png'

# A database failure must not be converted into an empty successful inventory.
export MOCK_PSQL_FAIL=1
if output=$(run_audit); then
	printf '%s\n' 'FAIL: database failure was hidden by audit pipeline' >&2
	exit 1
else
	status=$?
	[ "$status" -ne 0 ] || exit 1
fi
unset MOCK_PSQL_FAIL

# Unexpected symlinks and nested directories are reported and fail closed.
ln -s image-present.png "$TMP/uploads/image-link.png"
if output=$(run_audit); then
	printf '%s\n' 'FAIL: symlink did not fail audit' >&2
	exit 1
else
	status=$?
	[ "$status" -eq 2 ] || exit 1
fi
rm "$TMP/uploads/image-link.png"
mkdir "$TMP/resources/nested"
if output=$(run_audit); then
	printf '%s\n' 'FAIL: nested directory did not fail audit' >&2
	exit 1
else
	status=$?
	[ "$status" -eq 2 ] || exit 1
fi

printf '%s\n' 'PASS: read-only managed-file audit reports orphans, detects missing files, propagates DB failures, and rejects symlinks/nested directories without mutation.'
