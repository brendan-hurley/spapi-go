# spapi-go

A Go client for Amazon's Selling Partner API (SP-API). 63 API packages
generated from the Swagger 2.0 specs at
[amzn/selling-partner-api-models](https://github.com/amzn/selling-partner-api-models),
plus a hand-written LWA OAuth layer.

**Zero third-party dependencies** — standard library only.

## Install

```bash
go get github.com/brendan-hurley/spapi-go@latest
```

## Quick start

```go
package main

import (
    "context"
    "net/http"

    "github.com/brendan-hurley/spapi-go/auth"
    sellers "github.com/brendan-hurley/spapi-go/apis/sellers"
)

func main() {
    lwa := auth.NewClient(auth.Credentials{
        ClientID:     "amzn1.application-oa2-client.xxxx",
        ClientSecret: "xxxx",
        RefreshToken: "Atzr|xxxx",
    })

    httpClient := &http.Client{
        Transport: auth.NewRoundTripper(lwa, nil),
    }

    cfg := sellers.NewConfiguration()
    cfg.HTTPClient = httpClient
    cfg.Servers = sellers.ServerConfigurations{{URL: "https://sellingpartnerapi-na.amazon.com"}}

    client := sellers.NewAPIClient(cfg)
    resp, _, err := client.SellersAPI.GetMarketplaceParticipations(context.Background()).Execute()
    _ = resp
    _ = err
}
```

See [`examples/smoketest/main.go`](examples/smoketest/main.go) for a
working example that hits the sandbox across Orders, Listings, and
Finances.

## Authentication

SP-API requires an LWA access token in the `x-amz-access-token` header
on every request. (Amazon dropped the AWS SigV4 requirement in 2023;
this client is LWA-only.)

**Refresh-token flow** (most operations):

```go
auth.Credentials{
    ClientID:     "...",
    ClientSecret: "...",
    RefreshToken: "Atzr|...",
}
```

**Grantless flow** (Notifications, Migration, etc.):

```go
auth.Credentials{
    ClientID:     "...",
    ClientSecret: "...",
    Scopes:       []string{auth.ScopeNotifications},
}
```

**Restricted Data Tokens (RDT)** for PII-bearing operations — fetch the
RDT via the Tokens API, then override per request:

```go
ctx := auth.WithTokenOverride(context.Background(), rdtValue)
order, _, err := client.OrdersV0API.GetOrder(ctx, orderID).Execute()
```

The `RoundTripper` honors the override and skips the LWA fetch.

## API package naming

Each spec file becomes one Go package. Package names are the spec
filename with separators stripped and all-lowercase:

| Spec file | Package path |
|-----------|--------------|
| `orders_2026-01-01.json` | `apis/orders20260101` |
| `ordersV0.json` | `apis/ordersv0` |
| `catalogItems_2022-04-01.json` | `apis/catalogitems20220401` |
| `sellers.json` | `apis/sellers` |

Since the names collide in use, alias on import:

```go
import (
    orders "github.com/brendan-hurley/spapi-go/apis/orders20260101"
    catalog "github.com/brendan-hurley/spapi-go/apis/catalogitems20220401"
)
```

## Endpoints

| Region | Host |
|--------|------|
| North America | `https://sellingpartnerapi-na.amazon.com` |
| Europe        | `https://sellingpartnerapi-eu.amazon.com` |
| Far East      | `https://sellingpartnerapi-fe.amazon.com` |
| Sandbox (NA)  | `https://sandbox.sellingpartnerapi-na.amazon.com` |

## Regenerating from upstream specs

This repo tracks the upstream Amazon specs via a sync script rather
than a submodule, so the checked-in `models/` dir is the source of
truth for what's been generated.

```bash
# 1. Pull the latest specs from amzn/selling-partner-api-models.
bash scripts/sync-specs.sh

# 2. Review the diff — understand what Amazon changed.
git diff --stat models/

# 3. Regenerate the Go packages.
bash scripts/generate.sh

# 4. Confirm everything still builds.
go build ./...
go test ./auth/...

# 5. Commit specs + generated code together.
git add models/ apis/ && git commit -m "sync specs to $(grep 'commit' models/UPSTREAM | awk '{print $3}' | cut -c1-7)"
```

Requires: Node (for `npx`), Java, Python 3, Go 1.22+.

`models/UPSTREAM` records the exact upstream commit the current
`models/` snapshot came from.

## Attribution & license

The SP-API specs in `models/` are the work of Amazon, sourced from
[amzn/selling-partner-api-models](https://github.com/amzn/selling-partner-api-models)
under the Apache License 2.0. The generated Go code under `apis/` is
derived from those specs. See [`LICENSE`](LICENSE) and
[`NOTICE`](NOTICE) for the full terms.

The hand-written `auth/` package is original to this repository and
also released under the Apache License 2.0.
