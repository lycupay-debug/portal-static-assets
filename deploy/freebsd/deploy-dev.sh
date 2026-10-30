#!/bin/sh
# Build, back up, migrate, and deploy only the isolated public development portal.
set -eu

if [ "$(id -u)" -ne 0 ]; then
  echo "Run as root." >&2
  exit 1
fi
if [ "${PORTAL_STATIC_ASSETS_DEV:-0}" != "1" ]; then
  echo "Set PORTAL_STATIC_ASSETS_DEV=1 to confirm this is a development-only deployment." >&2
  exit 2
fi

repo=$(CDPATH= cd -- "$(dirname -- "$0")/../.." && pwd)
branch=$(git -C "$repo" branch --show-current)
if [ "$branch" != "main" ]; then
  echo "Development deployment requires the main branch; current branch is $branch." >&2
  exit 1
fi
git -C "$repo" fetch origin
if [ "$(git -C "$repo" rev-parse HEAD)" != "$(git -C "$repo" rev-parse origin/main)" ]; then
  echo "Local main is not synchronized with origin/main." >&2
  exit 1
fi
if [ -n "$(git -C "$repo" status --porcelain)" ]; then
  echo "Repository must be clean and committed before development deployment." >&2
  exit 1
fi

stamp=$(date -u +%Y%m%dT%H%M%SZ)
stage="/var/tmp/portal-static-assets-dev-$stamp"
backup_dir="/var/backups/portal-static-assets-dev/$stamp"
bin_dir="/usr/local/libexec/portal-static-assets-dev"
caddy_file="/usr/local/etc/caddy/Caddyfile"
database_url_file="/usr/local/etc/portal-static-assets-dev/database-url"
database_name="personal_portal_dev"
mkdir -p "$stage" "$backup_dir"
chmod 0700 "$stage" "$backup_dir"
deployment_stopped=0
backup_complete=0
migration_started=0
dump_tmp="/tmp/portal-static-assets-dev-$stamp.dump"

on_exit() {
  rc=$?
  rm -f "$dump_tmp"
  if [ "$rc" -ne 0 ]; then
    echo "Development deployment failed; no automatic rollback was attempted." >&2
    echo "Build/diagnostic stage: $stage" >&2
    if [ "$backup_complete" -eq 1 ]; then
      echo "Verified pre-deployment backup: $backup_dir" >&2
    else
      echo "A complete pre-deployment backup was not yet created; no service/database migration was started." >&2
    fi
    if [ "$deployment_stopped" -eq 1 ] && [ "$migration_started" -eq 0 ]; then
      echo "No database migration was started; attempting to restart the previous development binaries." >&2
      if "$repo/deploy/freebsd/dev-runtime.sh" start; then
        deployment_stopped=0
      else
        echo "Previous development runtime did not restart. Inspect logs, then run: $repo/deploy/freebsd/dev-runtime.sh start" >&2
      fi
    elif [ "$deployment_stopped" -eq 1 ]; then
      echo "The development API/event core may remain stopped after migration began. Inspect logs, then start only after checking schema compatibility: $repo/deploy/freebsd/dev-runtime.sh start" >&2
    fi
  else
    echo "Development deployment completed: $stamp" >&2
  fi
}
trap on_exit EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

if ! grep -Fq 'handle /uploads/* {' "$repo/deploy/freebsd/Caddyfile.dev-public" || \
   ! grep -Fq 'redir https://{http.request.host}/api{uri} 308' "$repo/deploy/freebsd/Caddyfile.dev-public" || \
   ! grep -Fq 'handle /api/uploads/* {' "$repo/deploy/freebsd/Caddyfile.dev-public" || \
   ! grep -Fq 'uri strip_prefix /api' "$repo/deploy/freebsd/Caddyfile.dev-public" || \
   ! grep -Fq 'reverse_proxy 127.0.0.1:8082 {' "$repo/deploy/freebsd/Caddyfile.dev-public"; then
  echo "Development Caddy config must route /api/uploads through the Go image ownership gate and redirect legacy /uploads URLs." >&2
  exit 1
fi

if [ ! -s "$database_url_file" ]; then
  echo "Missing development database URL file: $database_url_file" >&2
  exit 1
fi
if ! command -v python3.14 >/dev/null 2>&1; then
  echo "python3.14 is required by the migration runner." >&2
  exit 1
fi
caddy validate --config "$repo/deploy/freebsd/Caddyfile.dev-public"

printf '%s\n' '=== frontend tests, API contract, and isolated build ==='
cd "$repo/frontend"
npm ci
npm test
npm run type-check
npm run api:lint
npm run api:check
npx vite build --outDir "$stage/frontend-dist"
node scripts/check-bundle-budget.mjs "$stage/frontend-dist"
node ../scripts/check-static-assets-layout.mjs "$stage/frontend-dist"

printf '%s\n' '=== Go unit/race tests, vet, and build ==='
cd "$repo/backend"
go test -race -count=1 ./...
go vet ./...
go build -trimpath -ldflags='-s -w' -o "$stage/api" ./api

printf '%s\n' '=== native C protocol and kqueue integration tests ==='
cd "$repo/event-core"
make clean all test
cp event-core "$stage/event-core"
make clean

