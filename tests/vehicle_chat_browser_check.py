"""Exercise engine/collection browsing and private complaint attachments locally."""
import json, struct, uuid, zlib
from pathlib import Path
from playwright.sync_api import sync_playwright, expect

ROOT=Path(__file__).resolve().parents[1]
BASE='http://127.0.0.1:8080'
checks=[]

def check(ok,label):
    assert ok,label
    checks.append(label)
    print('PASS: '+label,flush=True)

def png():
    def chunk(kind,data):
        return struct.pack('!I',len(data))+kind+data+struct.pack('!I',zlib.crc32(kind+data)&0xffffffff)
    return b'\x89PNG\r\n\x1a\n'+chunk(b'IHDR',struct.pack('!2I5B',32,32,8,2,0,0,0))+chunk(b'IDAT',zlib.compress((b'\0'+b'\x25\x80\xc0'*32)*32))+chunk(b'IEND',b'')

def login(page,email,password):
    page.goto(BASE+'/login.html')
    page.locator('[name=email]').fill(email)
    page.locator('[name=password]').fill(password)
    page.locator('form[action="/login"] button[type=submit]').click()
    page.wait_for_load_state('networkidle')

with sync_playwright() as pw:
    browser=pw.chromium.launch(executable_path='/usr/bin/google-chrome',headless=True,args=['--no-sandbox'])
    ctx=browser.new_context(viewport={'width':1440,'height':1000})
    page=ctx.new_page();errors=[]
    page.on('pageerror',lambda error:errors.append(str(error)))
    page.goto(BASE+'/')
    check(page.locator('#special-collections a').count()==4,'Homepage has Sports, Luxury, Classic and Limited collections')
    page.goto(BASE+'/cars.html?category=Sports')
    check(page.locator('#results-count').inner_text()=='6','Sports section displays all six matching cars')
    page.goto(BASE+'/cars.html?category=Luxury&engine=V6')
    check(page.locator('#results-count').inner_text()=='1','Luxury and V6 filters combine')
    check('lexus-preowned'==page.locator('.catalogue-grid .vehicle-card:visible').get_attribute('data-id'),'V6 filter selects matching model year')
    page.reload()
    check(page.locator('[name=engine]').input_value()=='V6','Engine filter survives page reload')
    page.goto(BASE+'/cars.html?q=V6')
    check(page.locator('#results-count').inner_text()=='1','Engine text is searchable')
    page.goto(BASE+'/car-details.html?id=bmw-x5')
    check('I6 · inline six-cylinder' in page.locator('.spec-table').inner_text(),'Detail page distinguishes inline six-cylinder from V6')
    page.goto(BASE+'/car-details.html?id=tesla-model-y')
    check('no combustion cylinders' in page.locator('.spec-table').inner_text(),'Electric cars display motor details')
    page.set_viewport_size({'width':390,'height':844})
    page.goto(BASE+'/cars.html?category=Luxury&engine=V6')
    check(page.evaluate('document.documentElement.scrollWidth<=innerWidth'),'New vehicle filters fit mobile')
    page.screenshot(path=str(ROOT/'.local/previews/vehicle-engine-mobile.png'),full_page=True)
    email='attachment-test-'+uuid.uuid4().hex[:8]+'@example.test';password='AttachmentBrowser123'
    page.goto(BASE+'/register.html')
    for name,value in {'name':'Complaint Attachment Test','email':email,'phone':'08012345678','password':password,'confirm-password':password}.items():
        page.locator('[name='+name+']').fill(value)
    page.locator('form[action="/register"] button[type=submit]').click()
    page.wait_for_load_state('networkidle')
    login(page,email,password)
    page.goto(BASE+'/support.html')
    expect(page.locator('#care-message')).to_be_enabled()
    page.locator('#care-attachment').set_input_files({'name':'complaint.png','mimeType':'image/png','buffer':png()})
    with page.expect_response(lambda response:'/api/support/send' in response.url and response.request.method=='POST') as sent:
        page.locator('.care-send').click()
    check(sent.value.status==201,'Customer sends complaint picture without requiring a caption')
    expect(page.locator('.care-attachment img')).to_be_visible()
    check(page.locator('.care-attachment img').evaluate('(img)=>img.complete&&img.naturalWidth>0'),'Customer sees uploaded image preview')
    state=ctx.request.get(BASE+'/api/support').json();cid=state['conversation']['id'];url=state['messages'][-1]['attachment']['url']
    check(browser.new_context().request.get(BASE+url).status==401,'Anonymous users cannot open complaint files')
    page.screenshot(path=str(ROOT/'.local/previews/complaint-customer-mobile.png'),full_page=True)
    check(page.evaluate('document.documentElement.scrollWidth<=innerWidth'),'Complaint chat fits mobile')
    adminctx=browser.new_context(viewport={'width':1440,'height':1000});admin=adminctx.new_page()
    admin.on('pageerror',lambda error:errors.append(str(error)))
    login(admin,'marvellousmordecaiotiti@gmail.com','password')
    admin.goto(BASE+'/admin/support?id='+str(cid))
    expect(admin.locator('.care-attachment img')).to_be_visible()
    admin.wait_for_function("[...document.querySelectorAll('.care-attachment img')].some(img=>img.complete&&img.naturalWidth>0)")
    check(True,'Admin sees customer complaint image')
    check(adminctx.request.get(BASE+url).status==200,'Approved admin can open private complaint picture')
    expect(admin.locator('#staff-attachment')).to_be_enabled()
    admin.locator('#staff-attachment').set_input_files({'name':'complaint-review.txt','mimeType':'text/plain','buffer':b'We have reviewed your complaint picture.'})
    with admin.expect_response(lambda response:'/api/admin/support/send' in response.url and response.request.method=='POST') as reply:
        admin.locator('.inbox-compose button[type=submit]').click()
    check(reply.value.status==201,'Admin can send a document back to the customer')
    expect(page.locator('.care-attachment').filter(has_text='complaint-review.txt')).to_be_visible(timeout=15000)
    document=ctx.request.get(BASE+'/api/support').json()['messages'][-1]['attachment']
    response=ctx.request.get(BASE+document['url'])
    check(response.status==200 and b'reviewed' in response.body(),'Customer can download the admin document')
    check('attachment;' in response.headers.get('content-disposition',''),'Documents download with the original file name')
    page.reload()
    expect(page.locator('.care-attachment img')).to_be_visible()
    check(page.locator('.care-attachment').count()==2,'Attachment history persists on reload')
    admin.screenshot(path=str(ROOT/'.local/previews/complaint-admin-desktop.png'),full_page=True)
    page.locator('#care-attachment').set_input_files({'name':'unsafe.html','mimeType':'text/html','buffer':b'<script>alert(1)</script>'})
    with page.expect_response(lambda response:'/api/support/send' in response.url and response.request.method=='POST') as invalid:
        page.locator('.care-send').click()
    check(invalid.value.status==400,'Unsupported file is rejected with a useful chat error')
    check(page.locator('#care-attachment').evaluate('(input)=>input.files.length')==1,'Failed upload keeps the selected file for correction')
    check(not errors,'No browser JavaScript errors')
    browser.close()
(ROOT/'.local/vehicle-chat-browser-results.json').write_text(json.dumps({'passed':len(checks),'checks':checks},indent=2)+'\n')
print(json.dumps({'passed':len(checks),'checks':checks},indent=2))
