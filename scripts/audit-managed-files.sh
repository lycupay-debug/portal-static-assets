#!/bin/sh
# Read-only inventory of managed image/resource files against PostgreSQL registrations.
# This script never deletes, moves, rewrites, or changes permissions on managed files.

set -eu

DB_NAME=${PORTAL_DB_NAME:-personal_portal_dev}
UPLOAD_DIR=${PORTAL_UPLOAD_DIR:-/var/db/portal-static-assets-dev/uploads}
RESOURCE_DIR=${PORTAL_RESOURCE_DIR:-/var/db/portal-static-assets-dev/resources}
PSQL_BIN=${PSQL_BIN:-psql}

for dir in "$UPLOAD_DIR" "$RESOURCE_DIR"; do
	if [ ! -d "$dir" ] || [ -L "$dir" ]; then
		printf 'ERROR: managed directory is missing or is a symlink: %s\n' "$dir" >&2
		exit 2
	fi
done

tmpdir=$(mktemp -d "${TMPDIR:-/tmp}/portal-managed-file-audit.XXXXXX")
trap 'rm -rf "$tmpdir"' EXIT HUP INT TERM

query_names() {
	query=$1
	output=$2
	# Redirect psql directly: POSIX sh has no pipefail, so a pipeline could hide DB errors.
	"$PSQL_BIN" -X -q -A -t -v ON_ERROR_STOP=1 -d "$DB_NAME" -c "$query" > "$output.raw"
	sed '/^[[:space:]]*$/d' "$output.raw" | LC_ALL=C sort -u > "$output"
}

list_managed_files() {
	dir=$1
	output=$2
	find "$dir" -type d ! -path "$dir" -prune -o -type f -print > "$output.paths"
	: > "$output"
	while IFS= read -r path; do
		basename "$path" >> "$output"
	done < "$output.paths"
	LC_ALL=C sort -u "$output" -o "$output"
}

list_symlinks() {
	dir=$1
	output=$2
	find "$dir" -type d ! -path "$dir" -prune -o -type l -print > "$output"
}

list_nested_directories() {
	dir=$1
	output=$2
	find "$dir" -type d ! -path "$dir" -print > "$output"
}

report_pair() {
	label=$1
	registered=$2
	on_disk=$3
	missing=$4
	unregistered=$5
	comm -23 "$registered" "$on_disk" > "$missing"
	comm -13 "$registered" "$on_disk" > "$unregistered"
	missing_count=$(wc -l < "$missing" | tr -d ' ')
	unregistered_count=$(wc -l < "$unregistered" | tr -d ' ')
	printf '\n[%s]\nregistered=%s on_disk=%s missing_registered_files=%s unregistered_disk_files=%s\n' \
		"$label" \
		"$(wc -l < "$registered" | tr -d ' ')" \
		"$(wc -l < "$on_disk" | tr -d ' ')" \
		"$missing_count" "$unregistered_count"
	if [ "$missing_count" -gt 0 ]; then
		printf '%s\n' 'MISSING_REGISTERED_FILES:'
		sed 's/^/  /' "$missing"
	fi
	if [ "$unregistered_count" -gt 0 ]; then
		printf '%s\n' 'UNREGISTERED_DISK_FILES (report only; no deletion):'
		sed 's/^/  /' "$unregistered"
	fi
}

query_names 'SELECT filename FROM portal.uploaded_images ORDER BY filename' "$tmpdir/images.db"
query_names "SELECT stored_filename FROM portal.teaching_resources WHERE stored_filename IS NOT NULL ORDER BY stored_filename" "$tmpdir/resources.db"
list_managed_files "$UPLOAD_DIR" "$tmpdir/images.disk"
list_managed_files "$RESOURCE_DIR" "$tmpdir/resources.disk"
list_symlinks "$UPLOAD_DIR" "$tmpdir/images.links"
list_symlinks "$RESOURCE_DIR" "$tmpdir/resources.links"
list_nested_directories "$UPLOAD_DIR" "$tmpdir/images.dirs"
list_nested_directories "$RESOURCE_DIR" "$tmpdir/resources.dirs"

report_pair images "$tmpdir/images.db" "$tmpdir/images.disk" "$tmpdir/images.missing" "$tmpdir/images.unregistered"
report_pair resources "$tmpdir/resources.db" "$tmpdir/resources.disk" "$tmpdir/resources.missing" "$tmpdir/resources.unregistered"

symlink_count=$(( $(wc -l < "$tmpdir/images.links" | tr -d ' ') + $(wc -l < "$tmpdir/resources.links" | tr -d ' ') ))
nested_directory_count=$(( $(wc -l < "$tmpdir/images.dirs" | tr -d ' ') + $(wc -l < "$tmpdir/resources.dirs" | tr -d ' ') ))
printf '\nmanaged_directory_symlinks=%s nested_directories=%s\n' "$symlink_count" "$nested_directory_count"
if [ "$symlink_count" -gt 0 ]; then
	printf '%s\n' 'SYMLINKS (manual investigation required):'
	cat "$tmpdir/images.links" "$tmpdir/resources.links"
fi
if [ "$nested_directory_count" -gt 0 ]; then
	printf '%s\n' 'NESTED_DIRECTORIES (manual investigation required):'
	cat "$tmpdir/images.dirs" "$tmpdir/resources.dirs"
fi

missing_total=$(( $(wc -l < "$tmpdir/images.missing" | tr -d ' ') + $(wc -l < "$tmpdir/resources.missing" | tr -d ' ') ))
if [ "$missing_total" -gt 0 ] || [ "$symlink_count" -gt 0 ] || [ "$nested_directory_count" -gt 0 ]; then
	printf '\nREAD_ONLY_AUDIT_WARNING: investigate missing registrations or unexpected filesystem entries. No changes made.\n'
	exit 2
fi

printf '\nREAD_ONLY_AUDIT_COMPLETE: no managed files or database rows were modified. Unregistered files, if any, are report-only.\n'
