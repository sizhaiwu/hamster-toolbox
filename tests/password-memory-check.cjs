const assert=require('node:assert/strict');
const {createUI,until,requests,storage}=require('./ui-check.cjs');
const windows=[];let failRead=false,failSave=false,unsupported=false;
function page(){const dom=createUI(17800,true,1280,w=>{const original=w.fetch;w.fetch=async(url,options)=>{
  if(url==='/api/password-memory'&&unsupported)return {ok:true,json:async()=>({Supported:false,Remember:false,Password:'',SavedID:''})};
  if(url==='/api/password-memory'&&failRead||url==='/api/password-memory-save'&&failSave)return {ok:false,text:async()=>'synthetic encrypted storage failure'};
  return original(url,options);
}});windows.push(dom.window);return dom.window;}
async function switchTo(w,tool){w.document.querySelector('[data-tool='+tool+']').click();await until(()=>w.document.querySelector('[data-tool='+tool+']').classList.contains('active')&&!w.document.getElementById('preview').disabled,'tool '+tool);}
function input(w,id,value){const el=w.document.getElementById(id);el.value=value;el.dispatchEvent(new w.Event('input',{bubbles:true}));}
(async()=>{
  let w=page(),$=id=>w.document.getElementById(id);await until(()=>$('title').textContent&&!$('preview').disabled,'startup');await switchTo(w,'archive');
  assert($('ArchiveRememberPassword').checked);input(w,'ArchivePassword','Archive-Only-Example');
  await switchTo(w,'extract');assert.equal($('ExtractPassword').value,'','archive password must not populate extraction');input(w,'ExtractPassword','Extract-Only-Example');
  await switchTo(w,'archive');assert.equal($('ArchivePassword').value,'Archive-Only-Example');
  w.close();w=page();await until(()=>$('title').textContent==='打包资源'&&!$('preview').disabled,'refresh restores archive');assert.equal($('ArchivePassword').value,'Archive-Only-Example');assert.equal($('ArchivePassword').dataset.secretHidden,'true');
  await switchTo(w,'extract');assert.equal($('ExtractPassword').value,'Extract-Only-Example');await switchTo(w,'archive');
  $('ArchiveRememberPassword').checked=false;$('ArchiveRememberPassword').dispatchEvent(new w.Event('change',{bubbles:true}));await until(()=>$('ArchivePasswordMemoryStatus').textContent.includes('已关闭'),'opt out cleared memory');assert.equal($('ArchivePassword').value,'Archive-Only-Example','opt out must not change the current input');
  w.close();w=page();await until(()=>$('title').textContent==='打包资源'&&!$('preview').disabled,'restart after opt out');assert.equal($('ArchivePassword').value,'');assert(!$('ArchiveRememberPassword').checked);
  const before=requests.filter(r=>r.path==='password-memory-save').length;input(w,'ArchivePassword','Temporary-Only');await switchTo(w,'format');await switchTo(w,'archive');assert.equal($('ArchivePassword').value,'Temporary-Only');assert.equal(requests.filter(r=>r.path==='password-memory-save').length,before,'opt-out input must not be persisted');
  $('ArchiveRememberPassword').checked=true;$('ArchiveRememberPassword').dispatchEvent(new w.Event('change',{bubbles:true}));await until(()=>$('ArchivePasswordMemoryStatus').textContent.includes('已加密记住'),'enable memory');
  input(w,'ArchivePassword','');$('ArchivePassword').dispatchEvent(new w.Event('change'));await until(()=>$('ArchivePasswordMemoryStatus').textContent.includes('已清空'),'clearing input clears memory');
  w.close();w=page();await until(()=>$('title').textContent==='打包资源'&&!$('preview').disabled,'restart after clearing');assert.equal($('ArchivePassword').value,'');
  $('manageArchivePasswords').click();await until(()=>$('passwordDialog').open&&!$('savePassword').disabled,'open manager');input(w,'PasswordName','示例常用密码');input(w,'PasswordValue','Saved-Only-Example');$('savePassword').click();await until(()=>!$('passwordDialog').open&&!$('ArchivePassword').disabled,'save and select');const id=$('ArchiveSavedPassword').value;assert(id,'selected saved password should stay selected');
  await switchTo(w,'format');await switchTo(w,'archive');assert.equal($('ArchiveSavedPassword').value,id);assert.equal($('ArchivePassword').value,'Saved-Only-Example');
  failSave=true;input(w,'ArchivePassword','Unsaved-Example');$('ArchivePassword').dispatchEvent(new w.Event('change'));await until(()=>$('ArchivePasswordMemoryStatus').textContent.includes('未保存'),'save failure visible');assert.equal($('ArchivePassword').value,'Unsaved-Example');assert(!$('preview').disabled);
  for(const r of requests.filter(r=>r.path==='save-settings'))assert(!('ArchivePassword'in r.body)&&!('ExtractPassword'in r.body),'ordinary settings must not contain passwords');
  for(const store of [w.localStorage,w.sessionStorage])for(let i=0;i<store.length;i++)assert(!/Archive-Only-Example|Extract-Only-Example|Temporary-Only|Saved-Only-Example|Unsaved-Example/.test(store.getItem(store.key(i))||''),'browser storage must not contain passwords');
  w.close();failSave=false;failRead=true;w=page();await until(()=>$('title').textContent==='打包资源'&&!$('preview').disabled,'failed read still allows temporary use');assert.match($('ArchivePasswordMemoryStatus').textContent,/读取失败/);assert(!$('ArchiveRememberPassword').checked);assert($('ArchiveRememberPassword').disabled);
  w.close();failRead=false;unsupported=true;w=page();await until(()=>$('title').textContent==='打包资源'&&!$('preview').disabled,'unsupported platform fallback');assert($('ArchiveRememberPassword').disabled);assert.match($('ArchivePasswordMemoryStatus').textContent,/暂不支持/);
  for(const window of windows)window.close();console.log('PASS: per-tool encrypted password memory, refresh, opt out, clearing, saved choice retention, visible storage failures and browser-storage exclusion');
})().catch(error=>{for(const window of windows)window.close();console.error(error);process.exit(1)});
