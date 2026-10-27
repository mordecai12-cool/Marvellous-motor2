(() => {
  const make = (tag, text, className) => { const node=document.createElement(tag);if(text!==undefined)node.textContent=text;if(className)node.className=className;return node; };
  const csrf = () => document.cookie.split('; ').find(cookie=>cookie.startsWith('mm-csrf='))?.split('=')[1]||'';
  const nonce = () => crypto.randomUUID();
  async function api(path, fields, file) {
    const options={credentials:'same-origin',signal:AbortSignal.timeout(10000)};
    if(fields){options.method='POST';if(file){const data=new FormData();for(const [key,value] of Object.entries(fields))data.append(key,value);data.append('csrf-token',csrf());data.append('attachment',file);options.body=data;options.signal=AbortSignal.timeout(30000);}else options.body=new URLSearchParams({...fields,'csrf-token':csrf()});}
    const response=await fetch(path,options);
    const json=response.headers.get('content-type')?.includes('application/json');
    const data=json?await response.json():{error:'Please reload this page before trying again.'};
    if(!response.ok){const error=Error(data.error||'Could not connect. Please try again.');error.status=response.status;throw error;}
    return data;
  }
  document.querySelectorAll('[data-support-file]').forEach(input=>{const clear=input.closest('.care-file-picker').querySelector('[data-clear-file]');input.addEventListener('change',()=>{clear.hidden=!input.files.length;});clear.addEventListener('click',()=>{input.value='';input.dispatchEvent(new Event('change'));});});
  const timestamp = value => new Date(value*1000).toLocaleString([], {month:'short',day:'numeric',hour:'2-digit',minute:'2-digit'});
  function renderMessages(feed,messages,ownKind,known) {
    const nearBottom=feed.scrollHeight-feed.scrollTop-feed.clientHeight<80, first=known.size===0;
    for(const message of messages){if(known.has(message.id))continue;
      feed.querySelector('.care-empty')?.remove();
      const article=make('article',undefined,'care-message '+(message.sender_kind===ownKind?'mine':'theirs'));article.dataset.messageId=message.id;
      const name=make('strong',message.sender_kind===ownKind?'You':message.sender_name,'care-message-name'),body=make('p',message.body),meta=make('small',timestamp(message.created_at));article.append(name,body);
      if(message.order_id){const link=make('a','Order #'+String(message.order_id).padStart(4,'0'),'care-order-link');link.href=(ownKind==='staff'?'/admin/order?id=':'/order-status?order-id=')+message.order_id;article.append(link);}
      if(message.attachment){const attachment=message.attachment;const link=make('a',undefined,'care-attachment');link.href=attachment.url;link.target='_blank';link.rel='noopener';if(attachment.type.startsWith('image/')){const image=make('img');image.src=attachment.url;image.alt=attachment.name;image.loading='lazy';link.append(image);}link.append(make('span',attachment.name+' · '+Math.max(1,Math.ceil(attachment.size/1024))+' KB'));article.append(link);}
      article.append(meta);
      const next=[...feed.querySelectorAll('[data-message-id]')].find(node=>Number(node.dataset.messageId)>message.id);feed.insertBefore(article,next||null);known.set(message.id,message);
    }
    if(first||nearBottom)feed.scrollTop=feed.scrollHeight;
  }
  const widget=document.querySelector('[data-support-widget]');
  if(widget){
    const embedded=document.body.classList.contains('support-page'),panel=widget.querySelector('.care-panel'),launcher=widget.querySelector('.care-launcher'),feed=widget.querySelector('.care-messages'),form=widget.querySelector('.care-compose'),input=form.querySelector('textarea'),send=form.querySelector('button[type=submit]'),context=form.querySelector('select'),feedback=widget.querySelector('.care-feedback'),older=widget.querySelector('.care-older');
    const fileInput=form.querySelector('[data-support-file]');
    const known=new Map();let authenticated=false,opened=false,busy=false,sending=false,pending=null,lastRead=0;
    const orderId=new URLSearchParams(location.search).get('order-id');
    if(orderId&&/^\d+$/.test(orderId)&&Number(orderId)>0){const option=new Option('About order #'+orderId,orderId);context.add(option);context.value=orderId;}
    const empty=()=>{if(!known.size&&!feed.querySelector('.care-empty')){const box=make('div',undefined,'care-empty');box.append(make('span','◇'),make('h3','Tell us what you need.'),make('p','Send your issue below. Our team will reply in this conversation.'));feed.append(box);}};
    async function refresh(before=0){
      if(busy||document.hidden||(!opened&&!authenticated))return;
      busy=true;
      try{
        const data=await api('/api/support'+(before?'?before='+before:''));authenticated=true;widget.querySelector('.care-gate').hidden=true;form.hidden=false;feed.hidden=false;widget.querySelector('.care-quick-replies').hidden=false;input.disabled=false;send.disabled=sending;
        const presence=widget.querySelector('.care-presence');presence.textContent=data.online?'A team member is online':'Leave a message · replies appear here';presence.classList.toggle('online',data.online);
        renderMessages(feed,data.messages,'customer',known);empty();widget.querySelector('.care-intro').hidden=known.size>0;widget.querySelector('.care-quick-replies').hidden=known.size>0;older.hidden=!data.has_older;
        const unread=data.conversation?.unread_customer||0,badge=widget.querySelector('.care-unread');badge.hidden=unread===0;badge.textContent=String(unread);
        if(data.conversation){const ref=widget.querySelector('.care-case-reference')||make('p',undefined,'care-case-reference');ref.textContent=data.conversation.case_reference+' · '+data.conversation.case_status.replaceAll('_',' ')+(data.conversation.assignee?' · '+data.conversation.assignee:'');widget.querySelector('.care-header').after(ref);}
        if(data.conversation?.status==='resolved')feedback.textContent='This issue was marked resolved. Sending a new message reopens the conversation.';
        if(opened&&known.size){const id=Math.max(...known.keys());if(id>lastRead){await api('/api/support/read',{'message-id':id});lastRead=id;badge.hidden=true;}}
      }catch(error){if(error.status===401){authenticated=false;widget.querySelector('.care-gate').hidden=false;form.hidden=true;feed.hidden=true;older.hidden=true;widget.querySelector('.care-quick-replies').hidden=true;widget.querySelector('.care-presence').textContent='Private customer support';}else{feedback.textContent='Connection interrupted. Your draft is saved on this page. Please retry.';}}
      finally{busy=false;}
    }
    async function open(){opened=true;panel.hidden=false;launcher.setAttribute('aria-expanded','true');await refresh();if(authenticated)input.focus();}
    function close(){if(embedded)return;opened=false;panel.hidden=true;launcher.setAttribute('aria-expanded','false');launcher.focus();}
    launcher.addEventListener('click',()=>opened?close():open());widget.querySelector('.care-close').addEventListener('click',close);document.addEventListener('keydown',event=>{if(event.key==='Escape'&&opened)close();});document.querySelectorAll('[data-open-chat]').forEach(button=>button.addEventListener('click',open));
    input.addEventListener('input',()=>widget.querySelector('.care-count').textContent=input.value.length+' / 2,000');
    widget.querySelectorAll('[data-care-topic]').forEach(button=>button.addEventListener('click',()=>{input.value=button.dataset.careTopic;input.dispatchEvent(new Event('input'));input.focus();}));
    older.addEventListener('click',()=>{if(known.size)refresh(Math.min(...known.keys()));});
    form.addEventListener('submit',async event=>{event.preventDefault();if(sending)return;const text=input.value.trim(),file=fileInput.files[0];if(!text&&!file)return;if(file&&file.size>5*1024*1024){feedback.textContent='Attachments must be 5 MB or smaller.';return;}
      const order=context.value;if(!pending||pending.text!==text||pending.order!==order||pending.file!==file)pending={text,order,file,id:nonce()};sending=true;send.disabled=true;feedback.textContent='Sending…';
      try{await api('/api/support/send',{'message':text,'order-id':order,'client-id':pending.id},pending.file);input.value='';fileInput.value='';fileInput.dispatchEvent(new Event('change'));input.dispatchEvent(new Event('input'));pending=null;feedback.textContent='Message sent.';await refresh();}catch(error){feedback.textContent=error.message;}finally{sending=false;send.disabled=!authenticated;}
    });
    setInterval(()=>{if(opened||authenticated)refresh();},4000);document.addEventListener('visibilitychange',()=>{if(opened)refresh();});if(embedded)open();
  }
  const inbox=document.querySelector('[data-support-inbox]');
  if(inbox){
    const list=inbox.querySelector('[data-inbox-list]'),feed=inbox.querySelector('[data-inbox-messages]'),form=inbox.querySelector('.inbox-compose'),input=form.querySelector('textarea'),send=form.querySelector('button[type=submit]'),feedback=inbox.querySelector('[data-inbox-feedback]'),availability=inbox.querySelector('[data-inbox-available]'),resolve=inbox.querySelector('[data-inbox-resolve]'),older=inbox.querySelector('[data-inbox-older]'),search=inbox.querySelector('[data-inbox-search]');
    const fileInput=form.querySelector('[data-support-file]'),caseForm=inbox.querySelector('[data-case-form]');
    const known=new Map();let selected=0,status='open',busy=false,sending=false,pending=null,lastRead=0,listSignature='';
    const initial=new URLSearchParams(location.search).get('id');if(initial&&/^\d+$/.test(initial))selected=Number(initial);
    async function presence(){try{await api('/api/admin/support/presence',{available:String(availability.checked&&!document.hidden)});inbox.querySelector('[data-inbox-presence]').textContent=availability.checked&&!document.hidden?'Available for chat':'Away';}catch(error){inbox.querySelector('[data-inbox-presence]').textContent='Disconnected';feedback.textContent=error.message;}}
    function select(id){if(sending){feedback.textContent='Please wait for your reply to finish sending.';return;}if(id!==selected){selected=id;input.disabled=true;send.disabled=true;inbox.querySelector('[data-inbox-name]').textContent='Loading conversation…';known.clear();feed.replaceChildren();pending=null;input.value='';fileInput.value='';fileInput.dispatchEvent(new Event('change'));fileInput.disabled=true;lastRead=0;listSignature='';}const url=new URL(location.href);url.searchParams.set('id',id);history.replaceState(null,'',url);refresh();}
    async function refresh(before=0){if(busy||document.hidden)return;busy=true;
      try{
        const params=new URLSearchParams();if(selected)params.set('id',selected);if(search.value.trim())params.set('q',search.value.trim());if(before)params.set('before',before);
        const requested=selected;const data=await api('/api/admin/support?'+params);if(requested!==selected){busy=false;return refresh();}
        const signature=JSON.stringify((data.conversations||[]).map(c=>[c.id,c.status,c.last_message,c.unread_staff,c.id===selected]));
        if(signature!==listSignature){list.replaceChildren();for(const c of data.conversations||[]){const button=make('button',undefined,'inbox-conversation'+(c.id===selected?' selected':''));button.type='button';button.dataset.conversationId=c.id;button.append(make('span',c.name,'inbox-person'),make('small',c.email),make('p',c.last_message),make('span',c.status==='resolved'?'Resolved':'Open','inbox-thread-stage'));if(c.unread_staff)button.append(make('b',String(c.unread_staff),'inbox-unread'));button.addEventListener('click',()=>select(c.id));list.append(button);}if(!data.conversations?.length)list.append(make('p','No conversations match this view. Customer messages will appear here.','inbox-empty'));listSignature=signature;}
        inbox.querySelector('.inbox-count').textContent=String(data.conversations?.length||0);
        if(!selected&&data.conversations?.length){selected=data.conversations[0].id;busy=false;return refresh();}
        if(data.conversation){const c=data.conversation;status=c.status;if(caseForm){caseForm.hidden=false;caseForm.querySelector('[data-case-reference]').textContent=c.case_reference;const assignee=caseForm.elements.namedItem('assigned');if(document.activeElement?.closest('[data-case-form]')!==caseForm){assignee.replaceChildren(new Option('Unassigned','0'));for(const person of data.available_staff||[])assignee.add(new Option(person.name,person.id));if(c.assigned_id && ![...assignee.options].some(option=>option.value===String(c.assigned_id)))assignee.add(new Option(c.assignee||'Assigned team',c.assigned_id));assignee.value=String(c.assigned_id);caseForm.elements.namedItem('case-status').value=c.case_status;}} inbox.querySelector('[data-inbox-name]').textContent=c.name;inbox.querySelector('[data-inbox-email]').textContent=c.email;inbox.querySelector('[data-inbox-status]').textContent=status==='resolved'?'Resolved':'Open conversation';resolve.hidden=false;resolve.textContent=status==='resolved'?'Reopen conversation':'Mark resolved';input.disabled=status==='resolved';fileInput.disabled=sending||status==='resolved';send.disabled=sending||status==='resolved';older.hidden=!data.has_older;renderMessages(feed,data.messages,'staff',known);if(known.size){const id=Math.max(...known.keys());if(id>lastRead){await api('/api/admin/support/read',{'conversation-id':selected,'message-id':id});lastRead=id;}}}
      }catch(error){feedback.textContent=error.message;}finally{busy=false;}
    }
    form.addEventListener('submit',async event=>{event.preventDefault();if(sending||!selected)return;const text=input.value.trim(),file=fileInput.files[0];if(!text&&!file)return;if(file&&file.size>5*1024*1024){feedback.textContent='Attachments must be 5 MB or smaller.';return;}if(!pending||pending.text!==text||pending.conversation!==selected||pending.file!==file)pending={text,conversation:selected,file,id:nonce()};sending=true;send.disabled=true;feedback.textContent='Sending reply…';try{await api('/api/admin/support/send',{'conversation-id':selected,'message':text,'client-id':pending.id},pending.file);input.value='';fileInput.value='';fileInput.dispatchEvent(new Event('change'));pending=null;feedback.textContent='Reply sent.';await refresh();}catch(error){feedback.textContent=error.message;}finally{sending=false;send.disabled=status==='resolved';}});
    caseForm?.addEventListener('submit',async event=>{event.preventDefault();if(!selected)return;try{await api('/api/admin/support/case',{'conversation-id':selected,'case-status':caseForm.elements.namedItem('case-status').value,assigned:caseForm.elements.namedItem('assigned').value});feedback.textContent='Complaint assignment and status saved.';document.activeElement?.blur();await refresh();}catch(error){feedback.textContent=error.message;}});
    resolve.addEventListener('click',async()=>{if(!selected)return;try{await api('/api/admin/support/status',{'conversation-id':selected,'status':status==='resolved'?'open':'resolved'});await refresh();}catch(error){feedback.textContent=error.message;}});
    older.addEventListener('click',()=>{if(known.size)refresh(Math.min(...known.keys()));});availability.addEventListener('change',presence);let searchTimer;search.addEventListener('input',()=>{clearTimeout(searchTimer);searchTimer=setTimeout(refresh,300);});
    document.addEventListener('visibilitychange',()=>{presence();if(!document.hidden)refresh();});window.addEventListener('pagehide',()=>{navigator.sendBeacon('/api/admin/support/presence',new URLSearchParams({'available':'false','csrf-token':csrf()}));});
    presence();refresh();setInterval(presence,20000);setInterval(refresh,3000);
  }
})();
