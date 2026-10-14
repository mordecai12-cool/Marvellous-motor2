# Marvellous Motors

Local upgrade of [mordecai12-cool/marvellous-motors](https://github.com/mordecai12-cool/marvellous-motors), preserving the existing Go, SQLite, HTML template, account, order, and administrative architecture.

This milestone uses a royal-blue and sky-blue design, as requested. It is a **local development preview**, with fictional sample inventory and indicative prices. No production payments, supplier integrations, verified dealer badges, or live shipment updates are implied.

## Open the project

Project folder: `/home/student/marvellous-motors`

```bash
cd /home/student/marvellous-motors
./open-local.sh
```

The site runs at **http://localhost:8080**. Go 1.26 is declared in `go.mod`; the Go launcher downloads the required toolchain and dependencies on first use. No Node installation, frontend build, or external image host is required.

```bash
./start-local.sh   # build and start the local server
./stop-local.sh    # stop this project's server
```

On this Linux desktop, the start script uses a transient systemd user service, so the site remains available after the terminal closes. On other systems it falls back to `nohup`. To reload Go code or inventory changes, stop and start the server. HTML, CSS, and JavaScript changes are available on browser refresh.

The server binds to `127.0.0.1:8080`. There is no deployment or GitHub push in this workflow.

## Operations and delivery

A dedicated admin workspace now includes order and payment review, delivery scheduling/dispatch/handover, customers, enquiries, owner-controlled staff access, and audit logs. Public admin registration is closed. See [the operations guide](docs/ADMIN_DELIVERY.md).

```bash
./open-admin.sh
```

## What's available

- Responsive homepage, collection, vehicle detail, about, contact, order, and enquiry pages.
- 16 listings, 14 distinct models, 9 makes; 7 new and 9 pre-owned vehicles.
- Make/text, condition, body, fuel, budget, year range, mileage, location, origin category, availability, and transmission filters.
- Sorting by price, year, and mileage; pagination; shareable search URLs.
- Saved favourites and saved searches in the current browser; comparisons of up to three vehicles.
- Model-specific photos, specifications, features, related vehicles, and sample prices in naira.
- Existing customer registration, sign-in, profiles, order history, and order tracking.
- Correct selected-vehicle checkout with price determined on the server, including selection retention during sign-in.
- Persisted vehicle sourcing, inspection, test-drive, finance, trade-in, dealer application, and support requests.
- Signed-in customers' private request history; authenticated admin review queue and logged enquiry status updates.
- Import planning calculator using user-entered USD estimates summed in integer cents.
- Generated session secrets, HttpOnly session cookies, CSRF protection, origin checks, basic authentication rate limiting, restricted static file serving, and private receipt access.
- Local health endpoint at `/health` and read-only inventory endpoint at `/api/vehicles`.

Dealer applications are manual enquiries; they do not create verified dealers or grant inventory publishing permissions. Finance requests are enquiries, not approvals. The import calculator does not retrieve official customs assessments or live supplier rates.

## Local database and accounts

`./start-local.sh` uses `.local/preview.db` and creates tables automatically. **The repository's original `marvellous.db` and original uploaded receipts are preserved.** No production/customer data is imported into the local preview.

The local launch script enables fictional customer and admin accounts. Their generated credentials are saved privately on this machine in:

```
.local/demo-accounts.json
```

Use the customer credentials at `/login.html` and the administrator credentials at `/admin-login.html`. The file is ignored by Git and blocked from HTTP. The browser tests create one fictional order and sourcing request in the preview database; these help demonstrate the account and admin flows.

To disable demo accounts for a fresh database:

```bash
MM_DEMO_MODE=false MM_DB_PATH=.local/clean-preview.db ./start-local.sh
```

Stop a running server first. Disabling demo seeding does not remove already-created accounts from an existing database.

## Configuration

`.env.example` documents the options; export them in your shell. The application does not automatically read `.env` files.

| Variable | Purpose |
| --- | --- |
| `MM_DB_PATH` | SQLite database path. Defaults to `.local/preview.db`, keeping the original repository database untouched. |
| `MM_SESSION_SECRET` | Optional session signing secret, at least 32 bytes. When absent, a random persistent secret is created in `.local/session-key`, mode 0600. |
| `MM_DEMO_MODE` | `true` seeds fictional local accounts; enabled by the launch script only. |
| `MM_SECURE_COOKIES` | `false` for local HTTP; use `true` only when serving through HTTPS. |

## Verification

```bash
go test -v ./...
go vet ./...
go build -o .local/marvellous-motors .
```

Optional browser checks use Python Playwright and the installed Google Chrome:

```bash
python3 -m venv .venv
.venv/bin/pip install playwright
./start-local.sh
.venv/bin/python tests/browser_check.py
```

The browser test uses generated local demo credentials and creates fictional demonstration records. Reports and screenshots are saved under `.local/`; they are ignored by Git. Backend tests use isolated temporary databases.

## Data and assets

`inventory.json` is embedded in the Go binary and is the sample inventory source. Update a listing there, then rebuild/restart. Page templates and shared `partials.html` render the same data. `showroom.js` handles filtering, saved collections, comparisons, and the cost estimator without a frontend framework. `legacy.css` preserves existing account/admin layouts; `style.css` adds the new design.

`requests.go` adds idempotent table creation and indexes for local enquiries and their admin audit events. Existing schema changes remain compatible with the repository database. This is not a migration to PostgreSQL or a full production migration system.

Photos are local files with attribution at `/image-credits.html` and in `image-credits.json`. Representative model photos can differ from the sample model year and trim. The Manrope font and its SIL Open Font License are stored in `fonts/`.

## Backup and restore

Stop the local server, copy `.local/preview.db` to a backup folder, then restart. To restore, stop the server and replace that database with your backup. Preserve `.local/session-key` if existing sessions should remain valid; resetting it signs all users out. Demo credentials and session secrets are private machine-local files; do not publish them.

## Production status

See [the audit](docs/PROJECT_AUDIT.md) and [the development roadmap](docs/ROADMAP.md). This milestone is ready for local review, not live commerce. Payment instructions in the inherited workflow still need real business configuration. Payment provider verification/webhooks, dealer publishing, production email/password reset delivery, reservation concurrency, verified inspections, multi-currency rates, and carrier integrations remain separate milestones requiring configuration and implementation.
