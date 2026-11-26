const assert=require('node:assert/strict');
const {setup,wait}=require('./explorer-check.cjs');
const response=value=>({ok:true,status:200,json:async()=>value});
const archive={Name:'<img onerror=bad>.zip',Path:'E:\\demo\\example.zip',Kind:'archive',Version:'version-archive'};
const enter=async t=>{await wait(()=>t.$('entries').children.length,'boot');t.$('location').value='E:\\demo';t.$('go').click();await wait(()=>t.$('entries').textContent.includes(archive.Name)&&!t.$('newFolder').disabled,'archive');t.d.querySelector('.entry-more').click();[...t.$('contextMenu').querySelectorAll('button')].find(b=>b.textContent==='解压资源…').click();await wait(()=>t.$('quickExtractDialog').open&&!t.$('quickExtractStatus').textContent.includes('正在读取'),'settings')};
(async()=>{
 let state={Status:'idle'},conflict=false,cancel=false,started=false;
 const config={Tool:'extract',Root:'E:\\demo\\',ExtractSource:archive.Path,Output:'E:\\demo\\解压资源',ExtractConflict:'rename',ExtractLayout:'separate'};
 const t=setup(true,{listHook:({body})=>body.Path==='E:\\demo'?response({Path:body.Path,Entries:[archive],Total:1}):null,requestHook:({url,route,body})=>{
  if(url.pathname==='/api/defaults')return response({SevenZip:'E:\\tools\\7z.exe'});
  if(url.pathname==='/api/settings')return response({Workspaces:{extract:{SevenZip:'E:\\configured\\7z.exe'}}});
  if(url.pathname==='/api/passwords')return response([{ID:'one',Name:'常用演示密码'}]);
  if(url.pathname==='/api/password-memory')return response({Supported:true,Remember:true,Password:'remembered',SavedID:'one'});
  if(url.pathname==='/api/password-get')return response({Password:'selected'});
  if(url.pathname==='/api/plan'){state={...config,Config:config,Tool:'extract',Status:'ready',TaskID:'task-one',PlanID:'plan-one',Actionable:conflict?0:1,Conflicts:conflict?1:0};return response({TaskID:'task-one'})}
  if(url.pathname==='/api/state')return response(state);
  if(url.pathname==='/api/preview')return response({Items:[{Source:archive.Path,Target:'E:\\demo\\解压资源\\example',Note:'保留原压缩包',Conflict:conflict?'请填写解压密码':''}]});
  if(url.pathname==='/api/execute'){started=true;state={...state,Status:'running',Operation:{Phase:'extract',PercentKnown:true,Percent:45},Logs:['synthetic progress']};return response({OK:true})}
  if(url.pathname==='/api/cancel'){cancel=true;state={...state,Status:'cancelled',Result:{Success:0,Failed:0,Skipped:0}};return response({OK:true})}
 }});
 let opened=0;t.w.open=()=>opened++;await enter(t);assert(t.$('quickExtractDialog').open);assert.equal(opened,0);assert.equal(t.$('quickExtractName').querySelectorAll('img').length,0);assert.equal(t.$('quickExtractProgram').value,'E:\\configured\\7z.exe');assert.equal(t.$('quickExtractPassword').value,'remembered');
 t.$('quickExtractEye').click();assert.equal(t.$('quickExtractPassword').value,'remembered');assert.equal(t.$('quickExtractEye').getAttribute('aria-pressed'),'true');
 conflict=true;t.$('quickExtractScan').click();await wait(()=>t.$('quickExtractPreview').textContent.includes('请填写解压密码'),'password conflict');assert(t.$('quickExtractRun').disabled);assert(!started);
 conflict=false;t.$('quickExtractPassword').value='manual';t.$('quickExtractPassword').dispatchEvent(new t.w.Event('input'));t.$('quickExtractScan').click();await wait(()=>!t.$('quickExtractRun').disabled,'ready plan');
 const plan=t.calls.filter(c=>c.url.pathname==='/api/plan').at(-1);assert(t.calls.some(c=>c.route==='password-memory-save'&&c.body.Remember&&c.body.Password==='manual'),'manual password uses existing encrypted memory');assert.equal(plan.body.ExtractSource,archive.Path);assert.equal(plan.body.ExtractDeleteSource,false);assert.equal(plan.body.ExtractConflict,'rename');assert.equal(plan.body.ExtractPassword,'manual');assert.equal(plan.headers['X-Toolbox-Tool'],'extract');assert.equal(plan.headers['X-Toolbox-Token'],'test');assert(t.calls.some(c=>c.route==='properties'&&c.body.Version==='version-archive'));
 t.$('quickExtractOutput').value='output';t.$('quickExtractOutput').dispatchEvent(new t.w.Event('input'));assert(t.$('quickExtractRun').disabled,'changed settings invalidate old preview');t.$('quickExtractScan').click();await wait(()=>!t.$('quickExtractRun').disabled,'rescanned');
 t.$('quickExtractRun').click();await wait(()=>started&&t.$('quickExtractProgress').value===45,'running progress');assert.equal(t.calls.find(c=>c.route==='execute').body.PlanID,'plan-one');t.$('quickExtractClose').click();assert.equal(t.$('quickExtractPassword').value,'');assert(!cancel,'closing must not cancel a running task');
 t.d.querySelector('.entry-more').click();[...t.$('contextMenu').querySelectorAll('button')].find(b=>b.textContent==='解压资源…').click();assert(t.$('quickExtractDialog').open);t.$('quickExtractCancel').click();await wait(()=>cancel&&t.$('quickExtractStatus').textContent.includes('已停止'),'cancel');assert.equal(t.calls.find(c=>c.route==='cancel').body.TaskID,'task-one');
 state={...state,Status:'running',Result:undefined};t.$('quickExtractClose').click();await enter(t);await wait(()=>t.$('quickExtractStatus').textContent.includes('45%'),'reopen attaches running task');state={...state,Status:'finished',Result:{Success:1,Skipped:0,Failed:0},Logs:['done']};await wait(()=>!t.$('quickExtractOpenOutput').hidden,'completion and output link',2000);assert(t.calls.filter(c=>c.route==='list').length>=3,'completion refreshes current folder');assert.equal(t.errors.length,0);t.dom.window.close();
 const remote=setup(true,{remoteAdmin:true,listHook:({body})=>body.Path==='E:\\demo'?response({Path:body.Path,Entries:[archive],Total:1}):null});await enter(remote);assert(remote.$('quickExtractDialog').open,'administrator can use inline extraction');remote.dom.window.close();
 let releaseProperty;
 const snapshot=setup(true,{listHook:({body})=>body.Path==='E:\\demo'?response({Path:body.Path,Entries:[archive],Total:1}):null,requestHook:({url,body})=>{
  if(url.pathname==='/files/api/properties')return new Promise(resolve=>releaseProperty=()=>resolve(response({})));
  if(url.pathname==='/api/plan')return {ok:false,status:400,text:async()=> 'synthetic stop after captured request'};
 }});await enter(snapshot);snapshot.$('quickExtractPassword').value='captured-before-close';snapshot.$('quickExtractScan').click();snapshot.$('quickExtractClose').click();releaseProperty();await wait(()=>snapshot.calls.some(c=>c.url.pathname==='/api/plan'),'captured scan request');assert.equal(snapshot.calls.find(c=>c.url.pathname==='/api/plan').body.ExtractPassword,'captured-before-close');assert.equal(snapshot.$('quickExtractPassword').value,'');snapshot.dom.window.close();
 let releaseMemory;
 const late=setup(true,{listHook:({body})=>body.Path==='E:\\demo'?response({Path:body.Path,Entries:[archive],Total:1}):null,requestHook:({url})=>url.pathname==='/api/password-memory'?new Promise(resolve=>releaseMemory=()=>resolve(response({Supported:true,Remember:true,Password:'must-not-reappear'}))):null});
 await wait(()=>late.$('entries').children.length,'late boot');late.$('location').value='E:\\demo';late.$('go').click();await wait(()=>late.$('entries').textContent.includes(archive.Name)&&!late.$('newFolder').disabled,'late archive');late.d.querySelector('.entry-more').click();[...late.$('contextMenu').querySelectorAll('button')].find(b=>b.textContent==='解压资源…').click();late.$('quickExtractClose').click();releaseMemory();await wait(()=>!late.$('quickExtractScan').disabled,'closed settings complete');assert.equal(late.$('quickExtractPassword').value,'','late password response cannot refill a closed dialog');late.dom.window.close();
 console.log('PASS: inline extract/passwords, safe defaults, no navigation, preview invalidation, authenticated tool slot, progress, non-cancelling close, exact-task cancel, reattachment and completion refresh');
})().catch(e=>{console.error(e);process.exitCode=1;setTimeout(()=>process.exit(1),100)});
