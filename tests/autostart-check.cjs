// Mock system registration only: this suite never changes Windows startup.
const assert=require('node:assert/strict');
const {createUI,until,setState}=require('./ui-check.cjs');
const {setup,wait}=require('./explorer-check.cjs');
const response=state=>({ok:true,json:async()=>({...state})});
(async()=>{
 setState({Status:'idle'});
 let state={Supported:true,CanEnable:true,Enabled:false,Message:'登录后在托盘运行'},calls=[],saveFailure=false,pending;
 const handler=async body=>{calls.push(body);if(Object.hasOwn(body,'Enabled')){if(saveFailure)return {ok:false,text:async()=> '拒绝访问'};if(pending)return new Promise(resolve=>{pending.finish=()=>{state.Enabled=body.Enabled;resolve(response(state))}});state.Enabled=body.Enabled;}return response(state)};
 const dom=createUI(17201,true,390,w=>{const base=w.fetch;w.fetch=(url,options)=>url==='/api/autostart'?handler(JSON.parse(options.body)):base(url,options)});
 const w=dom.window,$=id=>w.document.getElementById(id);
 await until(()=>$('appVersion').textContent.includes('0.1.0')&&!$('preview').disabled,'initialized');assert.equal(calls.length,0,'boot must not register startup');
 $('openGeneralSettings').click();assert.equal(calls.length,0,'playback tab must not query startup');
 $('generalTabStartup').click();await until(()=>!$('AutoStartEnabled').disabled,'status loaded');assert.equal($('AutoStartEnabled').checked,false);assert.equal(calls.length,1);assert.deepEqual(calls[0],{});
 $('AutoStartEnabled').click();await until(()=>$('autostartStatus').textContent.startsWith('已开启'),'enabled');assert.equal(state.Enabled,true);
 $('closeGeneralSettings').click();state.Enabled=false;$('openGeneralSettings').click();await until(()=>!$('AutoStartEnabled').disabled&&!$('AutoStartEnabled').checked,'external change refreshed');
 saveFailure=true;$('AutoStartEnabled').click();await until(()=>$('autostartStatus').textContent.includes('拒绝访问'),'failed save');assert.equal($('AutoStartEnabled').checked,false);assert.equal($('AutoStartEnabled').disabled,false);saveFailure=false;
 pending={};$('AutoStartEnabled').click();await until(()=>!!pending.finish,'pending save');assert($('AutoStartEnabled').disabled);$('closeGeneralSettings').click();$('openGeneralSettings').click();pending.finish();pending=null;await until(()=>!$('AutoStartEnabled').disabled&&$('AutoStartEnabled').checked,'close/reopen during save refreshes actual state');
 $('AutoStartEnabled').click();await until(()=>$('autostartStatus').textContent.startsWith('已关闭'),'disabled');assert.equal(state.Enabled,false);
 state={Supported:false,CanEnable:false,Enabled:false,Message:'目前仅支持 Windows'};$('generalTabStartup').click();await until(()=>$('autostartOption').hidden,'unsupported platform');assert($('AutoStartEnabled').disabled);assert.match($('autostartStatus').textContent,/Windows/);
 dom.window.close();
 const local=setup(true,{globalFeedback:true,autostartHook:handler});await wait(()=>local.$('openGeneralSettings').hidden===false&&local.calls.some(c=>c.route==='settings'),'local settings connected');state={Supported:true,CanEnable:true,Enabled:false,Message:'托盘运行'};
 local.$('openGeneralSettings').click();local.$('generalTabStartup').click();await wait(()=>!local.$('AutoStartEnabled').disabled,'explorer startup status');local.$('AutoStartEnabled').click();await wait(()=>local.$('autostartStatus').textContent.startsWith('已开启'),'explorer save');assert(state.Enabled);assert.equal(local.errors.length,0);local.dom.window.close();
 const guest=setup(false,{globalFeedback:true,autostartHook:handler});await wait(()=>guest.$('entries').children.length>0,'guest initialized');assert(guest.$('openGeneralSettings').hidden);assert(!guest.calls.some(c=>c.route==='autostart'));guest.dom.window.close();
 console.log('PASS: explicit default-off startup, read-only status, external changes, save rollback, pending close/reopen, Windows-only availability, local Explorer and guest isolation');
})().catch(error=>{console.error(error);process.exit(1)});
