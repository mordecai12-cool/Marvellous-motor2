# Operations and delivery upgrade

The local site now has a dedicated, responsive administration workspace at `/admin.html`, with a private sign-in page at `/admin-login.html`.

## Staff access

Public administrator registration and request-status lookup are closed, including the old URLs. Customer registration is separate and remains available for shopping accounts; a customer account never grants administrator access.

The local platform-owner credentials remain in `.local/demo-accounts.json`. This private, Git-ignored file is blocked from HTTP. Sign in with its administrator email and password. No credentials appear on the public website.

The owner can create administrator accounts from **Staff & access** and disable or reactivate staff. New accounts receive the ordinary administrator role. Administrators cannot create other staff, access staff management, or disable an owner. Existing owner accounts cannot be disabled through the staff screen.

Every protected admin request checks the current account status and role in the database. A disabled account is blocked immediately, including when it has an existing session cookie. Private receipt access performs the same live check. Session cookie role claims do not override the database role.

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
.venv/bin/python tests/admin_browser_check.py
.venv/bin/python tests/browser_check.py
```

Use a Python environment with Playwright installed and Google Chrome available. The admin browser test creates explicitly fictional order/payment evidence and a temporary staff account, tests the complete delivery flow, then disables the staff account. No real money or external carrier is involved. Reports and screenshots stay in `.local/`.

Live payment providers, bank verification integrations, shipping integrations, and production credential delivery remain separate configuration/development work. Manual staff approval is not a claim of automatic bank verification.

## Verified local results

All 16 backend tests pass, including the seven new tests for current staff authorisation, owner-only account creation, atomic payment review, delivery scheduling/dispatch/handover, customer order privacy, admin screen rendering, and resuming private payment-evidence submission. `go vet` and the local executable build pass.

The new admin/delivery browser suite passed 47 checks, including the complete payment-to-handover flow, customer-visible arrangements, staff creation/revocation, direct-URL access restrictions, responsive layouts, and mobile navigation. Browser reports are saved in `.local/admin-browser-test-results.json`.

The existing showroom/browser regression suite also passed all 57 checks. Both browser suites completed without JavaScript errors.
