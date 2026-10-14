"""Exercise the local, owner-controlled operations and delivery workflow."""
import json,time,uuid
from pathlib import Path
from playwright.sync_api import sync_playwright
ROOT=Path(__file__).resolve().parents[1]
BASE='http://127.0.0.1:8080'
checks=[];errors=[]
def check(ok,label):
 assert ok,label
 checks.append(label)
def snap(page,name):
 page.evaluate("document.querySelectorAll('img').forEach(i=>i.loading='eager')")
 page.evaluate("Promise.all([...document.images].map(i=>i.decode().catch(()=>{})))")
 page.screenshot(path=str(ROOT/'.local/previews'/name),full_page=True)
 check(page.evaluate('document.documentElement.scrollWidth<=innerWidth'),name+': no horizontal overflow')
def sign_in(page,email,password):
 page.goto(BASE+'/admin-login.html',wait_until='networkidle');page.locator('input[name=email]').fill(email);page.locator('input[name=password]').fill(password);page.locator('button[type=submit]').click();page.wait_for_url('**/admin.html')
with sync_playwright() as pw:
 browser=pw.chromium.launch(executable_path='/usr/bin/google-chrome',headless=True,args=['--no-sandbox'])
 customer=browser.new_context(viewport={'width':1440,'height':1000});page=customer.new_page();page.on('pageerror',lambda e:errors.append(str(e)))
 credentials=json.loads((ROOT/'.local/demo-accounts.json').read_text())
 page.goto(BASE+'/start-order?vehicle=toyota-camry',wait_until='networkidle');page.locator('#email').fill(credentials['customer_email']);page.locator('#password').fill(credentials['password']);page.locator('button[type=submit]').click();page.wait_for_url('**/order.html?vehicle=toyota-camry')
 snap(page,'delivery-checkout-desktop.png')
 page.locator('#address').fill('1 Fictional Operations Test Road');page.locator('#city').fill('Lagos');page.locator('#state').fill('Lagos');page.locator('button[type=submit]').click();page.wait_for_url('**/order')
 order_id=page.locator('input[name=order-id]').input_value();snap(page,'payment-review-customer-desktop.png')
 page.locator('#amount-paid').fill('48500000');page.locator('#reference').fill('FICTIONAL-LOCAL-TEST-'+order_id)
 receipt=b'%PDF-1.4\n% Fictional local testing document - not a bank receipt\n1 0 obj << /Type /Catalog >> endobj\n%%EOF\n'
 page.locator('#receipt').set_input_files({'name':'fictional-local-test.pdf','mimeType':'application/pdf','buffer':receipt});page.locator('button[type=submit]').click();page.wait_for_url('**/payment-pending.html?order-id='+order_id)
 check(page.locator('.progress-step.current').inner_text().startswith('02'),'Payment evidence is awaiting review, not automatically approved')
 owner=browser.new_context(viewport={'width':1440,'height':1000});admin=owner.new_page();admin.on('pageerror',lambda e:errors.append(str(e)));admin.on('dialog',lambda d:d.accept())
 admin.goto(BASE+'/admin-login.html',wait_until='networkidle');snap(admin,'admin-login-desktop.png')
 check(admin.locator('a[href="/admin-request.html"]').count()==0,'Staff login has no public registration link')
 sign_in(admin,credentials['admin_email'],credentials['password'])
 check(admin.locator('.ops-sidebar').is_visible(),'Owner can enter the new operations workspace')
 admin.goto(BASE+'/admin/payments',wait_until='networkidle');card=admin.locator('.payment-review-card').filter(has=admin.locator('input[name=payment-id]')).filter(has_text='FICTIONAL-LOCAL-TEST-'+order_id)
 card.locator('form[action="/admin/payments/verify"] button').click();admin.wait_for_load_state('networkidle')
 check('approved' in admin.locator('.ops-alert').inner_text(),'Payment evidence is manually approved')
 admin.goto(BASE+'/admin/order?id='+order_id,wait_until='networkidle');snap(admin,'admin-order-desktop.png')
 fields={'scheduled-date':'2026-10-20','partner':'Fictional Test Logistics','driver':'Demo Driver','driver-phone':'08000000000','reference':'DEMO-DISPATCH-'+order_id,'note':'Fictional local test: your vehicle handover is planned for 20 October.'}
 for field,value in fields.items():admin.locator('.delivery-planning-form [name="'+field+'"]').fill(value)
 admin.locator('.delivery-planning-form button[type=submit]').click();admin.wait_for_load_state('networkidle')
 check('Delivery details saved' in admin.locator('.ops-alert').inner_text(),'Delivery scheduling and dispatch details save')
 page.goto(BASE+'/order-status?order-id='+order_id,wait_until='networkidle')
 check('DEMO-DISPATCH-'+order_id in page.locator('.delivery-destination').inner_text(),'Dispatch reference is visible to the owning customer')
 check('20 Oct 2026' in page.locator('.delivery-destination').inner_text(),'Planned delivery date is visible to the customer')
 admin.locator('form[action="/admin/orders/delivery"] button').click();admin.wait_for_load_state('networkidle')
 check('Vehicle dispatched' in admin.locator('.ops-alert').inner_text(),'Dispatch is confirmed by staff')
 page.reload(wait_until='networkidle');check(page.locator('.delivery-status').inner_text()=='Out for delivery','Customer sees the dispatched stage')
 snap(page,'delivery-tracking-desktop.png')
 admin.locator('form[action="/admin/orders/delivered"] button').click();admin.wait_for_load_state('networkidle')
 check('Delivery confirmed' in admin.locator('.ops-alert').inner_text(),'Staff handover action is recorded')
 page.reload(wait_until='networkidle');check(page.locator('.delivery-status').inner_text()=='Delivered','Customer sees completed handover')
 check(page.locator('.customer-event-list article').count()==5,'Payment review, scheduling, dispatch, handover and submission appear in order history')
 for route,name in [('/admin.html','admin-overview-desktop.png'),('/admin/deliveries','admin-deliveries-desktop.png'),('/admin/payments','admin-payments-desktop.png'),('/admin/enquiries','admin-enquiries-desktop.png'),('/admin/team','admin-team-desktop.png')]:admin.goto(BASE+route,wait_until='networkidle');check(admin.locator('.ops-main h1').count()==1,route+': renders');snap(admin,name)
 admin.goto(BASE+'/admin/team',wait_until='networkidle');suffix=uuid.uuid4().hex[:8];email='local-test-'+suffix+'@example.test';password='TestOnly-'+uuid.uuid4().hex
 form=admin.locator('form[action="/admin/team/create"]');form.locator('[name=name]').fill('Fictional Test Staff '+suffix);form.locator('[name=email]').fill(email);form.locator('[name=password]').fill(password);form.locator('button[type=submit]').click();admin.wait_for_load_state('networkidle')
 check('Staff account created' in admin.locator('.ops-alert').inner_text(),'Owner can create an administrator')
 staff_context=browser.new_context(viewport={'width':390,'height':844});staff=staff_context.new_page();sign_in(staff,email,password)
 check(staff.locator('a[href="/admin/team"]').count()==0,'Ordinary staff do not see owner access management')
 response=staff_context.request.get(BASE+'/admin/team');check(response.status==403,'Staff cannot access owner management by direct URL')
 snap(staff,'admin-overview-mobile.png');staff.locator('.ops-menu').click();check(staff.locator('.ops-sidebar').is_visible(),'Mobile operations navigation opens')
 staff.locator('.ops-navigation a[href="/admin/deliveries"]').click();staff.wait_for_load_state('networkidle');snap(staff,'admin-deliveries-mobile.png')
 entry=admin.locator('.team-list article').filter(has_text=email);entry.locator('button[type=submit]').click();admin.wait_for_load_state('networkidle')
 response=staff_context.request.get(BASE+'/admin/orders',max_redirects=0);check(response.status==303,'Disabling staff blocks its already-signed-in session immediately')
 anonymous=browser.new_context(viewport={'width':390,'height':844});mobile=anonymous.new_page();mobile.on('pageerror',lambda e:errors.append(str(e)))
 for route in ['/admin.html','/admin/orders','/admin/payments','/admin/deliveries','/admin/customers','/admin/team','/admin/enquiries','/admin/activity']:check(anonymous.request.get(BASE+route,max_redirects=0).status==303,'Anonymous access blocked: '+route)
 check(anonymous.request.get(BASE+'/admin-request.html').status==403,'Public administrator registration page is closed')
 mobile.goto(BASE+'/admin-login.html',wait_until='networkidle');snap(mobile,'admin-login-mobile.png')
 page.set_viewport_size({'width':390,'height':844});page.reload(wait_until='networkidle');snap(page,'delivery-tracking-mobile.png')
 check(customer.request.get(BASE+'/admin/orders',max_redirects=0).status==303,'A signed-in customer cannot enter operations')
 check(not errors,'No browser JavaScript errors')
 browser.close()
report={'passed':len(checks),'checks':checks,'javascript_errors':errors}
(ROOT/'.local/admin-browser-test-results.json').write_text(json.dumps(report,indent=2)+'\n');print(json.dumps(report,indent=2))
