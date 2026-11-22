'use strict';
const $=id=>document.getElementById(id),ownerToken=new URLSearchParams(location.search).get('session')||'';
let noteFontSize=14,fontAppliedSequence=0,noteLockPending=false;
let autoLockMinutes=10,lastServerActivity=Date.now(),idleLockPending=false,apiRequestSequence=0,autoLockAppliedSequence=0;
let editorMode='split',previewRequest=0,notesEpoch=0,notesSession='',notes=[],current=null,dirty=false,editVersion=0,saveFlight=null,saveTimer,toastTimer,previewTimer,lastActivity=Date.now(),exists=false,twoFA=false,credentialAction='',restoring=null,closeBackdrop=false;
let writingMode='rich',notesSort='updated',categoryFilter='',listLimit=100,searchTimer,settingsTimer,selectedVersion='',pendingDraft=null;
let draftKey=null,draftSalt=null,draftVaultID='',draftFlight=Promise.resolve(),draftUploadTimer,draftEnvelope=null,backupWarning='';
const notePositions=new Map();
const notesResumeKey='hamster.notes.resume.v1',notesResumePreference='hamster.notes.refresh-unlock.v1';
let refreshUnlock=true;try{refreshUnlock=localStorage.getItem(notesResumePreference)!=='false'}catch{}
$('keepUnlockedOnRefresh').checked=refreshUnlock;
function forgetNotesResume(){try{sessionStorage.removeItem(notesResumeKey)}catch{}}
function rememberNotesResume(){
 if(!refreshUnlock||!notesSession){forgetNotesResume();return false}
 try{sessionStorage.setItem(notesResumeKey,JSON.stringify({Session:notesSession,Owner:ownerToken,NoteID:current?.ID||''}));return true}catch{return false}
}
$('keepUnlockedOnRefresh').onchange=()=>{
 refreshUnlock=$('keepUnlockedOnRefresh').checked;try{localStorage.setItem(notesResumePreference,String(refreshUnlock))}catch{}
 const saved=rememberNotesResume();setAutoLock(autoLockMinutes);
 say(refreshUnlock?(saved?'已开启：刷新当前标签页后保持解锁':'浏览器无法保存临时会话，刷新后仍需解锁'):'已关闭：刷新或离开页面时锁定笔记',refreshUnlock&&!saved);
};
async function startNotes(fromCache=false){
 let saved;try{saved=JSON.parse(sessionStorage.getItem(notesResumeKey)||'null')}catch{}
 const reload=performance.getEntriesByType?.('navigation')?.[0]?.type==='reload'||performance.navigation?.type===1;
 if(refreshUnlock&&(reload||fromCache)&&saved?.Owner===ownerToken&&/^[a-f0-9]{32}$/.test(saved?.Session||'')){
  notesSession=saved.Session;$('gateHelp').textContent='正在恢复当前标签页的解锁会话…';$('unlock').disabled=true;
  try{const data=await api('resume');await prepareDraft(data,'');enter(data,saved.NoteID);await offerRecoveredDraft(data);return}
  catch{notesSession='';forgetNotesResume();await status();say('上次解锁已失效或无法恢复，请重新输入主密码');return}
 }
 forgetNotesResume();await status();
}

const attachmentURLs=new Map();let attachmentGeneration=0,attachmentBytes=0;
const notesSplitKey='hamster.notes.split.v1';
let notesSplit={horizontal:50,vertical:50},notesSplitDrag=null;
try{const saved=JSON.parse(localStorage.getItem(notesSplitKey)||'null');for(const axis of ['horizontal','vertical'])if(Number.isFinite(saved?.[axis])&&saved[axis]>=10&&saved[axis]<=90)notesSplit[axis]=saved[axis]}catch{}
function notesSplitAxis(){return innerWidth<=960?'vertical':'horizontal'}
function applyNotesSplit(){
 const area=$('editorBody'),divider=$('noteDivider');
 area.style.setProperty('--notes-edit-width',notesSplit.horizontal+'fr');area.style.setProperty('--notes-preview-width',(100-notesSplit.horizontal)+'fr');area.style.setProperty('--notes-edit-height',notesSplit.vertical+'fr');area.style.setProperty('--notes-preview-height',(100-notesSplit.vertical)+'fr');
 const stacked=notesSplitAxis()==='vertical',value=notesSplit[notesSplitAxis()];divider.setAttribute('aria-orientation',stacked?'horizontal':'vertical');divider.setAttribute('aria-label','调整编辑区与预览区的'+(stacked?'高度':'宽度'));divider.setAttribute('aria-valuenow',String(value));divider.setAttribute('aria-valuetext','编辑区 '+value+'%，预览区 '+Math.round((100-value)*10)/10+'%');
}
function saveNotesSplit(){try{localStorage.setItem(notesSplitKey,JSON.stringify(notesSplit))}catch{}}
function resizeNotesSplit(value,axis=notesSplitAxis()){if(!Number.isFinite(value))return;notesSplit[axis]=Math.round(Math.min(90,Math.max(10,value))*10)/10;applyNotesSplit()}
function endNotesSplit(cancel=false){const drag=notesSplitDrag;if(!drag)return;notesSplitDrag=null;document.body.classList.remove('notes-resizing');if(cancel)resizeNotesSplit(drag.start,drag.axis);else saveNotesSplit();try{$('noteDivider').releasePointerCapture?.(drag.id)}catch{}}
$('noteDivider').addEventListener('pointerdown',e=>{if(e.button!==0||e.isPrimary===false||!current||editorMode!=='split')return;e.preventDefault();endNotesSplit(true);const axis=notesSplitAxis();notesSplitDrag={id:e.pointerId,axis,start:notesSplit[axis]};document.body.classList.add('notes-resizing');$('noteDivider').focus({preventScroll:true});try{$('noteDivider').setPointerCapture?.(e.pointerId)}catch{}});
document.addEventListener('pointermove',e=>{const drag=notesSplitDrag;if(!drag||e.pointerId!==drag.id)return;const r=$('editorBody').getBoundingClientRect(),handle=$('noteDivider').getBoundingClientRect(),stacked=drag.axis==='vertical',size=(stacked?r.height:r.width)-(stacked?handle.height:handle.width);if(size<=0)return;e.preventDefault();resizeNotesSplit(((stacked?e.clientY-r.top-handle.height/2:e.clientX-r.left-handle.width/2)/size)*100,drag.axis)},{passive:false});
document.addEventListener('pointerup',e=>{if(notesSplitDrag?.id===e.pointerId)endNotesSplit()});document.addEventListener('pointercancel',e=>{if(notesSplitDrag?.id===e.pointerId)endNotesSplit(true)});$('noteDivider').addEventListener('lostpointercapture',()=>endNotesSplit(true));window.addEventListener('blur',()=>endNotesSplit(true));window.addEventListener('resize',()=>{endNotesSplit(true);applyNotesSplit()});
$('noteDivider').addEventListener('dblclick',()=>{if(current&&editorMode==='split'){endNotesSplit(true);resizeNotesSplit(50);saveNotesSplit()}});
$('noteDivider').addEventListener('keydown',e=>{if(!current||editorMode!=='split')return;const stacked=notesSplitAxis()==='vertical',negative=stacked?'ArrowUp':'ArrowLeft',positive=stacked?'ArrowDown':'ArrowRight';if(![negative,positive,'Home','End'].includes(e.key))return;e.preventDefault();endNotesSplit(true);resizeNotesSplit(e.key==='Home'?10:e.key==='End'?90:notesSplit[notesSplitAxis()]+(e.key===negative?-1:1)*(e.shiftKey?10:2));saveNotesSplit()});
applyNotesSplit();
$('backToolbox').href='/?session='+encodeURIComponent(ownerToken);$('returnToolbox').href=$('backToolbox').href;
for(const id of ['backToolbox','returnToolbox'])$(id).onclick=e=>{if(e.button!==0||e.ctrlKey||e.metaKey||e.shiftKey||e.altKey)return;e.preventDefault();const destination=e.currentTarget.href;run(async()=>{if(noteLockPending)throw Error('请等待编辑锁定操作完成后再返回工具箱');rememberPosition();await flush();await saveEditorSettings();location.assign(destination)})};
for(const id of ['openNotesSecurity','settingsSecurityInfo'])$(id).onclick=()=>$('notesSecurityDialog').showModal();$('notesSecurityClose').onclick=()=>$('notesSecurityDialog').close();

