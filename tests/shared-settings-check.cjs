const assert=require('node:assert/strict'),fs=require('node:fs'),{JSDOM}=require('jsdom');
const {createUI,until,requests}=require('./ui-check.cjs');
const {setup,wait}=require('./explorer-check.cjs');
const sharing={Enabled:false,Port:18788,PasswordSet:true,WorkspacePasswordSet:true,Roots:[],URLs:[]};
async function exercise(w,ready){
 const $=id=>w.document.getElementById(id);await ready();
 $('openGeneralSettings').click();$('generalTabNetwork').click();
 assert.equal($('generalNetwork').hidden,false);assert.equal($('generalPlayback').hidden,true);
 $('openNetworkSettings').click();await until(()=>$('sharingDialog').open&&!$('shareSave').disabled,'common sharing dialog loaded');
 assert.equal($('sharePort').value,'18788');assert.match($('workspacePasswordState').textContent,/已设置/);
 $('shareBrowse').click();await until(()=>$('sharePickerError').textContent.includes('个目录')||$('sharePickerEntries').children.length,'directory picker');$('sharePickerCancel').click();
 $('shareCode').value='synthetic-only';$('shareSave').click();await until(()=>$('shareResultDialog').open,'save confirmation');assert.equal($('shareCode').value,'');$('shareResultClose').click();$('sharingClose').click();assert($('generalSettingsDialog').open);
 $('closeGeneralSettings').click();
}
(async()=>{
 const dom=createUI(15300,true,390,w=>{w.structuredClone=structuredClone});
 await exercise(dom.window,()=>until(()=>dom.window.document.getElementById('title').textContent&&!dom.window.document.getElementById('preview').disabled,'toolbox ready'));
 for(const path of ['sharing-status','sharing-save','sharing-folders'])assert(requests.some(r=>r.path===path),path);dom.window.close();
 const t=setup(true,{globalFeedback:true,requestHook:({route})=>{if(route==='sharing-status'||route==='sharing-save')return{ok:true,json:async()=>sharing};if(route==='sharing-folders')return{ok:true,json:async()=>({Path:'',Entries:[{Name:'Demo',Path:'E:/demo',Dir:true}],Total:1})}}});
 await exercise(t.w,()=>wait(()=>t.$('entries').children.length,'Explorer ready'));assert.equal(t.errors.length,0,t.errors.join('\n'));t.dom.window.close();
 const html=fs.readFileSync('web/notes.html','utf8').replace('</body>',fs.readFileSync('web/shared-settings.html','utf8')+'</body>');
 const notes=new JSDOM(html,{url:'http://localhost/notes/?session=notes-test-session',runScripts:'outside-only'}),w=notes.window,calls=[];
 w.structuredClone=structuredClone;w.HTMLDialogElement.prototype.showModal=function(){this.open=true};w.HTMLDialogElement.prototype.close=function(){this.open=false;this.dispatchEvent(new w.Event('close'))};
 w.fetch=async(url,options)=>{calls.push(url);assert.equal(options.headers['X-Toolbox-Token'],'notes-test-session');return{ok:true,json:async()=>url==='/api/settings'?{}:url==='/api/state'?{Status:'idle'}:url==='/api/sharing-folders'?{Path:'',Entries:[{Name:'Demo',Path:'E:/demo',Dir:true}],Total:1}:sharing}};
 for(const file of ['network-settings.js','task-feedback.js','notes-general.js'])w.eval(fs.readFileSync('web/'+file,'utf8'));
 await exercise(w,()=>until(()=>!w.document.getElementById('openGeneralSettings').disabled,'notes settings ready'));assert(calls.includes('/api/sharing-save'));notes.window.close();
 console.log('PASS: Toolbox, Explorer and Notes open/save common remote settings and directory picker through management APIs');
})().catch(error=>{console.error(error);process.exit(1)});
