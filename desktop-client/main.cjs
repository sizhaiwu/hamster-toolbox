'use strict';
const {app,BrowserWindow,Menu,dialog,shell,session,screen,Tray,ipcMain,Notification}=require('electron');
const path=require('node:path'),fs=require('node:fs'),http=require('node:http'),policy=require('./policy.cjs');
let launch,main,ready=false,tray,quitting=false,exiting=false,closePrompt=false,exitWithoutBackend=false;
const allowedClose=new WeakSet();
const scopes=new WeakMap(),managed=new WeakSet(),backgroundWindows=new WeakSet();
try{launch=policy.launchArguments(process.argv);if(!path.isAbsolute(launch.profile))throw Error('客户端配置路径无效。');app.setPath('userData',launch.profile)}catch(e){dialog.showErrorBox('仓鼠工具箱',e.message);app.exit(1)}
if(launch){
 app.commandLine.appendSwitch('lang','zh-CN');app.enableSandbox();
 if(!app.requestSingleInstanceLock())app.quit();else{
 const originFor=contents=>scopes.get(contents)||launch.origin;
 const focus=win=>{if(win&&!win.isDestroyed()){if(win.isMinimized())win.restore();win.show();win.focus()}};
 const external=url=>{if(policy.externalAllowed(url))shell.openExternal(url).catch(()=>{})};
 const stateFile=path.join(launch.profile,'window.json');
 function request(route,payload){return new Promise((resolve,reject)=>{const u=new URL(route,launch.url),token=new URL(launch.url).searchParams.get('session'),body=JSON.stringify(payload);const req=http.request(u,{method:'POST',headers:{'Content-Type':'application/json','X-Toolbox-Token':token,'Content-Length':Buffer.byteLength(body)}},res=>{let data='';res.setEncoding('utf8');res.on('data',part=>{data+=part;if(data.length>2*1024*1024)req.destroy(Error('服务响应过大'))});res.on('end',()=>{if(res.statusCode!==200){reject(Error(data.trim()||'服务请求失败'));return}try{resolve(JSON.parse(data))}catch{reject(Error('服务响应无效'))}})});req.setTimeout(5000,()=>req.destroy(Error('无法连接工具箱，请检查启动终端')));req.on('error',reject);req.end(body)})}
 const backend=action=>request('/api/desktop-client',{Action:action});
 const experience=require('./experience.cjs').createExperience({app,BrowserWindow,ipcMain,Notification,dialog,screen,fs,profile:launch.profile,launch:()=>launch,request,focus,open:address=>open({...launch,url:address})});
 const mediaDesktop=require('./media.cjs').createMediaDesktop({BrowserWindow,ipcMain,fs,profile:launch.profile,launch:()=>launch,screen,create:(address,kind,bounds)=>create(address,false,{bounds,state:{},media:kind})});
 function restore(){const win=main&&!main.isDestroyed()?main:BrowserWindow.getAllWindows().find(w=>!w.isDestroyed());if(win)focus(win);else create(launch.url,true)}
 function ensureTray(){
  if(launch.backendTray||tray)return;tray=new Tray(path.join(__dirname,'favicon.ico'));tray.setToolTip('仓鼠工具箱 · 点击重新打开');tray.on('click',restore);tray.on('double-click',restore);
  const entry=(label,destination)=>({label,click:()=>open({...launch,url:policy.trayAddress(launch.url,destination)})});
  tray.setContextMenu(Menu.buildFromTemplate([
   entry('打开工具箱','toolbox'),
   {label:'在浏览器中打开',click:()=>backend('browser').catch(error=>dialog.showMessageBox({type:'error',message:'无法在浏览器中打开工具箱：'+error.message}).catch(()=>{}))},
   {type:'separator'},entry('资源管理器','files'),entry('私密笔记','notes'),entry('计划任务','schedules'),entry('通用设置','settings'),
   {type:'separator'},{label:'退出工具箱',click:()=>requestExit()}
  ]));
 }
 // A refused loopback connection means the service is gone. A timeout or a
 // failed HTTP response must still respect its task/authorization checks.
 const backendStopped=error=>error?.code==='ECONNREFUSED';
 async function requestExit(){
  if(exiting||quitting)return;exiting=true;exitWithoutBackend=false;
  try{
   let state;try{state=await backend('exit-check')}catch(error){if(!backendStopped(error))throw error;exitWithoutBackend=true}
   if(state&&!state.CanExit)throw Error('任务仍在执行，请等待完成或取消。也可以先退出到系统托盘。');
   await experience.captureAll();experience.freeze();const windows=BrowserWindow.getAllWindows().filter(w=>!w.isDestroyed());
   if(!windows.length){await finishExit();return}
   for(const win of windows){if(!exiting)break;allowedClose.add(win);win.close()}
  }catch(error){exiting=false;exitWithoutBackend=false;experience.unfreeze();restore();dialog.showMessageBox({type:'info',message:error.message}).catch(()=>{})}
 }
 async function finishExit(){
  try{
   if(!exitWithoutBackend)try{await backend('exit')}catch(error){if(!backendStopped(error))throw error}
   quitting=true;tray?.destroy();tray=null;app.quit();
  }catch(error){exiting=false;exitWithoutBackend=false;experience.unfreeze();restore();dialog.showMessageBox({type:'info',message:error.message}).catch(()=>{})}
 }

 function attach(win,origin){
  scopes.set(win.webContents,origin);if(managed.has(win))return;managed.add(win);
  experience.attach(win);
  mediaDesktop.attach(win);
  const background=async()=>{
   if(backgroundWindows.has(win)||exiting||quitting)return;backgroundWindows.add(win);
   try{
    try{ensureTray()}catch{if(!win.isDestroyed())await dialog.showMessageBox(win,{type:'error',message:'无法创建系统托盘，请保留窗口或选择退出工具箱。'});return}
    try{await backend('background');if(!win.isDestroyed()&&!exiting&&!quitting)win.hide()}
    catch(error){
     if(win.isDestroyed()||exiting||quitting)return;
     if(backendStopped(error)){
      const {response}=await dialog.showMessageBox(win,{type:'info',title:'后台服务已停止',message:'工具箱后台服务已停止，无法继续在托盘运行。',detail:'可关闭桌面端，或保留当前窗口。关闭时仍会检查未保存内容；需要继续使用，请重新启动工具箱。',buttons:['关闭桌面端','保留窗口'],defaultId:0,cancelId:1,noLink:true});
      if(response===0)await requestExit();
     }else await dialog.showMessageBox(win,{type:'error',message:'无法转入后台：'+error.message});
    }
   }finally{backgroundWindows.delete(win)}
  };
  win.on('close',event=>{if(quitting||allowedClose.has(win)){allowedClose.delete(win);return}if(mediaDesktop.isMediaWindow(win)){mediaDesktop.rememberWindow(win);return}event.preventDefault();if(closePrompt||exiting)return;if(experience.settings.closeAction==='tray'){background();return}if(experience.settings.closeAction==='exit'){requestExit();return}closePrompt=true;dialog.showMessageBox(win,{type:'question',title:'关闭仓鼠工具箱',message:'关闭后希望如何处理？',detail:'退出到托盘会保留页面、播放和未保存内容，后台任务继续。退出工具箱会停止共享与网页播放。',buttons:['退出到系统托盘','退出工具箱','取消'],checkboxLabel:'记住选择（可在通用设置中修改）',defaultId:0,cancelId:2,noLink:true}).then(async({response,checkboxChecked})=>{if(win.isDestroyed())return;if(checkboxChecked&&response<2)experience.saveSettings({closeAction:response===0?'tray':'exit'});if(response===0)await background();else if(response===1)requestExit()}).catch(error=>{if(!win.isDestroyed())dialog.showMessageBox(win,{type:'error',message:'未能保存关闭设置：'+error.message})}).finally(()=>{closePrompt=false})});
  win.webContents.on('will-prevent-unload',event=>{if(!win.isDestroyed()&&dialog.showMessageBoxSync(win,{type:'question',buttons:['继续编辑','放弃未保存内容'],defaultId:0,cancelId:0,message:'页面有尚未保存的内容，确定离开吗？'})===1)event.preventDefault();else{exiting=false;experience.unfreeze();allowedClose.delete(win)}});
  let recovering=false;
  win.webContents.on('render-process-gone',()=>{if(win.isDestroyed()||recovering)return;recovering=true;dialog.showMessageBox(win,{type:'error',message:'页面进程已退出，后台任务仍在工具箱服务中运行。',buttons:['重新加载','关闭窗口'],defaultId:0,cancelId:1}).then(({response})=>{if(!win.isDestroyed()){if(response===0)win.reload();else win.close()}}).catch(()=>{}).finally(()=>{recovering=false})});
  win.webContents.on('before-input-event',(event,input)=>{if(input.type!=='keyDown')return;const modifier=input.control||input.meta,key=input.key.toLowerCase();if(key==='f11'){event.preventDefault();win.setFullScreen(!win.isFullScreen());return}if(key==='f5'||modifier&&key==='r'){event.preventDefault();win.webContents.reload();return}if(modifier&&['+','=','-','0'].includes(key)){event.preventDefault();experience.saveSettings({zoom:key==='0'?100:key==='-'?Math.max(75,[75,80,90,100,110,125,150,175,200].filter(n=>n<experience.settings.zoom).at(-1)||75):[75,80,90,100,110,125,150,175,200].find(n=>n>experience.settings.zoom)||200});return}if(modifier&&key==='f'){event.preventDefault();experience.command(win,{action:'find'});return}if(key==='f1'){event.preventDefault();experience.command(win,{action:'shortcuts'})}});
 }
 function create(address,primary=false,restored){
  const origin=new URL(address).origin;let state=restored?.bounds||{};if(primary&&!restored)try{state=JSON.parse(fs.readFileSync(stateFile,'utf8'))}catch{}
  const bounds=experience.bounds(state);
  const win=new BrowserWindow({...bounds,minWidth:Math.min(390,bounds.width),minHeight:Math.min(400,bounds.height),show:false,title:'仓鼠工具箱',icon:path.join(__dirname,'favicon.ico'),autoHideMenuBar:true,backgroundColor:restored?.media?'#090e14':'#f5f7fa',webPreferences:{...policy.webPreferences(),preload:path.join(__dirname,'preload.cjs')}});win._toolboxAddress=address;win._toolboxMedia=restored?.media||false;attach(win,origin);if(primary)main=win;if(restored)experience.seed(win,restored.state);
  // The first paint may still contain the shared Explorer shell. Media windows
  // wait until the renderer has painted its standalone preview/loading view.
  let painted=false,shown=false,previewTimeout;
  const reveal=()=>{if(shown||!painted||win.isDestroyed()||win._toolboxMedia&&!win._toolboxMediaReady)return;shown=true;if(previewTimeout)clearTimeout(previewTimeout);if(state.maximized===true)win.maximize();focus(win)};
  if(win._toolboxMedia){
   win.on('toolbox-media-ready',reveal);
   previewTimeout=setTimeout(()=>{if(shown||win.isDestroyed())return;win.close();dialog.showMessageBox({type:'error',title:'仓鼠预览',message:'预览页面未能加载，请重新打开文件。如果仍然失败，请重启工具箱。'}).catch(()=>{})},20000);
   win.once('closed',()=>clearTimeout(previewTimeout));
  }
  win.once('ready-to-show',()=>{painted=true;reveal()});
  if(primary)win.on('close',()=>{try{const b=win.getNormalBounds();fs.mkdirSync(launch.profile,{recursive:true});fs.writeFileSync(stateFile,JSON.stringify({...b,maximized:win.isMaximized()}))}catch{}});
  win.loadURL(address).catch(()=>{if(win.isDestroyed())return;focus(win);dialog.showMessageBox(win,{type:'error',message:'无法连接本机工具箱。请从托盘重新打开；后台任务状态由工具箱服务保存。',buttons:['重试','关闭窗口'],defaultId:0,cancelId:1}).then(({response})=>{if(!win.isDestroyed()){if(response===0)win.loadURL(address).catch(()=>{});else win.close()}})});return win;
 }
 function open(next){
  if(!ready){launch=next;return}
  const settings=new URL(next.url).hash==='#settings',entry=address=>{if(!settings||!address)return address||'';const u=new URL(address);u.hash='';return u.href};
  const match=BrowserWindow.getAllWindows().find(win=>!win.isDestroyed()&&policy.samePage(entry(win.webContents.getURL()||win._toolboxAddress),entry(next.url)));
  if(match){focus(match);if(settings){const show=()=>{if(!match.isDestroyed())experience.command(match,{action:'settings'})};if(match.webContents.isLoadingMainFrame?.())match.webContents.once('did-finish-load',show);else show()}return}
  // A new entry gets its own window: existing editors and notes keep their drafts.
  create(next.url,!main||main.isDestroyed());
 }
 app.on('web-contents-created',(_event,contents)=>{
  contents.on('will-attach-webview',event=>event.preventDefault());
  contents.on('will-navigate',(event,url)=>{if(!policy.navigationAllowed(url,originFor(contents))){event.preventDefault();external(url)}});
  contents.on('will-redirect',(event,url)=>{if(!policy.navigationAllowed(url,originFor(contents)))event.preventDefault()});
  contents.setWindowOpenHandler(({url})=>{if(policy.navigationAllowed(url,originFor(contents)))return {action:'allow',overrideBrowserWindowOptions:{...experience.bounds({}),autoHideMenuBar:true,webPreferences:{...policy.webPreferences(),preload:path.join(__dirname,'preload.cjs')}}};external(url);return {action:'deny'}});
  contents.on('did-create-window',win=>attach(win,originFor(contents)));
 });
 app.on('second-instance',(_event,args)=>{try{const next=policy.launchArguments(args);if(next.profile===launch.profile){launch=next;if(next.backendTray&&tray){tray.destroy();tray=null}open(next)}}catch{}});
 app.whenReady().then(()=>{
  Menu.setApplicationMenu(null);const ses=session.defaultSession;
  ses.setPermissionRequestHandler((contents,p,callback,details)=>callback(policy.permissionAllowed(p,details?.requestingUrl||contents.getURL(),originFor(contents))));
  ses.setPermissionCheckHandler((contents,p,origin)=>Boolean(contents)&&policy.permissionAllowed(p,origin||contents.getURL(),originFor(contents)));
  ses.on('will-download',(_event,item,contents)=>{
   const win=BrowserWindow.fromWebContents(contents);if(!win)return;let finished=false;const downloads=win._toolboxDownloads||(win._toolboxDownloads=new Map());downloads.set(item,{});
   const update=()=>experience.progress(win);
   item.on('updated',()=>{if(finished)return;downloads.set(item,{received:item.getReceivedBytes(),total:item.getTotalBytes()});update()});
   item.once('done',(_event,state)=>{finished=true;downloads.delete(item);update();if(state!=='completed'&&state!=='cancelled'&&!win.isDestroyed())dialog.showMessageBox(win,{type:'warning',message:'文件下载未完成，请检查磁盘空间和连接后重试。',detail:item.getFilename()})});update();
  });
  app.setAppUserModelId?.('HamsterToolbox');ready=true;const entries=experience.restoreEntries();if(entries.length){entries.forEach((entry,index)=>create(entry.url,index===0,entry));if(new URL(launch.url).pathname!=='/'||new URL(launch.url).hash==='#settings')open(launch)}else create(launch.url,true);experience.start();
 });
 app.on('window-all-closed',()=>{if(exiting){finishExit();return}if(!quitting){try{ensureTray()}catch{app.quit()}}});
 app.on('before-quit',()=>{quitting=true;experience.stop();tray?.destroy()});
 }
}
