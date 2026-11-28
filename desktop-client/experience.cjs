'use strict';
const path=require('node:path'),policy=require('./policy.cjs');
const defaults={closeAction:'ask',zoom:100,restoreWorkspace:true,notifications:true,backgroundSaving:true};
const toolNames={format:'目录统计命名',rename:'批量重命名',number:'目录批量编号',describe:'媒体信息命名',exif:'清理元数据',pdf:'PDF 图册索引',image:'压缩图片',video:'压缩视频',thumbnail:'视频缩略图',archive:'打包资源',extract:'解压资源'};
function sanitizeSettings(value={}){
 return{closeAction:['ask','tray','exit'].includes(value.closeAction)?value.closeAction:'ask',zoom:[75,80,90,100,110,125,150,175,200].includes(value.zoom)?value.zoom:100,...Object.fromEntries(['restoreWorkspace','notifications','backgroundSaving'].map(key=>[key,typeof value[key]==='boolean'?value[key]:defaults[key]]))};
}
function cleanWorkspace(value){
 const validPath=p=>typeof p==='string'&&p.length>0&&p.length<=32768&&!/[\x00-\x1f]/.test(p)&&path.isAbsolute(p);
 const result={};if(!value||typeof value!=='object')return result;
 if(Array.isArray(value.editorPaths))result.editorPaths=[...new Set(value.editorPaths.filter(validPath))].slice(0,100);
 if(validPath(value.activeEditorPath))result.activeEditorPath=value.activeEditorPath;
 if(typeof value.editorOpen==='boolean')result.editorOpen=value.editorOpen;
 // Directory state contains navigation only, no file contents or credentials.
 if(value.tabs&&Array.isArray(value.tabs.tabs)){
  const tabs=value.tabs.tabs.filter(t=>t&&validPath(t.path)&&t.root==='').slice(0,20).map(t=>({id:typeof t.id==='string'?t.id.slice(0,80):'',root:'',path:t.path,search:typeof t.search==='string'?t.search.slice(0,256):'',sort:['name','size','modified'].includes(t.sort)?t.sort:'name',order:t.order==='desc'?'desc':'asc',view:t.view==='list'?'list':'grid',offset:Math.max(0,Math.min(10000000,Number(t.offset)||0)),scroll:Math.max(0,Math.min(20000000,Number(t.scroll)||0))}));
  result.tabs={version:1,active:typeof value.tabs.active==='string'?value.tabs.active.slice(0,80):'',tabs};
 }
 return result;
}
function route(address,origin){
 if(!policy.trustedPage(address,origin))return null;const u=new URL(address);if(u.searchParams.get('desktop-media')==='1')return null;const entry={pathname:u.pathname};
 if(toolNames[u.searchParams.get('tool')])entry.tool=u.searchParams.get('tool');return entry;
}
function createExperience({app,BrowserWindow,ipcMain,Notification,dialog,screen,fs,profile,launch,request,focus,open,timers=globalThis}){
 const {setTimeout,clearTimeout}=timers;
 const read=(name,fallback)=>{try{const text=fs.readFileSync(path.join(profile,name),'utf8');return text.length<=2*1024*1024?JSON.parse(text):fallback}catch{return fallback}};
 const write=(name,value)=>{fs.mkdirSync(profile,{recursive:true});const dest=path.join(profile,name),temp=dest+'.tmp';fs.writeFileSync(temp,JSON.stringify(value));fs.renameSync(temp,dest)};
 let settings=sanitizeSettings(read('desktop-settings.json',{})),saved=read('workspace.json',[]),frozen=false,timer,pollTimer,polling=false,stopped=false,bootstrapped=false,lastSound=true;
 const workspace=new WeakMap(),completed=new Set(),notes=new Set();
 const windows=()=>BrowserWindow.getAllWindows().filter(w=>!w.isDestroyed());
 const notifyState=()=>{for(const win of windows())win.webContents.send?.('hamster:desktop-state',{...settings})};
 const command=(win,value)=>{if(!win.isDestroyed())win.webContents.send?.('hamster:desktop-command',value)};
 function snapshot(){if(frozen||!settings.restoreWorkspace)return;const entries=windows().map(win=>{const entry=route(win.webContents.getURL(),launch().origin);if(!entry)return null;return{...entry,bounds:{...win.getNormalBounds(),maximized:win.isMaximized()},state:cleanWorkspace(workspace.get(win))}}).filter(Boolean);if(entries.length){saved=entries;write('workspace.json',entries)}}
 function scheduleSave(){clearTimeout(timer);timer=setTimeout(()=>{try{snapshot()}catch{}},500);timer.unref?.()}
 function applyZoom(){for(const win of windows())win.webContents.setZoomFactor?.(settings.zoom/100)}
 function saveSettings(change){
  if(!change||typeof change!=='object'||Array.isArray(change)||Object.keys(change).some(k=>!Object.keys(defaults).includes(k)))throw Error('桌面设置无效');
  const next=sanitizeSettings({...settings,...change});for(const key of Object.keys(change))if(next[key]!==change[key])throw Error('桌面设置值无效');
  write('desktop-settings.json',next);settings=next;
  if(!settings.restoreWorkspace){saved=[];write('workspace.json',[])}else{frozen=false;scheduleSave()}
  applyZoom();notifyState();return{...settings};
 }
 function progress(win){
  if(win.isDestroyed())return;const downloading=[...(win._toolboxDownloads?.values()||[])];let done=0,total=0,unknown=false;
  for(const item of downloading){done+=item.received||0;total+=item.total||0;unknown||=!item.total}
  const task=win._toolboxTaskProgress;
  if(task!==undefined&&task!==-1){if(task===2)unknown=true;else{done+=task*100;total+=100}}
  win.setProgressBar?.(unknown?2:total?Math.min(1,done/total):-1);
 }
 async function poll(){
  if(stopped||polling)return;clearTimeout(pollTimer);polling=true;
  try{
   const state=await request('/api/state',{Summary:true});if(stopped)return;
   const tasks=[...new Map([state,...(state.Tasks||[]),...(state.Completed||[])].filter(t=>t.PlanID||t.TaskID).map(t=>[t.TaskID||t.PlanID,t])).values()];
   const running=tasks.filter(t=>['running','scanning'].includes(t.Status));let total=0,done=0,unknown=false;
   for(const task of running){if(task.Total>0){total+=task.Total;const fraction=task.Operation?.PercentKnown?Math.max(0,Math.min(0.99,task.Operation.Percent/100)):0;done+=Math.max(0,Math.min((task.Done||0)+fraction,task.Total))}else unknown=true}
   for(const win of windows()){win._toolboxTaskProgress=running.length?(unknown?2:total?done/total:2):-1;progress(win)}
   const ended=tasks.filter(t=>['finished','error','cancelled'].includes(t.Status)&&!completed.has(t.TaskID||t.PlanID));
   for(const task of ended)completed.add(task.TaskID||task.PlanID);
   if(completed.size>1024){const kept=[...completed].slice(-512);completed.clear();for(const id of kept)completed.add(id)}
   const fresh=bootstrapped?ended:[];bootstrapped=true;
   if(fresh.length&&settings.notifications&&Notification?.isSupported?.()&&!windows().some(w=>w.isFocused?.()&&w.isVisible?.()&&!w.isMinimized())){
    try{lastSound=(await request('/api/settings',{})).TaskSound!==false}catch{}
    if(stopped)return;
    const failed=fresh.some(t=>t.Status==='error'||t.Result?.Failed),single=fresh.length===1?fresh[0]:null;
    const title=failed?'工具箱任务完成，请检查结果':'工具箱任务已完成';
    const body=single?(toolNames[single.Tool]||'文件处理')+' · 成功 '+(single.Result?.Success||0)+' 项，失败 '+(single.Result?.Failed||0)+' 项':fresh.length+' 个任务已结束，点击查看处理结果。';
    const notice=new Notification({title,body,silent:!lastSound,icon:path.join(__dirname,'favicon.ico')});notes.add(notice);
    notice.once('click',()=>{const u=new URL('/',launch().url);u.searchParams.set('session',new URL(launch().url).searchParams.get('session'));if(single?.Tool&&toolNames[single.Tool])u.searchParams.set('tool',single.Tool);open(u.href)});
    for(const event of ['close','failed'])notice.once(event,()=>notes.delete(notice));notice.show();
   }
  }catch{for(const win of windows()){win._toolboxTaskProgress=-1;progress(win)}}
  finally{polling=false;if(!stopped){pollTimer=setTimeout(poll,windows().some(w=>w.isVisible?.()&&!w.isMinimized())?2000:4000);pollTimer.unref?.()}}
 }
 const senderWindow=event=>{
  const win=BrowserWindow.fromWebContents(event.sender);
  if(!win||win.isDestroyed()||event.senderFrame!==event.sender.mainFrame||!policy.trustedPage(event.senderFrame?.url,launch().origin))throw Error('此页面不能管理桌面功能');return win;
 };
 if(ipcMain){
  ipcMain.on('hamster:desktop-bootstrap',event=>{try{const win=senderWindow(event);event.returnValue={settings:{...settings},workspace:settings.restoreWorkspace?workspace.get(win)||{}:{}}}catch{event.returnValue={}}});
  ipcMain.handle('hamster:desktop',async(event,action,value)=>{
   const win=senderWindow(event);
   switch(action){
    case 'settings':return{...settings};
    case 'settings-save':return saveSettings(value);
    case 'zoom':{let zoom=typeof value==='number'?value:settings.zoom;if(value==='in')zoom=[75,80,90,100,110,125,150,175,200].find(n=>n>settings.zoom)||200;if(value==='out')zoom=[200,175,150,125,110,100,90,80,75].find(n=>n<settings.zoom)||75;if(value==='reset')zoom=100;return saveSettings({zoom})}
    case 'find':if(typeof value?.text!=='string'||!value.text.trim()||value.text.length>2000)throw Error('请输入查找文字');win.webContents.findInPage(value.text,{forward:value.forward!==false,findNext:true});return true;
    case 'find-stop':win.webContents.stopFindInPage('clearSelection');return true;
    case 'workspace':{const clean=cleanWorkspace(value);workspace.set(win,clean);scheduleSave();return true}
    case 'workspace-get':return settings.restoreWorkspace?workspace.get(win)||{}:{};
    case 'drop-paths':{
     if(!Array.isArray(value)||!value.length||value.length>1000)throw Error('请拖入文件或文件夹（单次最多 1000 项）');
     const items=[];
     for(const p of value){if(typeof p!=='string'||!path.isAbsolute(p)||p.length>32768||/[\x00-\x1f]/.test(p))throw Error('拖入路径无效');const st=await fs.promises.stat(p);if(!st.isDirectory()&&!st.isFile())throw Error('仅支持普通文件和目录');items.push({Path:p,Name:path.basename(p),Dir:st.isDirectory()})}
     return items;
    }
    default:throw Error('桌面操作无效');
   }
  });
 }
 return{
  get settings(){return settings},saveSettings,progress,command,
  attach(win){
   win.webContents.on('did-finish-load',()=>{win.webContents.setZoomFactor?.(settings.zoom/100);scheduleSave()});
   win.on('move',scheduleSave);win.on('resize',scheduleSave);win.on('closed',()=>{if(!frozen)scheduleSave()});
   const background=()=>{command(win,{action:'background',saving:settings.backgroundSaving});try{snapshot()}catch{}};
   win.on('hide',background);win.on('minimize',background);
   win.on('show',()=>command(win,{action:'foreground'}));win.on('restore',()=>command(win,{action:'foreground'}));
   win.webContents.on('found-in-page',(_event,result)=>command(win,{action:'find-result',matches:result.matches,activeMatchOrdinal:result.activeMatchOrdinal}));
  },
  bounds(state){return policy.displayBounds(state,screen.getAllDisplays?.().map(d=>d.workArea)||[screen.getPrimaryDisplay().workArea])},
  restoreEntries(){if(!settings.restoreWorkspace||!Array.isArray(saved))return[];if(new URL(launch().url).pathname!=='/'||new URL(launch().url).searchParams.has('tool'))return[];return saved.slice(0,32).flatMap(e=>{try{if(!e||typeof e.pathname!=='string')return[];const url=new URL(e.pathname,launch().url);if(!policy.trustedPage(url.href,launch().origin)||url.search||url.hash)return[];url.searchParams.set('session',new URL(launch().url).searchParams.get('session'));if(toolNames[e.tool])url.searchParams.set('tool',e.tool);return[{url:url.href,bounds:e.bounds&&typeof e.bounds==='object'?e.bounds:{},state:cleanWorkspace(e.state)}]}catch{return[]}})},
  seed(win,state){workspace.set(win,cleanWorkspace(state))},
  async captureAll(){
   if(!settings.restoreWorkspace)return;
   await Promise.all(windows().map(async win=>{if(!policy.trustedPage(win.webContents.getURL(),launch().origin)||!win.webContents.executeJavaScript)return;let timeout;try{const state=await Promise.race([win.webContents.executeJavaScript('window.toolboxDesktopWorkspace ? window.toolboxDesktopWorkspace.capture() : null'),new Promise(resolve=>{timeout=setTimeout(()=>resolve(null),1500)})]);if(state&&!win.isDestroyed())workspace.set(win,cleanWorkspace(state))}catch{}finally{clearTimeout(timeout)}}));
  },
  freeze(){try{snapshot()}catch{}frozen=true;clearTimeout(timer)},
  unfreeze(){frozen=false},
  start(){return ipcMain?poll():Promise.resolve()},
  stop(){stopped=true;clearTimeout(timer);clearTimeout(pollTimer);for(const notice of notes)notice.close();notes.clear()}
 };
}
module.exports={defaults,sanitizeSettings,cleanWorkspace,route,createExperience};
