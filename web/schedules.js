(()=>{
'use strict';const $=id=>document.getElementById(id),token=new URLSearchParams(location.search).get('session')||'';
$('back').href='/?session='+encodeURIComponent(token);let data={Jobs:[],Runs:[]},editing=false,loading=false,platform='';
let taskPage=0,sortKey='',sortDirection=1;const selectedJobs=new Set();let visibleJobs=[];
try{const size=localStorage.getItem('hamster.schedules.page-size.v1');if(['20','50','100'].includes(size))$('pageSize').value=size}catch{}
let runPage=0;
try{const size=localStorage.getItem('hamster.schedules.history-page-size.v1');if(['20','50','100'].includes(size))$('runPageSize').value=size}catch{}
let logJob=null,logRunID='',logEpoch=0,logPending=false;
const status={running:'运行中',success:'成功',failed:'失败',timeout:'超时',cancelled:'已停止',interrupted:'中断'};
const date=v=>v&&!v.startsWith('0001-')?new Date(v).toLocaleString():'—';
async function api(body){const r=await fetch('/api/schedules',{method:'POST',headers:{'Content-Type':'application/json','X-Toolbox-Token':token,...(['list','logs','detail'].includes(body.Action)?{'X-Hamster-Poll':'1'}:{})},body:JSON.stringify(body)});if(!r.ok){const error=Error(await r.text());error.status=r.status;throw error}return r.json()}
function node(tag,text,cls){const n=document.createElement(tag);if(text!==undefined)n.textContent=text;if(cls)n.className=cls;return n}
function button(text,action){const b=node('button',text);b.type='button';b.onclick=async()=>{b.disabled=true;try{await action();await refresh()}catch(e){$('message').textContent=e.message}finally{b.disabled=false}};return b}
let scheduleDecision=null;
function askSchedule({title,message,names=[],action='确认',danger=false}){
 if(scheduleDecision)return Promise.resolve(false);
 const dialog=$('scheduleConfirmDialog'),previousEditing=editing;editing=true;
 $('scheduleConfirmTitle').textContent=title;$('scheduleConfirmMessage').textContent=message;$('acceptScheduleConfirm').textContent=action;$('acceptScheduleConfirm').classList.toggle('danger-action',danger);$('scheduleConfirmItems').replaceChildren();for(const name of names)$('scheduleConfirmItems').append(node('li',name));
 dialog.returnValue='cancel';dialog.showModal();$('cancelScheduleConfirm').focus();return new Promise(resolve=>{scheduleDecision={resolve,previousEditing}});
}
$('cancelScheduleConfirm').onclick=()=>$('scheduleConfirmDialog').close('cancel');$('acceptScheduleConfirm').onclick=()=>$('scheduleConfirmDialog').close('accept');$('scheduleConfirmDialog').addEventListener('close',()=>{const pending=scheduleDecision;scheduleDecision=null;if(pending){editing=pending.previousEditing;pending.resolve($('scheduleConfirmDialog').returnValue==='accept')}});
function render(){
 $('examplesHint').hidden=!(data.Jobs||[]).some(j=>j.ID.startsWith('example-'));$('jobs').replaceChildren();$('summary').textContent=`${data.Jobs?.length||0} 个任务 · ${data.Active||0} 个运行中 · ${data.Queued||0} 个等待`;
 for(const id of selectedJobs)if(!(data.Jobs||[]).some(j=>j.ID===id))selectedJobs.delete(id);
 renderTaskTable();
 renderRunHistory();
 if($('output').open&&$('logAuto').checked)renderTaskLogs();
}
function renderRunHistory(){
 const runs=data.Runs||[],size=Number($('runPageSize').value),pages=Math.max(1,Math.ceil(runs.length/size));
 runPage=Math.max(0,Math.min(runPage,pages-1));const offset=runPage*size;
 $('runs').replaceChildren();$('allRunCount').textContent='（'+runs.length+'）';
 for(const r of runs.slice(offset,offset+size)){const b=button('',()=>openTaskLogs({ID:r.JobID,Name:r.Name},r.ID));b.className='run';b.append(node('span',r.Name),node('span',date(r.Started)),node('span',status[r.Status]||r.Status,r.Status));$('runs').append(b)}
 if(!runs.length)$('runs').append(node('p','执行后会在这里显示结果。','muted'));
 $('runPageInfo').textContent='共 '+runs.length+' 条'+(runs.length?'，显示 '+(offset+1)+'–'+Math.min(offset+size,runs.length)+' 条':'');
 $('runPageNumber').textContent=(runPage+1)+' / '+pages;
 $('firstRuns').disabled=$('previousRuns').disabled=runPage===0;
 $('lastRuns').disabled=$('nextRuns').disabled=runPage>=pages-1;
}
$('runPageSize').onchange=()=>{runPage=0;try{localStorage.setItem('hamster.schedules.history-page-size.v1',$('runPageSize').value)}catch{}renderRunHistory()};
$('firstRuns').onclick=()=>{runPage=0;renderRunHistory()};$('previousRuns').onclick=()=>{runPage--;renderRunHistory()};$('nextRuns').onclick=()=>{runPage++;renderRunHistory()};$('lastRuns').onclick=()=>{runPage=Math.max(0,Math.ceil((data.Runs||[]).length/Number($('runPageSize').value))-1);renderRunHistory()};
function renderTaskTable(){
 const query=$('taskSearch').value.trim().toLocaleLowerCase(),filter=$('statusFilter').value;
 let jobs=(data.Jobs||[]).map(j=>({...j,last:(data.Runs||[]).find(r=>r.JobID===j.ID),running:(data.Runs||[]).some(r=>r.JobID===j.ID&&r.Status==='running')}));
 jobs=jobs.filter(j=>j.Name.toLocaleLowerCase().includes(query)&&(filter==='all'||filter==='enabled'&&j.Enabled||filter==='paused'&&!j.Enabled||filter==='running'&&j.running||filter==='queued'&&j.Queued||filter==='invalid'&&!!j.Error||filter==='failed'&&['failed','timeout','interrupted','cancelled'].includes(j.last?.Status)));
 if(sortKey)jobs.sort((a,b)=>{let n=0;if(sortKey==='name')n=a.Name.localeCompare(b.Name,'zh-CN',{numeric:true});else if(sortKey==='status')n=(a.running?2:a.Enabled?1:0)-(b.running?2:b.Enabled?1:0);else if(sortKey==='next')n=(a.Enabled?Date.parse(a.Next)||0:0)-(b.Enabled?Date.parse(b.Next)||0:0);else if(sortKey==='last')n=(Date.parse(a.last?.Started)||0)-(Date.parse(b.last?.Started)||0);return n*sortDirection});
 const size=Number($('pageSize').value),pages=Math.max(1,Math.ceil(jobs.length/size));taskPage=Math.min(taskPage,pages-1);const offset=taskPage*size;
 $('jobs').replaceChildren();visibleJobs=jobs.slice(offset,offset+size).map(j=>j.ID);
 for(const j of jobs.slice(offset,offset+size)){
  const row=node('tr');row.dataset.jobId=j.ID;
  const cell=(label,content,cls)=>{const td=node('td',undefined,cls);td.dataset.label=label;if(typeof content==='string')td.textContent=content;else td.append(content);row.append(td);return td};
  const checkbox=node('input');checkbox.type='checkbox';checkbox.checked=selectedJobs.has(j.ID);checkbox.setAttribute('aria-label','选择任务 '+j.Name);checkbox.onchange=()=>{if(checkbox.checked)selectedJobs.add(j.ID);else selectedJobs.delete(j.ID);syncTaskSelection()};cell('选择',checkbox,'task-select-cell');
  const name=node('strong',j.Name,'task-name');const nameCell=cell('任务名称',name);if(j.Error)nameCell.append(node('small',j.Error,'job-error failed')); cell('类型',j.Kind==='http'?j.Method+' 请求':j.Shell+' 脚本','task-type');
  const stateButton=button(j.running?'运行中':j.Queued?'等待执行':j.Error?'需处理':j.Enabled?'已启用':'已暂停',()=>api({Action:'toggle',ID:j.ID}));stateButton.className='task-state '+(j.running?'state-running':j.Enabled?'state-enabled':'state-paused');stateButton.title=j.Error|| (j.running?'请先停止当前任务':j.Enabled?'点击暂停此任务':'点击启用此任务');stateButton.disabled=j.running;cell('状态',stateButton);
  cell('执行周期',describePeriod(j),'task-cycle');cell('下次执行时间',j.Enabled?date(j.Next):'—','task-date');const lastCell=cell('上次执行时间',j.last?date(j.last.Started):'—','task-date');if(j.last)lastCell.append(node('small',status[j.last.Status]||j.last.Status,'last-result '+j.last.Status));
  const actions=node('div',undefined,'task-row-actions');
  actions.append(button('执行',async()=>{if(await askSchedule({title:'执行任务',message:'现在执行此任务？脚本或请求可能修改文件和外部服务。',names:[j.Name],action:'确认执行'}))await api({Action:'run',ID:j.ID})}),button('编辑',async()=>open(await api({Action:'detail',ID:j.ID}))),button('日志',()=>openTaskLogs(j)),button('复制',async()=>{const copy=await api({Action:'detail',ID:j.ID});copy.ID='';copy.Name+='（副本）';copy.Enabled=false;open(copy)}),button('删除',async()=>{if(await askSchedule({title:'删除任务',message:'删除此计划任务？已有执行记录保留，已处理的文件不会回滚。',names:[j.Name],action:'确认删除',danger:true}))await api({Action:'delete',ID:j.ID})}));
  if(j.running||j.Queued)actions.append(button(j.Queued?'取消等待':'停止',()=>api({Action:'cancel',ID:j.ID})));cell('操作',actions,'task-operations');$('jobs').append(row)
 }
 if(!jobs.length){const tr=node('tr'),td=node('td',data.Jobs?.length?'没有匹配的任务，请调整搜索或筛选条件。':'还没有计划任务。点击“添加任务”开始。','table-empty');td.colSpan=8;tr.append(td);$('jobs').append(tr)}
 syncTaskSelection();
 $('taskPageInfo').textContent='共 '+jobs.length+' 条'+(jobs.length?'，显示 '+(offset+1)+'–'+Math.min(offset+size,jobs.length)+' 条':'');$('taskPageNumber').textContent=(taskPage+1)+' / '+pages;$('previousTasks').disabled=taskPage===0;$('nextTasks').disabled=taskPage>=pages-1;
 for(const b of document.querySelectorAll('[data-job-sort]')){const selected=b.dataset.jobSort===sortKey;b.closest('th').setAttribute('aria-sort',selected?(sortDirection===1?'ascending':'descending'):'none');b.dataset.direction=selected?(sortDirection===1?'↑':'↓'):'↕'}
}
$('taskSearch').addEventListener('input',()=>{taskPage=0;selectedJobs.clear();renderTaskTable()});$('statusFilter').onchange=()=>{taskPage=0;selectedJobs.clear();renderTaskTable()};$('pageSize').onchange=()=>{taskPage=0;try{localStorage.setItem('hamster.schedules.page-size.v1',$('pageSize').value)}catch{}renderTaskTable()};$('previousTasks').onclick=()=>{taskPage=Math.max(0,taskPage-1);renderTaskTable()};$('nextTasks').onclick=()=>{taskPage++;renderTaskTable()};for(const b of document.querySelectorAll('[data-job-sort]'))b.onclick=()=>{if(sortKey===b.dataset.jobSort)sortDirection*=-1;else{sortKey=b.dataset.jobSort;sortDirection=1}taskPage=0;renderTaskTable()};
function syncTaskSelection(){const count=visibleJobs.filter(id=>selectedJobs.has(id)).length;$('selectPage').checked=!!visibleJobs.length&&count===visibleJobs.length;$('selectPage').indeterminate=count>0&&count<visibleJobs.length;$('selectPage').disabled=!visibleJobs.length;$('selectedCount').textContent='已选 '+selectedJobs.size+' 项';$('applyBatch').disabled=!selectedJobs.size;$('clearSelection').disabled=!selectedJobs.size}
$('selectPage').onchange=()=>{for(const id of visibleJobs){if($('selectPage').checked)selectedJobs.add(id);else selectedJobs.delete(id)}renderTaskTable()};$('clearSelection').onclick=()=>{selectedJobs.clear();renderTaskTable()};
$('applyBatch').onclick=async()=>{const operation=$('batchOperation').value,ids=[...selectedJobs];if(!ids.length)return;if(operation==='export'){openTransfer('export');return}if(['run','delete'].includes(operation)&&!await askSchedule({title:(operation==='run'?'批量执行':'批量删除')+' · '+ids.length+' 项',message:operation==='run'?'现在执行所选任务？脚本或请求可能修改文件和外部服务。超过并发数会排队，同一任务不会重复入队。':'删除所选计划任务？已有执行记录保留，运行中的任务不会删除。',names:ids.map(id=>(data.Jobs||[]).find(j=>j.ID===id)?.Name||'已不存在的任务'),action:operation==='run'?'确认执行':'确认删除',danger:operation==='delete'}))return;$('applyBatch').disabled=true;try{const result=await api({Action:'batch',Operation:operation,IDs:ids});const rows=result.Results||[];$('batchResultSummary').textContent='成功 '+rows.filter(r=>r.OK).length+' 项，未成功 '+rows.filter(r=>!r.OK).length+' 项。';$('batchResults').replaceChildren();for(const r of rows){const line=node('p');line.append(node('strong',r.Name||'任务已不存在'),node('span',r.OK?(r.Queued?' · 已排队':operation==='run'?' · 已启动':' · 成功'):' · '+r.Error,r.OK?'success':'failed'));$('batchResults').append(line)}if(operation==='delete')for(const r of rows)if(r.OK)selectedJobs.delete(r.ID);if(operation==='run'){selectedJobs.clear();for(const r of rows)if(!r.OK&&(data.Jobs||[]).some(j=>j.ID===r.ID))selectedJobs.add(r.ID);if(selectedJobs.size)$('batchResultSummary').textContent+=' 已保留未执行成功项的选择，稍后重试可避免重复执行已启动项。'}$('batchResultDialog').showModal();await refresh()}catch(error){$('message').textContent=error.message}finally{syncTaskSelection()}};$('closeBatchResult').onclick=()=>$('batchResultDialog').close();
let transferMode='',transferPayload=null,transferExport=null,transferEpoch=0,importInFlight=false,importResultOrigin=0;
function showImportResult(result,error,origin){
 importResultOrigin=origin;const failed=Boolean(error),count=result?.Count||0,issues=result?.Issues||[];
 $('importResultTitle').textContent=failed?(error.status?'导入失败':'暂未确认导入结果'):count?(issues.length?'导入完成（部分跳过）':'导入完成'):'未新增任何任务';
 $('importResultDialog').dataset.result=failed?'failed':count?'success':'warning';
 $('importResultSummary').textContent=failed?(error.status?'导入请求失败，请查看下面的原因。':'未能确认服务器是否已完成导入。请先刷新任务列表核对，再决定是否重试，避免重复导入。'):'已导入 '+count+' 项，跳过 '+issues.length+' 项。'+(count?'新任务全部暂停，请检查后再启用。':'请查看跳过原因；已有任务未被覆盖。');
 $('importResultCounts').hidden=failed;$('importSuccessCount').textContent=String(count);$('importSkippedCount').textContent=String(issues.length);$('importResultItems').replaceChildren();
 for(const name of result?.Names||[])$('importResultItems').append(node('p','已导入 · '+name,'success'));
 for(const issue of issues){const line=node('p');line.append(node('strong',issue.Name),node('span',' · '+issue.Reason));$('importResultItems').append(line)}
 if(error)$('importResultItems').append(node('p',error.message,'failed'));
 $('importResultDetails').open=failed||issues.length>0;
 if(!$('importResultDialog').open)$('importResultDialog').showModal();$('ackImportResult').focus();
}
$('closeImportResult').onclick=()=>$('importResultDialog').close();$('ackImportResult').onclick=()=>$('importResultDialog').close();$('importResultDialog').addEventListener('close',()=>{if($('transferDialog').open&&importResultOrigin===transferEpoch)$('transferDialog').close();$('tasksTab').click()});
async function completeImport(epoch){
 if(importInFlight)return;importInFlight=true;const payload=transferPayload,conflict=$('importConflict').value;$('confirmTransfer').textContent='正在导入…';$('transferStatus').textContent='正在导入，请稍候…';$('importFile').disabled=true;$('importConflict').disabled=true;
 try{const result=await api({Action:'import',Payload:payload,Conflict:conflict});
  if(epoch===transferEpoch){reportTransfer(result);$('transferStatus').textContent='已导入 '+result.Count+' 项，均为暂停状态；跳过 '+result.Issues.length+' 项。';transferPayload=null}
  if(result.Count){$('taskSearch').value='';$('statusFilter').value='all';taskPage=0;selectedJobs.clear()}
  showImportResult(result,null,epoch);await refresh();
 }catch(error){if(epoch===transferEpoch)$('transferStatus').textContent=error.message;showImportResult(null,error,epoch)}
 finally{importInFlight=false;$('importFile').disabled=false;$('importConflict').disabled=false;if(epoch===transferEpoch){$('confirmTransfer').disabled=true;$('confirmTransfer').textContent='导入已结束'}}
}
function reportTransfer(result){$('transferReport').replaceChildren();for(const warning of result.Warnings||[])$('transferReport').append(node('p',warning,'failed'));for(const name of result.Names||[])$('transferReport').append(node('p','可导入 · '+name,'success'));for(const issue of result.Issues||[]){const p=node('p');p.append(node('strong',issue.Name),node('span',' · '+issue.Reason));$('transferReport').append(p)}}
function openTransfer(mode){transferEpoch++;transferMode=mode;transferPayload=null;transferExport=null;editing=true;$('importFile').value='';$('transferStatus').textContent='';$('transferReport').replaceChildren();$('transferTitle').textContent=mode==='import'?'导入计划任务':'导出计划任务';$('importOptions').hidden=mode!=='import';$('exportOptions').hidden=mode!=='export';$('exportScope').value=selectedJobs.size?'selected':'all';$('exportScope').querySelector('[value=selected]').disabled=!selectedJobs.size;$('exportFormat').value='toolbox';$('confirmTransfer').textContent=mode==='import'?'确认导入（全部暂停）':'生成并下载';$('confirmTransfer').disabled=mode==='import';if(mode==='import'&&importInFlight){$('confirmTransfer').disabled=true;$('importFile').disabled=true;$('importConflict').disabled=true;$('transferStatus').textContent='上一份文件正在导入，完成后会显示结果，请勿重复提交。'}$('transferDialog').showModal()}
async function previewImport(){const epoch=++transferEpoch;$('confirmTransfer').disabled=true;$('transferReport').replaceChildren();if(!transferPayload){$('transferStatus').textContent='请先选择 JSON 文件。';return}$('transferStatus').textContent='正在检查文件…';try{const r=await api({Action:'import-preview',Payload:transferPayload,Conflict:$('importConflict').value});if(epoch!==transferEpoch||!$('transferDialog').open)return;reportTransfer(r);$('transferStatus').textContent='可导入 '+r.Count+' 项，跳过 '+r.Issues.length+' 项。确认后新增，不修改原任务。';$('confirmTransfer').disabled=!r.Count}catch(e){if(epoch===transferEpoch)$('transferStatus').textContent=e.message}}
$('importTasks').onclick=()=>openTransfer('import');$('exportTasks').onclick=()=>openTransfer('export');$('closeTransfer').onclick=()=>$('transferDialog').close();$('transferDialog').addEventListener('close',()=>{transferEpoch++;transferPayload=null;transferExport=null;$('importFile').value='';editing=false});
$('importFile').onchange=async()=>{const file=$('importFile').files[0],epoch=++transferEpoch;transferPayload=null;$('confirmTransfer').disabled=true;if(!file)return;try{if(file.size>16*1024*1024)throw Error('导入文件不能超过 16 MB');const text=new TextDecoder('utf-8',{fatal:true}).decode(await file.arrayBuffer());if(epoch!==transferEpoch||!$('transferDialog').open)return;transferPayload=JSON.parse(text.replace(/^\uFEFF/,''));await previewImport()}catch(e){if(epoch===transferEpoch)$('transferStatus').textContent='读取失败：请使用 UTF-8 JSON 文件。'+e.message}};$('importConflict').onchange=previewImport;
function resetExport(){transferEpoch++;transferExport=null;$('transferReport').replaceChildren();$('transferStatus').textContent='';$('confirmTransfer').disabled=false;$('confirmTransfer').textContent='生成并下载'}$('exportFormat').onchange=resetExport;$('exportScope').onchange=resetExport;
function downloadTransfer(result){const format=$('exportFormat').value,blob=new Blob([JSON.stringify(result.Data,null,2)],{type:'application/json;charset=utf-8'}),url=URL.createObjectURL(blob),a=node('a');a.href=url;a.download=format==='bt'?'baota-crontab.json':'hamster-schedules.json';document.body.append(a);a.click();a.remove();setTimeout(()=>URL.revokeObjectURL(url),1000);$('transferStatus').textContent='已导出 '+result.Count+' 项。请妥善保管下载文件。'}
$('confirmTransfer').onclick=async()=>{const epoch=++transferEpoch;$('confirmTransfer').disabled=true;try{if(transferMode==='import'){await completeImport(epoch);return}
 if(transferExport){downloadTransfer(transferExport);$('confirmTransfer').disabled=false;return}
 const ids=$('exportScope').value==='selected'?[...selectedJobs]:[];if($('exportScope').value==='selected'&&!ids.length)throw Error('请先选择任务');const result=await api({Action:'export',Format:$('exportFormat').value,IDs:ids});if(epoch!==transferEpoch)return;reportTransfer(result);if(result.Issues.length||(result.Warnings||[]).length){transferExport=result;$('transferStatus').textContent=result.Issues.length?'可导出 '+result.Count+' 项，以下 '+result.Issues.length+' 项无法转换。可改用工具箱完整格式。':'可导出 '+result.Count+' 项。请核对下方宝塔执行行为差异后确认下载。';$('confirmTransfer').textContent='下载可兼容的 '+result.Count+' 项';$('confirmTransfer').disabled=!result.Count;return}downloadTransfer(result);$('confirmTransfer').disabled=false;
 }catch(error){if(epoch===transferEpoch){$('transferStatus').textContent=error.message;$('confirmTransfer').disabled=false}}};
let logRange='all',logClearing=false;
try{$('logAuto').checked=localStorage.getItem('hamster.schedules.logs.auto.v1')!=='false'}catch{}
function openTaskLogs(job,selectedID=''){
 logJob={ID:job.ID,Name:job.Name};logRunID=selectedID;logEpoch++;logPending=false;logRange='all';logClearing=false;
 $('logMode').value=selectedID?'single':'combined';$('logDates').hidden=true;$('logFrom').value='';$('logTo').value='';for(const b of document.querySelectorAll('[data-log-range]'))b.setAttribute('aria-pressed',String(b.dataset.logRange==='all'));
 $('logTitle').textContent='查看日志 · '+job.Name;$('logRecords').replaceChildren();$('log').textContent='正在读取日志…';$('runInfo').textContent='';$('logNotice').textContent='';$('logLocation').textContent='';$('logStats').textContent='';$('copyLog').disabled=true;$('clearLogs').disabled=true;
 if(!$('output').open)$('output').showModal();renderTaskLogs();
}
function updateLogText(text,copyable){if($('log').textContent!==text){const previous=$('log').scrollTop,atBottom=$('log').scrollHeight-$('log').clientHeight-previous<24;$('log').textContent=text;$('log').scrollTop=atBottom?$('log').scrollHeight:previous}$('copyLog').disabled=!copyable}
function filteredLogRecords(){const records=(data.Runs||[]).filter(r=>r.JobID===logJob.ID);return records}
function renderTaskLogs(){
 if(!logJob||!$('output').open||logClearing)return;
 const combined=$('logMode').value==='combined',job=(data.Jobs||[]).find(j=>j.ID===logJob.ID),records=filteredLogRecords();
 $('runFromLog').disabled=!job||job.Queued||records.some(r=>r.Status==='running');$('runFromLog').title=!job?'此任务已删除':job.Queued?'此任务已在等待队列':records.some(r=>r.Status==='running')?'此任务正在运行':'立即执行当前任务';
 if(!job&&!records.length){$('logRecords').replaceChildren();$('logLayout').classList.add('combined-log');$('logRecords').hidden=true;$('logStats').textContent='共 0 条记录';$('clearLogs').disabled=true;$('runInfo').textContent='任务已删除';updateLogText('此任务已删除，保留的日志已清空。',false);return}
 $('logLayout').classList.toggle('combined-log',combined);$('logRecords').hidden=combined;for(const b of document.querySelectorAll('[data-log-range]'))b.disabled=!combined;$('logDates').hidden=!combined||logRange!=='custom';
 if(combined){if(!logPending)loadCombinedLog();return}
 $('logRecords').replaceChildren();$('logStats').textContent='共 '+records.length+' 条执行记录';$('clearLogs').disabled=!records.some(r=>r.Status!=='running');
 if(!records.length){logRunID='';logEpoch++;logPending=false;$('runInfo').textContent='此任务还没有执行记录。';updateLogText('可以点击上方“执行任务”，或等待下次计划执行。',false);return}
 if(!records.some(r=>r.ID===logRunID)){logRunID=records[0].ID;logEpoch++;logPending=false;$('log').textContent='正在读取日志…';$('copyLog').disabled=true}
 for(const r of records){const b=node('button',undefined,'log-record');b.type='button';b.setAttribute('aria-pressed',String(r.ID===logRunID));b.append(node('span',date(r.Started)),node('small',status[r.Status]||r.Status,r.Status));b.onclick=()=>{logRunID=r.ID;reloadLogView()};$('logRecords').append(b)}
 const selected=records.find(r=>r.ID===logRunID);if(selected&&!logPending)loadTaskLog(selected);
}
async function loadCombinedLog(){
 const epoch=++logEpoch,jobID=logJob.ID;logPending=true;
 if(logRange==='custom'&&(!$('logFrom').value||!$('logTo').value)){$('runInfo').textContent='请选择开始与结束日期，再点击查询。';updateLogText('',false);logPending=false;return}
 try{const result=await api({Action:'logs',ID:jobID,Params:{range:logRange,from:$('logFrom').value,to:$('logTo').value}});if(epoch!==logEpoch||!$('output').open||logJob?.ID!==jobID)return;
 $('logLocation').textContent='日志目录：'+result.Directory;$('logLocation').title=result.Directory;$('logStats').textContent='共 '+result.Total+' 条 · '+((result.Bytes||0)/1024).toFixed(1)+' KB';$('clearLogs').disabled=!result.Completed;
 $('runInfo').textContent='显示 '+result.Matched+' / '+result.Total+' 条 · 从早到晚排列 · '+result.Timezone+(result.Truncated?' · 已截取末尾约 1 MB':'');updateLogText(result.Output||(result.Total?'此时间范围内没有执行记录。':'此任务还没有执行记录。'),Boolean(result.Output));
 }catch(error){if(epoch===logEpoch&&$('output').open){$('runInfo').textContent='读取日志失败：'+error.message;$('copyLog').disabled=true}}
 finally{if(epoch===logEpoch)logPending=false}
}
async function loadTaskLog(selected){
 const epoch=++logEpoch,jobID=logJob.ID;logPending=true;
 try{const r=await api({Action:'detail',ID:selected.ID});if(epoch!==logEpoch||!$('output').open||logJob?.ID!==jobID)return;
 if(r.JobID!==jobID)throw Error('日志不属于当前任务');$('runInfo').textContent=date(r.Started)+' · '+(status[r.Status]||r.Status)+(r.Finished&&!r.Finished.startsWith('0001-')?' · 结束：'+date(r.Finished):'');updateLogText(r.Output||(r.Status==='running'?'执行中，暂无输出；有新输出时会自动显示。':'无输出'),Boolean(r.Output));
 }catch(error){if(epoch===logEpoch&&$('output').open){$('runInfo').textContent='读取日志失败：'+error.message;$('copyLog').disabled=true}}
 finally{if(epoch===logEpoch)logPending=false}
}
function reloadLogView(){logEpoch++;logPending=false;renderTaskLogs()}
$('logMode').onchange=reloadLogView;$('logAuto').onchange=()=>{try{localStorage.setItem('hamster.schedules.logs.auto.v1',String($('logAuto').checked))}catch{}logEpoch++;logPending=false;if($('logAuto').checked)renderTaskLogs()};
for(const b of document.querySelectorAll('[data-log-range]'))b.onclick=()=>{logRange=b.dataset.logRange;for(const tab of document.querySelectorAll('[data-log-range]'))tab.setAttribute('aria-pressed',String(tab===b));reloadLogView()};$('applyLogDates').onclick=reloadLogView;
$('clearLogs').onclick=async()=>{if(!logJob||logClearing)return;const job={...logJob};if(!await askSchedule({title:'清空任务日志',message:'清空此任务全部已结束的执行记录和日志？不受当前时间筛选限制，清空后无法恢复。正在运行的记录与输出保留，任务配置和执行周期不变。',names:[job.Name],action:'确认清空',danger:true}))return;
 if(logJob?.ID!==job.ID||!$('output').open)return;logClearing=true;logEpoch++;logPending=false;$('clearLogs').disabled=true;
 try{const result=await api({Action:'clear-logs',ID:job.ID});if(logJob?.ID!==job.ID||!$('output').open)return;logRunID='';$('logNotice').textContent='已清空 '+result.Count+' 条已结束记录。'+(result.PendingFiles?'其中 '+result.PendingFiles+' 个日志文件暂时被占用，重启时会再次尝试清理。':'')}
 catch(error){if(logJob?.ID===job.ID&&$('output').open)$('logNotice').textContent='清空失败：'+error.message}
 finally{logClearing=false;await refresh();if(logJob?.ID===job.ID&&$('output').open&&!$('logAuto').checked)renderTaskLogs()}
};
$('runFromLog').onclick=async()=>{if(!logJob)return;const job={...logJob};if(!await askSchedule({title:'执行任务',message:'立即执行此任务？脚本或请求可能修改文件和外部服务。',names:[job.Name],action:'确认执行'}))return;$('runFromLog').disabled=true;try{await api({Action:'run',ID:job.ID});if(logJob?.ID===job.ID)$('logNotice').textContent='任务已提交，可在日志中查看进度。';await refresh();if(logJob?.ID===job.ID&&$('output').open&&!$('logAuto').checked)reloadLogView()}catch(error){if(logJob?.ID===job.ID){$('logNotice').textContent=error.message;$('runFromLog').disabled=false}}};

async function refresh(){if(loading)return;loading=true;try{data=await api({Action:'list'});platform=data.Platform;$('message').textContent=data.Error||'';render()}catch(e){$('message').textContent=e.message}finally{loading=false}}
function describePeriod(j){
 const clock=String(j.AtHour||0).padStart(2,'0')+':'+String(j.AtMinute||0).padStart(2,'0');
 switch(j.Period){case 'day':return '每天 '+clock;case 'day-n':return '每 '+j.Every+' 天 '+clock;case 'hour':return '每小时第 '+j.AtMinute+' 分钟';case 'hour-n':return '每 '+j.Every+' 小时，第 '+j.AtMinute+' 分钟';case 'minute-n':return '每 '+j.Every+' 分钟';case 'second-n':return '每 '+j.Every+' 秒';case 'week':return ['星期日','星期一','星期二','星期三','星期四','星期五','星期六'][j.Weekday]+' '+clock;case 'month':return '每月 '+j.Day+' 日 '+clock;default:return j.Interval?'固定间隔 '+j.Interval+' 秒':j.Cron}
}
let previewTimer,previewEpoch=0;
function scheduleFields(){const period=$('period').value;return {Period:period==='interval'?'':period,Every:Number($('every').value),AtHour:$('hour').disabled?0:Number($('hour').value),AtMinute:$('minute').disabled?0:Number($('minute').value),Day:Number($('day').value),Weekday:Number($('weekday').value),Interval:period==='interval'?Math.round(Number($('minutes').value)*60):0,Cron:$('cron').value}}
function queueSchedulePreview(){
 clearTimeout(previewTimer);const epoch=++previewEpoch,j=scheduleFields();
 $('scheduleDescription').textContent=j.Period==='cron'?'按照自定义 Cron 规则执行。':describePeriod(j)+'执行。';$('scheduleTimes').replaceChildren();$('schedulePreviewHint').textContent='正在计算接下来的执行时间…';
 const inputs=[...$('periodFields').querySelectorAll('input,select'),$('cron')].filter(el=>!el.disabled);
 if(inputs.some(el=>!el.value||!el.checkValidity())){$('schedulePreviewHint').textContent='请先填写有效的周期参数。';return}
 previewTimer=setTimeout(async()=>{if(!$('editor').open||epoch!==previewEpoch)return;
 try{const result=await api({Action:'preview',Job:j});if(epoch!==previewEpoch||!$('editor').open)return;
 for(const time of result.Times||[])$('scheduleTimes').append(node('li',time));
 $('schedulePreviewHint').textContent='按服务端当前时间 '+result.Now+' 推算接下来 5 次（'+result.Timezone+'）。仅作示例，保存并启用后才执行；任务未完成时不会重复运行。';
 }catch(error){if(epoch===previewEpoch&&$('editor').open)$('schedulePreviewHint').textContent='无法生成执行时间：'+error.message}
 },250);
}
function fields(){$('retryPOSTField').hidden=$('method').value!=='POST';const p=$('period').value;$('httpFields').hidden=$('kind').value!=='http';$('scriptFields').hidden=$('kind').value!=='script';
 $('periodFields').hidden=p==='cron';const limits={'day-n':[31,'天'],'hour-n':[24,'小时'],'minute-n':[60,'分钟'],'second-n':[59,'秒']};
 const visibility={everyField:!!limits[p],weekField:p==='week',dayField:p==='month',hourField:['day','day-n','week','month'].includes(p),minuteField:['day','day-n','hour','hour-n','week','month'].includes(p),intervalFields:p==='interval',cronField:p==='cron'};
 for(const [id,visible]of Object.entries(visibility)){$(id).hidden=!visible;for(const input of $(id).querySelectorAll('input,select'))input.disabled=!visible}
 if(limits[p]){$('every').max=limits[p][0];$('everyUnit').textContent=limits[p][1]}
 $('periodHint').textContent=({'day-n':'按日历日步长执行，从每月 1 日重新计数。例如 N=2：1、3、5 日……。','hour-n':'按小时步长执行，每天从 0 点重新计数。例如 N=2：0、2、4 点……。','minute-n':'按分钟步长执行，每小时从第 0 分钟重新计数。','second-n':'高频执行：工具箱运行后每 N 秒触发，同一任务未结束时跳过，不补跑。建议先手动测试。','month':'没有所选日期的月份会跳过。','interval':'保留旧任务的固定间隔，从启动或执行时重新计时。'})[p]||'按服务端电脑本地时间执行。';
}
function restorePeriod(j){
 if(j.Period){$('period').value=j.Period;$('every').value=j.Every||1;$('hour').value=j.AtHour||0;$('minute').value=j.AtMinute||0;$('day').value=j.Day||1;$('weekday').value=j.Weekday??1;$('cron').value=j.Cron||'';return}
 if(j.Interval){$('period').value='interval';$('minutes').value=j.Interval/60;return}
 const f=(j.Cron||'').trim().split(/\s+/),num=v=>/^\d+$/.test(v||'');
 $('period').value='cron';$('cron').value=j.Cron||'';
 if(f.length===5&&num(f[0])){$('minute').value=Number(f[0]);if(num(f[1])){$('hour').value=Number(f[1]);if(f[2]==='*'&&f[3]==='*'&&f[4]==='*')$('period').value='day';else if(f[2]==='*'&&f[3]==='*'&&num(f[4])){$('period').value='week';$('weekday').value=Number(f[4])%7}else if(num(f[2])&&f[3]==='*'&&f[4]==='*'){$('period').value='month';$('day').value=Number(f[2])}}else if(f.slice(1).every(v=>v==='*'))$('period').value='hour'}
}
function open(j){$('form').reset();$('formError').textContent='';$('jobID').value=j?.ID||'';$('editorTitle').textContent=j?.ID?'编辑计划任务':'新建计划任务';$('shell').replaceChildren();for(const name of platform==='windows'?['powershell','cmd','sh','bash']:['sh','bash']){const o=node('option',name);o.value=name;$('shell').append(o)}
 if(j){for(const [id,key]of Object.entries({name:'Name',kind:'Kind',shell:'Shell',script:'Script',directory:'Directory',url:'URL',method:'Method',body:'Body',timeout:'Timeout',expectedStatus:'ExpectedStatus',responseContains:'ResponseContains',retryCount:'RetryCount',retryDelay:'RetryDelay'}))$(id).value=j[key]||'';$('headers').value=JSON.stringify(j.Headers||{},null,2);$('enabled').checked=j.Enabled;$('retryCount').value=String(j.RetryCount||0);$('retryDelay').value=j.RetryDelay||5;$('retryPOST').checked=!!j.RetryPOST;$('readPaths').value=(j.ReadPaths||[]).join('\n');$('writePaths').value=(j.WritePaths||[]).join('\n');restorePeriod(j);}
 fields();editing=true;$('editor').showModal();$('name').focus();queueSchedulePreview();
}
window.addEventListener('hamster-schedule-overlay',event=>{editing=Boolean(event.detail)});window.addEventListener('hamster-schedule-template',event=>{const job={...event.detail,ID:'',Enabled:false};open(job)});
$('create').onclick=()=>open();$('refresh').onclick=refresh;$('kind').onchange=fields;$('method').onchange=fields;$('period').onchange=()=>{fields();queueSchedulePreview()};for(const id of ['every','weekday','day','hour','minute','minutes','cron'])$(id).addEventListener('input',queueSchedulePreview);$('close').onclick=()=>$('editor').close();$('editor').onclose=()=>{editing=false;clearTimeout(previewTimer);previewEpoch++};$('closeOutput').onclick=()=>$('output').close();$('output').addEventListener('close',()=>{logEpoch++;logPending=false;logJob=null;logRunID=''});$('refreshLogs').onclick=async()=>{await refresh();if(!$('logAuto').checked)reloadLogView()};$('copyLog').onclick=async()=>{try{await navigator.clipboard.writeText($('log').textContent);$('copyLog').textContent='已复制';setTimeout(()=>$('copyLog').textContent='复制日志',1500)}catch{$('runInfo').textContent+=' · 请选中输出后复制'}};
$('form').onsubmit=async e=>{e.preventDefault();$('save').disabled=true;try{let headers={};if($('headers').value.trim()){headers=JSON.parse($('headers').value);if(!headers||Array.isArray(headers)||typeof headers!=='object'||Object.values(headers).some(v=>typeof v!=='string'))throw Error('请求头需为 JSON 对象，值为字符串')}
 const job={Name:$('name').value,Kind:$('kind').value,Shell:$('shell').value,Script:$('script').value,Directory:$('directory').value,URL:$('url').value,Method:$('method').value,Body:$('body').value,Headers:headers,Timeout:Number($('timeout').value),Enabled:$('enabled').checked,ExpectedStatus:$('kind').value==='http'?Number($('expectedStatus').value):0,ResponseContains:$('kind').value==='http'?$('responseContains').value:'',RetryCount:$('kind').value==='http'?Number($('retryCount').value):0,RetryDelay:Number($('retryDelay').value),RetryPOST:$('retryPOST').checked,ReadPaths:$('kind').value==='script'?$('readPaths').value.split(/\r?\n/).map(p=>p.trim()).filter(Boolean):[],WritePaths:$('kind').value==='script'?$('writePaths').value.split(/\r?\n/).map(p=>p.trim()).filter(Boolean):[],...scheduleFields()};await api({Action:'save',ID:$('jobID').value,Job:job});$('editor').close();await refresh()}catch(e){$('formError').textContent=e.message}finally{$('save').disabled=false}};
refresh();setInterval(()=>{if(!document.hidden&&!editing)refresh()},4000);
})();
