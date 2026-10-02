# Server setup

This setup builds the proxy fork from this checkout. It also builds the quota
ledger panel from commit `a9eda98ad825b166f6516f7b04c7b4a2b4f890de` of the UI fork.
Both are installed in one image. Panel auto-updates are disabled.

Use a Debian or Ubuntu server with Docker Engine and the Docker Compose plugin. Install
Git and OpenSSL. Start with 2 CPU cores and 4 GB RAM to build both projects.
The server needs outbound access to GitHub, package registries, and the providers.

## Install

Point your domain's DNS A record to the server. Add an AAAA record only if IPv6
works on that server. Allow inbound TCP ports 80 and 443. UDP port 443 is optional
for HTTP/3. Caddy uses ports 80 and 443, so they must be free.

Run on the server:

```bash
git clone --branch feat/earliest-reset-routing --single-branch https://github.com/rexlManu/CLIProxyAPI.git
cd CLIProxyAPI/deploy
./init.sh proxy.example.com
docker compose config --quiet
docker compose up -d --build
docker compose ps
docker compose logs --tail=50
```

Replace `proxy.example.com` with your domain. Caddy obtains and renews the HTTPS
certificate. It forwards streaming responses and WebSocket upgrades. No streaming
read timeout is set. The proxy port is available only inside the Compose network.

If a reverse proxy already uses ports 80 and 443, do not start this Caddy service.
Adapt the setup to your existing reverse proxy first.

## Add accounts

Open `https://proxy.example.com/management.html`. Use the management key from
`deploy/data/credentials.txt` to sign in. This file also contains the separate API
key for your clients. Keep this file private.

In the panel, use OAuth login to add each Codex and Claude account. For a remote
server, paste the final callback URL into the panel when asked. A localhost
callback page can fail to load in your browser, but its URL still contains the
code needed for login. Do not send that URL to other people.

You can also upload existing CLIProxyAPI auth files through the panel. This
avoids a new login for those accounts. Refresh quotas for every account page
after import so the proxy has their reset times.

Use `https://proxy.example.com/v1` as the OpenAI-compatible base URL for Codex.
Use `https://proxy.example.com` as the Anthropic base URL for Claude clients.
Use the generated API key for proxy requests. Codex upstream WebSockets and
earliest-reset routing are enabled. Claude uses HTTP streaming.

## Check the installation

```bash
curl -fsS https://proxy.example.com/management.html -o /dev/null
docker compose ps
```

In the panel, confirm that the quota page opens in ledger view. After account
login, check that models and quotas load. Send one request from each client.
Provider requests require account credentials, so a clean install cannot test
them before login.

## Data and updates

Configuration is in `deploy/data/config/config.yaml`. Account files are in
`deploy/data/auths`. These directories survive image rebuilds. Caddy certificates
are in Docker volumes. Back up the data directory securely before updates.
The management key in the config is hashed on first startup. The original key
remains in the private credentials file.

To update the proxy from this feature branch:

```bash
git pull --ff-only
docker compose build --build-arg COMMIT="$(git rev-parse HEAD)" proxy
docker compose up -d
```

Change `MANAGEMENT_UI_COMMIT` in `deploy/Dockerfile` to update the panel. Do not
run `init.sh` again during updates. It stops if setup files already exist.

To stop services and keep data:

```bash
docker compose down
```

Do not add `--volumes` unless you intend to remove Caddy's stored certificates.
