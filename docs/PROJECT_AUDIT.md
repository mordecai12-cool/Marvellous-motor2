# Project audit — 14 October 2026

## Repository and collaboration

- Existing repository: https://github.com/mordecai12-cool/marvellous-motors
- Original default branch: `main`, baseline commit `eaef005025274ef60e0c615d35f855be3e64d22b`.
- Local development branch: `local/showroom-upgrade`.
- Recent commits identify `mordecai12-cool` as the contributor responsible for the initial car page, Camry details, image uploads, admin requests, and account workflows. The original Git history has been preserved.
- The clone was initially clean. No unrelated `mindgrow` files were modified.
- Shared issues/PRs were not changed. No new repository, deployment, push, or pull request was created.

## Original state

The application was a Go HTTP server using SQLite, Gorilla cookie sessions, bcrypt passwords, server-rendered account/admin templates, and static marketing pages. There was no README, test suite, frontend package manager, or deployment configuration.

Existing features included customer registration/sign-in, profiles, password-reset token generation, customer order tracking, uploaded payment receipts, admin payment verification, delivery status changes, and super-admin approval of admin applications.

The public showroom had three hardcoded cards. Only the Camry had a detailed page; several vehicle links went nowhere. Displayed Camry prices and years disagreed, and every new order was saved as a ₦25,000,000 Camry regardless of what the customer selected. The home template also contained duplicate trailing content.

## Issues addressed in this milestone

- Replaced hardcoded vehicle displays with shared inventory and model-specific detail URLs.
- Expanded the catalogue to 16 sample listings covering new and pre-owned cars, luxury and affordable options, sedans, SUVs, a coupe, and petrol/diesel/hybrid/electric powertrains.
- Fixed order persistence and payment summaries to use the chosen vehicle and server-side catalogue price.
- Preserved the chosen car across the login redirect.
- Added responsive blue showroom pages, useful navigation, favourites, saved searches, comparison, filtering, pagination, and clear empty states.
- Added manual sourcing and other enquiry workflows, private customer views, admin review, and enquiry audit events.
- Added an import planning calculator with explicit user-supplied estimates and integer cents.
- Removed the application-wide hardcoded session signing key; generated a private persistent local key and set session cookie attributes.
- Added CSRF tokens, request origin checks, body limits, security headers, and basic per-address authentication rate limiting.
- Replaced the root directory file server with an explicit asset/page allowlist. Source code, `.git`, database files, `.local` secrets, and raw inventory/attribution JSON are not public static resources.
- Restricted uploaded receipts to their owning customer or an authenticated administrator; blocked payment submission against another customer's order.
- Added stronger password validation and payment input validation; removed personal delivery information from order logs and password-reset tokens from logs.
- Restricted delivery state updates to their expected prior states.
- Created separate local preview storage and randomly generated fictional demo account credentials; left the original database and uploaded files unchanged.
- Added backend and real-browser checks, local start/stop/open commands, a health endpoint, asset credits, and setup documentation.

## Current database/API status

The original customer, admin, order, payment, and password-reset schemas remain in use. New `enquiries` and `enquiry_events` tables store manual requests and authorised status changes. A customer index supports account-isolated request lookup. Sample inventory is embedded from JSON and exposed through a read-only endpoint; it is not a live dealer inventory management system.

The local preview database starts without original customer records. Browser tests add explicitly fictional records. Money in the import calculator uses integer cents; inherited payment/order fields still use SQLite REAL and Go float64 and need migration before real payment processing.

## Remaining security and reliability work

- The original repository tracks a database and receipt files. They are preserved locally and blocked from HTTP here, but a production handover must remove sensitive data from deploy artifacts and assess existing repository exposure.
- Implement production email verification and password reset delivery. Token creation currently exists, but no email transport is configured.
- Complete payment transaction idempotency, receipt filesystem/database atomicity, audit logs for all financial actions, refund handling, and provider-side verification.
- Add stronger administrator role separation and session revocation; production monitoring and persistent/distributed rate limits.
- Add validated media storage with full production retention and malware handling policies where required.
- Add inventory reservations, concurrency controls, constraints/foreign keys, formal versioned migrations, and a production database/storage plan.
- Complete search/accessibility/performance testing beyond the exercised desktop/mobile routes.

## Test results

The inherited application had no test files. Initial `go test ./...` compiled successfully and reported `[no test files]`.

The new backend suite has nine tests covering catalogue integrity and assets, vehicle pages/API behavior, selected-car pricing, login selection retention, CSRF/origin rejection, receipt/payment ownership, validated and account-isolated requests, weak-password rejection, and encoded path traversal blocking. All nine passed; `go vet ./...` and the executable build passed.

The real Chrome/Playwright run passed 57 checks for filters/sorting/pagination, saved cars and searches, comparison, selected-car checkout, persistent customer orders, sourcing enquiries, admin updates, import arithmetic, private file/receipt access, desktop/mobile overflow and image loading, mobile navigation, and absence of JavaScript errors.

Reports: `.local/browser-test-results.json`. Visual review: `.local/previews/`. These are local artifacts, not committed customer data.

## Deployment readiness

Ready for local review. Not ready for live automotive commerce. No production payments, dealer verification, supplier agreements, live tracking, finance partnerships, government tax assessments, or external messaging are fabricated or enabled. Production payment and supplier credentials and business agreements were not requested or used.
