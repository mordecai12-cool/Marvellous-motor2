# Public deployment: free hosting only

Source: `mordecai12-cool/Marvellous-motor2`, branch `my-isolated-project`.

Status: production preparation is tested; no authenticated hosting deployment or live public URL has been verified. The owner has authorized **$0 hosting only**. Do not upgrade a plan, create a paid subscription, or add billable services to a paid workspace.

## Railway Free setup

Railway advertises a $0 Free plan with $1 of monthly usage credit, 0.5 GB RAM and a 0.5 GB volume. Trial accounts initially receive a time-limited grant. Eligibility, remaining credit and workspace billing must be checked in the actual account before deployment. This allowance does not guarantee continuous availability. See [pricing](https://railway.com/pricing) and [trial and retention rules](https://docs.railway.com/pricing/free-trial). Trial volumes can be deleted after credits expire under the provider's retention policy; keep backups.

1. Add Railway to this assistant and sign in to the existing Railway account. Do not paste passwords or API tokens in chat. Alternatively use Railway's dashboard directly.
2. Verify the selected workspace is on **Free**, or an unupgraded free trial with no automatic paid transition. Stop if Railway requires a paid upgrade. Do not downgrade or alter an unrelated workspace.
3. Create a new project and one Go web service from `mordecai12-cool/Marvellous-motor2`. Select **`my-isolated-project`**, not `main`. Disable automatic deployment until the settings below are complete.
4. Use **Railpack**. Its Go builder reads Go 1.26 from `go.mod`, builds `out`, and uses `railpack.json` to package templates, images, fonts and browser scripts. Keep the default build command and start with `./out`. The final image excludes repository databases, receipts, source code and machine-local credentials.
5. Attach the Free plan's **0.5 GB persistent volume**, mounted at `/var/data`, to this service before its first production start. Use one replica, up to the Free plan's 512 MB RAM limit. Do not add another database service.
6. Set the environment variables in the table below. Let Railway assign `PORT`. No signing secret needs to be entered: the application generates a private session key on its persistent volume. An optional private `MM_SESSION_SECRET` must be random and at least 32 bytes long.
7. Enable **Serverless/App Sleeping**, then deploy so that it takes effect. Configure `/health` as the deployment health check, allow up to 300 seconds, and use an on-failure restart policy with at most three retries. Do not add uptime pings that keep the service awake. Close customer-care tabs when finished; chat polling keeps the app active while used.
8. Deploy, inspect build/runtime logs, and wait for a healthy deployment. Generate a Railway HTTPS domain and use the actual URL returned by Railway. Confirm that its deployed commit matches this branch.
9. Complete the public verification below. Record the real URL, deployed commit, Free plan status, storage mount and test results only after observing them.

Railpack configuration follows the official [configuration reference](https://railpack.com/config/file/) and [Go builder](https://railpack.com/languages/golang/). Railway's old `railway.json` deployment format is deprecated for new services; service settings here are applied through Railway's dashboard/API. See [configuration migration](https://docs.railway.com/infrastructure-as-code).

[Serverless](https://docs.railway.com/deployments/serverless) sleeps idle services and wakes them on requests; the first request can be slow or return a temporary 502. Active usage still consumes credits. Check usage in Railway and keep the project within the Free allowance. If the service stops because credits are exhausted, do not upgrade automatically. Back up the database and uploaded receipts before any provider retention deadline.

Render's free web service cannot persist this app's SQLite database and receipts across restarts. Do not deploy this full application there with temporary storage. No paid Render Blueprint is included. See [Render Free limitations](https://render.com/docs/free).

## Production environment

| Variable | Value |
| --- | --- |
| `MM_ENV` | `production` |
| `MM_HOST` | `0.0.0.0` |
| `PORT` | Assigned by Railway |
| `MM_DATA_DIR` | `/var/data` |
| `MM_DB_PATH` | `/var/data/marvellous.db` |
| `MM_UPLOADS_DIR` | `/var/data/uploads` |
| `MM_SECURE_COOKIES` | `true` |
| `MM_DEMO_MODE` | `false` |
| `MM_OWNER_ONLY` | `true` |
| `MM_SESSION_SECRET` | Optional random private secret; otherwise a persistent key is generated |

The templates and public assets must remain alongside the binary. Use one instance with one persistent disk; independent SQLite replicas do not share customer data. SIGTERM triggers graceful shutdown before closing the database.

A fresh production database initializes the configured owner once using its existing bcrypt hash. It does not copy local preview accounts/orders/receipts, seed fictional demo users, or reset existing passwords. The one reserved additional owner signup remains available without approval, as requested; subsequent staff must be invited or created by an owner.

Listings, prices and checkout instructions are demonstration business data until the business supplies verified details. Hosting does not configure bank verification, a payment provider, email delivery or carrier GPS.

## Verification

Local automated checks completed during preparation:

- 52 Go tests passed, including the complete suite with the race detector; `go vet ./...` passed.
- Seven isolated production checks passed: cloud port/health, real catalogue templates, secure CSRF cookie/signup, correct owner without demo customers, graceful SIGTERM, persistent database/receipt/key after restart, and blocked private HTTP paths.
- Railpack 0.41.0 successfully generated the build plan, resolving Go 1.26 and the configured runtime asset list. A separate runtime package using the compiled Go binary and that asset list passed health, showroom, stylesheet, browser-script, admin-login and support-page checks. The hosted container build has not run yet.
- These checks used a separate temporary production database. The localhost preview was preserved.

Run backend checks with:

```bash
go test -v ./...
go test -race -v ./...
go vet ./...
```

After hosting returns an actual HTTPS URL:

1. Load `/health`; require HTTP 200 with `status: ok` and `mode: production`.
2. Open the showroom and vehicle details. Check images, styles and all template fragments load.
3. Register a temporary customer, sign in, view its customer ID, and test an enquiry and support conversation.
4. Sign in as owner and reply in the admin support inbox. Verify the customer receives the reply.
5. Check signed-out requests cannot read the customer dashboard or a receipt; another customer's shipment must stay private.
6. Restart the service and verify accounts, the test conversation and an uploaded fictional receipt remain available, with the same session key on the mounted volume.
7. Check that session and CSRF cookies use Secure over public HTTPS, and private files/source remain unavailable.
8. Remove temporary records through supported account/admin flows where available; do not claim cleanup or verification that has not actually occurred.

A passing local test suite is preparation, not evidence of a public deployment. Record provider-side results after deploying.
