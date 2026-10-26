"""Check shared admin login, admin creation, password settings and payment details locally."""
import json, sqlite3, uuid
from pathlib import Path
from playwright.sync_api import sync_playwright

ROOT = Path(__file__).resolve().parents[1]
BASE = 'http://127.0.0.1:8080'
checks = []

def check(ok, label):
    assert ok, label
    checks.append(label)
    print("PASS: " + label, flush=True)

def login(page, email, password):
    page.goto(BASE + '/admin-login.html')
    page.locator('[name=email]').fill(email)
    page.locator('[name=password]').fill(password)
    page.locator('button[type=submit]').click()
    page.wait_for_url('**/admin.html')

def change_password(page, current, new):
    form = page.locator('form').filter(has=page.locator('[name=current-password]'))
    form.locator('[name=current-password]').fill(current)
    form.locator('[name=new-password]').fill(new)
    form.locator('[name=confirm-password]').fill(new)
    form.locator('button[type=submit]').click()
    page.wait_for_load_state('networkidle')
    check('Your password has been changed' in page.inner_text('main'), 'Password change confirmation')

with sqlite3.connect(ROOT / '.local/preview.db') as db:
    original = db.execute('SELECT bank_name,account_name,account_number FROM payment_account WHERE id=1').fetchone()
try:
    with sync_playwright() as pw:
        browser = pw.chromium.launch(executable_path='/usr/bin/google-chrome', headless=True, args=['--no-sandbox'])
        ctx = browser.new_context(viewport={'width':1440,'height':1000})
        page = ctx.new_page()
        errors = []
        page.on('pageerror', lambda error: errors.append(str(error)))
        login(page, 'marvellousmordecaiotiti@gmail.com', 'password')
        check(page.locator('.ops-sidebar').is_visible(), 'Marvellous built-in admin signs in')
        page.goto(BASE + '/admin/settings')
        page.locator('[name=bank-name]').fill('Fictional Browser Test Bank')
        page.locator('[name=account-name]').fill('Fictional Test Motors')
        page.locator('[name=account-number]').fill('0123456789')
        page.get_by_role('button', name='Save payment account').click()
        check('Payment account saved' in page.inner_text('main'), 'Admin saves bank details')
        page.reload()
        check(page.locator('[name=account-number]').input_value() == '0123456789', 'Account number preserves leading zero on reload')
        page.screenshot(path=str(ROOT / '.local/previews/admin-settings-desktop.png'), full_page=True)
        page.set_viewport_size({'width':390,'height':844})
        page.wait_for_timeout(300)
        check(page.evaluate('document.documentElement.scrollWidth <= innerWidth'), 'Admin settings fits mobile')
        page.screenshot(path=str(ROOT / '.local/previews/admin-settings-mobile.png'), full_page=True)
        page.set_viewport_size({'width':1440,'height':1000})
        page.goto(BASE + '/admin/team')
        email = 'settings-test-' + uuid.uuid4().hex[:8] + '@example.test'
        old, new = 'BrowserTestInitial123', 'BrowserTestChanged123'
        form = page.locator('form[action="/admin/team/create"]')
        for name, value in {'name':'Settings Test Admin','email':email,'password':old}.items():
            form.locator('[name='+name+']').fill(value)
        form.locator('button[type=submit]').click()
        check('Staff account created' in page.inner_text('main'), 'Built-in admin creates another admin')
        staffctx = browser.new_context()
        staff = staffctx.new_page()
        login(staff, email, old)
        staff.goto(BASE + '/admin/settings')
        change_password(staff, old, new)
        staff.locator('form[action="/admin-logout"] button').click()
        login(staff, email, new)
        staff.goto(BASE + '/admin/team')
        staff.locator('#invite-email').fill('invited-' + uuid.uuid4().hex[:8] + '@example.test')
        staff.locator('form[action="/admin/team/invite"] button').click()
        check(staff.locator('[name="signup-link"]').count() > 0 or '/admin-signup.html?token=' in staff.content(), 'New admin can invite another admin')
        second = browser.new_context().new_page()
        login(second, 'emmanlemichel2019@gmail.com', 'Ikese2020')
        check(second.locator('.ops-sidebar').is_visible(), 'Emmanuel built-in admin uses same login')
        creds = json.loads((ROOT / '.local/demo-accounts.json').read_text())
        customerctx = browser.new_context(viewport={'width':390,'height':844})
        customer = customerctx.new_page()
        customer.goto(BASE + '/login.html')
        customer.locator('[name=email]').fill(creds['customer_email'])
        customer.locator('[name=password]').fill(creds['password'])
        customer.locator('button[type=submit]').click()
        customer.wait_for_load_state('networkidle')
        customer.goto(BASE + '/account/settings')
        check(customer.locator('[name=current-password]').is_visible(), 'Customer has password settings')
        check(customer.evaluate('document.documentElement.scrollWidth <= innerWidth'), 'Customer settings fits mobile')
        customer.screenshot(path=str(ROOT / '.local/previews/customer-settings-mobile.png'), full_page=True)
        change_password(customer, creds['password'], 'CustomerBrowserChanged123')
        change_password(customer, 'CustomerBrowserChanged123', creds['password'])
        customer.goto(BASE + '/start-order?vehicle=toyota-camry')
        for name, value in {'address':'1 Fictional Settings Test Road','city':'Lagos','state':'Lagos'}.items():
            customer.locator('[name='+name+']').fill(value)
        customer.get_by_role('button', name='Continue to payment').click()
        customer.wait_for_load_state('networkidle')
        check('0123456789' in customer.inner_text('main'), 'New order payment shows configured account')
        order_id = customer.locator('[name=order-id]').input_value()
        customer.goto(BASE + '/order-payment?order-id=' + order_id)
        check('Fictional Browser Test Bank' in customer.inner_text('main'), 'Resumed payment shows configured bank')
        check(customer.evaluate('document.documentElement.scrollWidth <= innerWidth'), 'Payment page fits mobile')
        customer.screenshot(path=str(ROOT / '.local/previews/payment-account-mobile.png'), full_page=True)
        check(customerctx.request.get(BASE + '/admin/settings', max_redirects=0).status == 303, 'Customer cannot access admin settings')
        page.goto(BASE + '/admin/team')
        page.on('dialog', lambda dialog: dialog.accept())
        page.locator('.team-list article').filter(has_text=email).get_by_role('button', name='Disable').click()
        check(staffctx.request.get(BASE + '/admin/team', max_redirects=0).status == 303, 'Disabled admin loses access immediately')
        check(not errors, 'No JavaScript errors')
        browser.close()
finally:
    with sqlite3.connect(ROOT / '.local/preview.db') as db:
        db.execute('UPDATE payment_account SET bank_name=?,account_name=?,account_number=? WHERE id=1', original)
(ROOT / '.local/settings-browser-results.json').write_text(json.dumps(checks, indent=2) + '\n')
print(json.dumps({'passed':len(checks), 'checks':checks}, indent=2))
