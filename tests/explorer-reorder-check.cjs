const assert=require('node:assert/strict');
const {setup,wait}=require('./explorer-check.cjs');
const workspace=JSON.stringify({version:1,active:'tab-b',tabs:['a','b','c'].map(x=>({id:'tab-'+x,root:'',path:'E:\\'+x,back:[{root:'',path:'E:\\history-'+x,scroll:120}],forward:[],view:'grid',search:'',offset:0,scroll:0}))});
const ready=t=>wait(()=>t.$('notice').textContent!=='正在读取目录…'&&t.$('entries').children.length&&t.d.querySelectorAll('#fileTabs [role=tab]').length,'ready');
const state=t=>JSON.parse(t.w.sessionStorage.getItem('hamster.files.tabs.v1.local'));
const ids=t=>[...t.d.querySelectorAll('#fileTabs [role=tab]')].map(n=>n.id);
const action=(t,label)=>[...t.$('contextMenu').querySelectorAll('button')].find(b=>b.textContent===label);
function context(t,id){t.d.getElementById(id).dispatchEvent(new t.w.MouseEvent('contextmenu',{bubbles:true,cancelable:true,clientX:100,clientY:100}))}
function drag(t,from,to,after=true,axis='x'){
 to.getBoundingClientRect=()=>({left:100,right:200,top:100,bottom:140,width:100,height:40});
 t.d.elementFromPoint=()=>to;
 from.dispatchEvent(new t.w.MouseEvent('pointerdown',{bubbles:true,cancelable:true,button:0,clientX:20,clientY:20}));
 for(const type of ['pointermove','pointerup'])t.d.dispatchEvent(new t.w.MouseEvent(type,{bubbles:true,cancelable:true,button:0,clientX:axis==='x'?(after?190:110):150,clientY:axis==='y'?(after?135:105):120}));
}
(async()=>{
 const store={value:{version:1,view:'grid',tileSize:180,favorites:['E:\\one','F:\\two','G:\\three']}},t=setup(true,{workspace,preferencesStore:store});await ready(t);
 const lists=t.calls.filter(c=>c.route==='list').length;
 context(t,'tab-a');assert(action(t,'关闭其他标签页'));assert.equal(state(t).active,'tab-b','right-clicking background tab does not activate it');action(t,'向右移动').click();assert.deepEqual(ids(t),['tab-b','tab-a','tab-c']);assert.equal(t.calls.filter(c=>c.route==='list').length,lists);
 drag(t,t.d.getElementById('tab-c').parentElement,t.d.getElementById('tab-b').parentElement,false);assert.deepEqual(ids(t),['tab-c','tab-b','tab-a']);assert.equal(state(t).active,'tab-b');assert.equal(state(t).tabs.find(tab=>tab.id==='tab-a').back[0].path,'E:\\history-a');
 // External drops and cross-list drags must never change navigation order.
 const before=ids(t);t.d.getElementById('tab-a').parentElement.dispatchEvent(new t.w.MouseEvent('drop',{bubbles:true,cancelable:true}));assert.deepEqual(ids(t),before);
 const favorites=[...t.$('favorites').children];drag(t,favorites[0],favorites[2],true,'y');await wait(()=>store.value.favorites[2]==='E:\\one','persist quick access');assert.equal(t.$('location').value,'E:\\b');assert.equal(t.calls.filter(c=>c.route==='mutate').length,0);
 const saved=state(t);t.d.getElementById('tab-b').dispatchEvent(new t.w.KeyboardEvent('keydown',{key:'F10',shiftKey:true,bubbles:true,cancelable:true}));assert(!t.$('contextMenu').hidden);action(t,'复制标签页').click();await wait(()=>ids(t).length===4&&state(t).active!=='tab-b'&&t.$('notice').textContent!=='正在读取目录…','duplicate tab');assert.equal(t.$('location').value,'E:\\b');assert.equal(state(t).tabs.find(tab=>tab.id===state(t).active).back[0].path,'E:\\history-b');
 context(t,'tab-b');action(t,'关闭右侧标签页').click();await wait(()=>ids(t).length===2&&state(t).active==='tab-b'&&t.$('notice').textContent!=='正在读取目录…','close right switches only when needed');assert.deepEqual(ids(t),['tab-c','tab-b']);context(t,'tab-c');action(t,'关闭其他标签页').click();await wait(()=>ids(t).length===1&&state(t).active==='tab-c'&&t.$('notice').textContent!=='正在读取目录…','close others');context(t,'tab-c');assert(action(t,'关闭标签页').disabled);assert(action(t,'向左移动').disabled);assert.equal(t.errors.length,0,t.errors.join('\n'));t.dom.window.close();
 const restored=setup(true,{workspace:JSON.stringify(saved),preferencesStore:store});await ready(restored);assert.deepEqual(ids(restored),['tab-c','tab-b','tab-a']);assert.equal(restored.$('favorites').firstElementChild.dataset.path,'F:\\two');restored.dom.window.close();
 const failure=setup(true,{preferencesStore:store,preferenceHook:()=>({ok:false,status:500,text:async()=>'synthetic save failure'})});await ready(failure);const original=[...failure.$('favorites').children].map(row=>row.dataset.path);drag(failure,failure.$('favorites').firstElementChild,failure.$('favorites').lastElementChild,true,'y');await wait(()=>failure.$('notice').textContent.includes('保存失败'),'visible save failure');await wait(()=>failure.$('favorites').firstElementChild.dataset.path===original[0],'rollback');assert.deepEqual([...failure.$('favorites').children].map(row=>row.dataset.path),original);failure.dom.window.close();
 const guest=setup(false);await ready(guest);assert(guest.$('quickAccess').hidden);guest.$('newTab').click();await wait(()=>guest.d.querySelectorAll('#fileTabs [role=tab]').length===2&&guest.$('notice').textContent!=='正在读取目录…','guest tabs');const guestIds=[...guest.d.querySelectorAll('#fileTabs [role=tab]')].map(n=>n.id);drag(guest,guest.d.getElementById(guestIds[1]).parentElement,guest.d.getElementById(guestIds[0]).parentElement,false);assert.deepEqual(ids(guest),guestIds.reverse());assert(!guest.calls.some(call=>call.route==='preferences'||call.route==='mutate'));assert.equal(guest.errors.length,0);guest.dom.window.close();
 console.log('PASS: tab menus, drag order/history restoration, background isolation, duplicate/close groups, external-drop rejection, quick-access persistence/rollback and guest isolation');
})().catch(e=>{console.error(e);process.exitCode=1});
