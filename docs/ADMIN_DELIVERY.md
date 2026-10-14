# Operations and delivery upgrade

The local site now has a dedicated, responsive administration workspace at `/admin.html`, with a private sign-in page at `/admin-login.html`.

## Staff access

The old administrator-request and status-lookup URLs are closed. One additional signup at `/admin-signup.html` is reserved for `marvellousmordecaiotiti@gmail.com` and activates immediately, without approval. A durable, transactional signup slot closes after that account registers; deleting or disabling an account never reopens it. Later administrators require an owner-created invitation or direct account creation. Customer registration is separate and remains available for shopping accounts; a customer account never grants administrator access.

The local platform-owner credentials remain in `.local/demo-accounts.json`. This private, Git-ignored file is blocked from HTTP. Sign in with its administrator email and password. No credentials appear on the public website.

The owner can invite administrators from **Staff & access**, create accounts directly for assisted setup, and disable or reactivate staff. New accounts receive the ordinary administrator role. Administrators cannot create other staff, access staff management, or disable an owner. Existing owner accounts cannot be disabled through the staff screen.

Every protected admin request checks the current account status and role in the database. A disabled account is blocked immediately, including when it has an existing session cookie. Private receipt access performs the same live check. Session cookie role claims do not override the database role.


## Invitation-only admin signup

1. Sign in as the platform owner at `/admin-login.html`.
2. Open **Staff & access** and enter a colleague's work email under **Bring the right people in**.
3. Generate an invitation and copy the private signup link. Share it yourself; the application does not send an email automatically.
4. The invited person opens the link, enters their full name, chooses a password and confirms it. The email is fixed by the invitation.
5. A successful signup creates an approved ordinary administrator and returns to staff sign in. The form cannot request owner privileges.

Invitations expire after 48 hours, can be used once and may be revoked by the owner. Generating a replacement invalidates earlier unused links for the same email. Only token hashes are stored; the raw link is displayed once on the protected owner screen and never put into the audit log. Signup checks the inviter's current authority, expiry and revocation again inside the account-creation transaction. Invalid input and account conflicts do not consume an invitation or overwrite an existing account. Customer accounts never grant staff access.

