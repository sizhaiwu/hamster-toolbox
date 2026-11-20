const assert=require('node:assert/strict');
const {createUI,until,setState,storage}=require('./ui-check.cjs');
(async()=>{
 setState({Status:'idle'});let dom=createUI(17601,true,390),w=dom.window,$=id=>w.document.getElementById(id);
 await until(()=>$('title').textContent&&!$('preview').disabled,'initialized');
 w.document.querySelector('[data-tool="extract"]').click();await until(()=>$('title').textContent==='解压资源'&&!$('preview').disabled,'extract form');
 const started=new Date(Date.now()-60000).toISOString();
 let task={Status:'running',Tool:'extract',PlanID:'progress-demo',Total:2,Done:0,Logs:[],Items:[],Result:{},Operation:{Tool:'extract',Source:'E:/synthetic/large <img>.001',Phase:'extract',ItemNumber:1,ItemTotal:2,Percent:25,PercentKnown:true,Started:started,PhaseStarted:started}};
 const show=async()=>{setState(structuredClone(task));await w.poll()};await show();
 assert(!$('extractProgressPanel').hidden);assert.equal($('extractProgressBar').value,25);assert.equal($('progress').value,0,'batch count must not be replaced with stage percentage');assert.match($('extractProgressLabel').textContent,/1 \/ 2/);assert.match($('extractProgressDetail').textContent,/预计本阶段剩余/);assert.equal($('extractProgressSource').textContent,'large <img>.001');assert.equal($('extractProgressPanel').querySelector('img'),null);
 task.Operation.Percent=0;await show();assert.equal($('extractProgressBar').value,0);assert($('extractProgressBar').hasAttribute('value'),'real zero is a known value');
 task.Operation={...task.Operation,Phase:'validate',PercentKnown:false,FilesDone:150};await show();assert.equal($('extractProgressBar').hasAttribute('value'),false);assert.equal($('extractProgressPercent').textContent,'进行中');assert.match($('extractProgressPhase').textContent,/检查/);assert.match($('extractProgressDetail').textContent,/已检查 150/);assert(!$('extractProgressDetail').textContent.includes('预计'));
 task.Operation={...task.Operation,Phase:'publish',PercentKnown:true,Percent:60,FilesDone:3,FilesTotal:5};await show();assert.match($('extractProgressPhase').textContent,/写入/);assert.match($('extractProgressDetail').textContent,/3 \/ 5/);
 // Reload restores the authoritative running state without restarting a task.
 storage.LastTool='extract';dom.window.close();dom=createUI(17602);w=dom.window;$=id=>w.document.getElementById(id);await until(()=>$('title').textContent==='解压资源'&&$('appVersion').textContent,'reloaded');await w.poll();assert(!$('extractProgressPanel').hidden);assert.equal($('extractProgressBar').value,60);
 task.Done=1;task.Operation={...task.Operation,Source:'E:/synthetic/next.zip',ItemNumber:2,Phase:'extract',Percent:5};await show();assert.equal($('progress').value,1);assert.match($('extractProgressLabel').textContent,/2 \/ 2/);assert.equal($('extractProgressBar').value,5);
 for(const Status of ['finished','cancelled','ready']){task.Status=Status;await show();assert($('extractProgressPanel').hidden)}
 task.Status='running';task.Tool='image';await show();assert($('extractProgressPanel').hidden);dom.window.close();
 console.log('PASS: live archive progress separate from batch count, known zero/indeterminate phases, ETA, file counts, safe names, reload recovery, next archive reset and terminal/tool isolation');
})().catch(error=>{console.error(error);process.exit(1)});