printf '%s\n' '=== real Go/C IPC and restart integration ==='
cd "$repo/backend"
PORTAL_EVENT_CORE_BINARY="$stage/event-core" go test -race -count=1 -run '^TestIPCClientAgainstRealEventCore$' ./api

printf '%s\n' '=== repository cleanliness after validation ==='
git -C "$repo" diff --check
if [ -n "$(git -C "$repo" status --porcelain)" ]; then
  echo "Validation generated or modified tracked/untracked repository files; inspect before deployment." >&2
  git -C "$repo" status --short >&2
  exit 1
fi

printf '%s\n' '=== stop only the development API/event core for a consistent snapshot ==='
deployment_stopped=1
"$repo/deploy/freebsd/dev-runtime.sh" stop

printf '%s\n' '=== back up development database, managed files, and current runtime ==='
mkdir -p "$bin_dir"
chmod 0755 "$bin_dir"
install -d -o personalportal -g personalportal -m 0750 \
  /var/db/portal-static-assets-dev \
  /var/db/portal-static-assets-dev/uploads \
  /var/db/portal-static-assets-dev/resources
su -m postgres -c "pg_dump -Fc -d '$database_name' -f '$dump_tmp'"
install -o root -g wheel -m 0600 "$dump_tmp" "$backup_dir/$database_name.dump"
rm -f "$dump_tmp"
pg_restore -l "$backup_dir/$database_name.dump" > "$backup_dir/pg_restore.list"
sha256 "$backup_dir/$database_name.dump" > "$backup_dir/database.sha256"
tar -czf "$backup_dir/uploads.previous.tar.gz" -C /var/db/portal-static-assets-dev uploads
tar -czf "$backup_dir/resources.previous.tar.gz" -C /var/db/portal-static-assets-dev resources
sha256 "$backup_dir/uploads.previous.tar.gz" "$backup_dir/resources.previous.tar.gz" > "$backup_dir/storage.sha256"

cp -p "$bin_dir/api" "$backup_dir/api.previous"
cp -p "$bin_dir/event-core" "$backup_dir/event-core.previous"
tar -czf "$backup_dir/frontend-dist.previous.tar.gz" -C "$repo/frontend" dist
cp -p "$caddy_file" "$backup_dir/Caddyfile.public.previous"
if [ -f /usr/local/etc/caddy/Caddyfile.portal-dev ]; then
  cp -p /usr/local/etc/caddy/Caddyfile.portal-dev "$backup_dir/Caddyfile.local-dev.previous"
fi
printf 'commit=%s\nbranch=%s\nstamp=%s\ndatabase=%s\n' \
  "$(git -C "$repo" rev-parse HEAD)" "$branch" "$stamp" "$database_name" > "$backup_dir/metadata.txt"
backup_complete=1

printf '%s\n' '=== apply any pending migrations to personal_portal_dev only ==='
migration_started=1
PERSONAL_PORTAL_DB_NAME="$database_name" \
PERSONAL_PORTAL_DATABASE_URL_FILE="$database_url_file" \
PERSONAL_PORTAL_MIGRATION_BACKUP="$backup_dir/$database_name.dump" \
  "$repo/deploy/freebsd/migrate.sh"

printf '%s\n' '=== install validated binaries and switch development frontend ==='
install -o root -g wheel -m 0555 "$stage/api" "$bin_dir/api.next"
install -o root -g wheel -m 0555 "$stage/event-core" "$bin_dir/event-core.next"
mv -f "$bin_dir/api.next" "$bin_dir/api"
mv -f "$bin_dir/event-core.next" "$bin_dir/event-core"

previous_dist="$repo/frontend/dist.previous-$stamp"
mv "$repo/frontend/dist" "$previous_dist"
mv "$stage/frontend-dist" "$repo/frontend/dist"
chown -R root:wheel "$repo/frontend/dist"
find "$repo/frontend/dist" -type d -exec chmod 0755 {} +
find "$repo/frontend/dist" -type f -exec chmod 0644 {} +

printf '%s\n' '=== start development API with registration disabled and isolated file storage ==='
"$repo/deploy/freebsd/dev-runtime.sh" start
deployment_stopped=0

printf '%s\n' '=== validate and reload the public development Caddy gateway ==='
caddy validate --config "$repo/deploy/freebsd/Caddyfile.dev-public"
install -o root -g wheel -m 0644 "$repo/deploy/freebsd/Caddyfile.dev-public" "$caddy_file"
caddy validate --config "$caddy_file"
service caddy reload

printf '%s\n' '=== verify public development portal ==='
"$repo/deploy/freebsd/health-check-dev.sh"

mv "$previous_dist" "$backup_dir/frontend-dist.previous.directory"
sha256 "$bin_dir/api" "$bin_dir/event-core" > "$backup_dir/runtime.sha256"

echo "Development deployment stamp: $stamp"
echo "Commit: $(git -C "$repo" rev-parse HEAD)"
echo "Backup: $backup_dir"
echo "Public development URL: https://www.lycupaynotion.top"
echo "Formal production deployment was not performed."
