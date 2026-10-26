"""Check the deployed photo/gallery contract against a running local preview."""
import os
from pathlib import Path
from playwright.sync_api import sync_playwright, expect
BASE=os.environ.get('MM_TEST_URL','http://127.0.0.1:8080')
ROOT=Path(__file__).resolve().parents[1]
with sync_playwright() as pw:
    browser=pw.chromium.launch(executable_path='/usr/bin/google-chrome',headless=True,args=['--no-sandbox'])
    page=browser.new_page(viewport={'width':1440,'height':1000});errors=[]
    page.on('pageerror',lambda error:errors.append(str(error)))
    vehicles=page.request.get(BASE+'/api/vehicles').json()
    assert len(vehicles)==50
    checked=set();retouched=0
    for index,vehicle in enumerate(vehicles,1):
        response=page.goto(BASE+'/car-details.html?id='+vehicle['id']);assert response.status==200,vehicle['id']
        buttons=page.locator('[data-photo]');assert buttons.count()>0,vehicle['id']
        for button in buttons.all():
            path=button.get_attribute('data-photo')
            if path not in checked:
                image=page.request.get(BASE+path)
                assert image.ok and image.headers.get('content-type','').startswith('image/'),path
                checked.add(path)
            if button.get_attribute('data-retouched')=='true':
                button.click();expect(page.locator('[data-photo-caption]')).to_contain_text('Retouched to remove people');retouched+=1
        if index%10==0:print(f'PASS: {index} vehicle galleries checked',flush=True)
    assert retouched==22,retouched
    for file in ROOT.glob('*.js'):page.evaluate('source => { new Function(source); }',file.read_text())
    page.goto(BASE+'/cars.html?category=Sports&engine=I4')
    expect(page.locator('#results-count')).to_have_text('6')
    page.get_by_role('button',name='Remove filter: I4 · four-cylinder',exact=True).click()
    expect(page.locator('#results-count')).to_have_text('30')
    page.goto(BASE+'/car-details.html?id=mazda-mx5')
    page.locator('[data-enlarge]').click();expect(page.locator('.gallery-lightbox')).to_be_visible()
    page.keyboard.press('ArrowRight');page.keyboard.press('Escape');expect(page.locator('.gallery-lightbox')).not_to_be_visible()
    page.set_viewport_size({'width':390,'height':844});page.goto(BASE+'/cars.html?category=Sports')
    assert page.evaluate('document.documentElement.scrollWidth<=innerWidth')
    assert not errors,errors
    Path(ROOT/'.local/previews').mkdir(parents=True,exist_ok=True)
    page.evaluate("document.querySelectorAll('img').forEach(img=>{if(img.getClientRects().length)img.loading='eager'})")
    page.evaluate("Promise.all([...document.images].filter(img=>img.getClientRects().length).map(img=>img.decode()))")
    page.screenshot(path=str(ROOT/'.local/previews/clean-sports-mobile.png'),full_page=True)
    page.set_viewport_size({'width':1440,'height':1000});page.screenshot(path=str(ROOT/'.local/previews/clean-sports-desktop.png'),full_page=True)
    browser.close()
    print(f'PASS: 50 detail pages, {len(checked)} gallery photographs, {retouched} retouch disclosures, keyboard viewer and mobile layout')
