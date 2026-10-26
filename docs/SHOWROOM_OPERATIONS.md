# Showroom operations

All features use SQLite and the same server routes locally and when hosted. Restart/redeploy the compiled Go server after backend changes; updating templates alone can expose links that an older binary does not implement. Keep the database and upload directory on persistent storage; see DEPLOYMENT.md.

## Admin workspace

Use the regular login. Approved admins have access to inventory, route pricing, appointments, sales follow-ups, complaints, the dealership profile and customer reviews. Customers cannot grant staff access. Only owners change staff access status.

- **Inventory**: create/edit vehicles, mark Draft/Available/Reserved/Sold, upload actual stock photos and condition/service history, record VIN and previous owners, tag Everyday/Sports/Luxury/Classic/Limited Edition. Drafts stay private. Private documents are accessible to staff and the actual buyer after payment approval. Public inventory omits private document metadata. Cancellation of an unpaid order releases its reservation; handover marks stock sold.
- **Collections**: one compact dropdown, with engine filters separate. Active filters can be removed individually. A conflicting filter offers a collection-only view. The 50 sample listings include 30 Sports models, 16 Classics and two limited-production models: BMW M4 CSL and Porsche 911 Sport Classic. Categories overlap. Prices/mileage are illustrative and licensed photos are representative models, not dealership stock.
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

Both scripts create isolated production-mode databases and uploads, then remove them. They do not change localhost stock or business settings. The business workflow covers 35 checks, including all new screens, draft privacy, actual photos/documents, duplicate registration, pricing/discounts/percentages, login destination retention, booking/calendar, CRM, saved checkout total, reservation/payment/handover, verified buyer moderation, complaints and mobile layouts. The catalogue/recovery checks cover the compact dropdown, all vehicles, filter conflict recovery, correctly tagged classic/limited sample models, admin routes and the end-to-end reset/session invalidation flow.

## Expanded sample collections

Seven additional models are seeded in code and inserted without overwriting existing inventory or admin edits: 1967 Ford Mustang 289 V8 convertible, 1989 Mercedes-Benz 300 SL R107, 1965 Volkswagen Beetle 1200, 2024 Ford Mustang GT, 2024 Toyota GR Supra 3.0, 2019 Mazda MX-5 2.0 and 2023 Porsche 911 Sport Classic. The Mustang classic belongs to both Classic and Sports. The Porsche is Sports/Luxury/Limited Edition and is not tagged Classic merely because its model name contains that word.

These are browsing/test examples, not confirmed stock: prices and mileage are illustrative, owners/history are unknown, and each page identifies representative model photography. Existing stock states and edits remain intact across restarts. The Beetle uses Flat4, distinct from an inline four-cylinder; manual transmissions and years back to 1965 are selectable. Production and local builds use the same embedded inventory and licensed local assets.

Model references: [Toyota 2024 Supra](https://pressroom.toyota.com/powerful-performance-striking-style-the-2024-toyota-gr-supra/), [Ford GT V8](https://www.ford.com/product/50l-gen-4-mustang-crate-engine-auto-p2905862210), [Mazda 2019 MX-5](https://news.mazdausa.com/2018-06-20-2019-mazda-mx-5-miata-packs-more-punch-features), [Volkswagen Beetle history](https://www.volkswagen-newsroom.com/en/the-volkswagen-beetle-a-success-story-2341/vw-beetle-through-the-years-2361), [Mercedes R107 owner's manual](https://www.mercedes-benz-classic-store.com/en/library/technical-literature/operating-instructions/1060/mercedes-benz-owner-s-manual-r-107-300-420-500-sl-1985-1989), [Porsche Sport Classic specifications and 1,250-unit production](https://newsroom.porsche.com/en_US/2022/products/porsche-911-sport-classic-world-premiere-28165.html). Wikimedia Commons original photo/author/licence details are stored in vehicle-gallery.json and image-credits.json and displayed on /image-credits.html. Representative trim differences are disclosed rather than copying a photographed car's history or identity.

## Fifty-car collection update

The catalogue now contains exactly 50 distinct sample listings. The latest 27 additions comprise 14 modern sports cars and 13 classics; ten of the classics are also tagged Sports. In total: Sports 30, Classic 16, Luxury 16, Everyday 10 and Limited Edition 2 (categories overlap). Existing inventory edits and status changes remain preserved by insert-only initialization. Admins can continue adding their actual inventory beyond the sample collection.

New modern models: BMW M4 CSL and M2, Audi R8 V10 and TT, Nissan 370Z NISMO and GT-R, Chevrolet Corvette C7 and Camaro SS Convertible, Dodge Challenger R/T, Ford Mustang EcoBoost, Mercedes-AMG GT, Toyota GT86, Subaru BRZ and Mazda RX-8.

New classics: Porsche 944 and 928 S4, BMW 325i Cabriolet and 635 CSi, Mercedes-Benz 280 SE and 230 E, Jaguar E-Type, MG MGB Roadster, Triumph TR6, Volvo 1800 S, Ford Capri 3000 GT, Chevrolet Corvette C3 and Datsun 240Z.

V10 and Rotary options now work in both public engine filters and the admin inventory editor. The banner shows the actual inventory count. Existing compact filters, classic year options, sample disclosures, local image assets and photographer/licence credits remain in place.

Additional manufacturer/heritage references: [BMW M4 CSL](https://www.press.bmwgroup.com/usa/article/detail/T0392153EN_US/the-bmw-m4-csl?language=en_US), [BMW 325i](https://www.bmwgroup-classic.com/en/models/bmw-classics/product-description-page.ad-110-1.bmw-325i-e30.html), [Audi 2017 model specifications](https://www.audi.com/content/dam/gbp2/downloads/report/annual-reports/2017/en/Financial_part_2017.pdf), [Nissan 370Z](https://www.nissanusa.com/vehicles/discontinued/370z-coupe.html), [Mazda RX-8 rotary engine](https://news.mazdausa.com/download/RX-8-Full-line-release-Final.pdf), [Toyota 2017 86](https://www.toyota.co.nz/globalassets/documents/brochures/2017/86-brochure-05-17.pdf), [Ford 2018 Mustang](https://media.ford.com/content/dam/fordmedia/North%20America/US/product/2018/mustang/18-mustang.pdf), [Mercedes-AMG GT](https://media.mbusa.com/news/my2016-mercedes-amg-gt-gt-s-quick-reference-guide), [Dodge Challenger trim/engine catalogue](https://store.mopar.com/v-2015-dodge-challenger), [Porsche transaxle heritage](https://newsroom.porsche.com/en/press-kits/exhibition-kit-forever-young.html), [Jaguar E-Type heritage](https://www.jaguarheritage.com/vehicle-collection/1961-jaguar-e-type-series-1-3-8-litre-77-rw/), [MG MGB homologation](https://historicdb.fia.com/make/mg?page=1), [Volvo P1800 heritage](https://www.volvocars.com/us/cars/legacy-models/p1800-1800/). Manufacturer references describe model families; the sample data is not evidence of actual stock specifications or condition.
