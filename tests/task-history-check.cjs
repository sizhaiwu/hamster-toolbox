const assert=require('node:assert/strict');
const {createUI,until,setState,storage}=require('./ui-check.cjs');
const pause=()=>new Promise(r=>setTimeout(r,25));
(async()=>{
 let detailResolve=null,scans=[],records=[],liveStates=[];
 const config={Tool:'image',Root:'E:/synthetic',Output:'E:/results',Recursive:true,Workers:2,ImageFormats:['.png'],PngMode:'webp-png',PngQuality:90,Quality:75,Magick:'C:/tools/magick.exe'};
 const task={Version:1,ID:'a'.repeat(48),Tool:'image',Status:'interrupted',Started:'2026-01-01T00:00:00Z',Result:{Success:2,Failed:1,Skipped:0},Total:4,Done:3,Config:config,Independent:true,Logs:['synthetic task log']};
 records=[task];let canRetry=true;
 const dom=createUI(10120,true,390,w=>{const base=w.fetch;w.confirm=()=>true;w.fetch=async(url,options)=>{
  const route=url.replace('/api/',''),body=JSON.parse(options.body);
  const response=value=>({ok:true,json:async()=>JSON.parse(JSON.stringify(value))});
  if(route==='state'){const state=await (await base(url,options)).json(),tool=options.headers['X-Toolbox-Tool'];const selected=liveStates.find(s=>s.Tool===tool)||(state.Tool===tool||!state.Tool?state:{Tool:tool,Status:'idle',Logs:[]});return response({...selected,Tasks:liveStates});}
  if(route==='task-history')return response({Records:records,Interrupted:records.filter(r=>r.Status==='interrupted').length,Keep:50});
  if(route==='task-history-detail'){const record=records.find(r=>r.ID===body.ID)||task;if(detailResolve)return new Promise(resolve=>{detailResolve.resolve=()=>resolve(response({Record:record,Items:[],Total:0,Offset:0,Remaining:2,CanRetry:canRetry}))});return response({Record:record,Items:[{Item:{Source:'E:/synthetic/<script>.png',Conflict:''},Status:'failed',Message:'test failure'}],Total:101,Offset:body.Offset||0,Remaining:2,CanRetry:canRetry});}
  if(route==='task-history-clear'){records=[];return response({ok:true});}
  if(route==='plan'){scans.push(body);return base(url,options)}
  return base(url,options);
 }});
 const w=dom.window,$=id=>w.document.getElementById(id);await until(()=>!$('preview').disabled,'ready');await until(()=>!$('historyRecoveryNotice').hidden,'interrupted notice');
 $('viewInterruptedTasks').click();await until(()=>!$('taskHistoryDetail').hidden,'record loaded');
 assert.match($('taskHistoryHeading').textContent,/压缩图片.*上次中断/);assert.equal($('taskHistoryItems').querySelector('script'),null);assert.match($('taskHistoryItems').textContent,/<script>\.png/);assert.equal($('retryTaskHistory').disabled,false);
 $('taskHistoryNext').click();await until(()=>$('taskHistoryPage').textContent.startsWith('101–'),'history next page');
 $('restoreTaskHistory').click();await until(()=>!$('taskHistoryDialog').open&&!$('preview').disabled,'restore config');
 assert.equal($('Root').value,config.Root);assert.equal($('Output').value,config.Output);assert.equal(scans.length,0,'restoring never executes or scans');assert.equal($('retryUnfinished').hidden,false);
 $('Quality').value='80';$('Quality').dispatchEvent(new w.Event('input',{bubbles:true}));$('retryUnfinished').click();await until(()=>scans.length===1,'targeted rescan');assert.equal(scans[0].RetryTaskID,task.ID);assert.equal(scans[0].Quality,80,'edited parameters used');
 setState({Status:'finished',Tool:'image',Items:[],Logs:[],Result:{},Total:0,HistoryID:'test-finished'});await w.eval('poll()');await pause();
 $('openTaskHistory').click();await until(()=>!$('taskHistoryDetail').hidden,'reopen history');canRetry=false;task.Independent=false;task.Tool='rename';task.Config={Tool:'rename',Root:'E:/synthetic',Prefix:'new-',Files:true,Dirs:true};
 $('refreshTaskHistory').click();await until(()=>$('taskHistoryRecoveryHint').textContent.includes('完整预览')||$('taskHistoryRecoveryHint').textContent.includes('重新扫描'),'rename caution');assert.equal($('retryTaskHistory').disabled,true,'rename replay unavailable');
 detailResolve={};$('refreshTaskHistory').click();await until(()=>!!detailResolve.resolve,'slow detail');$('closeTaskHistory').click();detailResolve.resolve();await pause();assert.equal($('taskHistoryDialog').open,false,'late response cannot reopen dialog');detailResolve=null;
 const video={...task,ID:'b'.repeat(48),Tool:'video',Status:'running',Done:1,Total:10,Config:{Tool:'video',Root:'E:/video'},Result:{Success:0,Failed:0,Skipped:0}};
 records=[task,video];liveStates=[{Tool:'video',Status:'running',HistoryID:video.ID,Done:1,Total:10}];
 $('openTaskHistory').click();await until(()=>$('taskHistoryList').textContent.includes('1/10'),'history lists background task progress');
 video.Status='finished';video.Done=10;video.Result.Failed=10;liveStates=[{Tool:'video',Status:'finished',HistoryID:video.ID,Result:video.Result}];
 await w.poll();await until(()=>$('taskHistoryList').textContent.includes('压缩视频 · 失败'),'history refreshes another tool completion without reopening');
 liveStates=[{Tool:'image',Status:'running',TaskID:'active-image',Total:4,Done:1},{Tool:'rename',Status:'running',TaskID:'active-rename'}];
 await w.poll();assert($('Root').disabled);assert($('restoreTaskHistory').disabled,'cannot replace history target while that tool is busy');
 liveStates=liveStates.filter(s=>s.Tool!=='rename');await w.poll();assert(!$('restoreTaskHistory').disabled,'other running tools do not block restoring an idle target');
 liveStates.push({Tool:'rename',Status:'running',TaskID:'started-on-another-page'});$('restoreTaskHistory').click();await until(()=>$('taskHistoryStatus').textContent.includes('其他页面开始任务'),'restore rechecks target state at action time');assert($('taskHistoryDialog').open);assert.equal($('title').textContent,'压缩图片');
 liveStates=liveStates.filter(s=>s.Tool!=='rename');await w.poll();
 $('restoreTaskHistory').click();await until(()=>$('title').textContent==='批量重命名'&&!$('taskHistoryDialog').open,'restore idle tool while another task runs');assert.equal($('Root').value,'E:/synthetic');assert(!w.document.querySelector('[data-tool=image]').disabled);
 $('openTaskHistory').click();await until(()=>!$('taskHistoryDetail').hidden,'clear history');$('clearTaskHistory').click();await until(()=>$('taskHistoryList').textContent.includes('还没有'),'cleared');assert.equal($('taskHistoryList').textContent,'还没有执行记录。');assert.equal($('historyRecoveryNotice').hidden,true);
 await pause();dom.window.close();
 console.log('PASS: history recovery, pagination, targeted rescan, stale response isolation, background progress/completion refresh, target-scoped restore and clearing');
})().catch(e=>{console.error(e);process.exitCode=1;setTimeout(()=>process.exit(1),100)});
