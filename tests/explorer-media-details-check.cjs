const assert=require('node:assert/strict');
const {setup,wait,activateEntry}=require('./explorer-check.cjs');
const ready=async(t,path)=>wait(()=>t.calls.some(call=>call.route==='list')&&t.$('entries').children.length>0&&t.$('location').value===path&&t.$('notice').textContent!=='正在读取目录…','ready '+path);
const go=async(t,path)=>{t.$('location').value=path;t.$('go').click();await ready(t,path)};
const flush=()=>new Promise(resolve=>setTimeout(resolve,25));
const videos=Array.from({length:1000},(_,i)=>({Name:`video${i}.mp4`,Path:`E:\\demo\\video${i}.mp4`,Kind:'video',Size:1000+i,Modified:'2026-01-01T00:00:00Z'}));
const observers=[];
class Observer {
 constructor(callback,options){this.callback=callback;this.options=options;this.targets=new Set();observers.push(this)}
 observe(target){this.targets.add(target)} unobserve(target){this.targets.delete(target)} disconnect(){this.targets.clear()}
 fire(targets,visible=true){this.callback(targets.map(target=>({target,isIntersecting:visible})))}
}
(async()=>{
 const pending=[],t=setup(true,{extraEntries:videos,IntersectionObserver:Observer,mediaInfoHook:({body,signal})=>new Promise((resolve,reject)=>{const task={body,signal,resolve:()=>resolve({ok:true,json:async()=>({Duration:125,Width:1920,Height:1080})})};pending.push(task);signal.addEventListener('abort',()=>reject(Object.assign(Error('cancelled'),{name:'AbortError'})),{once:true})})});
 await ready(t,'');await go(t,'E:\\demo');
 assert.equal(t.calls.filter(call=>call.route==='media-info').length,0,'1000 offscreen videos must not start probes');
 const observer=observers.at(-1),cards=[...observer.targets];assert.equal(cards.length,1000);assert.equal(observer.options.rootMargin,'180px 0px');
 observer.fire(cards.slice(0,5));await wait(()=>pending.length===2,'two visible probes');assert.equal(pending.length,2,'concurrency limited to two');
 pending[0].resolve();await wait(()=>pending.length===3,'next visible probe');await wait(()=>cards[0].querySelector('.entry-duration').textContent==='02:05','duration label');assert(cards[0].querySelector('.entry-duration').title.includes('1920 × 1080'));
 observer.fire(cards.slice(3,5),false);pending[1].resolve();pending[2].resolve();await flush();assert.equal(pending.length,3,'offscreen queued files must be removed');
 observer.fire([cards[10]]);await wait(()=>pending.length===4,'additional visible video');const cancelled=pending[3];
 await go(t,'E:\\other');assert(cancelled.signal.aborted,'directory navigation cancels old media requests');assert.equal(observer.targets.size,0);
 await go(t,'E:\\demo');const next=observers.at(-1),first=[...next.targets][0],before=pending.length;next.fire([first]);await wait(()=>first.querySelector('.entry-duration').textContent==='02:05','cached duration');assert.equal(pending.length,before,'returning to unchanged media uses cache');
 assert.equal(t.errors.length,0,t.errors.join('\n'));t.dom.window.close();

 // The preview's video queue shares the lazy scheduler/cache with the directory.
 const queuePending=[],q=setup(false,{extraEntries:videos.map(item=>({...item,Path:item.Name})),IntersectionObserver:Observer,mediaInfoHook:({body,signal})=>body.Lightweight?{ok:true,json:async()=>({Fragmented:false})}:new Promise((resolve,reject)=>{queuePending.push({body,signal,resolve});signal.addEventListener('abort',()=>reject(Object.assign(Error('cancelled'),{name:'AbortError'})),{once:true})})});
 await ready(q,'演示共享');const sharedObserver=observers.at(-1),mainCards=[...sharedObserver.targets];
 sharedObserver.fire([mainCards[0]]);await wait(()=>queuePending.length===1,'directory duration pending');
 activateEntry(q.d.querySelector('[data-kind=video] .entry-open'));q.$('viewerQueueToggle').click();
 const queueCards=[...q.$('viewerQueueItems').children];assert.equal(queueCards.length,1000);assert.equal(queueCards.filter(card=>card.querySelector('.queue-duration')).length,1000);
 sharedObserver.fire(queueCards.slice(0,4));await flush();assert.equal(queuePending.length,1,'same video in directory and preview shares its in-flight probe');
 queuePending[0].resolve({ok:true,json:async()=>({Duration:3661,Width:1920,Height:1080})});
 await wait(()=>queueCards[0].querySelector('.queue-duration').textContent==='1:01:01','preview queue duration from shared cache');await wait(()=>queuePending.length===3,'two additional preview probes');assert.equal(queuePending.length,3,'1000-file queue probes only visible rows, max two at a time');
 assert(q.calls.filter(call=>call.route==='media-info').every(call=>call.url.searchParams.get('root')==='share-id'),'queue probes retain shared-root scope');
 q.$('viewerQueueClose').click();assert.equal(q.$('viewerQueueItems').children.length,0);assert(queuePending.slice(1).every(task=>task.signal.aborted),'hiding queue cancels its active requests');await flush();
 q.$('viewerQueueToggle').click();const reopened=[...q.$('viewerQueueItems').children];sharedObserver.fire([reopened[0]]);await wait(()=>reopened[0].querySelector('.queue-duration').textContent==='1:01:01','reopened queue reuses duration');assert.equal(queuePending.length,3);
 sharedObserver.fire([reopened[1]]);await wait(()=>queuePending.length===4,'retry cancelled queue row');queuePending[3].resolve({ok:false,status:400,text:async()=>'invalid video'});await wait(()=>reopened[1].querySelector('.queue-duration').textContent==='未知','unreadable queue duration');
 sharedObserver.fire([reopened[4]]);await wait(()=>queuePending.length===5,'queue request before viewer close');q.$('viewerClose').click();assert(queuePending[4].signal.aborted,'closing preview cancels queue probes');assert.equal(q.errors.length,0,q.errors.join('\n'));q.dom.window.close();

 // Properties expose useful media details while keeping untrusted values as text.
 const properties=setup(false,{extraEntries:[{Name:'clip.mp4',Path:'clip.mp4',Kind:'video',Size:100}],propertiesHook:({body})=>({ok:true,json:async()=>({Name:'clip.mp4',Path:body.Path,Kind:'video',Size:100,Width:1920,Height:1080,Duration:3661,VideoCodec:'h264',AudioCodec:'aac',Container:'mp4'})})});
 await ready(properties,'演示共享');const video=properties.d.querySelector('[data-kind=video] .entry-open');video.dispatchEvent(new properties.w.MouseEvent('contextmenu',{bubbles:true,cancelable:true}));[...properties.$('contextMenu').querySelectorAll('button')].find(b=>b.textContent==='属性').click();await wait(()=>properties.$('propertiesList').textContent.includes('1920 × 1080'),'video resolution');assert(properties.$('propertiesList').textContent.includes('1:01:01'));assert(properties.$('propertiesList').textContent.includes('h264'));assert(!properties.$('propertiesList').textContent.includes('E:\\'));properties.dom.window.close();

 // History retains scroll, pagination and view even after a page refresh.
 const h=setup(true,{listHook:({body})=>body.Path?{ok:true,json:async()=>({Path:body.Path,Parent:'E:\\parent',Entries:[{Name:'child',Path:'E:\\parent\\child',Dir:true,Kind:'file'}],Total:2000,Offset:body.Offset})}:null});
 let y=0;Object.defineProperty(h.w,'scrollY',{get:()=>y,configurable:true});h.w.scrollTo=options=>{y=options.top};await ready(h,'');await go(h,'E:\\parent');await flush();h.$('view').click();h.$('next').click();await wait(()=>h.calls.filter(call=>call.route==='list').at(-1).body.Offset===200&&h.$('notice').textContent!=='正在读取目录…','parent second page');await flush();y=1850;
 activateEntry(h.d.querySelector('.entry-open'));await ready(h,'E:\\parent\\child');await flush();y=250;h.$('back').click();await ready(h,'E:\\parent');await flush();assert.equal(y,1850);assert.equal(h.calls.filter(call=>call.route==='list').at(-1).body.Offset,200);assert(h.$('entries').classList.contains('list'));
 h.$('forward').click();await ready(h,'E:\\parent\\child');await flush();assert.equal(y,250);const saved=h.w.sessionStorage.getItem('hamster.files.tabs.v1.local');h.dom.window.close();
 const refreshed=setup(true,{workspace:saved});let restoredY=0;refreshed.w.scrollTo=options=>{restoredY=options.top};await ready(refreshed,'E:\\parent\\child');await flush();refreshed.$('back').click();await ready(refreshed,'E:\\parent');await flush();assert.equal(restoredY,1850);assert.equal(refreshed.calls.filter(call=>call.route==='list').at(-1).body.Offset,200);refreshed.dom.window.close();
 console.log('PASS: 1000-file directory/preview duration, shared cache and in-flight deduplication, two-probe limit, hidden/closed queue cancellation, failure fallback, scoped properties and scroll/page/view restoration');
})().catch(error=>{console.error(error);process.exitCode=1});
