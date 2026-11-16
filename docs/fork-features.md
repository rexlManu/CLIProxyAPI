# Quota routing and WebSocket transport

For a new server installation, see [Server setup](../deploy/README.md).

This fork uses the management UI from
https://github.com/rexlManu/Cli-Proxy-API-Management-Center.
The UI opens the quota ledger by default. It has provider totals, account rows,
quota bars, reset times, and an email visibility control. Dark mode uses a black
background and white primary text.

## Claude Code 2.1.321 compatibility

The default Claude software profile uses Claude Code `2.1.321`, Stainless SDK
`0.128.0`, and Node `v26.12.1`. The billing version and beta headers use the
same CLI baseline. This supports models such as Fable 5.2 that reject Claude
Code versions below `2.1.290`.

This backports these upstream changes:

- [b4e7dbd2](https://github.com/router-for-me/CLIProxyAPI/commit/b4e7dbd2c984ec4a74d829ee0425430a20146c77) updates the measured CLI profile, beta ordering, and model capability gates.
- [542a525f](https://github.com/router-for-me/CLIProxyAPI/commit/542a525fcf2a20ec920e538d94d78471b9dda2f5) recognizes native User-Agent extension fields.
- [59304330](https://github.com/router-for-me/CLIProxyAPI/commit/593043301416879f5476939045db063003928771) preserves request fields without storing them as the credential's device identity.
- [6db7d4b8](https://github.com/router-for-me/CLIProxyAPI/commit/6db7d4b839d86c5b24e17bbe73bcd4db18ef1b1a) adds Fable 5.2 preserved-thinking and timing support.
- [6876a2eb](https://github.com/router-for-me/CLIProxyAPI/commit/6876a2eb416d10235f4aac7458bcd04c751e26f7) recognizes background requests with `X-App: cli-bg`.

The fork keeps its existing native-client allowlist and opt-in Agent SDK CLI
compatibility. Quota polling uses the same profile defaults as model requests.

Explicit `upstream.claude.header-defaults` values still override the baseline.
Remove obsolete software overrides, or update the User-Agent, package version,
and runtime version together. Cached profiles normalize to the configured
baseline. No credential or cache deletion is needed.

## Agent SDK client compatibility

Upstream v8.0.36 recognizes `sdk-ts` requests as native Claude Code and passes
through their SDK identity. The optional setting below applies the existing
CLI cloak policy to `sdk-*` clients instead:

```yaml
upstream:
  claude:
    cloak-agent-sdk: true
```

The default is `false`. Interactive CLI and VS Code requests keep their native
behavior. Explicit `never` or `uncloak` credential policies still take precedence.
With cloaking active, SDK requests use the CLI User-Agent and `cc_entrypoint=cli`.
The existing cloak path can also move system instructions, alias tool names,
and adjust cache markers. Caller instructions remain part of the request unless
strict mode is enabled.
Messages, streaming Messages, and token counting use the same selection policy.
User payload rules still run after the built-in payload changes.

This is a client identity compatibility option. It does not increase a quota or
guarantee lower usage. Anthropic controls subscription accounting. See the
[Agent SDK quota report](https://media.wtf/claude-5h-window-agent-sdk.html).

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

## Persistent quota history

The proxy records Claude and Codex quota readings every five minutes. Readings
are stored indefinitely in `quota-history.jsonl` inside the auth directory.
The file contains account indexes and quota values, without account tokens.
Keep the auth volume when you deploy a replacement.

The management API exposes readings at `GET /v8/management/quota/history`.
The optional `since` query parameter uses Unix seconds and defaults to the
last 24 hours. Use `since=0` to read all stored history.

The Claude and Codex quota tabs show combined weekly capacity, historical
consumption per hour, and a forecast for the next one, three, or seven days.
Forecasts use recent consumption and provider reset times. They are estimates.
Resets and missing readings do not count as consumption. Accounts with no
reading in the last hour do not contribute to the forecast or current capacity.
History starts with the first deployment of this feature.
