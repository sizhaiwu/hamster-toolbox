const assert=require('node:assert/strict'),fs=require('node:fs'),{JSDOM}=require('jsdom');
const {wait}=require('./explorer-check.cjs');
(async()=>{
 const token='synthetic-remote-admin',dom=new JSDOM(fs.readFileSync('web/schedules.html','utf8'),{url:'https://workspace.test/schedules.html?session='+token,runScripts:'outside-only',pretendToBeVisual:true}),w=dom.window,d=w.document,calls=[],errors=[];
 let expired=false,job={ID:'fixture-job',Name:'remote fixture',Kind:'script',Shell:'sh',Script:'echo fixture',Period:'day',AtHour:9,Timeout:60,Enabled:false,Next:new Date().toISOString()};
 try{
  w.setInterval=()=>0;w.HTMLDialogElement.prototype.showModal=function(){this.open=true};w.HTMLDialogElement.prototype.close=function(){this.open=false;this.dispatchEvent(new w.Event('close'))};w.addEventListener('error',event=>errors.push(event.error||event.message));
  w.fetch=async(url,options)=>{
   assert.equal(options.headers['X-Toolbox-Token'],token,'every remote API must use the remote capability');const q=JSON.parse(options.body);calls.push({url,q,headers:options.headers});
   if(expired&&url==='/api/schedules')return{ok:false,status:401,headers:{get:()=> 'expired-admin'},text:async()=> '管理员会话已过期'};
   let value={};if(url==='/api/settings')value={TaskSound:false};if(q.Action==='list')value={Jobs:[job],Runs:[],Platform:'linux',Active:0};if(q.Action==='detail')value=job;if(q.Action==='save')job={...job,...q.Job};
   return{ok:true,status:200,headers:{get:()=>null},json:async()=>value};
  };
  for(const name of ['schedules.js','schedule-controls.js','schedule-library.js','schedule-status.js','remote-workspace.js'])w.eval(fs.readFileSync('web/'+name,'utf8'));
  await wait(()=>d.querySelector('#jobs button'),'remote tasks');
  assert(d.querySelector('header .head-actions #remoteSessionMenu'),'remote session belongs in the header');assert.equal(d.querySelector('#back').getAttribute('href'),'/?session='+token);
  assert(calls.filter(call=>call.q.Action==='list').every(call=>call.headers['X-Hamster-Poll']==='1'),'background polls must not extend the administrator session');
  [...d.querySelectorAll('#jobs button')].find(b=>b.textContent==='编辑').click();await wait(()=>d.querySelector('#editor').open,'remote editor');assert.equal(d.querySelector('#script').value,'echo fixture');
  d.querySelector('#name').value='remote edited';d.querySelector('#form').dispatchEvent(new w.Event('submit',{cancelable:true}));await wait(()=>calls.some(call=>call.q.Action==='save'),'remote save');assert.equal(calls.find(call=>call.q.Action==='save').headers['X-Hamster-Poll'],undefined);
  await wait(()=>!d.querySelector('#editor').open,'editor saved');expired=true;d.querySelector('#refresh').click();await wait(()=>d.querySelector('#remoteSessionExpired')?.open,'expired administrator dialog');
  d.querySelector('#refresh').click();await new Promise(resolve=>setTimeout(resolve,0));assert.equal(d.querySelectorAll('#remoteSessionExpired').length,1);assert.equal(d.querySelector('.remote-login-again').getAttribute('href'),'/admin/?reason=expired');assert.equal(errors.length,0,String(errors));
  console.log('PASS remote schedules UI: administrator token, header session menu, editing/saving, passive polling and deduplicated session-expiry prompt');
 }finally{w.close()}
})().catch(error=>{console.error(error);process.exitCode=1});
