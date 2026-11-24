const assert=require('node:assert/strict');
const {createUI,until}=require('./ui-check.cjs');
const jobs={image:{Tool:'image',Status:'running',PlanID:'image-one',TaskID:'image-task',Config:{Tool:'image',Root:'E:/demo/images',Output:'E:/demo/compressed'},Total:10,Done:2,Logs:['image log']},video:{Tool:'video',Status:'running',PlanID:'video-one',TaskID:'video-task',Total:1,Done:0,Logs:['video log'],Operation:{PercentKnown:true,Percent:55}}};
let completed=[],previewGate;const shared=new Map(),calls=[];const windows=[];
const storage={getItem:key=>shared.get(key)??null,setItem:(key,value)=>shared.set(key,String(value)),removeItem:key=>shared.delete(key)};
function page(options={}){return createUI(17600,true,1280,w=>{
  if(options.navigationType)Object.defineProperty(w.performance,'getEntriesByType',{value:()=>[{type:options.navigationType}]});
  for(const [key,value] of Object.entries(options.session||{}))w.sessionStorage.setItem(key,value);
  Object.defineProperty(w,'localStorage',{value:storage});const original=w.fetch;
  w.fetch=async(url,options)=>{
    const tool=options.headers['X-Toolbox-Tool']||'format',body=JSON.parse(options.body);calls.push({url,tool,body});
    if(url==='/api/state'){const state=jobs[tool]||{Tool:tool,Status:'idle',Logs:[]};return {ok:true,json:async()=>JSON.parse(JSON.stringify({...state,Tasks:Object.values(jobs),Completed:completed}))}}
    if(url==='/api/preview'){
      if(previewGate?.tool===tool){previewGate.requested=true;await previewGate.promise}
      return {ok:true,json:async()=>({PlanID:body.PlanID,Items:[],Total:0,Offset:0,Limit:100})};
    }
    if(url==='/api/plan'){
      const TaskID='local-scan-'+calls.length;
      jobs[tool]={Tool:tool,Status:'ready',TaskID,PlanID:'plan-'+TaskID,Config:{...body},Total:1,Actionable:1,Logs:[]};
      return {ok:true,json:async()=>({ok:true,TaskID})};
    }
    if(url==='/api/cancel')return {ok:true,json:async()=>({ok:true})};
    return original(url,options);
  };
})}
(async()=>{
  const a=page();windows.push(a.window);let w=a.window,$=id=>w.document.getElementById(id);
  await until(()=>!$('preview').disabled&&w.document.querySelector('[data-tool=image] .tool-task-badge').textContent==='2/10','idle tool remains usable and background state is visible');
  assert.equal(w.document.querySelector('[data-tool=image] .tool-task-badge').textContent,'2/10');
  assert.equal(w.document.querySelector('[data-tool=video] .tool-task-badge').textContent,'运行 55%');
  w.document.querySelector('[data-tool=image]').click();await until(()=>$('title').textContent==='压缩图片'&&$('Root').disabled&&!w.document.querySelector('[data-tool=video]').disabled,'switch into running image');
  assert.equal($('Root').value,'E:/demo/images');assert.equal($('Output').value,'E:/demo/compressed','first visit must retain the running task output');
  w.document.querySelector('[data-tool=video]').click();await until(()=>$('title').textContent==='压缩视频'&&!$('cancel').hidden,'switch between running tools');
  assert.match($('logs').textContent,/video log/);assert.doesNotMatch($('logs').textContent,/image log/);
  $('cancel').click();await until(()=>calls.some(c=>c.url==='/api/cancel'),'cancel request');const cancel=calls.find(c=>c.url==='/api/cancel');assert.equal(cancel.tool,'video');assert.equal(cancel.body.TaskID,'video-task');
  w.document.querySelector('[data-tool=format]').click();await until(()=>$('title').textContent==='目录统计命名'&&!$('Root').disabled,'configure another tool while both jobs run');
  const b=page();windows.push(b.window);await until(()=>!b.window.document.getElementById('preview').disabled&&b.window.document.querySelector('[data-tool=image] .tool-task-badge').textContent==='2/10','second address connected');
  const ended=(Tool,PlanID)=>({Tool,PlanID,Status:'finished',Result:{Success:1,Failed:0,Skipped:0}});
  completed=[ended('image','image-one'),ended('video','video-one'),ended('rename','fast-old-run')];
  jobs.image={...jobs.image,PlanID:'image-two',TaskID:'next-image',Done:0};jobs.video=completed[1];
  await w.poll();assert($('taskResultDialog').open);
  // A previous page or old application version may write shared acknowledgements.
  // They must not suppress another tab's independent popup.
  shared.set('hamster.toolbox.task-notices.v1',JSON.stringify(completed.map(c=>({id:c.PlanID,acknowledged:true,sounded:true}))));
  for(const c of completed)shared.set('hamster.toolbox.task-notices.v1.claim.'+c.PlanID,JSON.stringify({owner:'another-tab',expires:Date.now()+60000}));
  await b.window.poll();assert(b.window.document.getElementById('taskResultDialog').open,'other address must also show completion');
  const copySession=win=>Object.fromEntries(Array.from({length:win.sessionStorage.length},(_,i)=>{const key=win.sessionStorage.key(i);return [key,win.sessionStorage.getItem(key)]}));
  const unreadSession=copySession(w);
  for(const win of windows){let count=0;while(win.document.getElementById('taskResultDialog').open&&count<5){count++;win.document.getElementById('taskResultClose').click()}assert.equal(count,3,'each task, including a replaced fast job, is queued');await win.poll();assert(!win.document.getElementById('taskResultDialog').open,'acknowledgement is per-page and does not repeat');assert(win.document.querySelector('[data-tool=video] .tool-task-badge').hidden,'closing completion hides only the ended badge');assert(!win.document.querySelector('[data-tool=image] .tool-task-badge').hidden,'old completion must not hide the newer running task');if(win===w)assert(!b.window.document.querySelector('[data-tool=video] .tool-task-badge').hidden,'one page does not consume another page unread state')}
  const fresh=page({navigationType:'navigate',session:unreadSession});await until(()=>fresh.window.document.querySelector('[data-tool=image] .tool-task-badge').textContent==='0/10','new window connected');assert(!fresh.window.document.getElementById('taskResultDialog').open,'new navigation must not replay old tasks, including cloned unread storage');assert(fresh.window.document.querySelector('[data-tool=video] .tool-task-badge').hidden);
  const freshSession=copySession(fresh.window);fresh.window.close();const refreshed=page({navigationType:'reload',session:freshSession});await until(()=>refreshed.window.document.querySelector('[data-tool=image] .tool-task-badge').textContent==='0/10','refresh after new navigation');assert(!refreshed.window.document.getElementById('taskResultDialog').open,'ignored old results must remain ignored after refresh');refreshed.window.close();
  for(let i=0;i<300;i++)w.toolboxFeedback.markTaskRead({TaskID:'synthetic-read-'+i,PlanID:'synthetic-read-'+i});await w.poll();assert(w.document.querySelector('[data-tool=video] .tool-task-badge').hidden,'bounded read storage must retain the current slot acknowledgement');
  w.document.querySelector('[data-tool=rename]').click();await until(()=>$('title').textContent==='批量重命名'&&!$('preview').disabled,'open idle rename');
  const localConfig={...w.config()};
  jobs.rename={Tool:'rename',Status:'ready',TaskID:'rename-preview-one',PlanID:'rename-plan-one',Config:localConfig,Total:1,Actionable:1,Logs:[]};
  await w.poll();assert(!$('execute').disabled,'matching preview can recover on another page');
  jobs.rename={...jobs.rename,TaskID:'rename-preview-two',PlanID:'rename-plan-two',Config:{...localConfig,Root:'E:/other/page-root'}};
  await w.poll();assert($('execute').disabled,'cannot run a remote plan under old paths');assert.match($('message').textContent,/参数.*预览不同/);
  jobs.rename={...jobs.rename,Config:localConfig};await w.poll();assert($('execute').disabled,'a replaced plan cannot be silently adopted even with matching public settings');
  jobs.rename={...jobs.rename,Status:'running',TaskID:'remote-running',Config:{...localConfig,Root:'E:/actual/running'},Logs:['remote running log']};
  await w.poll();assert.equal($('Root').value,'E:/actual/running');assert($('Root').disabled,'remote task is displayed read-only');assert.match($('logs').textContent,/remote running log/);
  let releasePreview;previewGate={tool:'rename',promise:new Promise(resolve=>{releasePreview=resolve})};
  jobs.rename={...jobs.rename,Status:'ready',PlanID:'slow-preview',Logs:['stale rename log']};
  const delayed=w.poll();await until(()=>previewGate.requested,'slow old preview requested');
  w.document.querySelector('[data-tool=format]').click();await until(()=>$('title').textContent==='目录统计命名'&&!$('Root').disabled,'navigate while preview is in flight');
  previewGate=null;releasePreview();await delayed;await until(()=>!$('preview').disabled,'new tool is usable after delayed response');assert.doesNotMatch($('logs').textContent,/stale rename log/,'old preview response must not replace new tool logs');
  jobs.rename={...jobs.rename,Status:'finished',Result:{Failed:1,Success:0}};await w.poll();await until(()=>w.document.querySelector('[data-tool=rename] .tool-task-badge').textContent==='失败','failed task is visible in navigation');
  while($('taskResultDialog').open)$('taskResultClose').click();
  w.toolboxSettings.TaskPopup=false;
  jobs.rename={...jobs.rename,TaskID:'view-ended',PlanID:'view-ended',Logs:['retained ended task log']};await w.poll();assert(!w.document.querySelector('[data-tool=rename] .tool-task-badge').hidden,'popup-disabled results remain unread until viewed');
  w.document.querySelector('[data-tool=rename]').click();await until(()=>w.document.querySelector('[data-tool=rename] .tool-task-badge').hidden,'viewing ended tool clears its badge');assert.match($('logs').textContent,/retained ended task log/,'viewing does not discard logs');
  jobs.rename={...jobs.rename,Status:'error',TaskID:'scan-error',PlanID:'',Error:'synthetic scan error'};await w.poll();assert(!w.document.querySelector('[data-tool=rename] .tool-task-badge').hidden);w.document.querySelector('[data-tool=rename]').click();await until(()=>w.document.querySelector('[data-tool=rename] .tool-task-badge').hidden,'scan failure without a plan can be marked read');
  w.document.querySelector('[data-tool=archive]').click();await until(()=>$('title').textContent==='打包资源'&&!$('preview').disabled,'open archive');
  $('Root').value='E:/demo/archive';
  jobs.archive={Tool:'archive',Status:'ready',TaskID:'foreign-secret-plan',PlanID:'foreign-secret-plan',Config:{...w.config()},Total:1,Actionable:1,Logs:[]};
  await w.poll();assert($('execute').disabled,'redacted passwords cannot authorize a foreign archive plan');assert.match($('message').textContent,/密码.*重新扫描/);
  $('preview').click();await until(()=>!$('execute').disabled,'explicit local rescan authorizes the accepted task');
  w.document.querySelector('[data-tool=video]').click();await until(()=>$('title').textContent==='压缩视频'&&!$('preview').disabled,'return to idle video');
  jobs.video={Tool:'video',Status:'ready',TaskID:'empty-video-scan',PlanID:'empty-video-plan',Config:{...w.config()},Total:0,Actionable:0,Logs:[]};
  await w.poll();assert.match($('message').textContent,/没有符合条件的视频.*体积和原码率筛选/,'empty video scans explain filtering instead of name replacement');
  assert.match($('videoFilterSummary').textContent,/100 MB.*1000 kbps/,'active thresholds remain visible when advanced settings are folded');
  const videoNav=w.document.querySelector('[data-tool=video]');assert.equal(videoNav.querySelector('.tool-task-badge').textContent,'无项目');assert($('execute').disabled);
  jobs.video={...jobs.video,Total:3,Actionable:0,Conflicts:3};await w.poll();assert.equal(videoNav.querySelector('.tool-task-badge').textContent,'全跳过');assert.match(videoNav.title,/可执行 0 项，跳过 3 项/);assert($('execute').disabled);
  jobs.video={...jobs.video,Actionable:1,Conflicts:2};await w.poll();assert.equal(videoNav.querySelector('.tool-task-badge').textContent,'待执行');assert.match(videoNav.title,/可执行 1 项，跳过 2 项/);assert(!$('execute').disabled,'mixed preview still permits executable entries');
  videoNav.click();await until(()=>!$('preview').disabled,'reenter ready tool');assert(!videoNav.querySelector('.tool-task-badge').hidden,'an executable preview must remain pending when viewed');
  jobs.video={...jobs.video,TaskID:'empty-view',PlanID:'empty-view',Actionable:0,Conflicts:3};await w.poll();assert(!videoNav.querySelector('.tool-task-badge').hidden);videoNav.click();await until(()=>videoNav.querySelector('.tool-task-badge').hidden,'viewed no-op scan is cleared');
  jobs.video={...jobs.video,Status:'running',TaskID:'next-live-task',PlanID:'next-live-task',Total:4,Done:1};await w.poll();assert.equal(videoNav.querySelector('.tool-task-badge').textContent,'1/4');assert(!videoNav.querySelector('.tool-task-badge').hidden,'new tasks must reappear after previous task was read');
  await new Promise(r=>setTimeout(r,150));for(const win of windows)win.close();
  console.log('PASS: independent tasks, multi-page queued completion, remote plan/password guards, running parameter sync, delayed preview isolation and failure badges');
})().catch(e=>{console.error(e);for(const win of windows)win.close();process.exit(1)});
