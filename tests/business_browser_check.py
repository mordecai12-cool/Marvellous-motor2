"""Full sales workflow in an isolated production-mode test database."""
import json, os, socket, struct, subprocess, tempfile, time, uuid, zlib
from datetime import datetime, timedelta, timezone
from pathlib import Path
from playwright.sync_api import sync_playwright, expect

ROOT=Path(__file__).resolve().parents[1]
checks=[]
def check(ok,label):
    assert ok,label
    checks.append(label)
    print('PASS: '+label,flush=True)
def png():
    def chunk(kind,data):return struct.pack('!I',len(data))+kind+data+struct.pack('!I',zlib.crc32(kind+data)&0xffffffff)
    return b'\x89PNG\r\n\x1a\n'+chunk(b'IHDR',struct.pack('!2I5B',80,50,8,2,0,0,0))+chunk(b'IDAT',zlib.compress((b'\0'+b'\x30\x80\xb0'*80)*50))+chunk(b'IEND',b'')
def fill(form,values):
    for name,value in values.items():form.locator('[name="'+name+'"]').fill(str(value))
def signin(page,base,email,password,path):
    page.goto(base+'/login.html')
    fill(page.locator('form[action="/login"]'),{'email':email,'password':password})
    page.locator('form[action="/login"] button[type=submit]').click()
    page.wait_for_url('**/'+path)
def submit(page,form):
    form.locator('button[type=submit]').first.click()
    page.wait_for_load_state('networkidle')
def shot(page,name):
    page.screenshot(path=str(ROOT/'.local/previews'/name),full_page=True)
    check(page.evaluate('document.documentElement.scrollWidth<=innerWidth'),name+': fits viewport')

