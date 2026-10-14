"""Exercise actual customer/staff chat and shipment updates on the local preview."""
import json,uuid
from datetime import date,timedelta
from pathlib import Path
from playwright.sync_api import sync_playwright,expect
ROOT=Path(__file__).resolve().parents[1];BASE='http://127.0.0.1:8080';checks=[];errors=[]
def check(ok,label):
    assert ok,label
    checks.append(label)
def snapshot(page,name):
    page.evaluate("Promise.all([...document.images].map(i=>i.decode().catch(()=>{})))")
    page.screenshot(path=str(ROOT/'.local/previews'/name),full_page=True)
    check(page.evaluate('document.documentElement.scrollWidth<=innerWidth'),name+': no horizontal overflow')
with sync_playwright() as pw:
    browser=pw.chromium.launch(executable_path='/usr/bin/google-chrome',headless=True,args=['--no-sandbox'])
    guest_context=browser.new_context(viewport={'width':390,'height':844});guest=guest_context.new_page();guest.on('pageerror',lambda e:errors.append(str(e)))
    guest.goto(BASE+'/support.html',wait_until='networkidle');expect(guest.locator('.care-gate')).to_be_visible()
    check(guest.locator('[data-support-widget]').count()==1,'Customer-care page has one chat panel')
    check(guest.locator('.care-gate a[href="/login.html"]').is_visible(),'Guests sign in before entering private chat')
    snapshot(guest,'customer-care-guest-mobile.png')
    credentials=json.loads((ROOT/'.local/demo-accounts.json').read_text());suffix=uuid.uuid4().hex[:8]
    customer_context=browser.new_context(viewport={'width':1440,'height':1000});customer=customer_context.new_page();customer.on('pageerror',lambda e:errors.append(str(e)))
    customer.goto(BASE+'/start-order?vehicle=toyota-camry',wait_until='networkidle');customer.locator('#email').fill(credentials['customer_email']);customer.locator('#password').fill(credentials['password']);customer.locator('button[type=submit]:visible').click();customer.wait_for_url('**/order.html?vehicle=toyota-camry')
    customer.locator('#address').fill('1 Fictional Shipment Chat Test Road');customer.locator('#city').fill('Lagos');customer.locator('#state').fill('Lagos');customer.locator('button[type=submit]:visible').click();customer.wait_for_url('**/order')
    order_id=customer.locator('input[name=order-id]').input_value();reference='FICTIONAL-SHIPMENT-CHAT-'+suffix
    customer.locator('#amount-paid').fill('48500000');customer.locator('#reference').fill(reference);customer.locator('#receipt').set_input_files({'name':'fictional-shipment-chat.pdf','mimeType':'application/pdf','buffer':b'%PDF-1.4\n% Fictional local test, not a bank receipt\n%%EOF\n'});customer.locator('button[type=submit]:visible').click();customer.wait_for_url('**/payment-pending.html?order-id='+order_id)
    customer.locator('.confirmation-actions [data-open-chat]').click();expect(customer.locator('.care-compose textarea')).to_be_enabled()
    check(customer.locator('#care-context').input_value()==order_id,'Chat automatically offers the current order reference')
    message='Fictional local issue '+suffix+': can you help with my delivery?'
    customer.locator('#care-message').fill(message);customer.locator('.care-send').click();expect(customer.locator('.care-message p').filter(has_text=message)).to_be_visible(timeout=15000)
    check(customer.locator('.care-message .care-order-link').filter(has_text=order_id).count()>=1,'Sent chat message includes the owned order reference')
    owner_context=browser.new_context(viewport={'width':1440,'height':1000});owner=owner_context.new_page();owner.on('pageerror',lambda e:errors.append(str(e)));owner.on('dialog',lambda d:d.accept())
    owner.goto(BASE+'/admin-login.html',wait_until='networkidle');owner.locator('input[name=email]').fill(credentials['admin_email']);owner.locator('input[name=password]').fill(credentials.get('admin_password',credentials['password']));owner.locator('button[type=submit]:visible').click();owner.wait_for_url('**/admin.html');owner.goto(BASE+'/admin/support',wait_until='networkidle')
    expect(owner.locator('[data-inbox-messages] .care-message p').filter(has_text=message)).to_be_visible(timeout=15000)
    check('Available' in owner.locator('[data-inbox-presence]').inner_text(),'Staff availability is confirmed by a heartbeat')
    expect(customer.locator('.care-presence')).to_contain_text('online',timeout=15000)
    reply='Fictional local reply '+suffix+': your payment evidence is with our team.'
    owner.locator('#staff-support-message').fill(reply);owner.locator('.inbox-compose button[type=submit]').click();expect(customer.locator('.care-message p').filter(has_text=reply)).to_be_visible(timeout=15000)
    check(True,'Staff reply appears for the customer without reloading')
    snapshot(owner,'customer-care-admin-desktop.png');snapshot(customer,'customer-care-order-desktop.png')
    owner.locator('[data-inbox-resolve]').click();expect(owner.locator('[data-inbox-status]')).to_have_text('Resolved');expect(customer.locator('.care-feedback')).to_contain_text('resolved',timeout=15000)
    customer.locator('#care-message').fill('Fictional follow-up '+suffix);customer.locator('.care-send').click();expect(owner.locator('[data-inbox-status]')).to_have_text('Open conversation',timeout=15000)
    check(True,'Customer follow-up reopens the same resolved conversation')
    payload='<img src=x onerror="window.chatXSS=true"> literal test '+suffix
    customer.locator('#care-message').fill(payload);customer.locator('.care-send').click();expect(owner.locator('.care-message p').filter(has_text=payload)).to_be_visible(timeout=15000)
    check(owner.locator('[data-inbox-messages] img').count()==0 and owner.evaluate('window.chatXSS!==true'),'Chat renders HTML-like message content as safe text')
    owner.locator('[data-inbox-available]').uncheck();expect(customer.locator('.care-presence')).to_contain_text('Leave a message',timeout=15000)
    check(True,'Customer availability changes when staff go away')
    owner.locator('[data-inbox-available]').check();expect(customer.locator('.care-presence')).to_contain_text('online',timeout=15000)
    customer.set_viewport_size({'width':390,'height':844});snapshot(customer,'customer-care-order-mobile.png')
    owner.set_viewport_size({'width':390,'height':844});snapshot(owner,'customer-care-admin-mobile.png');owner.set_viewport_size({'width':1440,'height':1000})
    check(customer_context.request.get(BASE+'/api/admin/support').status==401,'Customer cannot read the staff support inbox')
    check(guest_context.request.get(BASE+'/api/support').status==401,'Anonymous customer chat history is private')
    owner.goto(BASE+'/admin/payments',wait_until='networkidle');owner.locator('.payment-review-card').filter(has_text=reference).locator('form[action="/admin/payments/verify"] button').click();owner.wait_for_load_state('networkidle')
    owner.goto(BASE+'/admin/order?id='+order_id,wait_until='networkidle')
    values={'scheduled-date':(date.today()+timedelta(days=7)).isoformat(),'partner':'Fictional Shipment Test Logistics','driver':'Fictional Test Driver','driver-phone':'08000000000','reference':'DEMO-TRACK-'+suffix,'note':'Fictional local shipment acceptance test.'}
    for name,value in values.items():owner.locator('.delivery-planning-form [name="'+name+'"]').fill(value)
    owner.locator('.delivery-planning-form button[type=submit]').click();owner.wait_for_load_state('networkidle');owner.locator('form[action="/admin/orders/delivery"] button').click();owner.wait_for_load_state('networkidle')
    customer.goto(BASE+'/shipment-tracking?order-id='+order_id,wait_until='networkidle');expect(customer.locator('[data-shipment-label]')).to_have_text('Shipment dispatched')
    check(customer.locator('[data-shipment-reference]').inner_text()=='DEMO-TRACK-'+suffix,'Shipment shows the real saved dispatch reference')
    form=owner.locator('.shipment-checkpoint-form');form.locator('[name=stage]').select_option('in_transit');form.locator('[name=location]').fill('Fictional Ibadan Hub');form.locator('[name=note]').fill('Fictional local update: vehicle is in transit.');form.locator('button[type=submit]:visible').click();owner.wait_for_load_state('networkidle')
    expect(customer.locator('[data-shipment-label]')).to_have_text('In transit',timeout=15000);expect(customer.locator('[data-shipment-location]')).to_have_text('Fictional Ibadan Hub',timeout=15000)
    check(True,'Shipment stage and location refresh without reloading')
    snapshot(customer,'shipment-tracking-mobile.png');customer.set_viewport_size({'width':1440,'height':1000});snapshot(customer,'shipment-tracking-desktop.png');snapshot(owner,'shipment-checkpoints-admin-desktop.png')
    form=owner.locator('.shipment-checkpoint-form');form.locator('[name=stage]').select_option('delivery_exception');form.locator('[name=location]').fill('Fictional Ibadan Hub');form.locator('[name=note]').fill('Fictional local update: a short test delay was recorded.');form.locator('button[type=submit]:visible').click();owner.wait_for_load_state('networkidle');expect(customer.locator('[data-shipment-label]')).to_have_text('Delivery update needed',timeout=15000)
    check(customer.locator('[data-milestone].complete').count()==2,'Delivery exception preserves confirmed shipment progress')
    owner.locator('form[action="/admin/orders/delivered"] button').click();owner.wait_for_load_state('networkidle');expect(customer.locator('[data-shipment-label]')).to_have_text('Delivered & handed over',timeout=15000)
    check(customer.locator('[data-milestone].complete').count()==5,'Recorded handover completes all shipment milestones')
    check('Delivered & handed over' in customer.locator('[data-shipment-events]').inner_text(),'Shipment history includes the recorded handover')
    check(owner.locator('.shipment-checkpoint-form').count()==0,'Completed delivery cannot receive further checkpoints')
    check(guest_context.request.get(BASE+'/api/shipment?order-id='+order_id).status==401,'Anonymous shipment access is blocked')
    customer.goto(BASE+'/support.html',wait_until='networkidle');expect(customer.locator('.care-message p').filter(has_text=reply)).to_be_visible(timeout=15000)
    check(True,'Saved conversation survives navigation to the customer-care page')
    snapshot(customer,'customer-care-hub-desktop.png');customer.set_viewport_size({'width':390,'height':844});snapshot(customer,'customer-care-hub-mobile.png')
    owner.goto(BASE+'/admin/support',wait_until='networkidle');expect(owner.locator('[data-inbox-name]')).not_to_have_text('Select a conversation.');owner.locator('[data-inbox-resolve]').click();expect(owner.locator('[data-inbox-status]')).to_have_text('Resolved');owner.locator('[data-inbox-available]').uncheck()
    check(not errors,'No browser JavaScript errors')
    browser.close()
report={'passed':len(checks),'checks':checks,'javascript_errors':errors};(ROOT/'.local/shipment-chat-browser-test-results.json').write_text(json.dumps(report,indent=2)+'\n');print(json.dumps(report,indent=2))
