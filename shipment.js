(() => {
  const root = document.querySelector('[data-shipment-tracker]');
  if (!root) return;
  const id = root.dataset.orderId;
  const labels = {dispatched:'Shipment dispatched',delivered:'Delivered & handed over',in_transit:'In transit',arrived_at_hub:'At a delivery hub',out_for_delivery:'On the final delivery leg',delivery_exception:'Delivery update needed'};
  const element = (tag, value) => { const node = document.createElement(tag); node.textContent = value; return node; };
  const date = value => { const parsed = new Date(value.includes('T') ? value : value.replace(' ', 'T')+'Z'); return Number.isNaN(parsed.getTime()) ? value : parsed.toLocaleString([], {dateStyle:'medium',timeStyle:'short'}); };
  let busy = false, eventSignature = '';
  async function refresh() {
    if (busy || document.hidden) return;
    busy = true;
    try {
      const response = await fetch('/api/shipment?order-id='+encodeURIComponent(id), {credentials:'same-origin',signal:AbortSignal.timeout(10000)});
      if (!response.ok) throw Error(response.status === 401 ? 'Sign in again to continue tracking.' : 'Connection interrupted. Your latest update is shown below.');
      const data = await response.json();
      const fields = {label:data.label,location:data.location||'No location reported yet',reported:data.last_reported||'Our team will post a checkpoint after dispatch.',carrier:data.carrier||'To be confirmed',reference:data.reference||'Not assigned yet',date:data.planned_date,driver:data.driver||'To be confirmed'};
      Object.entries(fields).forEach(([key,value]) => { root.querySelector('[data-shipment-'+key+']').textContent=value; });
      root.querySelectorAll('[data-milestone]').forEach(node => { const step=Number(node.dataset.milestone);node.classList.toggle('complete',step<=data.progress);node.classList.toggle('current',step===data.progress&&data.progress<5);if(step===data.progress)node.setAttribute('aria-current','step');else node.removeAttribute('aria-current'); });
      const signature=data.events.map(event=>event.id).join(',');
      if (signature !== eventSignature) {
        const history=root.querySelector('[data-shipment-events]');
        if(data.events.length){const fragment=document.createDocumentFragment();data.events.forEach(event=>{const article=document.createElement('article'), details=document.createElement('div');details.append(element('h3',labels[event.stage]||event.stage),element('strong',event.location),element('p',event.note),element('small',date(event.created_at)));article.append(element('span','✓'),details);fragment.append(article);});history.replaceChildren(fragment);}
        eventSignature=signature;
      }
      root.querySelector('[data-shipment-sync]').textContent='Checked '+new Date(data.checked_at).toLocaleTimeString([], {hour:'2-digit',minute:'2-digit'});
      root.querySelector('.shipment-live-pill').classList.remove('connection-error');
    } catch(error) { root.querySelector('[data-shipment-sync]').textContent=error.message;root.querySelector('.shipment-live-pill').classList.add('connection-error'); }
    finally { busy=false; }
  }
  refresh();setInterval(refresh,5000);document.addEventListener('visibilitychange',refresh);
})();