with tempfile.TemporaryDirectory(prefix='mm-business-') as directory:
    with socket.socket() as sock:sock.bind(('127.0.0.1',0));port=sock.getsockname()[1]
    base='http://127.0.0.1:'+str(port)
    env={**os.environ,'MM_ENV':'production','MM_HOST':'127.0.0.1','PORT':str(port),'MM_DATA_DIR':directory,'MM_DB_PATH':directory+'/business.db','MM_UPLOADS_DIR':directory+'/uploads','MM_DEMO_MODE':'false','MM_SECURE_COOKIES':'false'}
    log=open(directory+'/server.log','w+')
    server=subprocess.Popen([str(ROOT/'.local/marvellous-motors')],cwd=ROOT,env=env,stdout=log,stderr=log)
    try:
        with sync_playwright() as pw:
            browser=pw.chromium.launch(executable_path='/usr/bin/google-chrome',headless=True,args=['--no-sandbox'])
            adminctx=browser.new_context(viewport={'width':1440,'height':1000});admin=adminctx.new_page();errors=[]
            admin.on('pageerror',lambda error:errors.append(str(error)))
            admin.on('dialog',lambda dialog:dialog.accept())
            for _ in range(80):
                try:
                    if adminctx.request.get(base+'/health').ok:break
                except Exception:pass
                time.sleep(.1)
            else:log.flush();raise RuntimeError(Path(directory+'/server.log').read_text())
            signin(admin,base,'marvellousmordecaiotiti@gmail.com','password','admin.html')
            for route in ['/admin/inventory','/admin/pricing','/admin/appointments','/admin/sales','/admin/dealership']:
                response=admin.goto(base+route)
                check(response.status==200 and admin.locator('.ops-main h1').count()==1,route+': admin screen loads')
            admin.goto(base+'/admin/inventory')
            stock=admin.locator('form[enctype="multipart/form-data"]')
            fill(stock,{'brand':'Browser Motors','model':'Touring','trim':'V6 Premium','vin':'TEST-CHASSIS-001','year':'2024','price':'10000000','mileage':'12000','engine':'3.5L V6','drive':'FWD','location':'Lagos, Nigeria','previous_owners':'1','description':'Isolated browser test vehicle, with actual fixture photos.','condition_report':'Small scratch on rear bumper disclosed.','service_history':'Service invoices recorded.','documents':'Inspection report and ownership papers available.','offer_name':'Browser Savings','offer_discount':'1000000'})
            for name,value in {'stock_status':'Draft','condition':'Used','body':'Sedan','fuel':'Petrol','engine_layout':'V6','transmission':'Automatic','origin':'Foreign used'}.items():stock.locator('[name='+name+']').select_option(value)
            stock.locator('[name=categories][value=Everyday]').uncheck();stock.locator('[name=categories][value=Luxury]').check()
            stock.locator('[name=photos]').set_input_files([{'name':'front.png','mimeType':'image/png','buffer':png()},{'name':'interior.png','mimeType':'image/png','buffer':png()}])
            stock.locator('[name=files]').set_input_files({'name':'inspection.pdf','mimeType':'application/pdf','buffer':b'%PDF-1.4\nFictional inspection fixture\n%%EOF'})
            stock.locator('[name=actual_photos]').check();submit(admin,stock)
            expect(admin.locator('.ops-alert')).to_contain_text('saved')
            vehicle=admin.locator('form[enctype="multipart/form-data"] [name=id]').input_value()
            check(not any(v['id']==vehicle for v in adminctx.request.get(base+'/api/vehicles').json()),'Draft vehicle stays out of the public inventory')
            stock=admin.locator('form[enctype="multipart/form-data"]');stock.locator('[name=stock_status]').select_option('Available');submit(admin,stock)
            public=next(v for v in adminctx.request.get(base+'/api/vehicles').json() if v['id']==vehicle)
            check(public['actual_photos'] and public['stock_status']=='Available','Admin publishes actual stock with condition and history')
            doc=admin.get_by_role('link',name='inspection.pdf',exact=True).get_attribute('href')
            check(not public.get('documents_files'),'Public inventory does not expose private document metadata')
            customerctx=browser.new_context(viewport={'width':1440,'height':1000});customer=customerctx.new_page();customer.on('pageerror',lambda error:errors.append(str(error)))
            customer.goto(base+'/car-details.html?id='+vehicle)
            check('Small scratch' in customer.inner_text('main') and 'TEST-CHASSIS-001' in customer.inner_text('main'),'Buyer sees real condition, history and chassis details')
            check(customer.locator('.gallery-thumbnails button').count()==2,'Uploaded stock gallery replaces the sample gallery')
            check(customerctx.request.get(base+doc).status==404,'Unpublished ownership/inspection document stays private')
            customer.goto(base+'/cars.html')
            check('Browser Motors' in customer.locator('[name=brand]').inner_text(),'Make filters include newly added brands')
            # Convert a provisional route into a complete business tariff.
            admin.goto(base+'/admin/pricing')
            detail=admin.locator('details').filter(has_text='Within-Lagos planning allowance')
            detail.locator('summary').click();rate=detail.locator('form')
            fill(rate,{'delivery':'200000','fees':'25000','insurance':'10000','taxes':'5000','port_fees':'30000','insurance_percent':'1','duty_percent':'10','vat_percent':'5','label':'Browser complete tariff','notes':'Fictional complete tariff for isolated browser testing.'})
            rate.locator('[name=complete]').check();rate.locator('[name=estimated]').uncheck();submit(admin,rate)
            expected=10801500
            # Publish a visit slot before the car is reserved by checkout.
            admin.goto(base+'/admin/appointments')
            visit=admin.locator('form').filter(has=admin.locator('[name=action][value=slot]'))
            start=datetime.now(timezone(timedelta(hours=1))).replace(second=0,microsecond=0)+timedelta(days=2)
            fill(visit,{'start':start.strftime('%Y-%m-%dT%H:%M'),'end':(start+timedelta(hours=1)).strftime('%Y-%m-%dT%H:%M'),'location':'Isolated Test Showroom'})
            visit.locator('[name=vehicle]').select_option(vehicle);submit(admin,visit)
            check('Isolated Test Showroom' in admin.inner_text('main'),'Admin publishes a test-drive appointment slot')
            # Registration errors keep a usable form and never echo a password.
            buyer='buyer-'+uuid.uuid4().hex[:8]+'@example.test';password='BrowserPurchase123'
            customer.goto(base+'/register.html')
            reg=customer.locator('form[action="/register"]');values={'name':'Browser Buyer','email':buyer,'phone':'08012345678','password':password,'confirm-password':password};fill(reg,values);submit(customer,reg)
            customer.goto(base+'/register.html');reg=customer.locator('form[action="/register"]');values['email']=buyer.upper();fill(reg,values);submit(customer,reg)
            check('details already exist' in customer.locator('[role=alert]').inner_text(),'Duplicate account shows a clear error on the registration page')
            check(customer.locator('[name=name]').input_value()=='Browser Buyer' and customer.locator('[name=password]').input_value()=='','Duplicate registration retains contact details and clears passwords')
            customer.goto(base+'/quote?vehicle='+vehicle+'&country=Nigeria&state=Lagos&city=Ikeja')
            expect(customer.locator('[data-quote-order]')).to_be_visible()
            quote=customerctx.request.get(base+'/api/quote?vehicle='+vehicle+'&country=Nigeria&state=Lagos&city=Ikeja').json()
            check(quote['total']==expected*100 and quote['can_order'] and quote['offer']=='Browser Savings','Automatic price includes the car, all configured fees, percentage charges and offer')
            customer.locator('[data-quote-order]').click();customer.wait_for_url('**/login.html')
            fill(customer.locator('form[action="/login"]'),{'email':buyer,'password':password});submit(customer,customer.locator('form[action="/login"]'))
            customer.wait_for_url('**/order.html?vehicle='+vehicle)
            check(customer.locator('[name=state]').input_value()=='Lagos' and customer.locator('[name=city]').input_value()=='Ikeja','Chosen destination survives the normal login')
            customer.goto(base+'/appointments?vehicle='+vehicle)
            booking=customer.locator('form').filter(has=customer.locator('[name=slot]'));submit(customer,booking)
            check('requested' in customer.inner_text('main'),'Customer reserves an appointment slot')
            admin.goto(base+'/admin/appointments');confirmation=admin.locator('form').filter(has=admin.locator('[name=action][value=status]'));confirmation.locator('[name=status]').select_option('confirmed');submit(admin,confirmation)
            customer.goto(base+'/appointments');calendar=customer.locator('a[href^="/appointments/calendar"]');expect(calendar).to_be_visible();ics=customerctx.request.get(base+calendar.get_attribute('href')).text()
            check('BEGIN:VALARM' in ics and 'TRIGGER:-PT1H' in ics,'Confirmed appointment downloads a private calendar reminder')
            admin.goto(base+'/admin/sales');card=admin.locator('.sales-column article').filter(has_text='Browser Buyer');follow=card.locator('form');follow.locator('[name=stage]').select_option('Negotiating');follow.locator('[name=assigned]').select_option(index=1);fill(follow,{'followup':datetime.now().strftime('%Y-%m-%d'),'note':'Follow up with the buyer after the visit.'});submit(admin,follow)
            check(admin.locator('.sales-column').filter(has=admin.get_by_role('heading',name='Negotiating',exact=True)).locator('article').filter(has_text='Browser Buyer').count()==1,'Sales lead moves to Negotiating with a salesperson and follow-up')
            # Checkout freezes a server-calculated total and reserves stock.
            customer.goto(base+'/order.html?vehicle='+vehicle)
            order=customer.locator('form[action="/order"]');fill(order,{'address':'1 Fictional Buyer Road','country':'Nigeria','state':'Lagos','city':'Ikeja'})
            expect(order.locator('[data-quote-continue]')).to_be_enabled();submit(customer,order)
            check(customer.locator('#amount-paid').input_value()==str(expected)+'.00','Payment form uses the saved delivered total')
            order_id=customer.locator('[name=order-id]').input_value()
            check('₦10,801,500' in customer.inner_text('main'),'Checkout displays the saved itemised price')
            public=next(v for v in customerctx.request.get(base+'/api/vehicles').json() if v['id']==vehicle)
            check(public['stock_status']=='Reserved','Placing the order reserves that stock vehicle')
            customer.locator('#reference').fill('FICTIONAL-TEST-'+order_id);customer.locator('#receipt').set_input_files({'name':'payment.pdf','mimeType':'application/pdf','buffer':b'%PDF-1.4\nFictional test receipt\n%%EOF'})
            submit(customer,customer.locator('form[action="/payment"]'))
            admin.goto(base+'/admin/payments');review=admin.locator('form[action="/admin/payments/verify"]').filter(has=admin.locator('[name=payment-id]'));submit(admin,review)
            check(customerctx.request.get(base+doc).status==200,'Buyer can open shared vehicle documents after payment review')
            admin.goto(base+'/admin/order?id='+order_id)
            planning=admin.locator('.delivery-planning-form');fill(planning,{'partner':'Fictional Test Carrier','reference':'TEST-DISPATCH-'+order_id});submit(admin,planning)
            submit(admin,admin.locator('form[action="/admin/orders/delivery"]'));submit(admin,admin.locator('form[action="/admin/orders/delivered"]'))
            public=next(v for v in customerctx.request.get(base+'/api/vehicles').json() if v['id']==vehicle)
            check(public['stock_status']=='Sold','Confirmed handover marks the car Sold')
            customer.goto(base+'/account/reviews');review=customer.locator('form[action="/account/reviews"]');review.locator('[name=order]').select_option(order_id);fill(review,{'body':'The team explained the condition and delivery costs clearly.'});review.locator('[name=consent]').check();submit(customer,review)
            admin.goto(base+'/admin/dealership');profile=admin.locator('form').filter(has=admin.locator('[name=action][value=profile]'));fill(profile,{'name':'Marvellous Motors Test Showroom','email':'showroom@example.test','phone':'08012345678','address':'1 Isolated Test Showroom Road, Lagos','hours':'Monday–Friday, 09:00–17:00','story':'An isolated fictional profile used only by this test.','team':'Test Manager — Sales'});submit(admin,profile)
            moderation=admin.locator('form').filter(has=admin.locator('[name=action][value=review]'));submit(admin,moderation)
            customer.goto(base+'/dealership')
            check('1 Isolated Test Showroom Road' in customer.inner_text('main') and 'delivery costs clearly' in customer.inner_text('main'),'Dealership shows configured business details and a consenting delivered buyer review')
            # Complaint assignment and status work alongside picture attachments.
            customer.goto(base+'/support.html');expect(customer.locator('#care-message')).to_be_enabled();customer.locator('#care-message').fill('Please help me with the vehicle handover paperwork.');customer.locator('#care-attachment').set_input_files({'name':'complaint.png','mimeType':'image/png','buffer':png()});customer.locator('.care-send').click();expect(customer.locator('.care-attachment img')).to_be_visible(timeout=20000)
            state=customerctx.request.get(base+'/api/support').json();cid=state['conversation']['id'];admin.goto(base+'/admin/support?id='+str(cid));case=admin.locator('[data-case-form]');expect(case).to_be_visible();case.locator('[name=case-status]').select_option('investigating');case.locator('[name=assigned]').select_option(index=1);case.locator('button[type=submit]').click()
            expect(admin.locator('[data-inbox-feedback]')).to_contain_text('saved')
            state=customerctx.request.get(base+'/api/support').json()['conversation'];check(state['case_status']=='investigating' and state['assigned_id']>0 and state['case_reference'].startswith('MM-S'),'Complaint gets a case number, investigating status and assigned admin')
            shot(admin,'business-complaint-desktop.png')
            admin.goto(base+'/admin/inventory?id='+vehicle);shot(admin,'business-inventory-desktop.png')
            admin.set_viewport_size({'width':390,'height':844});admin.goto(base+'/admin/sales');shot(admin,'business-sales-mobile.png')
            customer.set_viewport_size({'width':390,'height':844});customer.goto(base+'/car-details.html?id='+vehicle);check(customer.locator('.vehicle-mobile-actions').is_visible(),'Mobile car page offers enquiry, booking and call actions');shot(customer,'business-vehicle-mobile.png')
            customer.goto(base+'/quote?vehicle=toyota-camry&country=United+Kingdom&state=England&city=London');expect(customer.locator('[data-quote-feedback]')).to_contain_text('unconfirmed');check(customer.locator('[data-quote-order]').is_hidden(),'Incomplete international shipping estimate cannot masquerade as an all-in checkout total');shot(customer,'business-quote-mobile.png')
            check(not errors,'No browser JavaScript errors')
            browser.close()
    finally:
        server.terminate()
        try:server.wait(timeout=10)
        except subprocess.TimeoutExpired:server.kill();server.wait()
        log.close()
(ROOT/'.local/business-browser-results.json').write_text(json.dumps({'passed':len(checks),'checks':checks},indent=2)+'\n')
print(json.dumps({'passed':len(checks),'checks':checks},indent=2))
