"""Verify account privacy, stable IDs, responsive pages and the photo viewer.

Profile edits use the local demo account and restore its original details.
The reserved owner signup is inspected without consuming the person's real slot.
"""
import json
from pathlib import Path
from playwright.sync_api import sync_playwright
ROOT=Path(__file__).resolve().parents[1]
BASE='http://127.0.0.1:8080'
checks, errors=[],[]
def check(value,label):
 assert value,label
 checks.append(label)
def snap(page,name):
 page.screenshot(path=str(ROOT/'.local/previews'/name),full_page=True)
 check(page.evaluate('document.documentElement.scrollWidth<=innerWidth'),name+': no horizontal overflow')
with sync_playwright() as pw:
 browser=pw.chromium.launch(executable_path='/usr/bin/google-chrome',headless=True,args=['--no-sandbox'])
 context=browser.new_context(viewport={'width':1440,'height':1000});page=context.new_page();page.on('pageerror',lambda e:errors.append(str(e)))
 credentials=json.loads((ROOT/'.local/demo-accounts.json').read_text())
 page.goto(BASE+'/login.html',wait_until='networkidle');page.locator('#email').fill(credentials['customer_email']);page.locator('#password').fill(credentials['password']);page.locator('button[type=submit]:visible').click();page.wait_for_url('**/customer-dashboard.html')
 check(page.locator('.account-main h1').is_visible(),'Customer dashboard renders actual account data')
 customer_id=page.locator('.account-sidebar .customer-id').inner_text()
 check('MM-C' in customer_id,'Each customer has a visible stable customer ID')
 check(page.locator('[data-support-widget]').count()==1,'Dashboard has one customer-care widget')
 snap(page,'customer-dashboard-updated-desktop.png')
 page.goto(BASE+'/customer-profile',wait_until='networkidle');check(customer_id in page.locator('.account-sidebar').inner_text(),'Same customer ID on profile');snap(page,'customer-profile-updated-desktop.png')
 page.goto(BASE+'/edit-profile',wait_until='networkidle');original_name=page.locator('input[name=name]').input_value();original_phone=page.locator('input[name=phone]').input_value();original_email=page.locator('#email').input_value()
 try:
  page.locator('input[name=name]').fill('Local Profile Check');page.locator('input[name=phone]').fill('+234 801 234 5678');page.locator('form[action="/edit-profile"] button[type=submit]').click();page.wait_for_url('**/customer-profile?notice=saved')
  check('Local Profile Check' in page.locator('.profile-overview h2').inner_text(),'Profile changes save and update the account')
  check(page.locator('.account-notice').is_visible(),'Profile save provides confirmation')
  page.goto(BASE+'/edit-profile',wait_until='networkidle');check(page.locator('#email').input_value()==original_email,'Profile editing preserves account email');snap(page,'edit-profile-updated-desktop.png')
 finally:
  page.goto(BASE+'/edit-profile',wait_until='networkidle');page.locator('input[name=name]').fill(original_name);page.locator('input[name=phone]').fill(original_phone);page.locator('form[action="/edit-profile"] button[type=submit]').click();page.wait_for_url('**/customer-profile?notice=saved')
 for route,name in [('/customer-dashboard.html','dashboard'),('/customer-profile','profile'),('/edit-profile','profile-edit'),('/support.html','care')]:
  page.set_viewport_size({'width':390,'height':844});response=page.goto(BASE+route,wait_until='networkidle');check(response.status==200,route+': customer page renders');check(page.evaluate("getComputedStyle(document.body).backgroundColor")=="rgb(135, 206, 235)",route+': sky-blue background');snap(page,'customer-'+name+'-final-mobile.png')
 page.set_viewport_size({'width':1440,'height':1000})
 page.goto(BASE+'/cars.html',wait_until='networkidle');check(page.locator('.catalogue-grid .vehicle-details-link:visible').count()>0,'Vehicle listings have a clearly labeled Vehicle details button')
 vehicles=context.request.get(BASE+'/api/vehicles').json()
 for vehicle in vehicles:
  response=page.goto(BASE+'/car-details.html?id='+vehicle['id'],wait_until='networkidle');check(response.status==200,vehicle['id']+': vehicle details render')
  specifications=page.locator('.spec-table').inner_text();check(vehicle['engine'] in specifications and vehicle['body'] in specifications and vehicle['fuel'] in specifications,vehicle['id']+': engine, car type and fuel are shown in Vehicle details')
  buttons=page.locator('[data-photo]');check(buttons.count()>=1,vehicle['id']+': gallery has a photograph')
  check(buttons.first.get_attribute('data-photo')==vehicle['image'],vehicle['id']+': listing and main gallery photo match')
  for i in range(buttons.count()):
   buttons.nth(i).click();page.wait_for_function("() => document.querySelector('.gallery-main-image').complete && document.querySelector('.gallery-main-image').naturalWidth>0")
  check(page.locator('[data-photo-credit]').get_attribute('href') is not None,vehicle['id']+': photo attribution available')
 page.goto(BASE+'/car-details.html?id=bmw-x5',wait_until='networkidle');page.locator('[data-photo]').filter(has_text='Interior').click();check('Interior' in page.locator('[data-photo-caption]').inner_text(),'Interior selection changes the main photograph')
 page.locator('[data-enlarge]').click();check(page.locator('.gallery-lightbox').is_visible(),'Vehicle photograph opens in a full-size viewer');before=page.locator('[data-enlarged-label]').inner_text();page.keyboard.press('ArrowRight');check(page.locator('[data-enlarged-label]').inner_text()!=before,'Keyboard arrow navigates photographs');page.keyboard.press('Escape');check(not page.locator('.gallery-lightbox').is_visible(),'Escape closes photo viewer');snap(page,'vehicle-gallery-final-desktop.png')
 page.set_viewport_size({'width':390,'height':844});snap(page,'vehicle-gallery-final-mobile.png')
 anonymous=browser.new_context(viewport={'width':390,'height':844});guest=anonymous.new_page();guest.on('pageerror',lambda e:errors.append(str(e)))
 for route in ['/','/cars.html','/about.html','/contact.html','/login.html','/register.html','/forgot-password','/admin-login.html','/admin-signup.html']:
  response=guest.goto(BASE+route,wait_until='networkidle');check(response.status==200,route+': public page renders');check(guest.evaluate("getComputedStyle(document.body).backgroundColor")=="rgb(135, 206, 235)",route+': sky-blue background');snap(guest,route.strip('/').replace('.html','')+'-final-mobile.png')
 check(guest.evaluate("getComputedStyle(document.body).backgroundColor")=="rgb(135, 206, 235)",'Admin signup uses the requested sky-blue background')
 check(guest.locator('#staff-email').input_value()=='marvellousmordecaiotiti@gmail.com','Reserved signup is still available for the real second owner')
 check('No approval is required' in guest.locator('.staff-login-card').inner_text(),'The last signup activates immediately')
 check(anonymous.request.get(BASE+'/customer-dashboard.html',max_redirects=0).status==303,'Guests cannot view private customer dashboards')
 check(not errors,'No browser JavaScript errors')
 browser.close()
result={'passed':len(checks),'checks':checks,'javascript_errors':errors}
(ROOT/'.local/account-gallery-browser-results.json').write_text(json.dumps(result,indent=2))
print(json.dumps(result,indent=2))