Before the reserved slot is consumed, `/admin-signup.html` shows the last owner signup with a fixed email. Afterwards, account creation requires a valid invitation token. The two configured owner emails may hold owner privileges; other invited or directly created staff receive ordinary administrator access. Signup retains CSRF, form-origin checks, password hashing and authentication rate limiting. Its `strict-origin` referrer policy omits the invitation path/query from referrers while allowing normal same-origin form submissions; see [MDN's Referrer-Policy documentation](https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Headers/Referrer-Policy).

## Updated payment confirmation

`/payment-pending.html?order-id=...` and the order-status page share one responsive order layout. The heading uses the actual order and latest payment-evidence state: missing, pending review, correction requested, approved, dispatched or delivered. It no longer claims a receipt exists before submission, or stays on a pending heading after approval.

The customer sees the vehicle total, latest submitted amount/reference/date, their private receipt link, progress, delivery arrangements and team updates. A receipt-submission action appears only when evidence is missing or rejected. Order ownership remains enforced before any payment details are loaded. Pending evidence is not represented as approved payment; no automatic bank verification or live carrier tracking is claimed.

## Workspace

- Overview: actual order/review counts, delivery pipeline, recent orders, and recorded staff activity.
- Orders: server-side search and stage filters, individual customer/vehicle details, and the next action.
- Payment review: protected receipt access, submitted amount and reference, manual approval, and correction requests with a customer-visible reason.
- Deliveries: preparation, dispatch, and handover views, linked to delivery arrangements.
- Enquiries: sourcing, dealer applications, inspections, finance, trade-ins, and customer support.
- Customers: a customer directory and related orders.
- Staff & access: owner-only account administration.
- Activity log: the latest staff reviews, access changes, and delivery updates.

## Delivery workflow

1. The customer submits an order with a delivery address.
2. Submitted receipt evidence remains pending until an approved staff member reviews it.
3. Payment approval and the order transition to preparation are saved atomically. Repeated reviews cannot duplicate the action. Evidence that does not cover the order total cannot be approved.
4. Staff enter the planned date, delivery partner, driver/contact, dispatch reference, and customer-visible note. Dates and field lengths are validated on the server.
5. A delivery partner and dispatch reference are required before confirming dispatch.
6. Dispatch moves only a prepared order to **Out for delivery**.
7. Handover confirmation moves only a dispatched order to **Delivered**. Repeated confirmations are rejected, and completed delivery details cannot be edited.
8. The customer sees the current stage, planned date, contact/reference details, and recorded updates in their private order timeline.

Payment reviews, saved delivery arrangements, dispatch, and handover have recorded staff identities and timestamps. Updates are manual operations records; the platform does not invent live carrier tracking.

Customer order/delivery pages enforce order ownership, including the receipt-submission confirmation page. The payment-review step is incomplete while evidence is pending.

## Storage and testing

The upgrade adds `delivery_details`, `order_events`, and `admin_activity` tables without replacing the existing database or order records. Startup creates these tables and the order-event index idempotently.

```bash
./stop-local.sh
./start-local.sh
go test -v ./...
go vet ./...
```

Optional browser checks:

```bash
.local/browser-env/bin/python tests/signup_browser_check.py
.local/browser-env/bin/python tests/admin_browser_check.py
.local/browser-env/bin/python tests/browser_check.py
```

Use a Python environment with Playwright installed and Google Chrome available. The admin browser test creates explicitly fictional order/payment evidence and a temporary staff account, tests the complete delivery flow, then disables the staff account. No real money or external carrier is involved. Reports and screenshots stay in `.local/`.

Live payment providers, bank verification integrations, shipping integrations, and production credential delivery remain separate configuration/development work. Manual staff approval is not a claim of automatic bank verification.

## Verified local results

The earlier milestone passed all 25 backend tests; the current full results are recorded in `docs/LOCAL_VERIFICATION.md`. Those earlier checks included nine invitation/payment-confirmation/navigation tests and the seven earlier tests for current staff authorisation, owner-only account creation, atomic payment review, delivery scheduling/dispatch/handover, customer order privacy, admin screen rendering, and resuming private payment-evidence submission. `go vet` and the local executable build pass.

The new admin/delivery browser suite passed 47 checks, including the complete payment-to-handover flow, customer-visible arrangements, staff creation/revocation, direct-URL access restrictions, responsive layouts, and mobile navigation. Browser reports are saved in `.local/admin-browser-test-results.json`.

The existing showroom/browser regression suite also passed all 57 checks. The invitation/signup and updated payment-summary suite passed 31 checks, including actual signup, mismatched-password retries, one-use links, revocation, staff session disabling, private receipt access, current payment status, signed-in navigation, and desktop/mobile layouts. All three browser suites completed without JavaScript errors. The new report is `.local/signup-browser-test-results.json`.

## Shipment tracking and customer care

Customers open `/shipment-tracking?order-id=...` from their orders or dashboard. Staff record locations and checkpoints (in transit, arrived at hub, out for delivery, exception) from an order's operations page. Real dispatch and handover records join this timeline. The visible tracking page refreshes every five seconds; each update identifies its actual reporting time. This is staff-reported tracking, with no simulated GPS coordinates or carrier integration. Orders and receipts remain private to their customer and approved staff.

`/support.html` and the shared chat launcher provide persistent customer-care conversations. `/admin/support` provides the staff inbox, replies, unread counts, conversation resolution/reopening, and an availability toggle. Staff heartbeat presence expires after 60 seconds, so an offline or disabled staff account is not shown as online. Replies appear through periodic refreshes (3–4 seconds). Retry identifiers prevent duplicate messages; message limits, ownership checks and plain-text rendering apply on the server and client.

## Customer identity and account pages

Every customer already has a unique database ID. The dashboard, profile and admin customer directory now display it as `MM-C000001` (with the customer's actual number). IDs stay stable across profile edits and sign-in; they are identifiers, not passwords or access tokens. Profile updates can change only the signed-in customer's name and phone, keeping email and previous order details intact.

The customer dashboard shows actual private order totals, recent orders, tracking shortcuts and customer-care replies. Dashboard, profile, editing, sign-in, recovery, staff login and signup now use coordinated sky-blue backgrounds and white cards. Invalid customer sign-in retains the styled form, email matching ignores case, and recovery responses do not reveal whether an account exists. Recovery email delivery still requires an email-provider integration; no reset link is exposed publicly.

## Vehicle photographs

The vehicle viewer offers thumbnails, enlargement, arrow-key navigation and Escape-to-close. Available front, rear, side/exterior-angle and interior photographs are stored locally with attribution links. These show representative models and trims, not verified stock imagery. All 16 listings have a viewer; a listing's existing exterior image is retained when no sourced exterior photograph is available.

Additional browser checks:

```bash
.local/browser-env/bin/python tests/shipment_chat_browser_check.py
.local/browser-env/bin/python tests/account_gallery_browser_check.py
```

The reserved owner signup itself is exercised in isolated Go test databases so the real local slot remains available. Browser tests inspect it without registering on behalf of Marvellous Otit.
