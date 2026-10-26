(() => {
  'use strict';
  const read = key => { try { const v = JSON.parse(localStorage.getItem(key) || '[]'); return Array.isArray(v) ? v.filter(x => typeof x === 'string') : []; } catch { return []; } };
  let saved = new Set(read('mm-saved'));
  let compared = new Set(read('mm-compare').slice(0, 3));
  const persist = () => { try { localStorage.setItem('mm-saved', JSON.stringify([...saved])); localStorage.setItem('mm-compare', JSON.stringify([...compared])); } catch { /* Browsing remains available without storage. */ } };
  const toast = document.querySelector('.toast');
  let toastTimer;
  function announce(message) { if (!toast) return; toast.textContent = message; toast.classList.add('visible'); clearTimeout(toastTimer); toastTimer = setTimeout(() => toast.classList.remove('visible'), 3000); }
  const menu = document.querySelector('.menu-toggle');
  menu?.addEventListener('click', () => { const expanded = menu.getAttribute('aria-expanded') === 'true'; menu.setAttribute('aria-expanded', String(!expanded)); document.querySelector('.nav-links')?.classList.toggle('open', !expanded); });
  function updateControls() {
    document.querySelectorAll('.saved-count').forEach(el => el.textContent = saved.size);
    document.querySelectorAll('[data-save]').forEach(el => { const active = saved.has(el.dataset.save); el.setAttribute('aria-pressed', String(active)); el.classList.toggle('is-saved', active); el.textContent = el.classList.contains('save-button') ? (active ? '♥' : '♡') : (active ? '♥ Saved vehicle' : '♡ Save vehicle'); });
    document.querySelectorAll('[data-compare]').forEach(el => el.checked = compared.has(el.dataset.compare));
    const tray = document.querySelector('.compare-tray');
    if (tray) tray.hidden = compared.size === 0;
    const count = document.querySelector('#compare-count'); if (count) count.textContent = compared.size;
  }
  document.querySelectorAll('[data-save]').forEach(el => el.addEventListener('click', () => { const id = el.dataset.save; saved.has(id) ? saved.delete(id) : saved.add(id); persist(); updateControls(); applyFilters(); announce(saved.has(id) ? 'Added to your saved vehicles.' : 'Removed from saved vehicles.'); }));
  document.querySelectorAll('[data-compare]').forEach(el => el.addEventListener('change', () => { const id = el.dataset.compare; if (el.checked && compared.size >= 3) { el.checked = false; announce('Compare up to 3 vehicles. Remove one to add another.'); return; } el.checked ? compared.add(id) : compared.delete(id); persist(); updateControls(); }));
  document.querySelector('#clear-compare')?.addEventListener('click', () => { compared.clear(); persist(); updateControls(); });
  const form = document.querySelector('#catalogue-filters');
  const grid = document.querySelector('.catalogue-grid');
  const cards = grid ? [...grid.querySelectorAll('.vehicle-card')] : [];
  let currentPage = Math.max(1, Number.parseInt(new URLSearchParams(location.search).get('page') || '1', 10) || 1);
  const pageSize = 9;
  function applyFilters() {
    if (!form) return;
    const values = new FormData(form), query = String(values.get('q') || '').toLowerCase().trim();
    let count = 0;
    cards.forEach(card => {
      const d = card.dataset;
      const visible = (!values.getAll('category').filter(Boolean).length || values.getAll('category').filter(Boolean).some(category => d.categories.split(' · ').includes(category))) && (!values.get('engine') || d.engineLayout === values.get('engine')) && (!query || d.search.toLowerCase().includes(query)) && (!values.get('condition') || d.condition === values.get('condition')) && (!values.get('brand') || d.brand === values.get('brand')) && (!values.get('body') || d.body === values.get('body')) && (!values.get('fuel') || d.fuel === values.get('fuel') || (values.get('fuel') === 'Hybrid' && d.fuel === 'Electric')) && (!values.get('budget') || Number(d.price) < Number(values.get('budget'))) && (!values.has('saved') || saved.has(d.id)) && (!values.get('yearMin') || Number(d.year) >= Number(values.get('yearMin'))) && (!values.get('yearMax') || Number(d.year) <= Number(values.get('yearMax'))) && (!values.get('mileage') || Number(d.mileage) <= Number(values.get('mileage'))) && (!values.get('origin') || d.origin === values.get('origin')) && (!values.get('location') || d.location === values.get('location')) && (!values.get('availability') || d.availability === values.get('availability')) && (!values.get('transmission') || (values.get('transmission') === 'Automatic' ? ['Automatic','PDK automatic'].includes(d.transmission) : values.get('transmission') === 'CVT' ? ['CVT','e-CVT'].includes(d.transmission) : d.transmission === values.get('transmission')));
      card.hidden = !visible; if (visible) count++;
    });
    const sort = values.get('sort');
    const sorted = [...cards].sort((a,b) => sort === 'price-asc' ? Number(a.dataset.price)-Number(b.dataset.price) : sort === 'price-desc' ? Number(b.dataset.price)-Number(a.dataset.price) : sort === 'year-desc' ? Number(b.dataset.year)-Number(a.dataset.year) : sort === 'mileage-asc' ? Number(a.dataset.mileage)-Number(b.dataset.mileage) : cards.indexOf(a)-cards.indexOf(b));
    sorted.forEach(card => grid.append(card));
    const visibleCards = sorted.filter(card => !card.hidden);
    const pages = Math.max(1, Math.ceil(count / pageSize));
    currentPage = Math.min(currentPage, pages);
    visibleCards.forEach((card,i) => card.hidden = i < (currentPage-1)*pageSize || i >= currentPage*pageSize);
    document.querySelector('#page-label').textContent = count ? `Page ${currentPage} of ${pages}` : '';
    document.querySelector('#previous-page').disabled = currentPage <= 1;
    document.querySelector('#next-page').disabled = currentPage >= pages;
    document.querySelector('.pagination').hidden = count <= pageSize;
    document.querySelector('#results-count').textContent = count;
    document.querySelector('#results-context').textContent = values.has('saved') ? 'in your saved collection' : [...values].some(([key,value]) => value && key !== 'sort') ? 'matching your next chapter' : 'in the collection';
    document.querySelector('.empty-state').hidden = count !== 0;
    const selectedTypes=values.getAll('category').filter(Boolean), typeCount=cards.filter(card=>selectedTypes.some(type=>card.dataset.categories.split(' · ').includes(type))).length;
    const onlyTypes=document.querySelector('#category-only');if(onlyTypes)onlyTypes.hidden=count!==0 || !typeCount;
    const explanation=document.querySelector('#empty-explanation');if(explanation)explanation.textContent=selectedTypes.length ? (typeCount ? `There are ${typeCount} cars in ${selectedTypes.join(' / ')}, but other active filters hide them. Remove a filter below or show all cars in these types.` : `No ${selectedTypes.join(' / ')} cars are listed yet. Ask us to source one, or explore another type.`) : 'No vehicles match these filters. Remove a filter below or explore all vehicles.';
    const active = document.querySelector('#active-filters');
    if (active) {
      active.replaceChildren();
      const names = {q:'Search', yearMin:'From year', yearMax:'To year', saved:'Saved vehicles'};
      for (const [key,value] of values) {
        if (!value || key === 'sort') continue;
        const field = form.elements.namedItem(key);
        const selectedLabel = field?.selectedOptions?.[0]?.textContent || value;
        const label = key === 'saved' ? names.saved : names[key] ? `${names[key]}: ${selectedLabel}` : selectedLabel;
        const button = document.createElement('button');
        button.type = 'button';
        button.className = 'filter-chip';
        button.textContent = `${label} ×`;
        button.setAttribute('aria-label', `Remove filter: ${label}`);
        button.addEventListener('click', () => {
          if (field) { if (field.type === 'checkbox') field.checked = false; else field.value = ''; }
          currentPage = 1;
          applyFilters();
        });
        active.append(button);
      }
    }

    const url = new URL(location.href); url.search = '';
    for (const [key,value] of values) if (value && !(key === 'sort' && value === 'featured')) url.searchParams.append(key, key === 'saved' ? '1' : value);
    if (currentPage > 1) url.searchParams.set('page', currentPage);
    history.replaceState(null, '', url);
  }
  function restoreField(key,value) {const el=form.elements.namedItem(key);if(el && el.tagName){if(el.type==='checkbox')el.checked=value==='1';else el.value=value;}}
  if (form) {
    document.querySelectorAll('[data-category-count]').forEach(el=>el.textContent='('+cards.filter(card=>card.dataset.categories.split(' · ').includes(el.dataset.categoryCount)).length+')');
    document.querySelector('#category-only')?.addEventListener('click',()=>{const selected=new FormData(form).getAll('category').filter(Boolean);form.reset();selected.forEach(value=>restoreField('category',value));currentPage=1;applyFilters();});
    for(const name of ['yearMin','yearMax']){const select=form.elements.namedItem(name);if(select){select.replaceChildren(new Option('Any year',''));[...new Set(cards.map(card=>card.dataset.year))].sort((a,b)=>Number(a)-Number(b)).forEach(year=>select.add(new Option(year,year)));}}
    for(const name of ['brand','body','fuel','location']){const select=form.elements.namedItem(name);if(select){const title=select.options[0].textContent;select.replaceChildren(new Option(title,''));[...new Set(cards.map(card=>card.dataset[name]).filter(Boolean))].sort().forEach(value=>select.add(new Option(value,value)));}}
    const params = new URLSearchParams(location.search);
    for (const [key,value] of params) { restoreField(key,value); }
    const changed = () => { currentPage = 1; applyFilters(); }; form.addEventListener('input', changed); form.addEventListener('change', changed); form.addEventListener('submit', e => { e.preventDefault(); applyFilters(); });
    form.addEventListener('reset', () => { currentPage = 1; setTimeout(applyFilters, 0); });
    document.querySelector('#previous-page').addEventListener('click', () => { currentPage--; applyFilters(); grid.scrollIntoView({behavior:'smooth',block:'start'}); });
    document.querySelector('#next-page').addEventListener('click', () => { currentPage++; applyFilters(); grid.scrollIntoView({behavior:'smooth',block:'start'}); });
    let searches = read('mm-searches').filter(url => url.startsWith('/cars.html')).slice(0, 10);
    const searchSelect = document.querySelector('#saved-searches');
    const renderSearches = () => { searchSelect.replaceChildren(new Option('Your saved searches', '')); searches.forEach((url,i) => { const params=new URLSearchParams(url.split('?')[1]); const title=[params.get('q'),params.get('brand'),params.get('condition'),params.get('category'),params.get('engine'),params.get('body'),params.get('fuel')].filter(Boolean).join(' · ') || 'All vehicles';searchSelect.add(new Option(title, String(i))); }); };
    const storeSearches = () => { try { localStorage.setItem('mm-searches',JSON.stringify(searches)); } catch {} renderSearches(); };
    document.querySelector('#save-search').addEventListener('click', () => { const url=new URL(location.href);url.searchParams.delete('page');const value=url.pathname+url.search;if(searches.includes(value)){ announce('This search is already saved.');return;}searches.unshift(value);searches=searches.slice(0,10);storeSearches();announce('Search saved in this browser.'); });
    searchSelect.addEventListener('change', () => { if(searchSelect.value==='')return;const params=new URLSearchParams(searches[Number(searchSelect.value)].split('?')[1]);form.reset();for(const [key,value] of params){restoreField(key,value);}currentPage=1;applyFilters();document.querySelector('#delete-search').hidden=false; });
    document.querySelector('#delete-search').addEventListener('click', () => { if(searchSelect.value!==''){searches.splice(Number(searchSelect.value),1);storeSearches();document.querySelector('#delete-search').hidden=true;announce('Saved search removed.');} });
    renderSearches();
    document.querySelector('#empty-reset')?.addEventListener('click', () => form.reset()); applyFilters();
  }
  document.querySelectorAll('[data-featured-filter]').forEach(button => button.addEventListener('click', () => { document.querySelectorAll('[data-featured-filter]').forEach(b => b.classList.toggle('active', b === button)); document.querySelectorAll('.featured-grid .vehicle-card').forEach(card => { const value = button.dataset.featuredFilter; card.hidden = Boolean(value && !(value === 'Hybrid' ? ['Hybrid','Electric'].includes(card.dataset.fuel) : card.dataset.condition === value)); }); }));
  let inventoryPromise;
  const inventory = () => inventoryPromise ||= fetch('/api/vehicles').then(r => { if (!r.ok) throw Error('Vehicle data unavailable'); return r.json(); });
  const money = value => '₦' + Number(value).toLocaleString('en-NG');
  const dialog = document.querySelector('#compare-dialog');
  const element = (tag, value, className) => { const el = document.createElement(tag); if (value !== undefined) el.textContent = value; if (className) el.className = className; return el; };
  document.querySelector('#open-compare')?.addEventListener('click', async () => {
    if (compared.size < 2) { announce('Add one more vehicle to compare your options.'); return; }
    try {
      const data = await inventory(), selected = data.filter(v => compared.has(v.id));
      const wrap = document.querySelector('#compare-content'); wrap.replaceChildren();
      const table = element('table'); table.className = 'comparison-table';
      const head = element('thead'), titleRow = element('tr'); titleRow.append(element('th', 'Your shortlist'));
      selected.forEach(v => { const cell = element('th'), image = element('img'); image.src = v.image; image.alt = v.brand+' '+v.model; const link = element('a', v.brand+' '+v.model); link.href = '/car-details.html?id='+encodeURIComponent(v.id); cell.append(image,link); titleRow.append(cell); }); head.append(titleRow);table.append(head);
      const body = element('tbody');
      [['Asking price',v=>money(v.price)],['Condition',v=>v.condition === 'Used' ? 'Pre-owned' : 'Brand new'],['Model year',v=>v.year],['Mileage',v=>Number(v.mileage).toLocaleString()+' km'],['Body type',v=>v.body],['Fuel',v=>v.fuel],['Transmission',v=>v.transmission],['Drive',v=>v.drive],['Engine / motor',v=>v.engine],['Engine configuration',v=>v.engine_layout],['Collection',v=>v.categories.join(' · ')],['Features',v=>v.features.join(' · ')]].forEach(([label,value]) => { const row=element('tr');row.append(element('th',label));selected.forEach(v=>row.append(element('td',value(v))));body.append(row); });table.append(body);wrap.append(table);dialog.showModal();
    } catch { announce('Could not load the comparison. Please try again.'); }
  });
  document.querySelector('.dialog-close')?.addEventListener('click', () => dialog.close());
  dialog?.addEventListener('click', e => { if (e.target === dialog) { const box = dialog.getBoundingClientRect(); if (e.clientX < box.left || e.clientX > box.right || e.clientY < box.top || e.clientY > box.bottom) dialog.close(); } });
  const enquiry = document.querySelector('#enquiry-form');
  if (enquiry) {
    const id = new URLSearchParams(location.search).get('vehicle');
    if (id) inventory().then(data => { const v=data.find(v=>v.id===id);if(v) enquiry.elements.vehicle.value=v.year+' '+v.brand+' '+v.model+' '+v.trim; }).catch(()=>{});
    enquiry.addEventListener('submit', e => { e.preventDefault();const fields=new FormData(enquiry),subject=fields.get('vehicle') ? 'Vehicle enquiry: '+fields.get('vehicle') : 'Marvellous Motors enquiry';const body=fields.get('message')+'\n\nName: '+fields.get('name')+'\nReply email: '+fields.get('email');location.href='mailto:info@marvellousmotors.com?subject='+encodeURIComponent(subject)+'&body='+encodeURIComponent(body);announce('Your email draft is ready in your email app.'); });
  }
  updateControls();
})();
// Sum user-entered import costs in integer cents.
(() => {
  const inputs=[...document.querySelectorAll('[data-cost]')], output=document.querySelector('#estimated-total');
  if(!output)return;
  const update=()=>{let cents=0;inputs.forEach(input=>{const match=input.value.match(/^(\d+)(?:\.(\d{1,2}))?$/);if(match){const amount=Number(match[1])*100+Number((match[2]||'').padEnd(2,'0'));if(Number.isSafeInteger(amount)&&amount>=0)cents+=amount;}});output.textContent=Number.isSafeInteger(cents)?new Intl.NumberFormat('en-US',{style:'currency',currency:'USD'}).format(cents/100):'Check amounts';};
  inputs.forEach(input=>input.addEventListener('input',update));update();
})();
