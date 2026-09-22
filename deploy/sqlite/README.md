# Deploying with SQLite and no Redis

A single-container setup, in two variants that differ only in where the image comes from and
where the data lives:

| Directory | Image | Data |
|---|---|---|
| `deploy/sqlite/` | `ghcr.io/shinyes/cloudreve:4.20.0`, pulled | named volume `cloudreve_data` |
| `deploy/sqlite/local-image/` | `acloud:4.20.0-amd64`, built or loaded locally | bind mount `./cloudreve_data` |

Both are validated against the [compose specification][spec] schema. Pick one; they are
alternative deployments, not layers.

```bash
# registry image, named volume
cd deploy/sqlite
docker compose up -d

# locally built image, data directory next to the file
cd deploy/sqlite/local-image
docker compose up -d
```

Open <http://localhost:5212> and register: the first account becomes the administrator.

[spec]: https://github.com/compose-spec/compose-spec

## Why one container is enough

Upstream's compose file runs three services (Cloudreve, Postgres, Redis). Both extras are
optional for a single-node instance:

- **Postgres** - the SQLite driver is built into the binary. Nothing else is needed.
- **Redis** - when `Redis.Server` is empty the instance uses its in-memory cache, persisted
  to a file inside the data volume. See `application/dependency/dependency.go`, where an
  empty server selects the memo store.

Everything an instance owns - database, uploaded files, thumbnails, cache, generated secrets
- lives under `/cloudreve/data`: the named volume in the registry variant, the
`cloudreve_data` directory next to the compose file in the local-image variant. Backing up
is copying that one location:

```bash
# named volume
docker run --rm -v cloudreve_data:/data -v "$PWD":/backup alpine \
  tar czf /backup/cloudreve-$(date +%F).tar.gz -C /data .

# bind mount
tar czf cloudreve-$(date +%F).tar.gz -C deploy/sqlite/local-image/cloudreve_data .
```

Do it while the container is stopped, or copy the database with SQLite's backup command
rather than `cp`, so the copy cannot be taken mid-write.

## Configuration

Settings come from `data/conf.ini`, and any of them can be overridden by an environment
variable named `CR_CONF_<Section>.<Key>`; this compose file uses that to select SQLite
without touching the file. The override is logged at startup, so a misconfigured deployment
is visible in `docker compose logs`:

```
Override config "Database.DBFile" = "/cloudreve/data/cloudreve.db"
```

Dots in environment variable names are awkward in some shells and deployment platforms. The
same override can be written `CR_CONF_Section__Key` (double underscore) where needed.

### Things you may want to set

| Variable | Why |
|---|---|
| `CR_CONF_System.SessionSecret` | Generated on first start and stored in the database when unset. Set it to keep the value in your deployment config instead. |
| `CR_CONF_System.ProxyHeader=X-Forwarded-For` | When running behind a reverse proxy that terminates TLS, so client IPs and links are right. |
| `CR_CONF_Redis.Server=redis:6379` | Only if you later add Redis: several nodes, or a queue that must survive restarts. |
| `CR_CONF_System.LogLevel=warning` | Quieter logs once the instance is settled. |

### Image and tags

`:4.20.0` is this fork's build of upstream 4.19.1 plus the storage-policy work. Tags `latest`
and `v4` follow the newest release; pin an explicit version for a deployment you do not want
moving underneath you.

The image bundles LibreOffice, ffmpeg, vips, libraw and CJK fonts, and enables them through
environment variables, because that is how upstream builds it. That is most of the image's
size (~520 MB of ~560 MB). It buys Office document conversion, video thumbnails, RAW image
support and CJK rendering out of the box. A bare binary has all of those switched **off** by
default, so nothing else in the product depends on them.

### aria2

The image enables aria2 and runs it under supervisord on port 6888 (TCP and UDP), which is
why those ports are published. Remove them from the compose file if you do not use offline
downloads.
