'use strict';
function launchArguments(args) {
 const value=key=>args.find(v=>v.startsWith(key+'='))?.slice(key.length+1);
 const address=value('--toolbox-url'),profile=value('--toolbox-profile');
 if(!address||!profile)throw Error('请从仓鼠工具箱启动桌面客户端。');
 const u=new URL(address);
 if(u.protocol!=='http:'||u.hostname!=='127.0.0.1'||!u.port||u.username||u.password||!/^\/[a-z/.-]*$/i.test(u.pathname)||!/^[a-f0-9]{32,128}$/.test(u.searchParams.get('session')||''))throw Error('工具箱本机地址无效。');
 return {url:u.href,origin:u.origin,profile,backendTray:args.includes('--toolbox-tray')};
}
function navigationAllowed(address,origin) {try{return new URL(address).origin===origin}catch{return false}}
function externalAllowed(address) {try{const u=new URL(address);return ['https:','http:'].includes(u.protocol)&&!u.username&&!u.password&&!['127.0.0.1','localhost','[::1]'].includes(u.hostname)}catch{return false}}
function webPreferences() {return {nodeIntegration:false,nodeIntegrationInWorker:false,nodeIntegrationInSubFrames:false,contextIsolation:true,sandbox:true,webSecurity:true,allowRunningInsecureContent:false,webviewTag:false,spellcheck:false};}
function permissionAllowed(permission,address,origin) {return ['clipboard-sanitized-write','fullscreen'].includes(permission)&&navigationAllowed(address,origin);}
function samePage(a,b){try{const x=new URL(a),y=new URL(b);return x.origin===y.origin&&x.pathname===y.pathname&&x.search===y.search&&x.hash===y.hash}catch{return false}}
function windowBounds(state,area){const finite=(n,fallback)=>Number.isFinite(Number(n))&&Number(n)>0?Number(n):fallback;return {width:Math.round(Math.min(Math.max(320,finite(area.width,1280)),Math.max(390,finite(state.width,1280)))),height:Math.round(Math.min(Math.max(320,finite(area.height,860)),Math.max(400,finite(state.height,860))))}}
function trustedPage(address,origin){try{const u=new URL(address);return u.origin===origin&&['/','/index.html','/files/','/notes/','/notes/notes.html','/schedules.html'].includes(u.pathname)}catch{return false}}
function displayBounds(state,areas){
 state=state&&typeof state==='object'?state:{};
 const first=areas[0]||{x:0,y:0,width:1280,height:860};
 const positioned=Number.isFinite(state.x)&&Number.isFinite(state.y);
 const area=positioned?(areas.find(a=>state.x<a.x+a.width&&state.x+(Number(state.width)||800)>a.x&&state.y<a.y+a.height&&state.y+(Number(state.height)||600)>a.y)||first):first;
 const bounds=windowBounds(state,area);
 if(positioned){bounds.x=Math.round(Math.max(area.x,Math.min(state.x,area.x+area.width-bounds.width)));bounds.y=Math.round(Math.max(area.y,Math.min(state.y,area.y+area.height-bounds.height)))}
 return bounds;
}
function trayAddress(address,destination){
 const paths={toolbox:'/',files:'/files/',notes:'/notes/',schedules:'/schedules.html',settings:'/#settings'},base=new URL(address);
 if(!Object.hasOwn(paths,destination)||base.protocol!=='http:'||base.hostname!=='127.0.0.1'||!base.port||base.username||base.password||!/^[a-f0-9]{32,128}$/.test(base.searchParams.get('session')||''))throw Error('托盘入口无效');
 const target=new URL(paths[destination],base);target.searchParams.set('session',base.searchParams.get('session'));return target.href;
}
module.exports={samePage,windowBounds,displayBounds,trustedPage,launchArguments,navigationAllowed,externalAllowed,webPreferences,permissionAllowed,trayAddress};
