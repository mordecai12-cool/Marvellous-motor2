"""Local invitation signup and payment-summary acceptance checks.

Uses fictional local staff/order evidence. The temporary staff account is disabled
and the extra unused invitation is revoked before completing the run.
"""
import json
import uuid
from pathlib import Path

from playwright.sync_api import sync_playwright

ROOT = Path(__file__).resolve().parents[1]
BASE = "http://127.0.0.1:8080"
checks, errors = [], []


def check(condition, label):
    assert condition, label
    checks.append(label)


def snapshot(page, name):
    page.evaluate("Promise.all([...document.images].map(i => i.decode().catch(() => {})))")
    page.screenshot(path=str(ROOT / ".local/previews" / name), full_page=True)
    check(page.evaluate("document.documentElement.scrollWidth <= innerWidth"), name + ": no horizontal overflow")


def sign_in(page, email, password):
    page.goto(BASE + "/admin-login.html", wait_until="networkidle")
    page.locator("input[name=email]").fill(email)
    page.locator("input[name=password]").fill(password)
    page.locator("button[type=submit]:visible").click()
    page.wait_for_url("**/admin.html")


with sync_playwright() as pw:
    browser = pw.chromium.launch(executable_path="/usr/bin/google-chrome", headless=True, args=["--no-sandbox"])
    anonymous = browser.new_context(viewport={"width": 390, "height": 844})
    signup = anonymous.new_page()
    signup.on("pageerror", lambda error: errors.append(str(error)))
    signup.goto(BASE + "/admin-signup.html", wait_until="networkidle")
    check(signup.locator("#staff-email").input_value() == "marvellousmordecaiotiti@gmail.com", "Last public signup is reserved for the second owner")
    check("No approval is required" in signup.locator(".staff-login-card").inner_text(), "Reserved owner signup activates without approval")
    snapshot(signup, "admin-signup-invitation-mobile.png")
    signup.goto(BASE + "/admin-signup.html?token=" + "a" * 64, wait_until="networkidle")
    check("invalid" in signup.locator(".login-error").inner_text(), "Unknown invitation has helpful error feedback")
    credentials = json.loads((ROOT / ".local/demo-accounts.json").read_text())
    owner_context = browser.new_context(viewport={"width": 1440, "height": 1000})
    owner = owner_context.new_page()
    owner.on("pageerror", lambda error: errors.append(str(error)))
    owner.on("dialog", lambda dialog: dialog.accept())
    sign_in(owner, credentials["admin_email"], credentials.get("admin_password", credentials["password"]))
    owner.goto(BASE + "/admin/team", wait_until="networkidle")
    suffix = uuid.uuid4().hex[:8]
    email = "fictional-invited-" + suffix + "@example.test"
    password = "FictionalOnly-" + uuid.uuid4().hex
    invite_form = owner.locator('form[action="/admin/team/invite"]')
    invite_form.locator("input[name=email]").fill(email)
    invite_form.locator("button[type=submit]:visible").click()
    owner.wait_for_load_state("networkidle")
    invitation = owner.locator("#invitation-link").input_value()
    check(invitation.startswith(BASE + "/admin-signup.html?token="), "Owner receives a complete private invitation link")
    snapshot(owner, "admin-invitation-desktop.png")
    signup.goto(invitation, wait_until="networkidle")
    check(signup.locator("#staff-email").input_value() == email, "Invitation is bound to the owner's selected email")
    check(signup.locator("#staff-email").get_attribute("readonly") is not None, "Invited email cannot be edited in the signup form")
    snapshot(signup, "admin-signup-mobile.png")
    signup.set_viewport_size({"width": 1440, "height": 1000})
    snapshot(signup, "admin-signup-desktop.png")
    signup.locator("#staff-name").fill("Fictional Invited Staff " + suffix)
    signup.locator("#signup-password").fill(password)
    signup.locator("#signup-confirm").fill("DifferentFictionalPassword!26")
    signup.locator("button[type=submit]:visible").click()
    signup.wait_for_load_state("networkidle")
    check("do not match" in signup.locator(".login-error").inner_text(), "Server rejects mismatched passwords without consuming invitation")
    signup.locator("#signup-password").fill(password)
    signup.locator("#signup-confirm").fill(password)
    signup.locator("button[type=submit]:visible").click()
    signup.wait_for_url("**/admin-login.html?notice=account-created")
    check(signup.locator(".login-success").is_visible(), "Successful signup confirms account activation and returns to sign in")
    sign_in(signup, email, password)
    check(signup.locator(".ops-sidebar").is_visible(), "Invited administrator can sign in")
    check(anonymous.request.get(BASE + "/admin/team").status == 403, "Invited administrator cannot manage owner-only staff access")
    check(owner_context.request.get(invitation).status == 400, "Invitation cannot be used a second time")
    owner.goto(BASE + "/admin/team", wait_until="networkidle")
    entry = owner.locator(".team-list article").filter(has_text=email)
    entry.locator("button[type=submit]:visible").click()
    owner.wait_for_load_state("networkidle")
    check(anonymous.request.get(BASE + "/admin/orders", max_redirects=0).status == 303, "Owner disable immediately revokes the invited administrator's session")
    revoke_email = "fictional-revoke-" + suffix + "@example.test"
    invite_form = owner.locator('form[action="/admin/team/invite"]')
    invite_form.locator("input[name=email]").fill(revoke_email)
    invite_form.locator("button[type=submit]:visible").click()
    owner.wait_for_load_state("networkidle")
    revoked_link = owner.locator("#invitation-link").input_value()
    owner.goto(BASE + "/admin/team", wait_until="networkidle")
    owner.locator(".invitation-list article").filter(has_text=revoke_email).locator("button[type=submit]:visible").click()
    owner.wait_for_load_state("networkidle")
    check(owner_context.request.get(revoked_link).status == 400, "Owner can revoke an unused signup invitation")
    snapshot(owner, "admin-team-invitations-desktop.png")
    owner.set_viewport_size({"width": 390, "height": 844})
    snapshot(owner, "admin-team-invitations-mobile.png")
    customer_context = browser.new_context(viewport={"width": 1440, "height": 1000})
    customer = customer_context.new_page()
    customer.on("pageerror", lambda error: errors.append(str(error)))
    customer.goto(BASE + "/start-order?vehicle=toyota-camry", wait_until="networkidle")
    customer.locator("#email").fill(credentials["customer_email"])
    customer.locator("#password").fill(credentials["password"])
    customer.locator("button[type=submit]:visible").click()
    customer.wait_for_url("**/order.html?vehicle=toyota-camry")
    customer.locator("#address").fill("1 Fictional Signup Acceptance Road")
    customer.locator("#city").fill("Lagos")
    customer.locator("#state").fill("Lagos")
    customer.locator("button[type=submit]:visible").click()
    customer.wait_for_url("**/order")
    order_id = customer.locator("input[name=order-id]").input_value()
    missing = customer_context.request.get(BASE + "/payment-pending.html?order-id=" + order_id)
    check("Evidence not submitted" in missing.text(), "Order without receipt does not claim receipt submission")
    reference = "FICTIONAL-SIGNUP-CHECK-" + order_id
    customer.locator("#amount-paid").fill("48500000")
    customer.locator("#reference").fill(reference)
    customer.locator("#receipt").set_input_files({"name": "fictional-signup-test.pdf", "mimeType": "application/pdf", "buffer": b"%PDF-1.4\n% Fictional local test, not a bank receipt\n%%EOF\n"})
    customer.locator("button[type=submit]:visible").click()
    customer.wait_for_url("**/payment-pending.html?order-id=" + order_id)
    check(customer.locator('.nav-actions a[href="/customer-dashboard.html"]').count() == 1, "Signed-in customer navigation shows My account")
    check("Receipt received." in customer.locator("h1").inner_text(), "Confirmation heading reflects actual receipt submission")
    check(reference in customer.locator(".receipt-details").inner_text(), "Payment summary shows the submitted payment reference")
    check(customer.locator(".receipt-state").inner_text() == "Pending review", "Submitting evidence does not mark payment approved")
    check(customer.locator('a[href="/order-payment?order-id=' + order_id + '"]').count() == 0, "Pending review does not encourage duplicate receipt submission")
    check(customer_context.request.get(BASE + "/uploads/" + customer.locator(".receipt-download").get_attribute("href").split("/")[-1]).status == 200, "Owning customer can access their submitted receipt")
    snapshot(customer, "payment-pending-updated-desktop.png")
    customer.set_viewport_size({"width": 390, "height": 844})
    snapshot(customer, "payment-pending-updated-mobile.png")
    owner.goto(BASE + "/admin/payments", wait_until="networkidle")
    card = owner.locator(".payment-review-card").filter(has_text=reference)
    card.locator('form[action="/admin/payments/verify"] button').click()
    owner.wait_for_load_state("networkidle")
    customer.reload(wait_until="networkidle")
    check("Payment approved." in customer.locator("h1").inner_text(), "Old payment-pending URL reflects the current approved order state")
    check(customer.locator(".receipt-state").inner_text() == "Approved", "Payment summary reflects manual staff approval")
    check(not errors, "No browser JavaScript errors")
    browser.close()

report = {"passed": len(checks), "checks": checks, "javascript_errors": errors}
(ROOT / ".local/signup-browser-test-results.json").write_text(json.dumps(report, indent=2) + "\n")
print(json.dumps(report, indent=2))
