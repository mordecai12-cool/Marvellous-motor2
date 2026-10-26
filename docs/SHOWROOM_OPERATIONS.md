# Showroom operations

All features use SQLite and the same server routes locally and when hosted. Restart/redeploy the compiled Go server after backend changes; updating templates alone can expose links that an older binary does not implement. Keep the database and upload directory on persistent storage; see DEPLOYMENT.md.

## Admin workspace

Use the regular login. Approved admins have access to inventory, route pricing, appointments, sales follow-ups, complaints, the dealership profile and customer reviews. Customers cannot grant staff access. Only owners change staff access status.

- **Inventory**: create/edit vehicles, mark Draft/Available/Reserved/Sold, upload actual stock photos and condition/service history, record VIN and previous owners, tag Everyday/Sports/Luxury/Classic/Limited Edition. Drafts stay private. Private documents are accessible to staff and the actual buyer after payment approval. Public inventory omits private document metadata. Cancellation of an unpaid order releases its reservation; handover marks stock sold.
- **Collections**: one compact dropdown, with engine filters separate. Active filters can be removed individually. A conflicting filter offers a collection-only view. Existing sample inventory includes a Sports Porsche; Classic and Limited Edition stay empty until genuinely matching vehicles are added. No ordinary car is labelled limited edition to fill a category.
- **Appointments**: admins publish non-overlapping test-drive/inspection slots. Customers book a slot and staff confirm/cancel/complete it. Booking creates a sales lead. Confirmed bookings offer a private calendar file with a one-hour reminder. Dashboard notifications are onsite; there is no automatic email/SMS service.
- **Sales**: New, Contacted, Appointment, Negotiating and Sold stages, approved staff assignment, internal notes and follow-up dates.
- **Complaints**: a private case number, staff assignment, Open/Investigating/Awaiting customer/Resolved status, and customer/staff picture or document attachments.
- **Dealership**: enter real contact/address/opening hours/story/team. Reviews require a delivered order, explicit publication consent, and admin moderation; no fabricated testimonials.
- **Settings**: password changes and the bank name/account holder/10-digit payment account displayed at checkout.

## Automatic delivered prices

Admins configure origin, destination country/state/city, optional body type, delivery, paperwork, fixed insurance/taxes/port costs, insurance/duty/VAT percentages, validity, source and completeness. Percentage defaults are zero, not statutory rates. Admins must determine charges applicable to each route. Dates use Nigeria time for appointments and UTC for expiring recovery links.

The server applies any current vehicle offer, sums itemised charges in integer kobo, and freezes the quote at checkout. It rejects manipulated/stale prices and unavailable stock. The destination survives login. Incomplete or unconfirmed routes show a known subtotal and block checkout; unsupported destinations are never silently free. Real checkout requires a complete, confirmed rate. Local demo mode can use explicitly labelled sample rates.

Research references (26 October 2026), not binding carrier quotations:

- [Travo NG domestic guide](https://services.travo.ng/nigeria/logistics/car-delivery-service-in-nigeria-nationwide-vehicle-transport-by-travo-ng/): Lagos–Ibadan ₦60k–₦120k, Benin ₦120k–₦220k, Abuja ₦180k–₦350k+, Port Harcourt ₦200k–₦400k, Enugu ₦220k–₦420k. Seeded figures use indicative midpoints. Lagos ₦50k is a planning assumption, not a sourced quote. Actual distance, vehicle size, fuel and loading arrangements change costs.
- [ShipCars Africa to UK](https://shipcars.co.uk/africa-to-uk/): container service from £2,200; excludes additional destination charges. Seeded ₦3,960,000 uses a planning conversion of ₦1,800/£, not a live exchange rate, and remains incomplete. [Exchange-rate reference](https://ng.investing.com/currencies/gbp-ngn-historical-data).
- Nigeria to Ghana, USA and Canada: no verified vehicle tariff was found. Placeholders are inactive for ordering until staff provide full confirmed rates. No destination tax or customs advice is implied by reference figures.

## Assisted password recovery

The public forgot-password form records a request without revealing whether an account exists. It does not claim an email was sent. Under Account recovery, an approved admin verifies the customer's identity through established contact details, records a verification note and re-enters their own password. A new 30-minute, single-use link replaces the previous one. Share it privately through the customer's established contact channel; the application does not send messages automatically.

Only a SHA-256 hash of the random 256-bit secret is stored. The customer chooses a new password, old sessions are revoked, and outstanding recovery links are invalidated atomically. Tokens never appear in audit logs; staff cannot view existing passwords. Issuance is audited. Customer recovery does not reset admin accounts. Staff accounts remain managed by owners; seeded owner passwords are never automatically restored over changed passwords. Follow [OWASP recovery guidance](https://cheatsheetseries.owasp.org/cheatsheets/Forgot_Password_Cheat_Sheet.html).

## Verification

Run `go test ./...` and `go vet ./...`. Browser checks require Python Playwright and Chrome:

```
.venv/bin/python tests/business_browser_check.py
.venv/bin/python tests/catalogue_recovery_browser_check.py
```

Both scripts create isolated production-mode databases and uploads, then remove them. They do not change localhost stock or business settings. The business workflow covers 35 checks, including all new screens, draft privacy, actual photos/documents, duplicate registration, pricing/discounts/percentages, login destination retention, booking/calendar, CRM, saved checkout total, reservation/payment/handover, verified buyer moderation, complaints and mobile layouts. The catalogue/recovery checks cover the compact dropdown, all vehicles, filter conflict recovery, no fabricated classic/limited stock, admin routes and the end-to-end reset/session invalidation flow.
