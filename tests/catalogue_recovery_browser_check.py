"""Catalogue regression and assisted recovery against an isolated database."""
import json, os, socket, subprocess, tempfile, time
from pathlib import Path
from playwright.sync_api import sync_playwright, expect
ROOT=Path(__file__).resolve().parents[1]
checks=[]
def check(ok,label):
    assert ok,label
    checks.append(label)
    print('PASS: '+label,flush=True)
def fill(form,values):
    for key,value in values.items():form.locator('[name="'+key+'"]').fill(value)
def submit(page,form):
    form.locator('button[type=submit]').first.click();page.wait_for_load_state('networkidle')
with tempfile.TemporaryDirectory(prefix='mm-recovery-') as directory:
    with socket.socket() as sock:sock.bind(('127.0.0.1',0));port=sock.getsockname()[1]
    base='http://127.0.0.1:'+str(port)
    env={**os.environ,'MM_ENV':'production','MM_SECURE_COOKIES':'false','MM_HOST':'127.0.0.1','PORT':str(port),'MM_DATA_DIR':directory,'MM_DB_PATH':directory+'/test.db','MM_UPLOADS_DIR':directory+'/uploads','MM_DEMO_MODE':'false'}
    log=open(directory+'/server.log','w+')
    server=subprocess.Popen([str(ROOT/'.local/marvellous-motors')],cwd=ROOT,env=env,stdout=log,stderr=log)
    try:
        with sync_playwright() as pw:
            browser=pw.chromium.launch(executable_path='/usr/bin/google-chrome',headless=True,args=['--no-sandbox'])
            ctx=browser.new_context(viewport={'width':1440,'height':950});page=ctx.new_page();errors=[];page.on('pageerror',lambda e:errors.append(str(e)))
            for _ in range(60):
                try:
                    if ctx.request.get(base+'/health').ok:break
                except Exception:time.sleep(.2)
            page.goto(base+'/cars.html?condition=&body=&budget=')
            check(page.locator('#results-count').inner_text()=='16','Unfiltered catalogue shows all 16 vehicles')
            check(page.locator('select[name=category]').count()==1 and page.locator('fieldset.collection-choices').count()==0,'Collection uses one compact dropdown')
            check(page.locator('.catalogue-grid .vehicle-card:visible').count()==9,'First page actually displays vehicle cards')
            page.locator('[name=category]').select_option('Sports');check(page.locator('#results-count').inner_text()=='1','Sports selection shows the Porsche')
            page.locator('[name=engine]').select_option('I4');check(page.locator('#results-count').inner_text()=='0' and page.locator('#category-only').is_visible(),'Conflicting engine filter offers a clear recovery action')
            page.locator('#category-only').click();check(page.locator('#results-count').inner_text()=='1' and page.locator('[name=engine]').input_value()=='','Show all Sports clears conflicting filters')
            for category in ['Classic','Limited Edition']:
                page.locator('[name=category]').select_option(category);check('listed yet' in page.locator('#empty-explanation').inner_text(),category+' is available without inventing stock')
            page.set_viewport_size({'width':390,'height':844});page.goto(base+'/cars.html');check(page.evaluate('document.documentElement.scrollWidth<=innerWidth'),'Compact collection filter fits mobile');page.screenshot(path=str(ROOT/'.local/previews/compact-collection-mobile.png'),full_page=True)
            page.goto(base+'/register.html');form=page.locator('form[action="/register"]');fill(form,{'name':'Recovery Buyer','email':'recovery@example.test','phone':'08012345678','password':'OriginalBuyer123','confirm-password':'OriginalBuyer123'});submit(page,form)
            def login(target,email,password):
                target.goto(base+'/login.html');form=target.locator('form[action="/login"]');fill(form,{'email':email,'password':password});submit(target,form)
            login(page,'recovery@example.test','OriginalBuyer123')
            page.goto(base+'/forgot-password');form=page.locator('form');fill(form,{'email':'recovery@example.test'});submit(page,form)
            check('No email has been sent automatically' in page.inner_text('body'),'Forgot password records an honest assisted recovery request')
            actx=browser.new_context();admin=actx.new_page();login(admin,'marvellousmordecaiotiti@gmail.com','password')
            for route in ['/admin.html','/admin/inventory','/admin/pricing','/admin/appointments','/admin/sales','/admin/dealership','/admin/recovery']:
                response=admin.goto(base+route);check(response.status==200 and admin.locator('.ops-sidebar').count()==1,route+' returns a complete admin page')
            form=admin.locator('form[action="/admin/recovery"]');form.locator('[name=customer]').select_option(label=admin.locator('[name=customer] option').last.inner_text());fill(form,{'reason':'Verified using the previously recorded contact details.','admin-password':'password'});form.locator('[name=verified]').check();submit(admin,form)
            link=admin.locator('.recovery-link').get_attribute('href');check(bool(link),'Admin creates private recovery link after verification')
            rctx=browser.new_context();reset=rctx.new_page();reset.goto(base+link);form=reset.locator('form[action="/reset-password"]');fill(form,{'password':'RecoveredBuyer123','confirm-password':'RecoveredBuyer123'});submit(reset,form);check(reset.url.endswith('/login.html'),'Customer chooses a new password and returns to normal login')
            page.goto(base+'/customer-dashboard.html');check(page.url.endswith('/login.html'),'Password reset revokes previous signed-in customer sessions')
            check(rctx.request.get(base+link).status==400,'Recovery link cannot be reused')
            login(reset,'recovery@example.test','RecoveredBuyer123');check('customer-dashboard' in reset.url,'Recovered customer signs in with the new password')
            check(not errors,'No catalogue JavaScript errors');browser.close()
    finally:
        server.terminate();server.wait(timeout=10);log.close()
(ROOT/'.local/catalogue-recovery-results.json').write_text(json.dumps({'passed':len(checks),'checks':checks},indent=2)+'\n')
