// One completion notice per executed plan. Store IDs only, never paths or task logs.
(() => {
  const el = id => document.getElementById(id);
  const storageKey = 'hamster.toolbox.task-notices.v1';
  let records = [];
  try {
    const stored = JSON.parse(sessionStorage.getItem(storageKey) || '[]');
    if (Array.isArray(stored)) records = stored.filter(r => typeof r?.id === 'string').slice(-64);
  } catch {}
  const prefs = {CloseDialogOnBackdrop:false,TaskPopup:true, TaskSound:true,BrowserAutoPlay:true,BrowserMuted:false,RememberPlayback:false,RestoreTabs:false,DefaultPlayer:"browser",FontScale:100,ThumbnailCacheMB:256};
  window.toolboxSettings=prefs;
  let generalAPI,refreshGeneralSettings;
  window.addEventListener('storage',event=>{if(event.key==='hamster.toolbox.general.refresh')refreshGeneralSettings?.().catch(()=>{})});
  let backdropPress=null,backdropReleased=false;
  function outsideDialog(dialog,event){const r=dialog.getBoundingClientRect();return event.clientX<r.left||event.clientX>r.right||event.clientY<r.top||event.clientY>r.bottom}
  document.addEventListener('pointerdown',event=>{
    backdropPress=null;backdropReleased=false;
    const dialog=event.target;
    if(initialized&&prefs.CloseDialogOnBackdrop&&event.button===0&&dialog instanceof HTMLDialogElement&&dialog.open&&outsideDialog(dialog,event))backdropPress={dialog,pointer:event.pointerId,x:event.clientX,y:event.clientY};
  },true);
  document.addEventListener('pointermove',event=>{if(backdropPress&&event.pointerId===backdropPress.pointer&&Math.hypot(event.clientX-backdropPress.x,event.clientY-backdropPress.y)>8){backdropPress=null;backdropReleased=false}},true);
  document.addEventListener('pointerup',event=>{backdropReleased=Boolean(backdropPress&&event.pointerId===backdropPress.pointer&&event.target===backdropPress.dialog&&outsideDialog(backdropPress.dialog,event))},true);
  document.addEventListener('pointercancel',()=>{backdropPress=null;backdropReleased=false},true);
  document.addEventListener('click',event=>{
    const pressed=backdropPress,ready=backdropReleased;backdropPress=null;backdropReleased=false;
    if(!initialized||!prefs.CloseDialogOnBackdrop||!pressed||!ready||event.target!==pressed.dialog||!pressed.dialog.open||!outsideDialog(pressed.dialog,event))return;
    event.preventDefault();event.stopPropagation();
    if(!pressed.dialog.dispatchEvent(new Event('cancel',{cancelable:true})))return;
    const cancel=pressed.dialog.querySelector('button[id*="close" i],button[id*="cancel" i]');
    if(cancel){if(!cancel.disabled)cancel.click()}else pressed.dialog.close('cancel');
  },true);
  let save, label = () => '工具箱任务', initialized = false, writes = Promise.resolve();
  let context, currentID = '', previousFocus;
  const pending = new Map();
  function record(id) {
    let entry = records.find(r => r.id === id);
    if (!entry) { entry = {id}; records.push(entry); records = records.slice(-64); }
    return entry;
  }
  function persist() { try { sessionStorage.setItem(storageKey, JSON.stringify(records)); } catch {} try{localStorage.setItem(storageKey,JSON.stringify(records))}catch{} }
  const noticeWindow=String(Math.random())+Date.now();
  function synchronizeNotice(id){try{const shared=JSON.parse(localStorage.getItem(storageKey)||'[]');for(const remote of shared){const own=record(remote.id);own.acknowledged ||= remote.acknowledged;own.sounded ||= remote.sounded;}const lock=JSON.parse(localStorage.getItem(storageKey+'.claim.'+id)||'null');if(lock&&lock.owner!==noticeWindow&&lock.expires>Date.now())return false;localStorage.setItem(storageKey+'.claim.'+id,JSON.stringify({owner:noticeWindow,expires:Date.now()+5000}));}catch{}return true;}
  function unlockSound() {
    if (!initialized || !prefs.TaskSound) return;
    try {
      const Audio = window.AudioContext || window.webkitAudioContext;
      if (Audio && !context) context = new Audio();
      if (context?.state === 'suspended') context.resume().catch(() => {});
    } catch {}
  }
  document.addEventListener('pointerdown', unlockSound, {capture:true});
  document.addEventListener('keydown', unlockSound, {capture:true});
  async function chime() {
    try {
      if (!context || context.state !== 'running') return false;
      const start = context.currentTime;
      for (const [frequency, offset] of [[660,0],[880,0.18]]) {
        const oscillator = context.createOscillator(), gain = context.createGain();
        oscillator.type = 'sine'; oscillator.frequency.value = frequency;
        gain.gain.setValueAtTime(0, start + offset);
        gain.gain.linearRampToValueAtTime(0.12, start + offset + 0.02);
        gain.gain.exponentialRampToValueAtTime(0.001, start + offset + 0.28);
        oscillator.connect(gain); gain.connect(context.destination);
        oscillator.onended = () => { oscillator.disconnect(); gain.disconnect(); };
        oscillator.start(start + offset); oscillator.stop(start + offset + 0.3);
      }
      return true;
    } catch { return false; }
  }
  function describe(state) {
    const count = key => Math.max(0, Number(state.Result?.[key]) || 0);
    const success = count('Success'), failed = count('Failed'), skipped = count('Skipped');
    let title = '任务完成', kind = 'success', icon = '✓';
    if (state.Status === 'cancelled') { title = '任务已取消'; kind = 'warning'; icon = '–'; }
    else if (state.Status === 'error' || failed && !success) { title = '任务失败'; kind = 'error'; icon = '!'; }
    else if (failed) { title = '任务完成，部分失败'; kind = 'warning'; icon = '!'; }
    else if (!success && skipped) { title = '任务结束，全部跳过'; kind = 'warning'; icon = '–'; }
    let message = `成功 ${success} 项 · 跳过 ${skipped} 项 · 失败 ${failed} 项`;
    if (state.Status === 'cancelled') message += '\n已完成的处理结果会保留。';
    if (failed || state.Status === 'error') message += '\n请查看运行日志了解原因。';
    if (state.Error) message += '\n' + state.Error;
    return {title, kind, icon, message};
  }
  function displayNext() {
    if (el('taskResultDialog').open || !pending.size) return;
    const [id, notice] = pending.entries().next().value;
    pending.delete(id);
    if (record(id).acknowledged) { displayNext(); return; }
    currentID = id;
    const result = describe(notice.state);
    el('taskResultDialog').dataset.kind = result.kind;
    el('taskResultTitle').textContent = result.title;
    el('taskResultIcon').textContent = result.icon;
    el('taskResultTool').textContent = label(notice.state.Tool);
    el('taskResultMessage').textContent = result.message;
    el('taskSoundStatus').hidden = !notice.soundBlocked;
    el('taskSoundStatus').textContent = '提示音未播放：请先点击页面或在通用设置中试听，并检查浏览器是否静音。';
    previousFocus = document.activeElement;
    el('taskResultDialog').showModal();
  }
  function observe(state) {
    if (!initialized || !state.PlanID || !['finished','cancelled','error'].includes(state.Status)) return;
    const id = state.PlanID;if(!synchronizeNotice(id))return;const entry = record(id);
    if (entry.acknowledged || currentID === id || pending.has(id)) return;
    const notice = {state, soundBlocked:false};
    if (prefs.TaskSound && !entry.sounded) {
      entry.sounded = true;
      notice.soundBlocked = !context || context.state !== 'running';
      chime();
    }
    if (prefs.TaskPopup) { pending.set(id, notice); displayNext(); }
    else entry.acknowledged = true;
    persist();
  }
  el('taskResultClose').onclick = () => el('taskResultDialog').close();
  el('taskResultDialog').addEventListener('close', () => {
    if (currentID) record(currentID).acknowledged = true;
    currentID = ''; persist(); previousFocus?.focus(); displayNext();
  });
  let startupRevision=0,startupBusy=false,startupEnabled=false;
  function showStartup(state){startupEnabled=Boolean(state.Enabled);el('AutoStartEnabled').checked=startupEnabled;el('autostartOption').hidden=!state.Supported;el('AutoStartEnabled').disabled=startupBusy||!state.Supported||!state.CanEnable&&!startupEnabled;el('autostartStatus').textContent=state.Message||'仅当前用户登录后在系统托盘运行。'}
  async function refreshStartup(){
    if(startupBusy)return;const revision=++startupRevision;el('AutoStartEnabled').disabled=true;el('autostartStatus').textContent='正在读取自启状态…';
    try{if(!generalAPI)throw Error('请从本机工具箱打开设置');const state=await generalAPI('autostart');if(revision===startupRevision&&el('generalSettingsDialog').open)showStartup(state)}
    catch(error){if(revision===startupRevision&&el('generalSettingsDialog').open){el('AutoStartEnabled').disabled=true;el('autostartStatus').textContent='读取失败：'+error.message}}
  }
  el('AutoStartEnabled').onchange=async()=>{
    if(startupBusy)return;const enabled=el('AutoStartEnabled').checked,revision=++startupRevision;startupBusy=true;el('AutoStartEnabled').disabled=true;el('autostartStatus').textContent='正在保存自启设置…';
    try{const state=await generalAPI('autostart',{Enabled:enabled});startupBusy=false;if(revision===startupRevision&&el('generalSettingsDialog').open){showStartup(state);el('autostartStatus').textContent=(state.Enabled?'已开启。':'已关闭。')+(state.Message||'')}}
    catch(error){startupBusy=false;if(revision===startupRevision&&el('generalSettingsDialog').open){el('AutoStartEnabled').checked=startupEnabled;el('AutoStartEnabled').disabled=false;el('autostartStatus').textContent='保存失败：'+error.message}}
    finally{startupBusy=false;if(revision!==startupRevision&&el('generalSettingsDialog').open&&el('generalTabStartup').getAttribute('aria-selected')==='true')refreshStartup()}
  };
  const generalTabs=[['generalTabPlayback','generalPlayback'],['generalTabAppearance','generalAppearance'],['generalTabTask','generalTask'],['generalTabStartup','generalStartup']];
  const selectGeneralTab=index=>{generalTabs.forEach(([button,panel],i)=>{el(button).setAttribute('aria-selected',String(index===i));el(button).tabIndex=index===i?0:-1;el(panel).hidden=index!==i});if(index===3)refreshStartup()};
  generalTabs.forEach(([button],index)=>{el(button).onclick=()=>selectGeneralTab(index);el(button).onkeydown=event=>{let next;if(event.key==='ArrowRight')next=(index+1)%generalTabs.length;if(event.key==='ArrowLeft')next=(index+generalTabs.length-1)%generalTabs.length;if(event.key==='Home')next=0;if(event.key==='End')next=generalTabs.length-1;if(next!==undefined){event.preventDefault();selectGeneralTab(next);el(generalTabs[next][0]).focus()}}});
  el('openGeneralSettings').onclick = () => {
    el('generalSettingsStatus').textContent = '';
    el('generalSettingsDialog').showModal();
    if(el('generalTabStartup').getAttribute('aria-selected')==='true')refreshStartup();
  };
  el('closeGeneralSettings').onclick = () => el('generalSettingsDialog').close();
  el('generalSettingsDialog').addEventListener('close', () => {startupRevision++;el('openGeneralSettings').focus()});
  for (const key of Object.keys(prefs)) el(key).onchange = () => {
    const value = el(key).type==='checkbox'?el(key).checked:el(key).type==='number'||key==='FontScale'?Number(el(key).value):el(key).value;
    el('generalSettingsStatus').textContent = '正在保存…';
    const input = el(key); input.disabled = true;
    writes = writes.catch(() => {}).then(async () => {
      try {
        await save({[key]:value}); prefs[key] = value; applyGeneralStyle();try{localStorage.setItem('hamster.toolbox.general.refresh',String(Date.now()))}catch{}if(typeof persistTabs==='function')persistTabs();
        el('generalSettingsStatus').textContent = '已自动保存';
      } catch (error) {
        if(input.type==='checkbox')input.checked=prefs[key];else input.value=prefs[key];
        el('generalSettingsStatus').textContent = '保存失败：' + error.message;
      } finally { input.disabled = false; }
    });
  };
  el('testTaskSound').onclick = async () => {
    // A deliberate preview remains available even when automatic sound is disabled.
    const enabled = prefs.TaskSound; prefs.TaskSound = true; unlockSound(); prefs.TaskSound = enabled;
    try { if (context?.state === 'suspended') await context.resume(); } catch {}
    el('generalSettingsStatus').textContent = await chime() ? '已播放提示音；没有听到时，请检查系统音量和浏览器静音设置。' : '浏览器暂时无法播放声音，请检查声音权限。';
  };
  function applyGeneralStyle(){document.body.dataset.fontScale=String(prefs.FontScale||100);document.documentElement.style.setProperty('--user-font-scale',String((prefs.FontScale||100)/100));}
  el('clearThumbnailCache').onclick=async()=>{el('clearThumbnailCache').disabled=true;try{if(!generalAPI)throw Error('请从资源管理器打开设置后清理');await generalAPI('cache-clear');el('generalSettingsStatus').textContent='缩略图缓存已清空';}catch(e){el('generalSettingsStatus').textContent='清理失败：'+e.message}finally{el('clearThumbnailCache').disabled=false}};
  window.toolboxFeedback = {
    init(settings, persistSettings, toolLabel,requestAPI) {
      if(requestAPI){generalAPI=requestAPI;refreshGeneralSettings=async()=>window.toolboxFeedback.init(await requestAPI('settings'),persistSettings,toolLabel,requestAPI);}
      for(const key of Object.keys(prefs)){if(settings[key]!=null && settings[key]!=='' && settings[key]!==0 || settings[key]===false || key==='ThumbnailCacheMB'&&settings[key]===0)prefs[key]=settings[key];if(el(key).type==='checkbox')el(key).checked=prefs[key];else el(key).value=prefs[key];}applyGeneralStyle();
      save = persistSettings; label = toolLabel; initialized = true;
    },
    async connect(toolboxURL){
      const session=new URL(toolboxURL,location.href).searchParams.get('session');
      const request=async(path,data={})=>{const response=await fetch('/api/'+path,{method:'POST',headers:{'Content-Type':'application/json','X-Toolbox-Token':session},body:JSON.stringify(data)});if(!response.ok)throw Error((await response.text()).trim());return response.json()};
      generalAPI=async route=>{const response=await fetch('/files/api/'+route,{method:'POST',headers:{'Content-Type':'application/json','X-Hamster-Browser':'1'},body:'{}'});if(!response.ok)throw Error((await response.text()).trim());return response.json()};
      const settings=await request('settings');window.toolboxFeedback.init(settings,change=>request('ui-settings',change),id=>({format:'目录统计命名',rename:'批量重命名',number:'目录批量编号',describe:'媒体信息命名',exif:'清理元数据',pdf:'PDF 图册索引',image:'压缩图片',video:'压缩视频',thumbnail:'视频缩略图',archive:'打包资源',extract:'解压资源'})[id]||'工具箱任务',request);
      generalAPI=request;el('openGeneralSettings').hidden=false;
      if(window.toolboxVersion)request('info').then(info=>window.toolboxVersion.init(info,request)).catch(()=>{});
      const poll=async()=>{try{observe(await request('state',{Summary:true}));}catch{}};await poll();setInterval(poll,1500);
    },
    observe
  };
})();
