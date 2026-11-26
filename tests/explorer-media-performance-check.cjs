const assert=require('node:assert/strict');
const {setup,wait,activateEntry}=require('./explorer-check.cjs');
const ready=async t=>{await wait(()=>t.$('entries').children.length,'boot');t.$('location').value='E:\\demo';t.$('go').click();await wait(()=>!t.$('newFolder').disabled,'directory')};
function imageResponse(bytes=1024){let sent=false;return {ok:true,status:200,headers:{get:()=> 'image/jpeg'},body:{getReader:()=>({read:async()=>sent?{done:true}:(sent=true,{value:new Uint8Array(bytes),done:false}),cancel:async()=>{},releaseLock(){}})}}}
(async()=>{
 let revoked=[],created=0;
 const t=setup(true,{requestHook:({route,url})=>route==='raw'?imageResponse(url.searchParams.get('path').includes('large-')?5*1024*1024:1024):null});await ready(t);Object.defineProperty(t.d,'hidden',{value:false,configurable:true});
 t.w.URL.createObjectURL=()=> 'blob:http://localhost/image-'+(++created);t.w.URL.revokeObjectURL=url=>revoked.push(url);
 const raw=()=>t.calls.filter(c=>c.route==='raw');
 activateEntry(t.$('entries').children[1].querySelector('.entry-open'));let img=t.$('viewerBody').querySelector('img');await wait(()=>img.src.startsWith('blob:'),'foreground cached image');assert.equal(raw().length,1);
 img.dispatchEvent(new t.w.Event('load'));await wait(()=>raw().length===2,'only next image prefetched',1500);assert.equal(raw()[1].signal.aborted,false);assert.equal(raw()[1].url.searchParams.get('path'),'E:\\demo\\第二张.png');
 t.$('viewerNext').click();img=t.$('viewerBody').querySelector('img');await wait(()=>img.src.startsWith('blob:'),'next image reuses prefetched bytes');assert.equal(raw().length,2);assert(revoked.length>0,'leaving releases the old object URL');
 t.$('viewerPrevious').click();await wait(()=>t.$('viewerBody').querySelector('img').src.startsWith('blob:'),'previous image cache hit');assert.equal(raw().length,2);
 t.$('viewerClose').click();assert.equal(t.w.__imageCacheQA.state().pending,0);assert.equal(t.w.__imageCacheQA.state().count,2);
 // Seven 5 MiB bodies cannot grow the encoded cache beyond 24 MiB.
 const read=t.w.__imageCacheQA.read;
 for(let i=0;i<7;i++){await read({Path:'E:\\demo\\large-'+i+'.jpg',Size:5*1024*1024,Version:'v'+i},'',new t.w.AbortController().signal)}
 assert(t.w.__imageCacheQA.state().count<=6);assert(t.w.__imageCacheQA.state().bytes<=24*1024*1024);
 t.w.__imageCacheQA.clear();assert.deepEqual({...t.w.__imageCacheQA.state()},{count:0,bytes:0,pending:0});assert.equal(t.errors.length,0);t.dom.window.close();

 let aborted=false,release;
 const slow=setup(true,{requestHook:({route,signal})=>route==='raw'?new Promise(resolve=>{release=()=>resolve(imageResponse());signal.addEventListener('abort',()=>{aborted=true},{once:true})}):null});await ready(slow);
 activateEntry(slow.$('entries').children[1].querySelector('.entry-open'));const old=slow.$('viewerBody').querySelector('img');slow.$('viewerClose').click();assert(aborted,'closing aborts foreground IO');release();await new Promise(r=>setTimeout(r,0));assert.equal(old.getAttribute('src'),null);assert.equal(slow.w.__imageCacheQA.state().count,0,'late response cannot populate cache');slow.dom.window.close();

 let expired=false;const remote=setup(false,{requestHook:({route})=>route==='raw'?imageResponse():expired&&route==='preferences'?{ok:false,status:401,text:async()=> 'expired',headers:{get:()=>null}}:null});await wait(()=>remote.$('entries').children.length,'remote boot');
 await remote.w.__imageCacheQA.read({Path:'photo.jpg',Size:123,Version:'one'},'share-one',new remote.w.AbortController().signal);
 await remote.w.__imageCacheQA.read({Path:'photo.jpg',Size:123,Version:'one'},'share-two',new remote.w.AbortController().signal);
 assert.equal(remote.calls.filter(c=>c.route==='raw').length,2,'same filename in distinct shares must not share cache');expired=true;await remote.w.__imageCacheQA.api('preferences').catch(()=>{});assert.equal(remote.w.__imageCacheQA.state().count,0,'expired session clears private cache');remote.dom.window.close();
 console.log('PASS: bounded image bytes, next-image prefetch/cache reuse, URL release, cancellation/late response isolation and shared-session cleanup');
})().catch(e=>{console.error(e);process.exitCode=1});
