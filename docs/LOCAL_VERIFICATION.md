# Local verification — 14 October 2026

The saved project is `/home/student/marvellous-motors`, on `my-isolated-project`. The verification below was completed locally before the separate-branch publication recorded at the end. No deployment was published.

## Actual final results

- `go test -v ./...`: **44 tests passed**.
- `go test -race -v ./...`: **44 tests passed**, with no race warnings.
- `go vet ./...`: passed.
- Local executable rebuild and `/health`: passed.
- `git diff --check`: passed.
- Browser suites: **260 checks passed**, with no JavaScript errors.

| Browser suite | Result | JavaScript errors |
| --- | --- | --- |
| Showroom regression | 57 passed | 0 |
| Operations and delivery | 47 passed | 0 |
| Invitations and payment confirmation | 31 passed | 0 |
| Shipment tracking and chat | 30 passed | 0 |
| Accounts, customer IDs and galleries | 95 passed | 0 |

Detailed logs, JSON reports and screenshots are saved under `.local/`. Browser checks use real local pages and explicitly fictional test orders, payment evidence, enquiries, messages and staff accounts. Temporary ordinary staff accounts are disabled after the access checks. Profile tests restore the demo customer's original name and phone.

## Bugs corrected and behavior verified

- Customer email matching now ignores capitalization and surrounding whitespace.
- Invalid sign-in keeps the styled sign-in form with a useful error.
- Recovery responses do not disclose whether an email address has an account.
- Customer dashboard/profile data comes from the database and only the signed-in customer's records. Editing cannot change another customer or the sign-in email.
- Customer IDs display consistently as `MM-C000001`, using the actual database number.
- Approved ordinary staff can reply to customer care and appear online; disabling staff immediately removes access and online presence.
- Shipment milestones cannot regress, completed handovers cannot receive further checkpoints, and order/chat data remains private.
- Chat retries do not duplicate messages; unread status, resolution/reopening and persisted history work.
- Gallery images load locally on all 16 listings. Available view buttons, enlarged viewer, keyboard navigation, attribution and mobile layout work.
- Login and signup use the requested sky-blue background. Checked pages have no mobile horizontal overflow.

The reserved additional owner signup is tested with isolated test databases, including validation, immediate activation without approval, one-use closure, and no reopening after deletion. **The real reserved signup remains available for Marvellous Otit**. No account was registered on that person's behalf during browser verification.

Original preview order #8 still belongs to customer #4 and remains `payment_pending`; it was not changed by test workflows. Database backups are preserved under `.local/`.

## Run locally

```bash
cd /home/student/marvellous-motors
./start-local.sh
xdg-open http://localhost:8080
```

After Go or embedded inventory/gallery changes, rebuild by stopping and starting:

```bash
./stop-local.sh
./start-local.sh
```

## Run the checks

```bash
go test -v ./...
go test -race -v ./...
go vet ./...
.local/browser-env/bin/python tests/browser_check.py
.local/browser-env/bin/python tests/admin_browser_check.py
.local/browser-env/bin/python tests/signup_browser_check.py
.local/browser-env/bin/python tests/shipment_chat_browser_check.py
.local/browser-env/bin/python tests/account_gallery_browser_check.py
```

Run browser suites sequentially. They sign in using the private `.local/demo-accounts.json` file. Customer and administrator passwords are separate. Authentication has a per-minute rate limit; allow a minute between repeatedly rerunning sign-in-heavy suites if it is reached.

## Demonstrate the site

1. Open the collection, select a vehicle and choose available front/exterior/rear/interior thumbnails. Click **View full size**, use arrow keys and press Escape.
2. Sign in as a customer and open **My account**. Check the customer ID, actual order counts, orders and shipment links.
3. Open **My profile**, edit the name/phone and save. The ID and sign-in email stay unchanged.
4. Open an owned order and select **Track shipment**. It shows recorded checkpoints and refreshes while the page is open.
5. Open **Chat with us** or **Customer care**, and send a message about the order.
6. Sign in at `/admin-login.html` using the configured owner email and the password provided by the user. The application stores a bcrypt hash in code for first-time owner initialization and never resets an existing account at startup.
7. Open **Customer care** in operations, mark yourself available and reply. The customer receives the reply automatically; resolve or reopen the conversation.
8. From an order's operations page, enter delivery arrangements, dispatch and record a shipment checkpoint. The customer's tracking page shows the real saved update. Use a fictional demonstration order for this check.
9. The last public signup is reserved for `marvellousmordecaiotiti@gmail.com`. That person chooses a 12–72 character password and can sign in immediately. Once registered, public signup closes permanently. Owners then manage invitations/direct staff creation from **Staff & access**.

## Practical limits

Vehicle listings, prices and model photographs remain illustrative. Tracking is based on actual staff-entered checkpoints, with no carrier GPS feed. Customer care refreshes periodically rather than using WebSockets. Invitations are private links that the owner shares; automated invitation/reset email delivery and live payment-provider integration require separate configuration. The local preview is not deployed.

## Latest photo, vehicle details and colour correction — 14 October 2026

The latest changes passed all 46 backend tests under the race detector and 141 account/gallery browser checks, with no JavaScript errors. Those browser checks include all 16 vehicle pages, specification values, visible Vehicle details buttons, matched collection/gallery front images, image loading, keyboard photo navigation, private account pages and actual sky-blue backgrounds on public and customer pages. Previous workflow suite results above are from the earlier milestone.

All site blue styling now uses sky blue (`#87CEEB`) or dark navy blue (`#0B1739`), including the top bar, buttons, branding, banners, staff portal and account pages. A separate browser check verified the actual homepage background, top bar and brand colours.

The former gallery mixed photographs of different trims, colours and sample vehicles. Fourteen exterior galleries now use visually reviewed front/rear pairs from matching photo series. Unmatched side/rear pictures were removed. Collection cards use each gallery's front image. Interior images remain separately labeled model/trim references. The two listings without a matching rear photo do not display a rear-view button. Source-family validation and a regression test prevent an unrelated rear image from appearing in a gallery.

Vehicle details buttons open the listing's gallery and a specification panel covering engine/motor, car/body type, fuel, drive, transmission, make/model/trim, condition, mileage and year. Photos, Vehicle details and Features shortcuts help navigate the page.

Latest logs and review screenshots are under `.local/photo-correction/`. The server remains available at `http://localhost:8080`, and no changes were pushed.

## Completed local branch

The final navy accent is dark navy `#0B1739`, replacing the earlier bright CSS navy. Sky-blue page backgrounds remain `#87CEEB`. Browser measurements confirmed the top bar, brand and hero headings use dark navy, with readable text on the sky-blue surface. The local branch is `my-isolated-project`; its changes have not been pushed or merged into `main`. Unused mismatched sample photos are archived privately under `.local/archive/gallery-unused`; only the 40 actively displayed gallery assets are included in the branch.

## Separate-branch publication — 14 October 2026

At the user's request, `my-isolated-project` was pushed to `mordecai12-cool/Marvellous-motor2`. The initial published upgrade commit is `23b2fd4`. A documentation follow-up records the publication status. Only `refs/heads/my-isolated-project` was pushed; no merge or `main` update was performed. The original `mordecai12-cool/marvellous-motors` remote remains unchanged. The local branch tracks `motor2/my-isolated-project`.

Published branch: https://github.com/mordecai12-cool/Marvellous-motor2/tree/my-isolated-project
