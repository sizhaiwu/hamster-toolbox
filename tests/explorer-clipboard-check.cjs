const assert=require('node:assert/strict');
const {setup,wait}=require('./explorer-check.cjs');
const go=async(t,path)=>{t.$('location').value=path;t.$('go').click();await wait(()=>!t.$('newFolder').disabled&&t.$('location').value===path,'directory ready')};
function select(t,index){t.$('entries').children[index].querySelector('.entry-open').click()}
function menu(t,index,label){t.$('entries').children[index].querySelector('.entry-more').click();const item=[...t.$('contextMenu').children].find(b=>b.textContent===label);assert(item,label+' is available');return item}
function key(t,key){const event=new t.w.KeyboardEvent('keydown',{key,ctrlKey:true,bubbles:true,cancelable:true});t.d.dispatchEvent(event);return event}
(async()=>{
 const t=setup(true);await wait(()=>t.$('entries').children.length,'initial');assert(t.$('paste').disabled);await go(t,'E:\\source');assert(!t.$('paste').hidden);assert(t.$('copy').hidden);
 // Click the icon itself, exercising the real nested target in an icon menu.
 menu(t,1,'复制').querySelector('svg path').dispatchEvent(new t.w.MouseEvent('click',{bubbles:true}));
 assert.equal(t.$('clipboardStatus').textContent,'已复制 1 项');assert(!t.$('clipboardStatus').hidden);assert(!t.$('paste').disabled);assert.match(t.$('notice').textContent,/顶部.*粘贴/);
 await go(t,'E:\\target');assert.equal(t.$('clipboardStatus').textContent,'已复制 1 项','clipboard feedback survives navigation');
 t.$('paste').click();await wait(()=>t.calls.some(c=>c.route==='transfer')&&t.$('notice').textContent.includes('传输完成'),'toolbar paste without blank space');
 assert.equal(t.calls.find(c=>c.route==='transfer').body.Target,'E:\\target\\演示.jpg');assert.equal(t.calls.find(c=>c.route==='transfer').body.Action,'copy');assert(!t.$('paste').disabled,'copy can be pasted again');
 const folderPaste=menu(t,0,'粘贴到此目录');assert(!folderPaste.disabled);folderPaste.click();await wait(()=>t.calls.filter(c=>c.route==='transfer').length===2&&t.$('notice').textContent.includes('传输完成'),'paste directly into a folder');
 assert.equal(t.calls.filter(c=>c.route==='transfer')[1].body.Target,'E:\\demo\\相册\\演示.jpg');
 select(t,2);assert(!t.$('copy').hidden&&!t.$('cut').hidden);t.$('cut').click();assert.equal(t.$('clipboardStatus').textContent,'已剪切 1 项');await go(t,'E:\\cut-target');t.$('paste').click();
 await wait(()=>t.$('notice').textContent.includes('传输完成')&&t.$('paste').disabled,'cut paste clears clipboard');assert.equal(t.calls.filter(c=>c.route==='transfer').at(-1).body.Action,'move');assert(t.$('clipboardStatus').hidden);
 select(t,1);t.$('copy').focus();assert(key(t,'c').defaultPrevented);assert.equal(t.$('clipboardStatus').textContent,'已复制 1 项');
 t.$('location').focus();assert(!key(t,'x').defaultPrevented,'text editing retains normal clipboard shortcuts');assert.equal(t.$('clipboardStatus').textContent,'已复制 1 项');
 t.$('paste').focus();assert(key(t,'v').defaultPrevented);await wait(()=>!t.$('paste').disabled&&t.$('notice').textContent.includes('传输完成'),'keyboard paste');assert.equal(t.errors.length,0);t.dom.window.close();
 const invalid=setup(true,{listHook:({body})=>body.Path?{ok:true,json:async()=>({Path:body.Path,Parent:'',Entries:[{Name:'unavailable.txt',Path:'E:\\demo\\unavailable.txt',Kind:'text'}],Total:1,Offset:0})}:null});await wait(()=>invalid.$('entries').children.length,'invalid ready');await go(invalid,'E:\\demo');menu(invalid,0,'复制').click();assert.match(invalid.$('notice').textContent,/刷新目录/);assert(invalid.$('paste').disabled);invalid.dom.window.close();
 const guest=setup(false);await wait(()=>guest.$('entries').children.length,'guest ready');select(guest,1);assert(guest.$('copy').disabled&&guest.$('cut').disabled&&guest.$('paste').disabled);assert.equal(guest.errors.length,0);guest.dom.window.close();
 console.log('PASS: icon-menu copy feedback, toolbar paste with no blank area, folder-target paste, cut clearing, keyboard/input isolation, invalid selection feedback and read-only permissions');
})().catch(e=>{console.error(e);process.exitCode=1;setTimeout(()=>process.exit(1),100)});
