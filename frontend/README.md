# Hades Frontend

Next.js web UI for the [Hades Schema Registry](../README.md).

## Prerequisites

- Node.js 22+
- npm
- Hades backend running (see [../README.md](../README.md))

## Environment variables

Copy the example and fill in your values:

```bash
cp .env.example .env.local
```

| Variable | Default | Description |
|---|---|---|
| `BACKEND_URL` | `http://localhost:50051` | Hades backend URL (server-side proxy) |
| `NEXT_PUBLIC_DOMAIN` | `localhost` | Domain shown in UI install commands |
| `REGISTRY_HOST` | `example.com` | Registry host shown in generated `buf.yaml` snippets |

> `BACKEND_URL` is server-side only. All `/api/rpc/*` requests are proxied to it via Next.js rewrites.

## Development

```bash
npm install
npm run dev       # http://localhost:3000
```

The dev server proxies `/api/rpc/:path*` → `BACKEND_URL/:path*`, so the backend must be reachable.

## Build

```bash
npm run build
npm run start     # production server on :3000
```

## Docker

```bash
# Build
docker build -t hades-frontend .

# Run
docker run -p 3000:3000 \
  -e BACKEND_URL=http://hades-backend:50051 \
  -e NEXT_PUBLIC_DOMAIN=example.com \
  -e REGISTRY_HOST=example.com \
  hades-frontend
```

## Routes

| Path | Description |
|---|---|
| `/` | Public module browser |
| `/search` | Search modules |
| `/:owner/:module` | Module detail, files, commits, diffs |
| `/login` `/signup` | Authentication |
| `/settings` | User settings (auth-gated via middleware) |

## Tech stack

- [Next.js 16](https://nextjs.org/) (App Router)
- [Connect-Web](https://connectrpc.com/docs/web/getting-started) - typed RPC client for Hades API
- [Zustand](https://zustand.docs.pmnd.rs/) - client state
- [Marked](https://marked.js.org/) + [highlight.js](https://highlightjs.org/) - proto file rendering
