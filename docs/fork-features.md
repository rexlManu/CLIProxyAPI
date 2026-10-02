# Quota routing and WebSocket transport

For a new server installation, see [Server setup](../deploy/README.md).

This fork uses the management UI from
https://github.com/rexlManu/Cli-Proxy-API-Management-Center.
The UI opens the quota ledger by default. It has provider totals, account rows,
quota bars, reset times, and an email visibility control. Dark mode uses a black
background and white primary text.

## Configuration

For the v8 configuration layout:

```yaml
routing:
  strategy: earliest-reset

oauth:
  providers:
    codex:
      enable-websocket-upstream: true
```

Existing files that explicitly select `round-robin` retain that choice. Change
`routing.strategy` to `earliest-reset` to enable quota routing. Set it back to
`round-robin` to disable quota routing. Files with no routing strategy use
`earliest-reset`.

For a legacy configuration file, the Codex transport option is under
`codex.enable-websocket-upstream`.

## Account selection

The selector prefers the available Claude or Codex account whose longest quota
window resets first. For example, it repeatedly selects an account with weekly
quota left and a reset tomorrow before an account that resets in four days.
Claude model-specific weekly windows apply only to the requested model family.
The selector ignores expired reset data and does not use exhausted windows as a
reason to prefer an account.

Equal or unknown reset times use round-robin. Explicit account priority,
provider/model eligibility, cooldowns, pinned accounts, and existing session
bindings still apply. With session affinity enabled, the reset preference applies
when a session first selects an account or fails over. It does not move a healthy
session on every request.

Reset data comes from generation responses and Codex WebSocket quota events.
Refreshing quotas in the management UI also updates routing data through the v8
API. Only successful responses from the official Claude and Codex usage endpoints
can update that data. A quota refresh does not clear a cooldown or a request error.
Refresh each page of accounts before relying on reset order for the whole pool.

## WebSocket transport

The existing `GET /v1/responses` endpoint accepts a WebSocket upgrade and
`response.create` messages. A client that uses this endpoint can send several
turns over one connection. Use the normal proxy API key for authentication.

`oauth.providers.codex.enable-websocket-upstream: true` also lets an HTTP/SSE
client use the Codex WebSocket executor upstream. The option is off when omitted
and on in this fork's example configuration. Per-account `websockets: false`
forces HTTP. API-key credentials require an explicit `websockets: true` override.
Compact requests still use HTTP. Existing handshake fallback and failover rules
remain in place.

HTTP clients still make HTTP requests to the proxy. Switching their upstream
transport alone does not make the client connection persistent. The existing
executor reuses a socket when the caller supplies an execution session; requests
without an execution session use a temporary socket.

Claude subscription requests keep the existing HTTP streaming transport. This
fork does not add a Claude upstream WebSocket protocol.

The transport change uses the approach discussed in upstream PRs
[#4907](https://github.com/router-for-me/CLIProxyAPI/pull/4907) and
[#3562](https://github.com/router-for-me/CLIProxyAPI/pull/3562). It keeps this
checkout's execution lifecycle guards.

## Use the modified panel

The panel source is in the sibling management UI repository. To install a local
panel, build that repository with its documented Bun command and copy
`dist/index.html` to this server's static directory as `management.html`.
`MANAGEMENT_STATIC_PATH` can set that directory.

Set `management.disable-auto-update-panel: true` when using a locally built panel.
To use automatic downloads, publish a `management.html` release asset in the UI
fork. Creating the fork or pushing source changes does not create that asset.
