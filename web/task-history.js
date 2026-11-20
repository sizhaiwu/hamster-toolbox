(() => {
  window.setupTaskHistory = ({api,$,toolName,canChange,beforeOpen,restore,scan,message}) => {
    let records=[],selected='',detail=null,offset=0,total=0,loaded=null,locked=false,acting=false;
    let listRequest=0,detailRequest=0,lastFinished='';
    const statusName=r=>({running:'执行中',interrupted:'上次中断',cancelled:'已取消',finished:r.Result?.Failed?'部分失败':'已完成'})[r.Status]||r.Status;
    const timeName=value=>new Date(value).toLocaleString();
    const note=text=>{$('taskHistoryStatus').textContent=text;};
    function syncUI(value=locked,tool,root) {
      locked=value;
      const match=loaded&&loaded.Tool===tool&&loaded.Config.Root.trim()===root?.trim();
      if(tool!==undefined)$('retryUnfinished').hidden=!match;
      $('retryUnfinished').disabled=locked||acting;
      $('restoreTaskHistory').disabled=locked||acting||!detail||detail.Record.Status==='running';
      $('retryTaskHistory').disabled=locked||acting||!detail?.CanRetry;
      $('exportTaskHistory').disabled=acting||!detail;
      $('clearTaskHistory').disabled=locked||acting;
      $('taskHistoryPrevious').disabled=acting||offset===0;
      $('taskHistoryNext').disabled=acting||offset+100>=total;
    }
    function renderList() {
      $('taskHistoryList').replaceChildren();
      if(!records.length){$('taskHistoryList').textContent='还没有执行记录。';return;}
      for(const r of records) {
        const b=document.createElement('button');b.type='button';b.className='history-record';b.setAttribute('aria-pressed',String(r.ID===selected));
        const title=document.createElement('b');title.textContent=toolName(r.Tool)+' · '+statusName(r);
        const date=document.createElement('span');date.textContent=timeName(r.Started);
        const path=document.createElement('small');path.textContent=r.Config?.Root||'';path.title=path.textContent;
        b.append(title,date,path);b.onclick=()=>{selected=r.ID;offset=0;renderList();showDetail();};$('taskHistoryList').append(b);
      }
    }
    async function refresh(open=false,preferInterrupted=false) {
      const request=++listRequest;
      try {
        const data=await api('task-history');if(request!==listRequest)return;
        records=data.Records||[];
        $('historyRecoveryNotice').hidden=!(data.Interrupted>0);
        $('historyRecoveryText').textContent=data.Interrupted+' 个任务上次未正常结束，可在历史中检查并重新预览。';
        $('openTaskHistory').title='任务历史'+(data.Interrupted>0?' · '+data.Interrupted+' 个上次中断的任务':'');
        if(!open||!$('taskHistoryDialog').open)return;
        if(!records.some(r=>r.ID===selected))selected='';
        if(preferInterrupted)selected=records.find(r=>r.Status==='interrupted')?.ID||selected;
        selected=selected||records[0]?.ID||'';renderList();
        if(selected)await showDetail();else{$('taskHistoryDetail').hidden=true;$('taskHistoryEmpty').hidden=false;detail=null;}
        note('自动保留最近 '+(data.Keep||50)+' 次记录；空间不足时优先清理较旧记录。');
      } catch(e) {if(open&&request===listRequest)note('读取历史失败：'+e.message);}
      finally {syncUI();}
    }
    async function showDetail() {
      const id=selected,request=++detailRequest;detail=null;syncUI();note('正在读取记录…');
      $('taskHistoryDetail').hidden=true;$('taskHistoryEmpty').hidden=false;$('taskHistoryEmpty').textContent='正在读取记录…';
      try {
        const data=await api('task-history-detail',{ID:id,Offset:offset,Filter:$('taskHistoryFilter').value});
        if(request!==detailRequest||!$('taskHistoryDialog').open||selected!==id)return;
        detail=data;offset=data.Offset;total=data.Total;const r=data.Record;
        $('taskHistoryHeading').textContent=toolName(r.Tool)+' · '+statusName(r);
        $('taskHistorySummary').textContent=timeName(r.Started)+' · '+r.Total+' 项 · 成功 '+r.Result.Success+' / 失败 '+r.Result.Failed+' / 跳过 '+r.Result.Skipped;
        $('taskHistoryRoot').textContent=r.Config.Root||'—';$('taskHistoryOutput').textContent=r.Config.Output||'按工具默认规则';
        $('taskHistoryLogs').textContent=(r.Logs||[]).join('\n')||'没有已保存的日志。';
        $('taskHistoryRecoveryHint').textContent=r.Status==='running'?'任务仍在执行，不能重复恢复。可刷新记录查看进展。':r.Limited?'逐项记录已达到上限或保存异常，请载入设置并核对完整预览。':!r.Independent?'此工具只保留执行汇总。载入设置后重新扫描；改名、编号和打包需要检查当前目录，避免重复操作。':data.Remaining?'还有 '+data.Remaining+' 个失败或未完成的文件。重扫会检查当前文件与已有输出，不会直接重放旧操作。':'已完成或已跳过的文件不会再次重扫。';
        if(data.Warning)$('taskHistoryRecoveryHint').textContent=data.Warning;
        if(records.some(child=>child.ParentID===r.ID))$('taskHistoryRecoveryHint').textContent+=' 已有后续重扫记录，请查看较新的记录了解恢复结果。';
        $('taskHistoryItemsControls').hidden=!r.Independent;
        $('taskHistoryItems').replaceChildren();
        for(const item of data.Items||[]) {
          const row=document.createElement('div');row.className='history-file';
          const name=document.createElement('b');name.textContent=item.Item.Source.split(/[\\/]/).pop();name.title=item.Item.Source;
          const status=document.createElement('span');status.textContent=({pending:'未完成',failed:'失败',skipped:'已跳过',success:'成功'})[item.Status]||item.Status;
          const reason=document.createElement('small');reason.textContent=item.Message||item.Item.Conflict||'';row.append(name,status,reason);$('taskHistoryItems').append(row);
        }
        if(r.Independent&&!data.Items?.length)$('taskHistoryItems').textContent='此范围没有文件。';
        $('taskHistoryPage').textContent=total?(offset+1)+'–'+Math.min(offset+100,total)+' / '+total:'0 项';
        $('taskHistoryEmpty').hidden=true;$('taskHistoryDetail').hidden=false;note('');
      } catch(e) {if(request===detailRequest&&$('taskHistoryDialog').open){$('taskHistoryEmpty').textContent='记录无法读取。';note(e.message);}}
      finally {if(request===detailRequest)syncUI();}
    }
    async function open(preferInterrupted=false) {
      beforeOpen();if(!$('taskHistoryDialog').open)$('taskHistoryDialog').showModal();note('正在读取记录…');await refresh(true,preferInterrupted);
    }
    async function apply(retry) {
      if(acting||!detail||!canChange()||detail.Record.Status==='running')return;
      const data=detail;acting=true;syncUI();note('正在载入设置…');
      try {
        await restore(data.Record.Config);
        loaded=data.CanRetry?data.Record:null;
        $('taskHistoryDialog').close();
        if(retry)await scan(data.Record.ID);else message('已载入历史参数。密码需要重新填写；请核对目录与预览后再执行。');
      } catch(e) {note('载入失败：'+e.message);message(e.message,true);}
      finally {acting=false;syncUI(!canChange(),data.Record.Tool,data.Record.Config.Root);}
    }
    $('openTaskHistory').onclick=()=>open();$('viewInterruptedTasks').onclick=()=>open(true);
    $('closeTaskHistory').onclick=()=>$('taskHistoryDialog').close();
    $('taskHistoryDialog').addEventListener('close',()=>{detailRequest++;});
    $('refreshTaskHistory').onclick=()=>refresh(true);
    $('restoreTaskHistory').onclick=()=>apply(false);$('retryTaskHistory').onclick=()=>apply(true);
    $('retryUnfinished').onclick=async()=>{if(!loaded||!canChange())return;acting=true;syncUI();try{await scan(loaded.ID);}finally{acting=false;syncUI(!canChange());}};
    $('taskHistoryFilter').onchange=()=>{offset=0;showDetail();};
    $('taskHistoryPrevious').onclick=()=>{offset=Math.max(0,offset-100);showDetail();};$('taskHistoryNext').onclick=()=>{offset+=100;showDetail();};
    $('exportTaskHistory').onclick=()=>{
      if(!detail)return;const r=detail.Record;
      const text=toolName(r.Tool)+' · '+statusName(r)+'\n'+timeName(r.Started)+'\n\n'+(r.Logs||[]).join('\n');
      const url=URL.createObjectURL(new Blob([text],{type:'text/plain;charset=utf-8'})),a=document.createElement('a');a.href=url;a.download='hamster-task-'+r.ID.slice(0,8)+'.txt';a.click();setTimeout(()=>URL.revokeObjectURL(url),5000);
    };
    $('clearTaskHistory').onclick=async()=>{
      if(acting||!canChange()||!confirm('清理历史记录和保存的日志？正在执行的记录会保留；源文件和处理结果不会删除。'))return;
      acting=true;syncUI();try{await api('task-history-clear');loaded=null;$('retryUnfinished').hidden=true;selected='';await refresh(true);note('历史已清理，资源文件保持原样。');}catch(e){note('清理失败：'+e.message);}finally{acting=false;syncUI();}
    };
    refresh();
    return {syncUI,observe(state){if(loaded&&state.RetryOf===loaded.ID&&state.Status==='finished'&&!state.Result?.Failed){loaded=null;$('retryUnfinished').hidden=true;}const key=state.HistoryID+':'+state.Status;if(state.HistoryID&&['finished','cancelled','error'].includes(state.Status)&&key!==lastFinished){lastFinished=key;refresh($('taskHistoryDialog').open);}}};
  };
})();
