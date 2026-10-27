"""Check customer signup, profile, Benue checkout and deletion in an isolated production database.
Run after: go build -o .local/marvellous-motors-next .
"""
import os
import sqlite3
import subprocess
import tempfile
import time
import urllib.request
from pathlib import Path
from playwright.sync_api import sync_playwright

ROOT = Path(__file__).resolve().parents[1]
BASE = 'http://127.0.0.1:8082'
BINARY = ROOT / '.local/marvellous-motors-next'


def check(condition, message):
    assert condition, message
    print('PASS: ' + message, flush=True)


with tempfile.TemporaryDirectory(prefix='mm-checkout-') as directory:
    database = Path(directory) / 'marvellous.db'
    env = dict(os.environ, PORT='8082', MM_HOST='127.0.0.1', MM_ENV='production',
               MM_SECURE_COOKIES='false', MM_DEMO_MODE='false', MM_DATA_DIR=directory,
               MM_DB_PATH=str(database), MM_UPLOADS_DIR=str(Path(directory) / 'uploads'))
    log = open(Path(directory) / 'server.log', 'w+')
    process = None

    def start():
        global process
        process = subprocess.Popen([str(BINARY)], cwd=ROOT, env=env, stdout=log, stderr=log)
        for _ in range(100):
            if process.poll() is not None:
                log.seek(0)
                raise RuntimeError(log.read())
            try:
                if urllib.request.urlopen(BASE + '/health', timeout=1).status == 200:
                    return
            except OSError:
                time.sleep(.1)
        raise RuntimeError('Server did not become ready')

    def stop():
        process.terminate()
        process.wait(timeout=25)

    try:
        start()
        with sync_playwright() as pw:
            browser = pw.chromium.launch(executable_path='/usr/bin/google-chrome', headless=True,
                                        args=['--no-sandbox', '--disable-gpu', '--renderer-process-limit=1'])
            context = browser.new_context(viewport={'width': 1365, 'height': 900})
            page = context.new_page()
            errors = []
            page.on('pageerror', lambda error: errors.append(str(error)))
            page.goto(BASE + '/register.html')
            for name, value in {'name': 'Browser Test Driver', 'email': 'checkout@example.test',
                                'phone': '08012345678', 'password': 'BrowserFictional!26',
                                'confirm-password': 'BrowserFictional!26'}.items():
                page.locator(f'[name="{name}"]').fill(value)
            page.locator('button[type=submit]').click()
            page.wait_for_url('**/login.html?notice=registered')
            check('registered' in page.url, 'Customer registration succeeds')
            page.locator('[name=email]').fill('checkout@example.test')
            page.locator('[name=password]').fill('BrowserFictional!26')
            page.locator('button[type=submit]').click()
            page.wait_for_url('**/customer-dashboard.html')
            page.goto(BASE + '/account/settings')
            page.get_by_role('link', name='Edit my profile').click()
            page.locator('[name=name]').fill('Updated Browser Driver')
            page.locator('[name=phone]').fill('+234 801 234 5678')
            page.get_by_role('button', name='Save profile changes').click()
            page.wait_for_url('**/customer-profile?notice=saved')
            check('Updated Browser Driver' in page.inner_text('main'), 'Customer edits their own profile')
            stop()
            start()
            page.reload()
            check('Updated Browser Driver' in page.inner_text('main'), 'Account, session and profile survive server restart')
            page.goto(BASE + '/quote?vehicle=mercedes-c300')
            page.locator('[name=state]').fill('Benue')
            page.locator('[name=city]').fill('Otukpo')
            page.get_by_role('button', name='Calculate delivered price').click()
            page.locator('[data-quote-order]').wait_for(state='visible')
            check('2,000.00' in page.locator('[data-quote-lines]').inner_text(), 'Confirmed Benue delivery calculates at ₦2,000')
            page.locator('[data-quote-order]').click()
            page.wait_for_url('**/order.html?vehicle=mercedes-c300')
            page.wait_for_function("document.querySelector('[name=\"quote-token\"]').value.length > 0")
            check(page.locator('[name=state]').input_value() == 'Benue', 'Quote destination carries through to checkout')
            token = page.locator('[name=quote-token]').input_value()
            page.locator('[name=address]').fill('1 Fictional Test Street')
            check(page.locator('[name=quote-token]').input_value() == token,
                  'Typing the street address preserves the ready quote')
            page.get_by_role('button', name='Continue to payment').click()
            page.wait_for_url('**/order-payment?order-id=*')
            text = page.inner_text('main')
            check('8106333322' in text and 'Opay' in text and 'Otiti Mordecai Mavellouse' in text,
                  'Continue to payment opens saved order with supplied Opay details')
            page.reload()
            with sqlite3.connect(database) as db:
                check(db.execute('SELECT COUNT(*) FROM orders').fetchone()[0] == 1,
                      'Refreshing payment does not create another order')
                check(db.execute('SELECT name,phone FROM customers WHERE email=?', ('checkout@example.test',)).fetchone()
                      == ('Updated Browser Driver', '+234 801 234 5678'), 'Registration and profile details persist in SQLite')
            page.set_viewport_size({'width': 390, 'height': 844})
            check(page.evaluate('document.documentElement.scrollWidth <= innerWidth'), 'Payment page fits mobile screen')
            page.goto(BASE + '/account/settings')
            check(page.evaluate('document.documentElement.scrollWidth <= innerWidth'), 'Settings page fits mobile screen')
            (ROOT / '.local/previews').mkdir(parents=True, exist_ok=True)
            page.screenshot(path=str(ROOT / '.local/previews/customer-settings-mobile.png'), full_page=True)
            form = page.locator('form').filter(has=page.locator('[name=delete-password]'))
            form.locator('[name=delete-password]').fill('BrowserFictional!26')
            form.locator('[name=confirm-delete]').check()
            form.get_by_role('button', name='Permanently delete my account').click()
            page.wait_for_url('**/login.html')
            with sqlite3.connect(database) as db:
                check(db.execute('SELECT COUNT(*) FROM customers').fetchone()[0] == 0,
                      'Confirmed deletion removes the customer account')
                check(db.execute('SELECT customer_id FROM orders').fetchone()[0] is None,
                      'Deletion preserves dealership order records and unlinks customer access')
            page.goto(BASE + '/customer-dashboard.html')
            page.wait_for_url('**/login.html')
            check(not errors, 'No browser JavaScript errors')
            browser.close()
    finally:
        if process and process.poll() is None:
            stop()
        log.close()
