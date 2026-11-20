const assert=require('node:assert/strict');
const {setup,wait}=require('./explorer-check.cjs');
const key='hamster.files.tabs.v1.local';
const state=t=>JSON.parse(t.w.sessionStorage.getItem(key));
const active=t=>state(t)?.tabs.find(tab=>tab.id===state(t).active);
const ready=async(t,path)=>{try{await wait(()=>t.d.querySelectorAll('#fileTabs [role=tab]').length>0&&active(t)?.path===path&&t.$('location').value===path&&t.$('notice').textContent!=='正在读取目录…','ready: '+path)}catch(error){console.error({wanted:path,actual:t.$('location').value,notice:t.$('notice').textContent,workspace:state(t),errors:t.errors});throw error}};
const go=async(t,path)=>{t.$('location').value=path;t.$('go').click();await ready(t,path)};
const add=async t=>{const count=t.d.querySelectorAll('#fileTabs [role=tab]').length;t.$('newTab').click();await wait(()=>t.d.querySelectorAll('#fileTabs [role=tab]').length===count+1&&t.$('notice').textContent!=='正在读取目录…','new internal tab')};

(async()=>{
 const first=setup(true);await ready(first,'');await go(first,'E:\\albums');
 first.$('view').click();first.$('search').value='示例';first.$('search').dispatchEvent(new first.w.Event('input'));await wait(()=>active(first).search==='示例','saved search');
 await add(first);await go(first,'E:\\videos');const secondID=state(first).active,firstID=state(first).tabs[0].id;
 await go(first,'E:\\videos\\clips');first.$('back').click();await ready(first,'E:\\videos');assert.equal(first.$('forward').disabled,false);first.$('forward').click();await ready(first,'E:\\videos\\clips');
 first.d.getElementById(firstID).click();await ready(first,'E:\\albums');assert.equal(first.$('search').value,'示例');assert.equal(first.$('entries').classList.contains('list'),true);assert.equal(first.$('forward').disabled,true,'history from another tab must not leak');
 first.d.getElementById(secondID).click();await ready(first,'E:\\videos\\clips');assert.equal(first.$('search').value,'','new tabs start without the previous tab search');
 const saved=first.w.sessionStorage.getItem(key);assert(!/Password|Version|selection|owner-only-token/.test(saved),'workspace must not store credentials, file references or selections');first.dom.window.close();
 const restored=setup(true,{workspace:saved});await ready(restored,'E:\\videos\\clips');assert.equal(restored.d.querySelectorAll('#fileTabs [role=tab]').length,2);assert.equal(state(restored).active,secondID);restored.$('back').click();await ready(restored,'E:\\videos');
 restored.d.querySelector('.browser-tab.active .tab-close').click();await ready(restored,'E:\\albums');assert.equal(restored.d.querySelectorAll('#fileTabs [role=tab]').length,1);assert.equal(restored.$('search').value,'示例');assert.equal(restored.d.querySelector('.tab-close').disabled,true);assert.equal(restored.errors.length,0);restored.dom.window.close();

 // A slow response from a previous tab cannot replace the active location.
 let release;const delayed=setup(true,{listHook:({body})=>body.Path==='E:\\slow'?new Promise(resolve=>{release=()=>resolve({ok:true,json:async()=>({Path:'E:\\slow',Parent:'E:\\',Entries:[],Total:0,Offset:0})})}):null});
 await ready(delayed,'');await go(delayed,'E:\\first');await add(delayed);await go(delayed,'E:\\second');const tabA=state(delayed).tabs[0].id;
 delayed.$('location').value='E:\\slow';delayed.$('go').click();await wait(()=>Boolean(release),'slow listing pending');delayed.d.getElementById(tabA).click();await ready(delayed,'E:\\first');release();await new Promise(resolve=>setTimeout(resolve,20));assert.equal(delayed.$('location').value,'E:\\first');assert.equal(active(delayed).path,'E:\\first');delayed.dom.window.close();

 // Deleted/unavailable restore targets fall back only after a genuine failure.
 const missing=setup(true,{workspace:saved,listHook:({body})=>body.Path==='E:\\videos\\clips'?{ok:false,status:400,text:async()=>'directory missing'}:null});
 await wait(()=>missing.$('notice').textContent.includes('上次的目录已不存在'),'unavailable restore explanation');assert.equal(missing.$('location').value,'');assert.equal(active(missing).path,'');missing.dom.window.close();

 // Guest restore accepts only currently authorized roots and relative paths.
 const guestKey='hamster.files.tabs.v1.guest';
 const guestWorkspace=JSON.stringify({version:1,active:'bad',tabs:[{id:'bad',root:'removed-share',path:'private'},{id:'absolute',root:'share-id',path:'C:\\private'},{id:'good',root:'share-id',path:'photos',back:[],forward:[],view:'grid'}]});
 const guest=setup(false,{workspace:guestWorkspace});await wait(()=>guest.d.querySelectorAll('#fileTabs [role=tab]').length===1&&guest.$('location').value==='演示共享/photos','guest authorized restore');assert.equal(guest.$('rename').disabled,true);assert(guest.calls.filter(call=>call.route==='list').every(call=>call.url.searchParams.get('root')==='share-id'&&!call.body.Path.includes('private')));assert.equal(JSON.parse(guest.w.sessionStorage.getItem(guestKey)).tabs.length,1);guest.dom.window.close();

 const folders=setup(true);await ready(folders,'');await go(folders,'E:\\home');folders.d.querySelector('.entry-open').dispatchEvent(new folders.w.MouseEvent('auxclick',{bubbles:true,button:1}));await ready(folders,'E:\\demo\\相册');assert.equal(folders.d.querySelectorAll('#fileTabs [role=tab]').length,2);assert.equal(state(folders).tabs[0].path,'E:\\home');folders.dom.window.close();

 const mixed=setup(false,{roots:[{ID:'share-id',Name:'只读目录',Writable:false},{ID:'writable',Name:'可写目录',Writable:true}]});
 await wait(()=>mixed.d.querySelectorAll('#fileTabs [role=tab]').length===1&&mixed.$('notice').textContent!=='正在读取目录…','guest root ready');
 mixed.$('newTab').click();await wait(()=>mixed.d.querySelectorAll('#fileTabs [role=tab]').length===2&&mixed.$('notice').textContent!=='正在读取目录…','guest second tab');
 [...mixed.$('roots').querySelectorAll('button')].find(button=>button.textContent.includes('可写目录')).click();await wait(()=>mixed.$('newFolder').disabled===false,'writable guest share');
 const guestTabs=JSON.parse(mixed.w.sessionStorage.getItem(guestKey));mixed.d.getElementById(guestTabs.tabs[0].id).click();await wait(()=>mixed.$('location').value==='只读目录'&&mixed.$('notice').textContent!=='正在读取目录…','independent guest root');assert.equal(mixed.$('newFolder').disabled,true);assert.equal(mixed.calls.filter(call=>call.route==='list').at(-1).url.searchParams.get('root'),'share-id');mixed.dom.window.close();

 const limit=setup(true);await ready(limit,'');for(let i=1;i<20;i++)await add(limit);assert.equal(limit.$('newTab').disabled,true);const oldActive=state(limit).active,listCount=limit.calls.filter(call=>call.route==='list').length;limit.d.querySelector('.browser-tab:not(.active) .tab-close').click();assert.equal(limit.d.querySelectorAll('#fileTabs [role=tab]').length,19);assert.equal(state(limit).active,oldActive);assert.equal(limit.calls.filter(call=>call.route==='list').length,listCount,'closing a background tab must not reload the active directory');limit.dom.window.close();
 console.log('PASS: tabs restore across refresh, independent histories/search/views, close behavior, folder tab opening, stale response isolation, missing-folder fallback and guest permission filtering');
})().catch(error=>{console.error(error);process.exitCode=1});
