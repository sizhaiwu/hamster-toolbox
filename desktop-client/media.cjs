'use strict';
const path=require('node:path'),crypto=require('node:crypto'),policy=require('./policy.cjs');
const defaults={remember:false,focusAudio:false};
function cleanPreferences(value={}){return Object.fromEntries(Object.keys(defaults).map(key=>[key,typeof value[key]==='boolean'?value[key]:defaults[key]]))}
function cleanRecord(value){
 if(!value||typeof value!=='object')return null;const result={};
 for(const [key,min,max]of [['time',0,7*86400],['volume',0,1],['speed',0.25,4],['zoom',0.01,8],['x',-100000,100000],['y',-100000,100000]])if(typeof value[key]==='number'&&Number.isFinite(value[key]))result[key]=Math.max(min,Math.min(max,value[key]));
 if([0,90,180,270].includes(value.rotation))result.rotation=value.rotation;
 return result;
}
const isMediaAddress=address=>{try{const u=new URL(address);return u.pathname==='/files/'&&u.searchParams.get('desktop-media')==='1'}catch{return false}};
function createMediaDesktop({BrowserWindow,ipcMain,fs,profile,launch,screen,create}){
 const read=(name,fallback)=>{try{const value=fs.readFileSync(path.join(profile,name),'utf8');return value.length<=1024*1024?JSON.parse(value):fallback}catch{return fallback}};
 const write=(name,value)=>{fs.mkdirSync(profile,{recursive:true});const dest=path.join(profile,name),temp=dest+'.tmp';fs.writeFileSync(temp,JSON.stringify(value));fs.renameSync(temp,dest)};
 let preferences=cleanPreferences(read('desktop-media.json',{})),bounds=read('media-windows.json',{}),history=read('media-history.json',{});
 if(!history||typeof history!=='object'||Array.isArray(history))history={};if(!bounds||typeof bounds!=='object')bounds={};
 const windows=()=>BrowserWindow.getAllWindows().filter(w=>!w.isDestroyed());
 const command=(win,value)=>win.webContents.send?.('hamster:desktop-command',value);
 function saveHistory(){const kept=Object.entries(history).filter(([key,value])=>/^[a-f0-9]{64}$/.test(key)&&value&&typeof value==='object').sort((a,b)=>(b[1].updated||0)-(a[1].updated||0)).slice(0,500);history=Object.fromEntries(kept);write('media-history.json',history)}
 function keyFor(value){if(typeof value?.path!=='string'||!path.isAbsolute(value.path)||value.path.length>32768||/[\x00-\x1f]/.test(value.path))throw Error('媒体路径无效');return crypto.createHash('sha256').update(JSON.stringify([value.path,String(value.size||''),String(value.modified||'')])).digest('hex')}
 function focusAudio(active){for(const win of windows())if(policy.trustedPage(win.webContents.getURL(),launch().origin))command(win,{action:'media-audio-focus',active:!preferences.focusAudio||win===active})}
 if(ipcMain)ipcMain.handle('hamster:media',async(event,action,value)=>{
  const win=BrowserWindow.fromWebContents(event.sender);if(!win||win.isDestroyed()||event.senderFrame!==event.sender.mainFrame||!policy.trustedPage(event.senderFrame?.url,launch().origin))throw Error('此页面不能管理桌面媒体');
  switch(action){
   case 'ready':if(!win._toolboxMedia||!isMediaAddress(win.webContents.getURL()))throw Error('仅独立预览窗口支持就绪通知');win._toolboxMediaReady=true;win.emit('toolbox-media-ready');return true;
   case 'settings':return{...preferences};
   case 'settings-save':{
    if(!value||typeof value!=='object'||Object.keys(value).some(k=>!Object.keys(defaults).includes(k)))throw Error('媒体设置无效');const next=cleanPreferences({...preferences,...value});if(Object.keys(value).some(key=>next[key]!==value[key]))throw Error('媒体设置值无效');write('desktop-media.json',next);preferences=next;if(!next.remember){history={};write('media-history.json',history)}for(const other of windows())command(other,{action:'media-settings',value:{...next}});focusAudio(windows().find(w=>w.isFocused?.()));return{...preferences};
   }
   case 'open':{
    keyFor(value);if(!['image','video'].includes(value.kind))throw Error('仅支持图片与视频');const url=new URL('/files/',launch().url);url.searchParams.set('session',new URL(launch().url).searchParams.get('session'));url.searchParams.set('desktop-media','1');url.searchParams.set('media-path',value.path);url.searchParams.set('media-kind',value.kind);url.searchParams.set('media-folder',path.dirname(value.path));url.searchParams.set('media-offset',String(Math.max(0,Number(value.offset)||0)));if(['name','size','modified'].includes(value.sort))url.searchParams.set('media-sort',value.sort);if(['asc','desc'].includes(value.order))url.searchParams.set('media-order',value.order);
    const existing=windows().find(w=>w.webContents.getURL()===url.href);if(existing){if(!existing._toolboxMedia||existing._toolboxMediaReady){existing.show();existing.focus()}}else create(url.href,value.kind,thisBounds(value.kind));return true;
   }
   case 'pin':if(!isMediaAddress(win.webContents.getURL()))throw Error('仅独立预览窗口支持置顶');win.setAlwaysOnTop(Boolean(value));return win.isAlwaysOnTop();
   case 'history-get':if(!preferences.remember)return null;return history[keyFor(value)]?.record||null;
   case 'history-set':if(!preferences.remember)return false;{const record=cleanRecord(value?.record);if(!record)throw Error('观看记录无效');history[keyFor(value)]={updated:Date.now(),record};saveHistory();return true}
   case 'history-clear':history={};saveHistory();return true;
   case 'focus':if(win.isFocused?.())focusAudio(win);return true;
   default:throw Error('媒体操作无效');
  }
 });
 function thisBounds(kind){return policy.displayBounds(bounds[kind]||{width:1000,height:760},screen.getAllDisplays?.().map(d=>d.workArea)||[screen.getPrimaryDisplay().workArea])}
 return{isMediaWindow:win=>Boolean(win._toolboxMedia)||isMediaAddress(win.webContents.getURL()),attach(win){win.on('focus',()=>{if(preferences.focusAudio)focusAudio(win)})},rememberWindow(win){if(!isMediaAddress(win.webContents.getURL()))return;const kind=new URL(win.webContents.getURL()).searchParams.get('media-kind');if(['image','video'].includes(kind))try{bounds[kind]=win.getNormalBounds();write('media-windows.json',bounds)}catch{}},get preferences(){return preferences}};
}
module.exports={defaults,cleanPreferences,cleanRecord,isMediaAddress,createMediaDesktop};