function say(message,error=false){clearTimeout(toastTimer);$('notice').textContent=message;$('notice').classList.toggle('error',error);if(!error)toastTimer=setTimeout(()=>{$('notice').textContent=''},4000)}
async function api(action,body={}){
 const epoch=notesEpoch,requestSequence=++apiRequestSequence,startedAt=Date.now();const multipart=action==='restore'&&body.Backup instanceof Blob;
 let content=JSON.stringify(body),headers={'Content-Type':'application/json','X-Toolbox-Token':ownerToken,'X-Notes-Session':notesSession};
 if(multipart){content=new FormData();content.append('Backup',body.Backup);content.append('Password',body.Password||'');content.append('Code',body.Code||'');content.append('Confirm',String(Boolean(body.Confirm)));delete headers['Content-Type']}
 const response=await fetch('/api/notes/'+action,{method:'POST',credentials:'same-origin',cache:'no-store',headers,body:content});
 if(epoch!==notesEpoch)throw Error('笔记已锁定，已取消旧页面操作');
 if(!response.ok){const message=await response.text();if(response.status===401&&action!=='resume'){const hadDraft=dirty,preserved=await preserveDraft();clearWorkspace();if(hadDraft)say(preserved?'会话已锁定；未保存内容已保留为加密草稿，解锁后可恢复':'会话已锁定；未保存内容未能保留，请检查浏览器存储',true);}throw Error(message.trim()||'操作失败，请重试')}
 const result=await response.json();if(epoch!==notesEpoch)throw Error('笔记已锁定，已取消旧页面操作');if(notesSession&&action!=='status')lastServerActivity=Math.max(lastServerActivity,startedAt);if(Number.isInteger(result.AutoLockMinutes)&&requestSequence>=autoLockAppliedSequence){autoLockAppliedSequence=requestSequence;if(action!=='keepalive'||result.AutoLockMinutes!==autoLockMinutes)setAutoLock(result.AutoLockMinutes);}if(Number.isInteger(result.FontSize)&&requestSequence>=fontAppliedSequence){fontAppliedSequence=requestSequence;setNoteFont(result.FontSize,!['create','unlock','restore'].includes(action));}if(result.BackupWarning&&result.BackupWarning!==backupWarning){backupWarning=result.BackupWarning;say(backupWarning,true)}return result;
}
// Avoid classifying a local vault unlock as a website login in Chromium.
// Keep native masking when visual masking is unavailable, never expose a fallback field.
const supportsNoteSecretMask=Boolean(window.CSS?.supports?.('-webkit-text-security','disc'));
for(const field of document.querySelectorAll('.notes-secret')){field.type=supportsNoteSecretMask?'text':'password';field.dataset.secretReady='true';}
function setPasswordVisible(button,visible){const field=$(button.dataset.passwordFor),start=field.selectionStart,end=field.selectionEnd;field.dataset.secretHidden=String(!visible);field.type=supportsNoteSecretMask||visible?'text':'password';button.setAttribute('aria-pressed',String(visible));const label=(visible?'隐藏':'显示')+button.dataset.passwordLabel;button.setAttribute('aria-label',label);button.title=label;try{field.setSelectionRange(start,end)}catch{}}
function resetPasswordVisibility(scope=document){for(const button of scope.querySelectorAll('.password-eye'))setPasswordVisible(button,false)}
for(const button of document.querySelectorAll('.password-eye'))button.onclick=()=>setPasswordVisible(button,button.getAttribute('aria-pressed')!=='true');
function clearCredentials(){resetPasswordVisibility();for(const id of ['master','masterConfirm','otp','currentPassword','newPassword','newConfirm','credentialCode','enrollCode','otpSecret'])$(id).value='';$('otpQR').removeAttribute('src');$('recoveryCodes').textContent='';restoring=null;credentialAction=''}
function clearWorkspace(refresh=true,keepResume=false){window.hamsterNotesLayout?.reset();if(!keepResume)forgetNotesResume();clearAttachmentViews();window.hamsterRichEditor?.destroy();draftKey=null;draftSalt=null;draftEnvelope=null;draftVaultID='';pendingDraft=null;clearTimeout(draftUploadTimer);$('draftContent').value='';$('historyContent').value='';$('historyList').replaceChildren();$('trashList').replaceChildren();$('noteCategory').value='';$('noteTags').value='';notePositions.clear();endNotesSplit(true);notesEpoch++;noteLockPending=false;$('noteFontSize').disabled=true;$('toggleNoteLock').disabled=true;clearTimeout(saveTimer);clearTimeout(previewTimer);previewRequest++;$('previewStatus').textContent='';notesSession='';notes=[];current=null;updateLastSavedAt();dirty=false;saveFlight=null;editVersion++;$('noteTitle').value='';$('markdown').value='';$('markdownColors').replaceChildren();$('preview').srcdoc='';$('notesList').replaceChildren();$('searchNotes').value='';$('workspace').hidden=true;$('gate').hidden=false;$('security').hidden=true;$('lock').hidden=true;for(const dialog of document.querySelectorAll('dialog[open]:not(#remoteSessionExpired)'))dialog.close();clearCredentials();if(refresh)status().catch(e=>say(e.message,true))}
async function status(){const state=await api('status');exists=state.Exists;twoFA=state.TwoFA;$('gateTitle').textContent=exists?'解锁私密笔记':'创建私密笔记';$('gateHelp').textContent=exists?'输入主密码，继续查看本地笔记。':'主密码由你自行设置，建议使用独立的长密码。';$('unlock').textContent=exists?'解锁':'创建加密笔记库';$('unlock').disabled=false;$('confirmLabel').hidden=exists;$('otpLabel').hidden=!twoFA;$('masterConfirm').required=!exists;$('master').minLength=1;$('restoreLocked').hidden=exists&&!state.Damaged;if(state.Damaged){$('gateHelp').textContent='笔记文件损坏或无法读取，请使用有效的加密备份恢复。';$('unlock').disabled=true;$('restoreLocked').hidden=false}}
function setNoteFont(size,refresh=true){
 if(!Number.isInteger(size)||size<10||size>40)return;const changed=size!==noteFontSize;noteFontSize=size;$('noteFontSize').value=String(size);document.documentElement.style.setProperty('--note-font-size',size+'px');
 if(changed&&current&&editorMode!=='edit'&&refresh){clearTimeout(previewTimer);previewRequest++;run(renderPreview)}
}
function updateLastSavedAt(){
 const el=$('lastSavedAt'),date=new Date(current?.Updated||'');
 if(!current||!Number.isFinite(date.getTime())){el.hidden=true;el.textContent='';el.removeAttribute('datetime');el.removeAttribute('title');el.removeAttribute('aria-label');return}
 const time=date.toLocaleTimeString('zh-CN',{hour12:false,hour:'2-digit',minute:'2-digit',second:'2-digit'}),full=date.toLocaleString('zh-CN',{hour12:false});
 el.dateTime=date.toISOString();el.textContent='· '+time;el.title='最后成功保存：'+full;el.setAttribute('aria-label',el.title);el.hidden=false;
}
function syncNoteEditing(){
 updateLastSavedAt();
 const locked=Boolean(current?.Locked),blocked=!current||locked||noteLockPending;
 for(const id of ['noteTitle','markdown'])$(id).readOnly=locked||noteLockPending;
 $('save').disabled=blocked;$('deleteNote').disabled=blocked;$('toggleNoteLock').disabled=!current||noteLockPending;$('toggleNoteLock').textContent=locked?'解锁编辑':'锁定编辑';$('toggleNoteLock').setAttribute('aria-pressed',String(locked));$('toggleNoteLock').title=locked?'允许修改当前笔记':'锁定当前笔记的编辑，防止误改';$('editorBody').closest('.editor').classList.toggle('note-is-locked',locked);
 $('newNote').disabled=noteLockPending;$('importMarkdown').disabled=noteLockPending;
 window.hamsterRichEditor?.readOnly(blocked);for(const e of $('formatTools').querySelectorAll('button'))e.disabled=blocked;$('addAttachment').disabled=blocked;$('lock').disabled=noteLockPending;for(const id of ['openNoteDetails','openHistory'])$(id).disabled=!current||noteLockPending;if(locked)$('saveStatus').textContent='已锁定编辑';else if(current&&!dirty&&!saveFlight)$('saveStatus').textContent='已加密保存';
}
for(let size=10;size<=40;size++){$('noteFontSize').add(new Option(String(size),String(size)))}setNoteFont(14,false);
$('noteFontSize').onchange=()=>run(async()=>{const size=Number($('noteFontSize').value);$('noteFontSize').disabled=true;try{await api('font-size',{FontSize:size});say('字号已保存')}catch(e){$('noteFontSize').value=String(noteFontSize);throw e}finally{$('noteFontSize').disabled=!notesSession}});
$('toggleNoteLock').onclick=()=>run(async()=>{
 if(!current||noteLockPending)return;const id=current.ID,locked=!current.Locked;noteLockPending=true;syncNoteEditing();
 try{await flush();const target=notes.find(n=>n.ID===id);if(!target)throw Error('笔记不存在，请刷新列表');const data=await api('note-lock',{Note:{ID:id,Revision:target.Revision},Locked:locked});const at=notes.findIndex(n=>n.ID===id);if(at>=0)notes[at]=data.Note;if(current?.ID===id)current=data.Note;renderList();say(locked?'已锁定编辑，可阅读和导出':'已解锁编辑')}
 finally{noteLockPending=false;syncNoteEditing()}
});
function setAutoLock(minutes){
 autoLockMinutes=Number.isInteger(minutes)&&minutes>=0&&minutes<=10080?minutes:10;
 const preset=[0,1,5,10,30,60,120].includes(autoLockMinutes);$('autoLockChoice').value=preset?String(autoLockMinutes):'custom';$('autoLockCustomRow').hidden=preset;$('autoLockCustom').value=autoLockMinutes||10;
 $('editorHint').textContent='修改后自动加密保存 · '+(autoLockMinutes?'空闲 '+autoLockMinutes+' 分钟自动锁定':'不自动锁定');
 $('autoLockHelp').textContent=autoLockMinutes?'保存后生效并记忆；空闲超时、手动锁定或重启工具箱后需重新验证。':'不自动锁定时请在离开前手动锁定；重启工具箱后需重新验证。';
}
function setSecurity(){for(const id of ['disableOTP','renewRecovery'])$(id).hidden=!twoFA;$('setupOTP').hidden=twoFA}
function enter(data,preferredID=''){current=null;notePositions.clear();editorMode=data.EditorSettings?.View||'split';writingMode=data.EditorSettings?.Mode||'rich';notesSort=data.EditorSettings?.Sort||'updated';categoryFilter=data.EditorSettings?.Category||'';$('writingMode').value=writingMode;for(const [id,pos] of Object.entries(data.EditorSettings?.Positions||{}))notePositions.set(id,pos);$('notesSort').value=notesSort;listLimit=100;clearTimeout(toastTimer);$('notice').textContent='';notesSession=data.Session;notes=data.Notes||[];exists=true;twoFA=data.TwoFA;lastActivity=Date.now();lastServerActivity=lastActivity;setAutoLock(data.AutoLockMinutes??10);setNoteFont(data.FontSize??14,false);$('noteFontSize').disabled=false;clearCredentials();$('gate').hidden=true;$('workspace').hidden=false;$('security').hidden=false;$('lock').hidden=false;setSecurity();renderList();choose(notes.find(n=>n.ID===preferredID)||notes[0]||null);rememberNotesResume();if(!notes.length)$('workspace').classList.add('list-open');window.hamsterNotesLayout?.refresh()}
function renderList(){
 const text=$('searchNotes').value.trim().toLocaleLowerCase(),display=notes.map(n=>current?.ID===n.ID?{...current,Title:$('noteTitle').value,Markdown:$('markdown').value}:n);
 const categories=[...new Set(display.map(n=>n.Category).filter(Boolean))].sort((a,b)=>a.localeCompare(b));
 $('categoryFilter').replaceChildren(new Option('全部分类',''),...categories.map(c=>new Option(c,c)));$('categoryFilter').value=categoryFilter;$('noteCategories').replaceChildren(...categories.map(c=>{const o=document.createElement('option');o.value=c;return o}));
 const sorted=display.filter(n=>(!categoryFilter||n.Category===categoryFilter)&&(!text||`${n.Title}\n${n.Markdown}\n${n.Category||''}\n${(n.Tags||[]).join(' ')}`.toLocaleLowerCase().includes(text))).sort((a,b)=>Number(Boolean(b.Pinned))-Number(Boolean(a.Pinned))||(notesSort==='name'?a.Title.localeCompare(b.Title,undefined,{numeric:true}):Date.parse(notesSort==='created'?b.Created:b.Updated)-Date.parse(notesSort==='created'?a.Created:a.Updated)));
 const scroll=$('notesList').scrollTop,fragment=document.createDocumentFragment();
 for(const note of sorted.slice(0,listLimit)){const button=document.createElement('button');button.className='note-card';button.setAttribute('aria-current',String(current?.ID===note.ID));const title=document.createElement('b'),hint=document.createElement('small');title.textContent=(note.Pinned?'📌 ':'')+note.Title;hint.textContent=(note.Category?note.Category+' · ':'')+new Date(note.Updated).toLocaleString();button.append(title,hint);button.dataset.locked=String(Boolean(note.Locked));if(note.Locked)button.setAttribute('aria-label',note.Title+'，已锁定编辑');button.onclick=()=>run(async()=>{if(noteLockPending)throw Error('请等待编辑锁定操作完成');rememberPosition();await flush();choose(notes.find(n=>n.ID===note.ID)||null)});fragment.append(button)}
 if(sorted.length>listLimit){const more=document.createElement('button');more.className='note-list-more';more.textContent='再显示 100 篇（共 '+sorted.length+' 篇）';more.onclick=()=>{listLimit+=100;renderList()};fragment.append(more)}
 $('notesList').replaceChildren(fragment);$('notesList').scrollTop=scroll;$('noteCount').textContent=text||categoryFilter?sorted.length+' / '+notes.length+' 篇':notes.length+' 篇笔记';
}
function setEditorMode(mode){
 queueEditorSettings();endNotesSplit(true);editorMode=['edit','split','preview'].includes(mode)?mode:'split';$('editorBody').dataset.mode=editorMode;
 $('markdownPane').hidden=editorMode==='preview';$('previewPane').hidden=editorMode==='edit';$('noteDivider').hidden=editorMode!=='split';$('noteDivider').tabIndex=current&&editorMode==='split'?0:-1;$('noteDivider').setAttribute('aria-disabled',String(!current));applyNotesSplit();
 for(const [name,id] of [['edit','modeEdit'],['split','modeSplit'],['preview','modePreview']])$(id).setAttribute('aria-pressed',String(name===editorMode));
 clearTimeout(previewTimer);if(current&&editorMode!=='edit')run(renderPreview);else{previewRequest++;$('previewStatus').textContent=''}
}
function choose(note){rememberPosition();current=note?{...note}:null;rememberNotesResume();dirty=false;editVersion++;clearTimeout(previewTimer);previewRequest++;$('noteTitle').value=note?.Title||'';$('markdown').value=note?.Markdown||'';for(const id of ['noteTitle','markdown','save','modeEdit','modeSplit','modePreview','deleteNote','exportMarkdown'])$(id).disabled=!note;$('preview').srcdoc='';$('previewStatus').textContent='';$('saveStatus').textContent=note?'已加密保存':'新建或导入一篇笔记开始使用';$('workspace').classList.remove('list-open');window.hamsterNotesLayout?.refresh();setEditorMode(editorMode);mountWritingEditor();syncNoteEditing();renderList();restorePosition();run(()=>loadAttachmentViews(current))}
function changed(){if(!current||current.Locked||noteLockPending)return;updateMarkdownColors();dirty=true;editVersion++;queueEncryptedDraft();$('saveStatus').textContent='等待保存…';clearTimeout(saveTimer);saveTimer=setTimeout(()=>run(flush),1200);if(editorMode!=='edit'){previewRequest++;$('previewStatus').textContent='等待更新…';clearTimeout(previewTimer);previewTimer=setTimeout(()=>run(renderPreview),400)}}
async function flush(){clearTimeout(saveTimer);if(current?.Locked)return;if(saveFlight){await saveFlight;return flush()}if(!dirty||!current)return;const version=editVersion,session=notesSession,epoch=notesEpoch,note={...current,Title:$('noteTitle').value,Markdown:$('markdown').value};if(!note.Title.trim())throw Error('请填写笔记标题');$('saveStatus').textContent='正在加密保存…';saveFlight=(async()=>{const data=await api('save',{Note:note});if(session!==notesSession)return;const at=notes.findIndex(n=>n.ID===data.Note.ID);if(at<0)notes.unshift(data.Note);else notes[at]=data.Note;if(current?.ID===data.Note.ID||current===note||current?.ID===note.ID){current=data.Note;updateLastSavedAt();if(editVersion===version){dirty=false;$('saveStatus').textContent='已加密保存'}else{$('saveStatus').textContent='等待保存…';saveTimer=setTimeout(()=>run(flush),1200)}}renderList()})();const flight=saveFlight;try{await flight}catch(e){if(notesSession===session&&notesEpoch===epoch)$('saveStatus').textContent='保存失败，请点击保存重试';throw e}finally{if(saveFlight===flight)saveFlight=null}if(epoch===notesEpoch&&dirty)await flush()}
async function renderPreview(){
 if(!current||editorMode==='edit')return;
 const version=editVersion,session=notesSession,font=noteFontSize,request=++previewRequest;$('previewStatus').textContent='正在更新…';
 try{const data=await api('preview',{Note:{Markdown:$('markdown').value},FontSize:font});if(request===previewRequest&&session===notesSession&&version===editVersion&&font===noteFontSize&&editorMode!=='edit'){$('preview').srcdoc=previewWithAttachments(data.HTML);$('previewStatus').textContent='已同步'}}
 catch(e){if(request===previewRequest&&session===notesSession){$('previewStatus').textContent='预览更新失败';throw e}}
}
async function run(action){const epoch=notesEpoch;try{await action()}catch(e){if(epoch===notesEpoch&&typeof document!=='undefined'&&document.getElementById('notice'))say(e.message,true)}}
function download(name,content,type){const blob=new Blob([content],{type}),url=URL.createObjectURL(blob),a=document.createElement('a');a.href=url;a.download=name;a.click();setTimeout(()=>URL.revokeObjectURL(url),1000)}
function safeName(name){return name.replace(/[<>:"/\\|?*\x00-\x1f]/g,'_').slice(0,100)||'笔记'}
function decodeBytes(value){const binary=atob(value),b=new Uint8Array(binary.length);for(let i=0;i<b.length;i++)b[i]=binary.charCodeAt(i);return b}
function encodeBytes(b){let s='';for(let i=0;i<b.length;i+=16384)s+=String.fromCharCode(...b.subarray(i,i+16384));return btoa(s)}
function showRecovery(codes){$('recoveryCodes').textContent=codes.join('\n');$('recoverySaved').checked=false;$('recoveryClose').disabled=true;$('recoveryDialog').showModal()}
function credentials(action){resetPasswordVisibility($('credentialsDialog'));credentialAction=action;for(const id of ['currentPassword','newPassword','newConfirm','credentialCode'])$(id).value='';const restoringBackup=action==='restore';$('currentPassword').setAttribute('aria-label',restoringBackup?'备份的主密码':'当前主密码');const eye=$('credentialsDialog').querySelector('[data-password-for=currentPassword]');eye.dataset.passwordLabel=restoringBackup?'备份主密码':'当前主密码';setPasswordVisible(eye,false);$('credentialsSubmit').textContent=restoringBackup?'确认替换并恢复':'确认';$('credentialsTitle').textContent=restoringBackup?'恢复加密备份':action==='password'?'修改主密码':action==='2fa-disable'?'关闭验证器':'重新生成恢复码';$('credentialsHelp').textContent=restoringBackup?'此操作会用备份替换当前全部笔记、历史和回收站；会先保留当前完整备份。请输入备份的主密码，以及启用的动态码或恢复码，核对后点击“确认替换并恢复”。':'输入当前主密码'+(twoFA?'和未使用的动态码或恢复码。':'。');$('passwordLabel').firstChild.textContent=restoringBackup?'备份的主密码':'当前主密码';$('newPasswordLabel').hidden=action!=='password';$('newConfirmLabel').hidden=action!=='password';$('credentialCodeLabel').hidden=!restoringBackup&&!twoFA;$('currentPassword').required=true;$('newPassword').required=action==='password';$('newConfirm').required=action==='password';$('credentialsDialog').showModal();$('currentPassword').focus()}
$('unlockForm').onsubmit=e=>{e.preventDefault();if($('unlock').disabled)return;run(async()=>{if(!exists&&$('master').value!==$('masterConfirm').value)throw Error('两次主密码不一致');$('unlock').disabled=true;try{const password=$('master').value;const data=await api(exists?'unlock':'create',{Password:password,Code:$('otp').value});await prepareDraft(data,password);enter(data);await offerRecoveredDraft(data)}finally{$('unlock').disabled=false}})};

$('newNote').onclick=()=>run(async()=>{await flush();const data=await api('save',{Note:{Title:'未命名笔记',Markdown:''}});notes.unshift(data.Note);choose(data.Note);$('noteTitle').focus();$('noteTitle').select()});
$('noteTitle').oninput=changed;$('markdown').oninput=()=>{updateMarkdownColors();changed()};$('searchNotes').oninput=()=>{clearTimeout(searchTimer);searchTimer=setTimeout(()=>{listLimit=100;renderList()},150)};$('showList').onclick=()=>window.hamsterNotesLayout?.toggleList();
$('save').onclick=()=>run(flush);
for(const [id,mode] of [['modeEdit','edit'],['modeSplit','split'],['modePreview','preview']])$(id).onclick=()=>setEditorMode(mode);
$('deleteNote').onclick=()=>run(async()=>{await flush();if(!current||!confirm('将“'+current.Title+'”移入加密回收站？'))return;await api('delete',{Note:current,Confirm:true});notes=notes.filter(n=>n.ID!==current.ID);choose(notes[0]||null);say('已移入加密回收站')});
$('exportMarkdown').onclick=()=>run(async()=>{await flush();if(current&&confirm('导出的 Markdown 是未加密明文，可能包含密码。确认导出？'))download(safeName(current.Title)+'.md',current.Markdown,'text/markdown;charset=utf-8')});
$('lock').onclick=()=>run(async()=>{rememberPosition();await flush();await saveEditorSettings();const data=await api('lock');clearWorkspace();say(data.BackupWarning||'笔记已锁定',Boolean(data.BackupWarning))});
$('security').onclick=()=>{setSecurity();setAutoLock(autoLockMinutes);$('securityDialog').showModal();run(loadBackupStatus)};$('securityClose').onclick=()=>$('securityDialog').close();
$('autoLockChoice').onchange=()=>{$('autoLockCustomRow').hidden=$('autoLockChoice').value!=='custom';if($('autoLockChoice').value==='custom')$('autoLockCustom').focus()};
$('saveAutoLock').onclick=()=>run(async()=>{const choice=$('autoLockChoice').value,minutes=choice==='custom'?Number($('autoLockCustom').value):Number(choice);if(!Number.isInteger(minutes)||minutes<0||minutes>10080||(choice==='custom'&&minutes===0))throw Error('请输入 1–10080 的整数分钟数');$('saveAutoLock').disabled=true;$('autoLockChoice').disabled=true;$('autoLockCustom').disabled=true;try{await flush();await api('auto-lock',{AutoLockMinutes:minutes});say('自动锁定设置已保存')}finally{$('saveAutoLock').disabled=false;$('autoLockChoice').disabled=false;$('autoLockCustom').disabled=false}});

$('backup').onclick=()=>run(async()=>{await flush();await downloadCompleteBackup()});
for(const id of ['restore','restoreLocked'])$(id).onclick=()=>$('backupFile').click();
$('backupFile').onchange=()=>run(async()=>{const file=$('backupFile').files[0];$('backupFile').value='';if(!file)return;if(file.size>512*1024*1024)throw Error('完整加密备份最多 512 MB');await flush();restoring=file;credentials('restore')});
$('credentialsClose').onclick=()=>$('credentialsDialog').close();$('credentialsDialog').addEventListener('close',()=>{resetPasswordVisibility($('credentialsDialog'));for(const id of ['currentPassword','newPassword','newConfirm','credentialCode'])$(id).value='';restoring=null;credentialAction=''});
$('changePassword').onclick=()=>credentials('password');$('disableOTP').onclick=()=>credentials('2fa-disable');$('renewRecovery').onclick=()=>credentials('recovery-renew');
$('credentialsForm').onsubmit=e=>{e.preventDefault();if($('credentialsSubmit').disabled)return;run(async()=>{const action=credentialAction,password=$('currentPassword').value,newPassword=$('newPassword').value;if(action==='password'&&$('newPassword').value!==$('newConfirm').value)throw Error('两次新密码不一致');$('credentialsSubmit').disabled=true;try{await flush();const data=await api(action,{Password:$('currentPassword').value,NewPassword:$('newPassword').value,Code:$('credentialCode').value,Backup:action==='restore'?restoring:undefined,Confirm:action==='restore'});$('credentialsDialog').close();if(action==='restore'){await prepareDraft(data,password);for(const dialog of document.querySelectorAll('dialog[open]:not(#remoteSessionExpired)'))dialog.close();enter(data);await offerRecoveredDraft(data);say('加密备份已导入')}else{if(action==='password')await resetDraftPassword(newPassword);if(action==='2fa-disable')twoFA=false;setSecurity();if(data.RecoveryCodes)showRecovery(data.RecoveryCodes);else say(action==='password'?'主密码已修改，之后请使用新密码；旧备份仍使用旧密码':'验证器已关闭')}}finally{$('credentialsSubmit').disabled=false}})};
$('setupOTP').onclick=()=>run(async()=>{const data=await api('2fa-start');$('otpQR').src='data:image/png;base64,'+data.QR;$('otpSecret').value=data.Secret;$('enrollCode').value='';$('enrollDialog').showModal()});
$('enrollClose').onclick=()=>$('enrollDialog').close();$('enrollDialog').addEventListener('close',()=>{$('otpQR').removeAttribute('src');$('otpSecret').value='';$('enrollCode').value=''});
$('enrollForm').onsubmit=e=>{e.preventDefault();run(async()=>{$('enrollSubmit').disabled=true;try{const data=await api('2fa-enable',{Code:$('enrollCode').value});twoFA=true;setSecurity();$('enrollDialog').close();showRecovery(data.RecoveryCodes)}finally{$('enrollSubmit').disabled=false}})};
$('downloadRecovery').onclick=()=>download('hamster-notes-recovery-codes.txt','仓鼠工具箱私密笔记恢复码（每码限用一次，仍需主密码）\n\n'+$('recoveryCodes').textContent,'text/plain;charset=utf-8');
$('recoverySaved').onchange=()=>$('recoveryClose').disabled=!$('recoverySaved').checked;$('recoveryClose').onclick=()=>$('recoveryDialog').close();$('recoveryDialog').addEventListener('close',()=>{$('recoveryCodes').textContent=''});$('recoveryDialog').addEventListener('cancel',e=>{if(!$('recoverySaved').checked)e.preventDefault()});
$('importMarkdown').onclick=()=>$('markdownFiles').click();$('markdownFiles').onchange=()=>run(async()=>{const files=[...$('markdownFiles').files];$('markdownFiles').value='';if(!files.length)return;if(files.length>200)throw Error('每次最多导入 200 个 Markdown 文件');if(files.some(f=>! /\.(md|markdown)$/i.test(f.name)||f.size>1024*1024)||files.reduce((n,f)=>n+f.size,0)>10*1024*1024)throw Error('请选择 Markdown 文件，每篇最多 1 MB，每次合计最多 10 MB');await flush();const imported=[];for(const f of files)imported.push({Title:f.name.replace(/\.(md|markdown)$/i,''),Markdown:await f.text()});const data=await api('import-markdown',{Notes:imported});notes.push(...data.Notes);choose(data.Notes[0]);say('已导入 '+data.Notes.length+' 篇笔记并加密保存')});
document.addEventListener('keydown',e=>{lastActivity=Date.now();if((e.ctrlKey||e.metaKey)&&e.key.toLowerCase()==='s'&&notesSession){e.preventDefault();run(flush)}});for(const name of ['pointerdown','pointermove','wheel','touchstart'])document.addEventListener(name,()=>{lastActivity=Date.now()},{passive:true});
setInterval(()=>{
 if(!notesSession||autoLockMinutes===0||idleLockPending)return;
 const now=Date.now(),timeout=autoLockMinutes*60*1000;
 if(now-lastActivity>=timeout){idleLockPending=true;run(async()=>{let failed=false,preserved=false;const epoch=notesEpoch;try{await flushBeforeIdleLock()}catch{if(epoch!==notesEpoch){idleLockPending=false;return}failed=true;preserved=await preserveDraft()}try{await api('lock')}catch{}finally{clearWorkspace();idleLockPending=false;say(failed?(preserved?'空闲锁定；未保存内容已保留为加密草稿，解锁后可恢复':'空闲锁定；未保存内容未能保留，请检查磁盘和浏览器存储'):'空闲超时，笔记已锁定',failed)}})}
 else if(lastActivity>lastServerActivity&&now-lastServerActivity>=Math.min(60000,timeout/3)){run(()=>api('keepalive'))}
},5000);
window.addEventListener('beforeunload',e=>{rememberPosition();if(dirty){e.preventDefault();e.returnValue=''}});
window.addEventListener('pagehide',()=>{const keep=rememberNotesResume();if(notesSession&&!keep)fetch('/api/notes/lock',{method:'POST',keepalive:true,headers:{'Content-Type':'application/json','X-Toolbox-Token':ownerToken,'X-Notes-Session':notesSession},body:'{}'}).catch(()=>{});clearWorkspace(false,keep)});
window.addEventListener('pageshow',e=>{if(e.persisted){clearWorkspace(false,true);run(()=>startNotes(true))}});
let backdropDown=null;document.addEventListener('pointerdown',e=>{const d=e.target.closest('dialog');const r=d?.getBoundingClientRect();backdropDown=closeBackdrop&&d&&d.id!=='recoveryDialog'&&d.id!=='draftDialog'&&(e.clientX<r.left||e.clientX>r.right||e.clientY<r.top||e.clientY>r.bottom)?{d,x:e.clientX,y:e.clientY}:null});document.addEventListener('pointercancel',()=>{backdropDown=null});document.addEventListener('click',e=>{const b=backdropDown;backdropDown=null;if(!b||e.target!==b.d||Math.hypot(e.clientX-b.x,e.clientY-b.y)>8)return;const r=b.d.getBoundingClientRect();if(e.clientX>=r.left&&e.clientX<=r.right&&e.clientY>=r.top&&e.clientY<=r.bottom)return;const cancel=new Event('cancel',{cancelable:true});if(b.d.dispatchEvent(cancel))b.d.close()});
fetch('/api/settings',{method:'POST',headers:{'Content-Type':'application/json','X-Toolbox-Token':ownerToken},body:'{}'}).then(r=>r.ok?r.json():{}).then(p=>{closeBackdrop=Boolean(p.CloseDialogOnBackdrop)}).catch(()=>{});
run(startNotes);


// Rich text is a schema-based local editor. Markdown remains the canonical body.
function mountWritingEditor(){
 updateMarkdownColors();
 window.hamsterRichEditor?.destroy();
 const rich=writingMode==='rich'&&Boolean(window.hamsterRichEditor)&&Boolean(current);
 $('richEditor').hidden=!rich;$('markdown').hidden=rich;$('markdownSource').hidden=rich;$('sourceHeading').textContent=rich?'富文本编辑':'Markdown 编辑';
 if(rich)window.hamsterRichEditor.mount($('richEditor'),$('markdown').value,value=>{$('markdown').value=value;updateMarkdownColors();changed()},Boolean(current?.Locked||noteLockPending),id=>attachmentURLs.get(id)?.URL||'');
}
function rememberPosition(){if(!current)return;notePositions.set(current.ID,{Cursor:$('markdown').selectionStart||0,Scroll:Math.round(writingMode==='rich'?$('richEditor').scrollTop:$('markdown').scrollTop)});queueEditorSettings()}
function restorePosition(){const p=notePositions.get(current?.ID);if(!p)return;setTimeout(()=>{if(!current)return;$('markdown').scrollTop=p.Scroll||0;$('richEditor').scrollTop=p.Scroll||0;try{$('markdown').setSelectionRange(p.Cursor||0,p.Cursor||0)}catch{}},0)}
function queueEditorSettings(){clearTimeout(settingsTimer);if(!notesSession)return;settingsTimer=setTimeout(()=>run(saveEditorSettings),600)}
async function saveEditorSettings(){if(!notesSession)return;const valid=new Set(notes.map(n=>n.ID));await api('editor-settings',{EditorSettings:{Mode:writingMode,View:editorMode,Sort:notesSort,Category:categoryFilter,Positions:Object.fromEntries([...notePositions].filter(([id])=>valid.has(id)).slice(-2000))}})}
$('writingMode').onchange=()=>run(async()=>{rememberPosition();writingMode=$('writingMode').value;mountWritingEditor();syncNoteEditing();restorePosition();await saveEditorSettings()});
$('notesSort').onchange=()=>{notesSort=$('notesSort').value;listLimit=100;renderList();queueEditorSettings()};
$('categoryFilter').onchange=()=>{categoryFilter=$('categoryFilter').value;listLimit=100;renderList();queueEditorSettings()};
for(const button of $('formatTools').querySelectorAll('button')){button.onmousedown=e=>e.preventDefault();button.onclick=()=>{
 if(!current||current.Locked||noteLockPending)return;const command=button.dataset.format;
 if(writingMode==='rich'&&window.hamsterRichEditor){window.hamsterRichEditor.command(command);return}
 const field=$('markdown'),start=field.selectionStart,end=field.selectionEnd,selected=field.value.slice(start,end),wrappers={bold:['**','**'],italic:['*','*'],code:['`','`'],h2:['## ',''],bullet:['- ',''],ordered:['1. ',''],quote:['> ',''],table:['\n| 标题 | 标题 |\n| --- | --- |\n| 内容 | 内容 |\n','']};
 if(command==='undo'||command==='redo'){field.focus();document.execCommand?.(command);return}
 const pair=wrappers[command];if(!pair)return;field.focus();field.setSelectionRange(start,end);const replacement=pair[0]+selected+pair[1];if(!document.execCommand?.('insertText',false,replacement))field.setRangeText(replacement,start,end,'select');changed();
 }}
$('openFind').onclick=()=>{$('findBar').hidden=!$('findBar').hidden;if(!$('findBar').hidden)$('findText').focus()};$('closeFind').onclick=()=>$('findBar').hidden=true;
function findNext(){const text=$('findText').value;if(!text)return;let found=false;if(writingMode==='rich'&&window.hamsterRichEditor)found=window.hamsterRichEditor.find(text);else{const field=$('markdown'),source=field.value.toLocaleLowerCase(),query=text.toLocaleLowerCase();let at=source.indexOf(query,field.selectionEnd);if(at<0)at=source.indexOf(query);if(at>=0){field.focus();field.setSelectionRange(at,at+text.length);found=true}}$('findResult').textContent=found?'已定位':'未找到'}
$('findNext').onclick=findNext;$('findText').onkeydown=e=>{if(e.key==='Enter'){e.preventDefault();findNext()}};
$('openNoteDetails').onclick=()=>{if(!current)return;$('notePinned').checked=Boolean(current.Pinned);$('noteCategory').value=current.Category||'';$('noteTags').value=(current.Tags||[]).join(', ');for(const id of ['notePinned','noteCategory','noteTags','saveNoteDetails'])$(id).disabled=Boolean(current.Locked);$('noteDetailsDialog').showModal()};
$('saveNoteDetails').onclick=()=>run(async()=>{if(!current||current.Locked)return;const tags=[...new Set($('noteTags').value.split(/[,，]/).map(t=>t.trim()).filter(Boolean))];if(tags.length>12||tags.some(t=>[...t].length>30))throw Error('最多 12 个标签，每个不超过 30 字');current={...current,Pinned:$('notePinned').checked,Category:$('noteCategory').value.trim(),Tags:tags};changed();await flush();$('noteDetailsDialog').close()});
for(const button of document.querySelectorAll('[data-close]'))button.onclick=()=>$(button.dataset.close).close();
async function loadHistory(){if(!current)return;const id=current.ID,data=await api('history',{Note:{ID:id}});if(current?.ID!==id)return;selectedVersion='';$('restoreVersion').disabled=true;$('historyContent').hidden=true;$('historyContent').value='';$('historyList').replaceChildren();for(const v of data.Versions||[]){const b=document.createElement('button');b.textContent=new Date(v.At).toLocaleString()+' · '+v.Title;b.onclick=()=>run(async()=>{const result=await api('history-read',{Note:{ID:id},VersionID:v.ID});if(current?.ID!==id)return;selectedVersion=v.ID;$('historyContent').hidden=false;$('historyContent').value=result.Version.Note.Markdown;$('restoreVersion').disabled=Boolean(current.Locked)});$('historyList').append(b)}if(!data.Versions?.length)$('historyList').textContent='还没有历史版本。修改内容会自动保留快照，也可以手动保存。';$('snapshotNow').disabled=Boolean(current.Locked)}
$('openHistory').onclick=()=>run(async()=>{await flush();$('historyDialog').showModal();await loadHistory()});
$('snapshotNow').onclick=()=>run(async()=>{await flush();await api('snapshot',{Note:current});await loadHistory();say('已保存当前快照')});
$('restoreVersion').onclick=()=>run(async()=>{if(!current||!selectedVersion||!confirm('恢复此版本？当前已保存内容也会留作历史快照。'))return;await flush();const data=await api('history-restore',{Note:current,VersionID:selectedVersion,Confirm:true});notes=notes.map(n=>n.ID===data.Note.ID?data.Note:n);choose(data.Note);$('historyDialog').close();say('历史版本已恢复')});
async function loadTrash(){const data=await api('trash');$('trashList').replaceChildren();for(const item of data.Trash||[]){const row=document.createElement('div'),text=document.createElement('span'),restore=document.createElement('button'),remove=document.createElement('button');row.className='trash-row';text.textContent=item.Title+' · '+new Date(item.Deleted).toLocaleString();restore.textContent='恢复';remove.textContent='彻底删除';remove.className='danger';restore.onclick=()=>run(async()=>{const result=await api('trash-restore',{Note:{ID:item.ID},Confirm:true});notes.push(result.Note);choose(result.Note);await loadTrash();say('笔记已恢复')});remove.onclick=()=>run(async()=>{if(!confirm('彻底删除这篇笔记及其历史版本？此操作不能撤销，已有备份中的副本不会删除。'))return;await api('trash-delete',{Note:{ID:item.ID},Confirm:true});await loadTrash();say('已彻底删除')});row.append(text,restore,remove);$('trashList').append(row)}if(!data.Trash?.length)$('trashList').textContent='回收站为空'}
$('openTrash').onclick=()=>run(async()=>{await flush();$('trashDialog').showModal();await loadTrash()});
async function loadBackupStatus(){const data=await api('backup-status');const b=data.Settings||{Enabled:true,IntervalMinutes:30,Keep:10};$('autoBackupEnabled').checked=b.Enabled;$('autoBackupDirectory').value=b.Directory||'';$('autoBackupInterval').value=b.IntervalMinutes;$('autoBackupKeep').value=b.Keep;const latest=data.LastBackup&&!data.LastBackup.startsWith('0001-')?new Date(data.LastBackup).toLocaleString():'尚无自动备份';$('backupStatus').textContent=(data.Warning||('最近备份：'+latest+' · '+(data.Count||0)+' 份'))+'\n实际目录：'+(data.Directory||'默认数据目录');}
$('saveBackupSettings').onclick=()=>run(async()=>{const config={Enabled:$('autoBackupEnabled').checked,Directory:$('autoBackupDirectory').value.trim(),IntervalMinutes:Number($('autoBackupInterval').value),Keep:Number($('autoBackupKeep').value)};if(!Number.isInteger(config.IntervalMinutes)||config.IntervalMinutes<1||config.IntervalMinutes>10080||!Number.isInteger(config.Keep)||config.Keep<2||config.Keep>50)throw Error('备份间隔为 1–10080 分钟，保留 2–50 份');await flush();const data=await api('backup-settings',{BackupSettings:config});await loadBackupStatus();say(data.BackupWarning||'自动备份设置已保存',Boolean(data.BackupWarning))});
$('backupNow').onclick=()=>run(async()=>{await flush();await api('backup-now');await loadBackupStatus();say('加密备份已写入备份目录')});

// A domain-separated key from the unlocked vault protects browser drafts if the server session
// expires or its disk is temporarily unwritable. Never persist this key.
function draftStorageKey(id=draftVaultID){return 'hamster.notes.draft.v1.'+id}
async function deriveDraftKey(_password,salt){const data=await api('draft-key',{Draft:{Salt:encodeBytes(salt)}}),raw=decodeBytes(data.Key);try{return await crypto.subtle.importKey('raw',raw,{name:'AES-GCM'},false,['encrypt','decrypt'])}finally{raw.fill(0)}}
async function prepareDraft(data,password){
 draftKey=null;draftSalt=null;draftVaultID=data.VaultID||'';draftEnvelope=null;pendingDraft=null;
 if(!draftVaultID||!crypto?.subtle)return;
 notesSession=data.Session;const candidates=[];
 try{const local=JSON.parse(localStorage.getItem(draftStorageKey())||'null');if(local)candidates.push(local)}catch{}
 try{const stored=await api('draft-get');if(stored.Draft)candidates.push(stored.Draft)}catch{}
 for(const envelope of candidates){try{if(envelope.Version!==2||envelope.VaultID!==draftVaultID)continue;const key=await deriveDraftKey(password,decodeBytes(envelope.Salt));const plain=await crypto.subtle.decrypt({name:'AES-GCM',iv:decodeBytes(envelope.IV),additionalData:new TextEncoder().encode(draftVaultID)},key,decodeBytes(envelope.Ciphertext));const recovered=JSON.parse(new TextDecoder().decode(plain));if(recovered.VaultID===draftVaultID&&typeof recovered.Note?.Markdown==='string'&&recovered.Note.Markdown.length<=1024*1024&&Number.isFinite(recovered.At)&&(!pendingDraft||recovered.At>pendingDraft.At))pendingDraft=recovered}catch{}}
 draftSalt=crypto.getRandomValues(new Uint8Array(16));draftKey=await deriveDraftKey(password,draftSalt);
}
function queueEncryptedDraft(){
 if(!draftKey||!current||!dirty)return draftFlight;
 const key=draftKey,salt=draftSalt,id=draftVaultID,session=notesSession;
 const payload={VaultID:id,At:Date.now(),Note:{...current,Title:$('noteTitle').value,Markdown:$('markdown').value}};
 draftFlight=draftFlight.catch(()=>{}).then(async()=>{const iv=crypto.getRandomValues(new Uint8Array(12)),plain=new TextEncoder().encode(JSON.stringify(payload));const cipher=await crypto.subtle.encrypt({name:'AES-GCM',iv,additionalData:new TextEncoder().encode(id)},key,plain);plain.fill(0);const envelope={Version:2,VaultID:id,Salt:encodeBytes(salt),IV:encodeBytes(iv),Ciphertext:encodeBytes(new Uint8Array(cipher))};let local=false;try{localStorage.setItem(draftStorageKey(id),JSON.stringify(envelope));local=true}catch{}if(notesSession===session&&draftVaultID===id){draftEnvelope=envelope;clearTimeout(draftUploadTimer);draftUploadTimer=setTimeout(()=>uploadDraft(envelope,session).catch(()=>{}),300)}return local});return draftFlight;
}
async function uploadDraft(envelope,session){if(!session)return false;const controller=new AbortController(),timeout=setTimeout(()=>controller.abort(),2000);try{const response=await fetch('/api/notes/draft-put',{method:'POST',cache:'no-store',headers:{'Content-Type':'application/json','X-Toolbox-Token':ownerToken,'X-Notes-Session':session},body:JSON.stringify({Draft:envelope}),signal:controller.signal});return response.ok}finally{clearTimeout(timeout)}}
async function preserveDraft(){if(!dirty||!current)return true;if(!draftKey)return false;try{const local=await queueEncryptedDraft();clearTimeout(draftUploadTimer);let server=false;try{if(draftEnvelope)server=await uploadDraft(draftEnvelope,notesSession)}catch{}return local||server}catch{return false}}
async function offerRecoveredDraft(data){if(!pendingDraft)return;const draft=pendingDraft.Note,stored=notes.find(n=>n.ID===draft.ID);if(stored&&stored.Title===draft.Title&&stored.Markdown===draft.Markdown&&JSON.stringify(stored.Tags||[])===JSON.stringify(draft.Tags||[])&&Boolean(stored.Pinned)===Boolean(draft.Pinned)&&(stored.Category||'')===(draft.Category||'')){pendingDraft=null;return}$('draftDescription').textContent=(draft.Title||'未命名笔记')+' · '+new Date(pendingDraft.At).toLocaleString();$('draftContent').value=draft.Markdown;$('draftDialog').showModal()}
$('draftDialog').addEventListener('cancel',e=>e.preventDefault());
$('recoverDraft').onclick=()=>run(async()=>{if(!pendingDraft)return;const note={...pendingDraft.Note,ID:'',Revision:0,Locked:false,Title:('恢复草稿：'+(pendingDraft.Note.Title||'未命名笔记')).slice(0,200)};const data=await api('save',{Note:note});notes.push(data.Note);choose(data.Note);await discardEncryptedDraft();say('草稿已恢复为新笔记')});
async function discardEncryptedDraft(){try{localStorage.removeItem(draftStorageKey())}catch{}await api('draft-clear');pendingDraft=null;draftEnvelope=null;$('draftContent').value='';$('draftDialog').close()}
$('discardDraft').onclick=()=>run(async()=>{if(confirm('确认放弃这份未保存的草稿？'))await discardEncryptedDraft()});
async function resetDraftPassword(password){if(!draftVaultID||!crypto?.subtle)return;await draftFlight.catch(()=>{});clearTimeout(draftUploadTimer);try{localStorage.removeItem(draftStorageKey())}catch{}await api('draft-clear');draftSalt=crypto.getRandomValues(new Uint8Array(16));draftKey=await deriveDraftKey(password,draftSalt);draftEnvelope=null;pendingDraft=null;}

document.addEventListener('keydown',e=>{if((e.ctrlKey||e.metaKey)&&e.key.toLowerCase()==='f'&&notesSession&&!document.querySelector('dialog[open]')){e.preventDefault();$('findBar').hidden=false;$('findText').focus()}if(e.key==='Escape'&&!$('findBar').hidden)$('findBar').hidden=true});

async function flushBeforeIdleLock(){let timer;try{await Promise.race([flush(),new Promise((_,reject)=>{timer=setTimeout(()=>reject(Error('保存等待超时')),2500)})])}finally{clearTimeout(timer)}}


function clearAttachmentViews(){attachmentGeneration++;for(const entry of attachmentURLs.values())URL.revokeObjectURL(entry.URL);attachmentURLs.clear();attachmentBytes=0;$('noteAttachments').replaceChildren();$('noteAttachments').hidden=true;}
function noteImageMime(mime){return /^image\/(png|jpeg|gif|webp|bmp|avif)$/.test(mime||'')}
function previewWithAttachments(html){const doc=new DOMParser().parseFromString(html,'text/html');for(const image of doc.querySelectorAll('img[data-note-attachment]')){const entry=attachmentURLs.get(image.dataset.noteAttachment);if(entry){image.src=entry.PreviewURL;image.className='notes-attachment-preview'}}return '<!doctype html>'+doc.documentElement.outerHTML}
async function fetchNoteAttachment(note,id){const data=await api('attachment-read',{Note:{ID:note.ID},AttachmentID:id});const bytes=decodeBytes(data.Content);return {Attachment:data.Attachment,Blob:new Blob([bytes],{type:data.Attachment.Mime||'application/octet-stream'})}}
async function showAttachmentImage(note,a,generation=attachmentGeneration){
 if(attachmentURLs.has(a.ID))return;const result=await fetchNoteAttachment(note,a.ID);if(generation!==attachmentGeneration||current?.ID!==note.ID)return;
 while(attachmentURLs.size>=8||attachmentBytes+result.Blob.size>64*1024*1024){const first=attachmentURLs.keys().next().value;if(first==null)break;const entry=attachmentURLs.get(first);URL.revokeObjectURL(entry.URL);attachmentBytes-=entry.Size;attachmentURLs.delete(first)}
 const previewURL=await noteImagePreviewURL(result.Blob);if(generation!==attachmentGeneration||current?.ID!==note.ID)return;attachmentURLs.set(a.ID,{URL:URL.createObjectURL(result.Blob),PreviewURL:previewURL,Size:result.Blob.size});attachmentBytes+=result.Blob.size;window.hamsterRichEditor?.refreshAttachments();if(editorMode!=='edit')await renderPreview();
}
async function loadAttachmentViews(note){
 clearAttachmentViews();if(!note?.Attachments?.length)return;const generation=attachmentGeneration,fragment=document.createDocumentFragment();$('noteAttachments').hidden=false;
 for(const a of note.Attachments){const row=document.createElement('div'),name=document.createElement('span'),downloadButton=document.createElement('button');row.className='attachment-card';name.textContent=a.Name+' · '+(a.Size/1024/1024>=1?(a.Size/1024/1024).toFixed(1)+' MB':Math.ceil(a.Size/1024)+' KB');downloadButton.textContent='下载';downloadButton.title='下载解密后的文件';downloadButton.onclick=()=>run(async()=>{const data=await fetchNoteAttachment(note,a.ID);if(current?.ID!==note.ID)return;download(a.Name,data.Blob,'application/octet-stream');say('已下载解密后的文件，请妥善保管')});row.append(name);
 if(noteImageMime(a.Mime)){const view=document.createElement('button');view.textContent='查看图片';view.onclick=()=>run(()=>showAttachmentImage(note,a,generation));row.append(view)}
 row.append(downloadButton);if(!note.Locked){const remove=document.createElement('button');remove.textContent='移除';remove.title='历史版本和已有备份中的附件仍会保留';remove.onclick=()=>run(async()=>{if(noteLockPending||!confirm('从当前笔记移除此附件？历史版本和已有备份中的副本仍会保留。'))return;await flush();const data=await api('attachment-remove',{Note:current,AttachmentID:a.ID});notes=notes.map(n=>n.ID===data.Note.ID?data.Note:n);choose(data.Note)});row.append(remove)}fragment.append(row)}
 $('noteAttachments').append(fragment);
 let bytes=0,count=0;for(const a of note.Attachments){if(generation!==attachmentGeneration)break;if(noteImageMime(a.Mime)&&a.Size<=5*1024*1024&&bytes+a.Size<=20*1024*1024&&count<8){bytes+=a.Size;count++;try{await showAttachmentImage(note,a,generation)}catch(e){if(generation===attachmentGeneration)say('图片附件未能加载，可点击重试：'+a.Name,true)}}}
}
function attachmentLabel(name){return name.replace(/[\r\n]/g,' ').replace(/([\\\[\]])/g,'\\$1')}
async function addNoteAttachments(files){
 files=[...files];if(!files.length||!current||current.Locked||noteLockPending)return;if(files.length>20)throw Error('每次最多添加 20 个附件');if(files.some(f=>f.size>25*1024*1024))throw Error('每个附件最多 25 MB');
 await flush();const id=current.ID;noteLockPending=true;syncNoteEditing();
 try{for(let i=0;i<files.length;i++){const file=files[i];say('正在加密附件 '+(i+1)+' / '+files.length+'：'+file.name);const data=await api('attachment-add',{Note:current,Name:file.name,Content:encodeBytes(new Uint8Array(await file.arrayBuffer()))});if(current?.ID!==id)throw Error('当前笔记已切换');current=data.Note;notes=notes.map(n=>n.ID===id?data.Note:n);const a=current.Attachments.at(-1),field=$('markdown'),text='\n\n'+(noteImageMime(a.Mime)?'!':'')+'['+attachmentLabel(a.Name)+'](attachment:'+a.ID+')\n';if(writingMode==='rich'&&window.hamsterRichEditor)window.hamsterRichEditor.insertAttachment(a.ID,a.Name,noteImageMime(a.Mime));else{const source=field.value,start=field.selectionStart??source.length;field.setRangeText(text,start,start,'end')}dirty=true;editVersion++;queueEncryptedDraft();await flush();}
 mountWritingEditor();await loadAttachmentViews(current);say('已加密保存 '+files.length+' 个附件')
 }finally{noteLockPending=false;syncNoteEditing()}
}
$('addAttachment').onclick=()=>$('attachmentFiles').click();$('attachmentFiles').onchange=()=>{const files=[...$('attachmentFiles').files];$('attachmentFiles').value='';run(()=>addNoteAttachments(files))};
document.addEventListener('paste',e=>{if(!e.target.closest?.('#editorBody')||!e.clipboardData?.files?.length)return;e.preventDefault();e.stopPropagation();run(()=>addNoteAttachments(e.clipboardData.files))},true);
$('editorBody').addEventListener('dragover',e=>{if(e.dataTransfer?.types?.includes('Files'))e.preventDefault()});$('editorBody').addEventListener('drop',e=>{if(!e.dataTransfer?.files?.length)return;e.preventDefault();e.stopPropagation();run(()=>addNoteAttachments(e.dataTransfer.files))},true);
async function downloadCompleteBackup(){
 const response=await fetch('/api/notes/export-file',{method:'POST',cache:'no-store',headers:{'Content-Type':'application/json','X-Toolbox-Token':ownerToken,'X-Notes-Session':notesSession},body:'{}'});if(!response.ok)throw Error((await response.text()).trim()||'导出备份失败');const blob=await response.blob();if(response.headers.get('X-Notes-Backup-Format')==='zip'){const tail=new Uint8Array(await blob.slice(-22).arrayBuffer());if(tail.length!==22||tail[0]!==80||tail[1]!==75||tail[2]!==5||tail[3]!==6)throw Error('备份下载不完整，请检查附件文件后重试')}
 download('hamster-notes-backup.hamster-notes',blob,'application/octet-stream');say('已导出完整加密备份，包含笔记和附件，请单独妥善保存')
}

async function noteImagePreviewURL(blob){
 if(typeof createImageBitmap==='function'){let bitmap;try{bitmap=await createImageBitmap(blob);const scale=Math.min(1,960/bitmap.width,600/bitmap.height),canvas=document.createElement('canvas');canvas.width=Math.max(1,Math.round(bitmap.width*scale));canvas.height=Math.max(1,Math.round(bitmap.height*scale));canvas.getContext('2d').drawImage(bitmap,0,0,canvas.width,canvas.height);return canvas.toDataURL('image/webp',.85)}catch{}finally{bitmap?.close()}}
 return await new Promise((resolve,reject)=>{const reader=new FileReader();reader.onload=()=>resolve(reader.result);reader.onerror=()=>reject(Error('图片预览生成失败'));reader.readAsDataURL(blob)});
}

document.addEventListener('keydown',e=>{if(writingMode==='markdown'&&e.target===$('markdown')&&(e.ctrlKey||e.metaKey)&&['b','i'].includes(e.key.toLowerCase())){e.preventDefault();$('formatTools').querySelector('[data-format='+ (e.key.toLowerCase()==='b'?'bold':'italic') +']').click()}});

// Source colors never alter the canonical textarea value or selection.
function updateMarkdownColors(){
 const field=$('markdown'),host=$('markdownColors'),enabled=Boolean(window.hamsterNoteSyntax)&&!(writingMode==='rich'&&window.hamsterRichEditor)&&field.value.length<=200000;
 $('markdownSource').classList.toggle('source-colored',enabled);
 if(enabled)window.hamsterNoteSyntax.paintMarkdown(host,field.value);else host.replaceChildren();
 host.scrollTop=field.scrollTop;host.scrollLeft=field.scrollLeft;
}
$('markdown').addEventListener('scroll',()=>{const host=$('markdownColors');host.scrollTop=$('markdown').scrollTop;host.scrollLeft=$('markdown').scrollLeft},{passive:true});

let notesScrollFrame=0;
function syncNotesPreviewScroll(){
 if(notesScrollFrame)return;notesScrollFrame=(window.requestAnimationFrame||((fn)=>setTimeout(fn,16)))(()=>{notesScrollFrame=0;if(!current||editorMode!=='split')return;
 const rich=writingMode==='rich'&&Boolean(window.hamsterRichEditor),field=rich?$('richEditor'):$('markdown'),max=Math.max(0,field.scrollHeight-field.clientHeight),ratio=max?Math.min(1,field.scrollTop/max):0;
 let anchor=rich?window.hamsterRichEditor.scrollAnchor($('markdown').value):null;
 if(!anchor){const lines=Array.from($('markdownColors').children);let low=0,high=lines.length;while(low<high){const mid=(low+high)>>1;if(lines[mid].offsetTop<=field.scrollTop+10)low=mid+1;else high=mid}const index=Math.max(0,low-1),height=parseFloat(getComputedStyle($('markdown')).lineHeight)||23;anchor=lines.length?{line:index+Math.max(0,(field.scrollTop+10-lines[index].offsetTop)/height),fraction:0}:{line:ratio*Math.max(0,$('markdown').value.split('\n').length-1),fraction:0}}
 $('preview').contentWindow?.postMessage({type:'hamster-note-scroll',...anchor,ratio},'*');
 });
}
for(const id of ['markdown','richEditor'])$(id).addEventListener('scroll',syncNotesPreviewScroll,{passive:true});
$('preview').addEventListener('load',syncNotesPreviewScroll);
window.addEventListener('message',event=>{if(event.source===$('preview').contentWindow&&event.data?.type==='hamster-note-preview-ready')syncNotesPreviewScroll()});
