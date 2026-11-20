const assert=require('node:assert/strict');
const {setup,wait}=require('./explorer-check.cjs');
const ok=()=>({ok:true,status:200,json:async()=>({OK:true})});
const error=(status,text)=>({ok:false,status,text:async()=>text});
(async()=>{
 let authenticated=false,blockedCookie=false,bootFailure=false,pending;
 const test=setup(false,{globalFeedback:true,requestHook:async({route,body})=>{
  if(route==='boot'){if(!authenticated)return error(401,'请先输入访问密码');if(bootFailure)return error(500,'暂时无法读取共享目录')}
  if(route==='list'&&!authenticated)return error(401,'请先输入访问密码');
  if(route==='login'){
   if(body.Password!=='Public-Test-Only!')return error(401,'访问密码不正确');
   if(pending)return new Promise(resolve=>{pending.finish=()=>{authenticated=true;resolve(ok())}});
   if(!blockedCookie)authenticated=true;return ok();
  }
 }});
 const {$,w,calls}=test;
 await wait(()=>$('loginPanel').hidden===false&&calls.some(c=>c.route==='boot'),'first anonymous visit');assert($('app').hidden);
 $('accessCode').value='wrong';$('login').click();await wait(()=>$('loginError').textContent==='访问密码不正确'&&!$('login').disabled,'wrong password shown');assert($('app').hidden);
 pending={};$('accessCode').value='Public-Test-Only!';$('accessCode').dispatchEvent(new w.KeyboardEvent('keydown',{key:'Enter',bubbles:true}));await wait(()=>!!pending.finish,'login in flight');assert($('login').disabled);$('login').click();assert.equal(calls.filter(c=>c.route==='login').length,2,'pending login cannot be submitted twice');pending.finish();pending=null;
 await wait(()=>$('loginPanel').hidden&&!$('app').hidden&&$('entries').children.length===4,'password directly opens listing without reload');assert.equal($('accessCode').value,'');assert($('openGeneralSettings').hidden);assert($('openVersionInfo').hidden);assert(!calls.some(c=>c.route==='settings'||c.route==='version-update'));
 authenticated=false;await w.eval("navigate('')");await wait(()=>!$('loginPanel').hidden,'expired session prompts login');$('accessCode').value='Public-Test-Only!';$('login').click();await wait(()=>$('loginPanel').hidden&&!$('app').hidden&&!$('login').disabled,'reauthentication without reload');
 authenticated=false;await w.eval("navigate('')");blockedCookie=true;$('accessCode').value='Public-Test-Only!';$('login').click();await wait(()=>$('loginError').textContent.includes('Cookie')&&!$('login').disabled,'blocked cookie explained');assert($('app').hidden);
 blockedCookie=false;bootFailure=true;$('accessCode').value='Public-Test-Only!';$('login').click();await wait(()=>$('loginError').textContent.includes('共享页面加载失败')&&!$('login').disabled,'post-login initialization failure explained');assert.match($('loginError').textContent,/暂时无法读取/);
 bootFailure=false;$('accessCode').value='Public-Test-Only!';$('login').click();await wait(()=>$('loginPanel').hidden&&!$('app').hidden&&!$('login').disabled,'retry recovers without refreshing');assert.equal(test.errors.length,0,test.errors.join('\n'));test.dom.window.close();
 console.log('PASS: first-visit password login enters listing without reload, Enter/pending guard, bad password, expiry/relogin, blocked-cookie and initialization errors, retry and owner-setting isolation');
})().catch(error=>{console.error(error);process.exit(1)});
