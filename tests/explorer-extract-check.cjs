const assert=require('node:assert/strict');
const {setup,wait}=require('./explorer-check.cjs');
const response=x=>({ok:true,status:200,json:async()=>x});
(async()=>{
 const archive={Name:'示例.tar.gz',Path:'E:\\demo\\示例.tar.gz',Kind:'archive',Version:'archive-version'};
 const t=setup(true,{listHook:({body})=>body.Path==='E:\\demo'?response({Path:body.Path,Parent:'E:\\',Entries:[archive],Total:1}):null});
 await wait(()=>t.$('entries').children.length,'initial');t.$('location').value='E:\\demo';t.$('go').click();await wait(()=>t.$('entries').textContent.includes(archive.Name),'archive');
 let opened;t.w.open=(...args)=>{opened=args;return null};t.d.querySelector('.entry-more').click();
 const button=[...t.$('contextMenu').querySelectorAll('button')].find(b=>b.textContent==='解压资源…');assert(button&&!button.disabled);assert(button.querySelector('svg'));button.click();
 assert.equal(opened,undefined,'right-click stays in Explorer');await wait(()=>!t.$('quickExtractScan').disabled,'inline settings');assert(t.$('quickExtractDialog').open);assert.equal(t.$('quickExtractName').textContent,archive.Name);t.$('quickExtractAdvanced').click();const url=new URL(opened[0]);assert.equal(url.searchParams.get('tool'),'extract');assert.equal(url.searchParams.get('root'),'E:\\demo\\');assert.equal(url.searchParams.get('extract-source'),archive.Path);assert.equal(url.searchParams.get('session'),'test');assert.equal(opened[1],'_blank');assert.equal(opened[2],'noopener');assert(!t.calls.some(c=>c.route==='mutate'));t.dom.window.close();
 const guest=setup(false,{listHook:()=>response({Path:'',Parent:'',Entries:[{...archive,Path:'示例.tar.gz'}],Total:1})});await wait(()=>guest.$('entries').children.length,'guest');guest.d.querySelector('.entry-more').click();assert(!guest.$('contextMenu').textContent.includes('解压资源'));guest.dom.window.close();
 const {createUI,until,requests}=require('./ui-check.cjs');const source='E:\\demo\\示例.tar.gz';
 const before=requests.length,dom=createUI(30001,true,1024,w=>{w.history.replaceState({},'', '/?'+new URLSearchParams({session:'test',tool:'extract',root:'E:\\demo\\','extract-source':source}))});const $=id=>dom.window.document.getElementById(id);
 await until(()=>requests.slice(before).some(r=>r.path==='plan'),'linked archive auto preview');
 const request=requests.slice(before).find(r=>r.path==='plan');assert(!new URL(dom.window.location.href).searchParams.has('extract-source'));assert.equal(request.body.ExtractSource,source);assert.equal(request.body.Output,'');assert.equal($('extractSourceScope').hidden,false);assert.equal($('extractSourceName').textContent,source);assert(!requests.slice(before).some(r=>r.path==='execute'));
 await until(()=>!$('clearExtractSource').disabled,'scope button ready');$('clearExtractSource').click();assert.equal($('ExtractSource').value,'');assert.equal($('extractSourceScope').hidden,true);dom.window.close();
 console.log('PASS: archive context icon, selected source/root/session, inline dialog, optional advanced tab, guest isolation, automatic preview only and clear single-archive scope');
})().catch(e=>{console.error(e);process.exit(1)});
