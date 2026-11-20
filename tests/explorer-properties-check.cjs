const assert=require('node:assert/strict');
const {setup,wait}=require('./explorer-check.cjs');
async function ready(t){await wait(()=>t.$('entries').children.length,'start');t.$('location').value='E:\\demo';t.$('go').click();await wait(()=>!t.$('newFolder').disabled,'directory')}
function menu(t,index=0){t.$('entries').children[index].querySelector('.entry-more').click();for(const b of t.$('contextMenu').querySelectorAll('button')){assert(b.querySelector('svg.control-icon'),b.textContent+' lacks an icon');assert.equal(b.getAttribute('role'),'menuitem')}return [...t.$('contextMenu').children]}
(async()=>{
 let release,sizeSignal;
 const t=setup(true,{propertiesHook:async({body})=>({ok:true,json:async()=>({Name:'合成目录',Path:body.Path,Dir:true})}),directorySizeHook:({signal})=>new Promise(resolve=>{sizeSignal=signal;release=()=>resolve({ok:true,json:async()=>({Size:3072,Files:2,Directories:1,Skipped:0,Complete:true})})})});await ready(t);
 menu(t).find(b=>b.textContent==='属性').click();await wait(()=>t.d.querySelector('.property-size-button'),'folder size button');assert(!t.calls.some(c=>c.route==='properties-size'),'opening properties never walks directory');
 t.d.querySelector('.property-size-button').click();await wait(()=>release,'size requested');assert.equal(t.calls.find(c=>c.route==='properties-size').body.Version,'fixture-version');release();await wait(()=>t.$('propertiesList').textContent.includes('3.00 KiB'),'size displayed');assert(t.$('propertiesStatus').textContent.includes('2 个文件，1 个子目录'));
 release=null;t.d.querySelector('.property-size-button').click();await wait(()=>release,'recalculate');t.$('propertiesClose').click();assert(sizeSignal.aborted);release();await new Promise(r=>setTimeout(r,10));assert(!t.$('propertiesDialog').open);
 for(const index of [1,3]){menu(t,index);t.d.dispatchEvent(new t.w.MouseEvent('pointerdown',{bubbles:true}))}
 t.$('entries').dispatchEvent(new t.w.MouseEvent('contextmenu',{bubbles:true,cancelable:true,clientX:10,clientY:20}));for(const b of t.$('contextMenu').children)assert(b.querySelector('svg'),b.textContent);
 t.$('contextMenu').hidden=true;t.$('fileTabs').querySelector('.browser-tab').dispatchEvent(new t.w.MouseEvent('contextmenu',{bubbles:true,cancelable:true}));for(const b of t.$('contextMenu').children)assert(b.querySelector('svg'),b.textContent);
 assert.equal(t.errors.length,0);t.dom.window.close();
 const merge=setup(true,{transferHook:({body})=>!body.Conflict?{ok:false,status:409}:null});await ready(merge);merge.$('entries').children[1].querySelector('.entry-open').click();merge.w.__upgradeActions.copySelectedResources('copy');merge.$('location').value='E:\\target';merge.$('go').click();await wait(()=>!merge.$('newFolder').disabled,'target');
 const transferring=merge.w.__upgradeActions.pasteSelectedResources();await wait(()=>merge.d.querySelector('.transfer-conflict-dialog[open]'),'explicit conflict dialog');assert.equal(merge.d.querySelector('#transferConflictPolicy').value,'rename','safe default');merge.d.querySelector('#transferConflictPolicy').value='overwrite';[...merge.d.querySelectorAll('.transfer-conflict-dialog button')].find(b=>b.textContent==='继续').click();await transferring;assert(merge.calls.some(c=>c.route==='transfer'&&c.body.Conflict==='overwrite'));assert.equal(merge.errors.length,0);merge.dom.window.close();
 console.log('PASS: opt-in directory size, scoped request, live cancellation, late-result isolation, consistent menu icons and explicit merge choice');
})().catch(e=>{console.error(e);process.exitCode=1;setTimeout(()=>process.exit(1),100)});
