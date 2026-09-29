# Private Endpoints POC — findings

## Probe (Task 1) — 2026-09-29

Agents (ngrok 3.39.9, authenticated with the same PAT via `NGROK_AUTHTOKEN`):

```
python3 -m http.server 18080
ngrok http 18080 --url http://pe-probe-<u>.internal
ngrok http 18080 --url https://pe-probe-<u>.ngrok.direct
```

Both started without extra flags; `.ngrok.direct` needs no special syntax.

`Endpoints.List` (ngrok-api-go v9, private endpoints only shown; other rows were public):

```
url=https://pe-probe-<u>.ngrok.direct scheme= host= port=0 type=ephemeral bindings=[internal] pooling=false
url=http://pe-probe-<u>.internal      scheme= host= port=0 type=ephemeral bindings=[internal] pooling=false
```

- `Scheme`, `Host`, `Port` are empty in list results — everything must be parsed from `URL`.
- Both TLDs report `bindings=[internal]`. Hostname-suffix filtering and a `bindings` contains
  `internal` check would select the same set; the POC uses the suffix (see plan).

Private dial (`privatedial`, `ProtocolQUIC`, `quic.connect-endpoint.ngrok.com:443`, PAT auth):

```
pe-probe-<u>.internal:80 status: 200 OK
pe-probe-<u>.ngrok.direct:443 cert subject: CN=*.ngrok.direct issuer: CN=YE2,O=Let's Encrypt,C=US dns: [*.ngrok.direct ngrok.direct]
pe-probe-<u>.ngrok.direct:443 status: 200 OK
```

- Assumption confirmed: dialing an `https://…ngrok.direct` endpoint on 443 yields a TLS session
  terminated by ngrok with a publicly trusted wildcard cert. SNI passthrough on the shared :443
  listener works for https endpoints with verification on.
- quic-go warns `failed to sufficiently increase receive buffer size` (wanted 7168 kiB, got
  2048 kiB). Harmless for the POC; in-cluster it may be worth raising `net.core.rmem_max`.
