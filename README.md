# VINOVATE

**Apps for the consumer, by the consumer.**

VINOVATE is a lightweight Go storefront for directly distributing independent Android apps, games, AI tools and web apps.

## Stack

- Go
- `html/template`
- Vanilla CSS
- No Node
- No npm
- No React
- No Vite

## Security model

- Paid APKs fail closed until entitlement verification is implemented.
- APK files live outside the public static directory.
- Free downloads are routed through Go.
- Release SHA-256 values can be verified before serving.
- Path traversal is blocked with canonicalized storage paths.
- Security headers include CSP, clickjacking protection, MIME-sniffing protection and restrictive permissions policy.

## Run locally

```bash
go run .
```

Then open:

```text
http://localhost:8080
```

## Build

```bash
go test ./...
go vet ./...
go build -o vinovate .
```

## Catalog

Edit `catalog.json` to add or update apps.

For a release, place the APK in:

```text
private-apks/
```

Then set the exact APK filename and SHA-256 in `catalog.json`.

Generate a hash with:

```bash
sha256sum private-apks/your-app.apk
```

## Paid downloads

Paid apps deliberately return `402 Payment Required` until Stripe purchase entitlement verification is wired. Do not bypass this behavior in production.

## Deployment

The included Dockerfile is compatible with container hosts that provide a `PORT` environment variable, including Cloud Run.

---

**MORE FREEDOM • BETTER APPS • A BRIGHTER TOMORROW**
