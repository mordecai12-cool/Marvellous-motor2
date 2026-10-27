(() => {
 const money=value=>new Intl.NumberFormat('en-NG',{style:'currency',currency:'NGN'}).format(value/100);
 const node=(tag,text)=>{const el=document.createElement(tag);if(text!==undefined)el.textContent=text;return el;};
 document.querySelectorAll('[data-selected-vehicle]').forEach(select=>{const value=new URLSearchParams(location.search).get('vehicle');if(value&&[...select.options].some(o=>o.value===value))select.value=value;});
 document.querySelectorAll('[data-quote-form]').forEach(form=>{
  const widget=form.querySelector('[data-instant-quote]');if(!widget)return;
  const field=name=>form.elements.namedItem(name),lines=widget.querySelector('[data-quote-lines]'),feedback=widget.querySelector('[data-quote-feedback]'),notes=widget.querySelector('[data-quote-notes]'),total=widget.querySelector('[data-quote-total]'),source=widget.querySelector('[data-quote-source]'),order=form.querySelector('[data-quote-order]'),submit=form.querySelector('[data-quote-continue]');
  let timer,sequence=0,controller;
  const params=new URLSearchParams(location.search);for(const name of ['vehicle','country','state','city']){if(params.has(name)&&field(name))field(name).value=params.get(name);}
  const invalidate=()=>{field('quote-token').value='';if(order)order.hidden=true;};
  async function update(){
   const turn=++sequence;controller?.abort();invalidate();source.hidden=true;lines.replaceChildren();total.textContent='—';notes.textContent='';
   const vehicle=field('vehicle')?.value||field('vehicle-id')?.value,country=field('country')?.value.trim(),state=field('state')?.value.trim(),city=field('city')?.value.trim();
   if(!vehicle||!country||!state||!city){feedback.textContent='Choose your car and enter the destination country, state / region and city.';return;}
   feedback.textContent='Calculating your delivered price…';controller=new AbortController();
   try{
    const response=await fetch('/api/quote?'+new URLSearchParams({vehicle,country,state,city}),{credentials:'same-origin',signal:controller.signal});const data=await response.json();if(turn!==sequence)return;if(!response.ok)throw Error(data.error||'Could not calculate this route.');
    const dl=node('dl');for(const line of data.lines){const row=node('div');row.append(node('dt',line.label),node('dd',money(line.amount)));dl.append(row);}lines.append(dl);
    total.textContent=money(data.total);widget.querySelector('[data-quote-total-label]').textContent=data.complete?'Total including configured costs':'Known subtotal · costs still unconfirmed';notes.textContent=data.notes||'';
    feedback.textContent=!data.complete?'This route has unconfirmed costs. The amount shown is a subtotal, not an all-in total.':data.demo?'Sample listing / provisional rates — a planning estimate.':data.estimated?'Planning estimate. An admin must confirm this route before payment.':'All configured costs included. Your total is ready.';
    if(data.offer)feedback.textContent+=' Offer applied: '+data.offer+'.';if(data.source?.startsWith('https://')){source.href=data.source;source.hidden=false;}
    if(data.can_order){field('quote-token').value=data.token;if(order){order.href='/start-order?'+new URLSearchParams({vehicle,country,state,city});order.hidden=false;}}
   }catch(error){if(error.name!=='AbortError'&&turn===sequence)feedback.textContent=error.message||'Could not calculate this route. Please retry.';}
  }
  const schedule=event=>{
   if(!['vehicle','vehicle-id','country','state','city'].includes(event.target.name))return;
   invalidate();sequence++;controller?.abort();clearTimeout(timer);timer=setTimeout(update,event.type==='change'?50:350);
  };
  form.addEventListener('input',schedule);form.addEventListener('change',schedule);
  form.querySelector('[data-quote-refresh]')?.addEventListener('click',()=>{clearTimeout(timer);update();});
  if(submit)form.addEventListener('submit',async event=>{
   if(field('quote-token').value){submit.disabled=true;return;}
   event.preventDefault();clearTimeout(timer);await update();
   if(field('quote-token').value)form.requestSubmit(submit);
   else feedback.scrollIntoView({block:'center',behavior:'smooth'});
  });
  if(!submit)form.addEventListener('submit',event=>{event.preventDefault();update();});update();
 });
})();
