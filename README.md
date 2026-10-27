# Marvellous Motors

Local upgrade of [mordecai12-cool/marvellous-motors](https://github.com/mordecai12-cool/marvellous-motors), preserving the existing Go, SQLite, HTML template, account, order, and administrative architecture.

This milestone uses the requested sky-blue backgrounds with dark-navy accents (`#0B1739`) and white cards. It is a **local development preview**, with fictional sample inventory and indicative prices. No production payments, supplier integrations, verified dealer badges, or automatic carrier/GPS integrations are implied. Staff-reported shipment updates and customer-care replies refresh automatically.

## Open the project

Project folder: `/home/student/Marvellous-motor2`

```bash
cd /home/student/Marvellous-motor2
./open-local.sh
```

The site runs at **http://localhost:8080**. Go 1.26 is declared in `go.mod`; the Go launcher downloads the required toolchain and dependencies on first use. No Node installation, frontend build, or external image host is required.

```bash
./start-local.sh   # build and start the local server
./stop-local.sh    # stop this project's server
```

On this Linux desktop, the start script uses a transient systemd user service, so the site remains available after the terminal closes. On other systems it falls back to `nohup`. To reload Go code or inventory changes, stop and start the server. HTML, CSS, and JavaScript changes are available on browser refresh.

The server binds to `127.0.0.1:8080`. Starting the site locally does not deploy it or push to GitHub.

## Operations and delivery

A dedicated admin workspace now includes order and payment review, delivery scheduling/dispatch/handover, customers, enquiries, owner-controlled staff access, and audit logs. The two built-in owners are `marvellousmordecaiotiti@gmail.com` and `emmanlemichel2019@gmail.com`; both use `/login.html`. Approved admins can create accounts or generate email-bound invitation links from **Staff & access**. Invited admins choose their own passwords. Owners control disabling and re-enabling staff. Invitations expire after 48 hours, work once and can be revoked. See [the operations guide](docs/ADMIN_DELIVERY.md).

```bash
./open-admin.sh
```

## What's available

- Responsive homepage, collection, vehicle detail, about, contact, order, and enquiry pages.
- 50 listings, 44 distinct models, 22 makes; 7 new and 43 pre-owned sample vehicles.
- Make/text, condition, body, fuel, budget, year range, mileage, location, origin category, availability, and transmission filters.
- Sorting by price, year, and mileage; pagination; shareable search URLs.
- Saved favourites and saved searches in the current browser; comparisons of up to three vehicles.
- Vehicle photo galleries with enlargement, available exterior/interior views, attribution, specifications, features, related vehicles, and sample prices in naira.
- Customer registration and sign-in, stable customer IDs, professional dashboard/profile pages, private order history and shipment checkpoints.
- Persistent customer-care chat and staff inbox with replies, private picture/file attachments, unread counts, resolution and actual staff availability.
- Sports, Classic, Luxury, Everyday and Limited Edition collections, engine-configuration filters (including V6, V10, rotary and boxer layouts), engine details on listing cards and vehicle comparisons. See [vehicle browsing and complaint attachments](docs/VEHICLES_AND_CHAT.md).
- Correct selected-vehicle checkout with price determined on the server, including selection retention during sign-in.
- Persisted vehicle sourcing, inspection, test-drive, finance, trade-in, dealer application, and support requests.
- Signed-in customers' private request history; authenticated admin review queue and logged enquiry status updates.
- Import planning calculator using user-entered USD estimates summed in integer cents.
- Generated session secrets, HttpOnly session cookies, CSRF protection, origin checks, basic authentication rate limiting, restricted static file serving, and private receipt access.
- Local health endpoint at `/health` and read-only inventory endpoint at `/api/vehicles`.

Dealer applications are manual enquiries; they do not create verified dealers or grant inventory publishing permissions. Finance requests are enquiries, not approvals. The import calculator does not retrieve official customs assessments or live supplier rates.

## Local database and accounts

`./start-local.sh` uses `.local/preview.db` and creates tables automatically. **The repository's original `marvellous.db` and original uploaded receipts are preserved.** No production/customer data is imported into the local preview.

The local launch script enables a fictional customer account. Both requested owners are initialized in every mode from code-stored emails and bcrypt password hashes. A one-time migration also activates existing accounts for those emails with the requested initial credentials. Subsequent restarts preserve password changes. The fictional customer credentials are saved privately on this machine in:

```
.local/demo-accounts.json
```

Use the customer credentials at `/login.html`. Use either built-in owner account at `/login.html`. The file is ignored by Git and blocked from HTTP. The browser tests create one fictional order and sourcing request in the preview database; these help demonstrate the account and admin flows.

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

See [the audit](docs/PROJECT_AUDIT.md) and [the development roadmap](docs/ROADMAP.md). This milestone is ready for local review, not live commerce. Administrators configure the business bank name, account holder and 10-digit account number at **Settings** (`/admin/settings`). The latest saved details appear on both new and resumed payment pages. No fictitious bank account is configured by default. Customers can change their own password at `/account/settings`; admins change theirs at `/admin/settings`, confirming their current password. Payment provider verification/webhooks, dealer publishing, production email/password reset delivery, reservation concurrency, verified inspections, multi-currency rates, and carrier integrations remain separate milestones requiring configuration and implementation.

Current verification results and local demonstration steps are in [docs/LOCAL_VERIFICATION.md](docs/LOCAL_VERIFICATION.md).

The project branch is [`my-isolated-project` in Marvellous-motor2](https://github.com/mordecai12-cool/Marvellous-motor2/tree/my-isolated-project). No merge into `main` was performed, and the original `marvellous-motors` repository was not changed.

## Public HTTPS hosting

The hosting budget is **free only**. The full Go application is prepared for Railway from `my-isolated-project`, using its Free plan subject to account eligibility and available credits. `railpack.json` packages the application and public assets without bundling local databases, receipts or credentials. The provider account must be connected and its Free plan verified before creating services. No paid Render configuration is included, and no public deployment has been verified yet.

See [the deployment guide](docs/DEPLOYMENT.md) for account setup, production settings, persistent storage and verification.

## Admin and payment settings verification

Run `go test ./...` and `go vet ./...`. With the local server running and Python Playwright installed, run `.venv/bin/python tests/settings_browser_check.py`. This checks both built-in logins, admin creation and invitation, password updates, bank details in checkout and mobile layouts. It restores the original payment account after testing. Database-backed settings require persistent storage in deployment as described in [the deployment guide](docs/DEPLOYMENT.md).

The showroom contains 50 sample vehicles, including 30 Sports and 16 Classic entries (some cars belong to both). See [showroom operations](docs/SHOWROOM_OPERATIONS.md) for model details, photo attribution, admin inventory controls and verification.


### Customer accounts and Benue checkout

Customers register at `/register.html`; their name, email, phone and bcrypt password hash are committed to SQLite before registration succeeds. Duplicate email addresses show a clear sign-in prompt. Customers can edit their own name and phone through **My account → My profile → Edit profile**, also linked from **Settings**. Profile changes do not rewrite historical order details.

**Settings → Delete your account** requires the current password and explicit confirmation. It removes the account, chats and private support attachments, reviews, appointments, enquiries and recovery records. Existing order/payment records retain their original delivery/contact snapshots for dealership operations and are unlinked from customer access. Deletion signs the customer out; other sessions are rejected on their next request.

A fresh or previously empty payment configuration starts with **Opay · 8106333322 · Otiti Mordecai Mavellouse**, as supplied by the owner. Admins can replace it in **Admin → Settings**; future restarts preserve their changes. These are bank-transfer instructions; payment evidence still needs admin review.

Benue delivery starts at **₦2,000**, across all collection cities and vehicle types. The one-time migration preserves other existing fee amounts. Edit the route in **Admin → Pricing & routes**; `All` in Collection city matches any origin, and a blank destination city covers every city in the state. Route edits and disabling survive restarts. Other destinations still require their own confirmed rates. Order placement redirects to `/order-payment?order-id=…`; reloading this saved payment page does not create another order.

All account data needs the persistent database and upload storage described in [deployment setup](docs/DEPLOYMENT.md). Publishing source alone does not preserve data on an ephemeral hosting disk.

Additional end-to-end verification uses an isolated production database and does not modify preview customers or orders:

```bash
go build -o .local/marvellous-motors-next .
.venv/bin/python tests/customer_checkout_browser_check.py
```


### Admin-set delivery arrival estimates

After payment approval, open **Admin → Orders → Order details → Delivery arrangements**. Set the **Estimated arrival date** and optional **Estimated arrival time (Nigeria · WAT)**, then save. Customers see the estimated arrival and remaining time on their shipment tracker and order-details page. Tracker estimates refresh automatically with the existing shipment polling. Date-only estimates show today/tomorrow/days remaining; timed estimates show days/hours/minutes. Overdue estimates ask for an updated estimate, and completed deliveries show that the vehicle has arrived. Admins can revise or clear the estimate before handover. These are staff estimates, not GPS predictions. Existing date-only schedules remain supported.
