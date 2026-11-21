'use strict';
let previewPagingAbort=null,previewPageBusy=false,resourceClipboard=null,transferAction='move',transferController=null;
const uploadRelativePaths=new WeakMap();
function canPreviewPage(step){return step<0?offset>0:offset+rows.length<total}
async function previewBeyondPage(step){
 if(previewPageBusy||!canPreviewPage(step))return;
 const item=rows[previewIndex],kind=item?.Kind;if(!kind)return;
 const id=previewRequest,currentRoot=root,currentPath=path,controller=new AbortController();previewPagingAbort=controller;previewPageBusy=true;
 $('viewerPrevious').disabled=$('viewerNext').disabled=true;$('viewerHint').textContent='正在读取'+(step>0?'下一':'上一')+'页…';
 try{
  for(let next=offset+step*pageSize;next>=0&&next<total;next+=step*pageSize){
   const data=await api('list',{Path:currentPath,Search:$('search').value,Sort:$('sort').value,Order:$('order').value,Recursive:$('searchRecursive').checked,PageSize:pageSize,Offset:next},currentRoot,controller.signal);
   if(controller.signal.aborted||id!==previewRequest||root!==currentRoot||path!==currentPath)return;
   const candidates=data.Entries||[];let index=step>0?candidates.findIndex(x=>!x.Dir&&x.Kind===kind):candidates.findLastIndex(x=>!x.Dir&&x.Kind===kind);
   if(index<0)continue;
   rows=candidates;total=data.Total;offset=data.Offset??next;selection.clear();render();rememberTab();persistTabs();$('page').textContent=Math.floor(offset/pageSize)+1+' / '+Math.ceil(total/pageSize);$('previous').disabled=offset===0;$('next').disabled=offset+rows.length>=total;
   previewPagingAbort=null;await preview(index);return;
  }
  $('viewerHint').textContent='已经是'+(step>0?'最后':'第一')+'个'+(kind==='video'?'视频':kind==='image'?'图片':'同类文件')+'。';
 }catch(e){if(e.name!=='AbortError'&&id===previewRequest)$('viewerHint').textContent='读取失败：'+e.message}
 finally{previewPageBusy=false;if(previewPagingAbort===controller)previewPagingAbort=null;if($('viewer').open&&id===previewRequest){$('viewerPrevious').disabled=step<0||previewNeighbor(-1)<0&&!canPreviewPage(-1);$('viewerNext').disabled=step>0||previewNeighbor(1)<0&&!canPreviewPage(1)}}
}
function playbackBookmarkKey(item,itemRoot){return 'hamster.playback.'+JSON.stringify([local?'local':itemRoot,item.Path,item.Size,item.Modified||''])}
function installPlaybackBookmark(media,playback,item,itemRoot,id){
 if(!window.toolboxSettings?.RememberPlayback)return;
 const key=playbackBookmarkKey(item,itemRoot);let at=0,lastWrite=0;
 try{at=Math.max(0,Number(localStorage.getItem(key))||0)}catch{}
 media.dataset.resumeAt=String(at);
 media.addEventListener('loadedmetadata',()=>{if(id!==previewRequest||!at||!media.src.includes('/raw?'))return;if(at<media.duration-3)media.currentTime=at;at=0;media.dataset.resumeAt='0'});
 media.addEventListener('timeupdate',()=>{if(id!==previewRequest||Date.now()-lastWrite<2500)return;lastWrite=Date.now();const p=playback.position();try{if(p.duration&&p.time>=p.duration-3)localStorage.removeItem(key);else localStorage.setItem(key,String(p.time));const keys=Object.keys(localStorage).filter(k=>k.startsWith('hamster.playback.'));while(keys.length>500)localStorage.removeItem(keys.shift())}catch{}});
}
function showToolboxLauncher(folder){
 if(!local||!folder)return;
 $('toolboxLaunchPath').textContent=folder;$('toolboxLaunchTools').replaceChildren();
 for(const [id,name]of [['format','目录统计命名'],['rename','批量重命名'],['number','目录批量编号'],['describe','媒体信息命名'],['exif','清理元数据'],['pdf','PDF 图册索引'],['image','压缩图片'],['video','压缩视频'],['thumbnail','视频缩略图'],['archive','打包资源'],['extract','解压资源']]){
  const b=document.createElement('a'),url=new URL(boot.ToolboxURL,location.href);url.searchParams.set('tool',id);url.searchParams.set('root',folder);b.href=url.href;b.className='button-link';b.textContent=name;$('toolboxLaunchTools').append(b)
 }$('toolboxLaunchDialog').showModal();
}
function openArchiveExtraction(item){
 if(!local||!item||item.Dir||item.Kind!=='archive'||!item.Version||loading||mutating)return;
 const at=Math.max(item.Path.lastIndexOf('\\'),item.Path.lastIndexOf('/'));if(at<0)return;
 const url=new URL(boot.ToolboxURL,location.href);url.searchParams.set('tool','extract');url.searchParams.set('root',item.Path.slice(0,at+1));url.searchParams.set('extract-source',item.Path);
 window.open(url.href,'_blank','noopener');
}
function canPasteResources(target=path){return Boolean(resourceClipboard?.items.length&&resourceClipboard.root===root&&writable()&&(target||!local&&root)&&!loading&&!mutating)}
function syncResourceClipboard(){
 const items=selected(),stable=items.length>0&&items.every(item=>item.Version&&item.Kind!=='drive'),ready=!loading&&!mutating;
 $('copy').hidden=$('cut').hidden=!items.length;$('copy').disabled=$('cut').disabled=!stable||!ready||!writable();
 $('paste').disabled=!canPasteResources();
 const clip=resourceClipboard,label=clip?.action==='move'?'已剪切':'已复制';
 $('clipboardStatus').hidden=!clip;$('clipboardStatus').textContent=clip?label+' '+clip.items.length+' 项':'';
 $('paste').title=clip?'将'+(clip.action==='move'?'剪切':'复制')+'的 '+clip.items.length+' 项放到当前目录：'+clip.items.slice(0,3).map(item=>item.Name).join('、'):'先选中文件或目录，点击复制或剪切';
 $('paste').classList.toggle('clipboard-ready',!$('paste').disabled);
}
function copySelectedResources(action){
 if(loading||mutating)return;
 const items=selected();
 if(!items.length){say('请先选中要'+(action==='move'?'剪切':'复制')+'的文件或目录。',true);return}
 if(items.some(item=>!item.Version||item.Kind==='drive')){say('所选项目尚不能安全复制或剪切，请刷新目录后重新选择；不能复制整个磁盘。',true);return}
 if(!writable()){say('此共享为只读，不能在当前共享内剪切或粘贴。',true);return}
 resourceClipboard={action,root,items:items.map(x=>({...x}))};syncResourceClipboard();say('已'+(action==='move'?'剪切':'复制')+' '+items.length+' 项；进入目标目录后点击顶部“粘贴”，或右键目录粘贴。')
}
function openTransferPicker(action){transferAction=action;$('moveDialog').querySelector('h2').textContent=action==='copy'?'选择复制目标文件夹':'选择移动目标文件夹';$('moveSave').textContent=action==='copy'?'复制到这里':'移动到这里';$('moveDialog').showModal();browseMove(path)}
function chooseTransferConflict(item){
 return new Promise(resolve=>{
  const dialog=document.createElement('dialog');dialog.className='transfer-conflict-dialog';dialog.setAttribute('aria-label','目标存在同名项目');
  const heading=document.createElement('h2');heading.textContent='目标存在同名项目';const name=document.createElement('p');name.textContent=item.Name;
  const label=document.createElement('label');label.textContent='如何处理？';const select=document.createElement('select');select.id='transferConflictPolicy';select.setAttribute('aria-label','同名处理方式');
  for(const [value,text]of [['rename','保留两份（自动改名）'],['skip','合并目录，跳过同名文件'],['overwrite','合并目录，替换同名文件并备份原文件']]){const option=document.createElement('option');option.value=value;option.textContent=text;select.append(option)}
  label.append(select);const hint=document.createElement('p');hint.className='hint';hint.textContent='只处理源目录中存在的文件，目标独有文件保留。替换前校验新副本，原文件保存在目标旁的 .toolbox-replaced-… 目录。此选择用于本次剩余传输中的同名项目。';
  let choice=null;const actions=document.createElement('div');actions.className='dialog-actions';actions.append(button('取消',()=>dialog.close()),button('继续',()=>{choice=select.value;dialog.close()},'primary'));
  dialog.append(heading,name,label,hint,actions);dialog.addEventListener('close',()=>{dialog.remove();resolve(choice)},{once:true});document.body.append(dialog);dialog.showModal();
 });
}
async function streamTransfer(item,target,action,requestRoot,signal,conflict=''){
 const response=await fetch(endpoint('api/transfer',{},requestRoot),{method:'POST',credentials:'same-origin',headers:{'Content-Type':'application/json','X-Hamster-Browser':'1'},body:JSON.stringify({Action:action,Path:item.Path,Target:target,Version:item.Version,Conflict:conflict}),signal});
 if(response.status===409&&!conflict){const error=Error('目标存在同名项目');error.conflict=true;throw error}
 if(!response.ok)throw Error((await response.text()).trim());const reader=response.body.getReader(),decoder=new TextDecoder();let pending='',finished=false;
 let result={};const consume=line=>{if(!line.trim())return;const data=JSON.parse(line);if(data.Error)throw Error(data.Error);if(data.Total>0){$('uploadBar').value=data.Done/data.Total*100;if(data.Done===data.Total&&!data.Finished)say('正在校验副本：'+item.Name+'；校验成功后才发布并处理源文件。')}if(data.Finished){finished=true;result=data}};
 try{while(true){const {value,done}=await reader.read();pending+=decoder.decode(value||new Uint8Array(),{stream:!done});const lines=pending.split('\n');pending=lines.pop();for(const line of lines)consume(line);if(done)break}if(pending)consume(pending);if(!finished)throw Error('传输未完成，请核对目标目录。');return result}finally{if(!finished)await reader.cancel?.().catch(()=>{});reader.releaseLock()}
}
async function transferResources(items,target,action,requestRoot=root){
 if(mutating||!items.length)return;mutating=true;transferController=new AbortController();actions();$('uploadProgress').hidden=false;let completed=0,message='',bad=false,conflict='',skipped=0;const backups=[];
 try{for(const item of items){say((action==='copy'?'正在复制：':'正在移动：')+item.Name+' · '+(completed+1)+'/'+items.length);$('uploadBar').value=0;let result;
  try{result=await streamTransfer(item,join(target,item.Name),action,requestRoot,transferController.signal,conflict)}catch(e){if(!e.conflict)throw e;conflict=await chooseTransferConflict(item);if(!conflict)throw new DOMException('cancelled','AbortError');result=await streamTransfer(item,join(target,item.Name),action,requestRoot,transferController.signal,conflict)}
  if(result.Backup)backups.push(result.Backup);skipped+=result.Skipped||0;completed++}message='传输完成：'+completed+' 项'+(skipped?'；跳过 '+skipped+' 个同名文件，移动时这些源文件保留':'')+(backups.length?'；原文件备份：'+backups.join('；'):'');if(resourceClipboard?.action==='move'&&!skipped)resourceClipboard=null}
 catch(e){message='传输未全部完成：已完成 '+completed+' 项；'+(e.name==='AbortError'?'已取消，已完成的副本保留。':e.message);bad=true}
 finally{mutating=false;transferController=null;$('uploadProgress').hidden=true;await navigate(path,{remember:false,page:offset,force:true});say(message,bad);actions()}
}
function pasteSelectedResources(target=path){
 if(!resourceClipboard){say('请先复制或剪切文件，再点击粘贴。',true);return}
 if(resourceClipboard.root!==root){say('请在同一共享范围内粘贴；本机可跨盘复制。',true);return}
 if(!canPasteResources(target)){say('请先打开可修改的目标目录，等待当前操作完成后再粘贴。',true);return}
 return transferResources(resourceClipboard.items,target,resourceClipboard.action)
}
async function sampleUploadFile(file){const first=new Uint8Array(await file.slice(0,65536).arrayBuffer()),last=new Uint8Array(await file.slice(Math.max(65536,file.size-65536)).arrayBuffer());let binary='';for(const data of [first,last])for(let i=0;i<data.length;i+=8192)binary+=String.fromCharCode(...data.subarray(i,i+8192));return btoa(binary)}
async function uploadResumableFiles(files,directories=[]){
 if(mutating||!files.length&&!directories.length)return;mutating=true;const folder=path,itemRoot=root;transferController=new AbortController();actions();$('uploadProgress').hidden=false;let completed=0,failed=[];
 try{for(const directory of directories){await api('upload-init',{Path:folder,RelativePath:directory,Directory:true,Size:0,Sample:''},itemRoot,transferController.signal)}for(const file of files){
  if(transferController.signal.aborted)throw new DOMException('cancelled','AbortError');const relative=uploadRelativePaths.get(file)||file.webkitRelativePath||file.name;say('正在上传 '+relative+' · '+(completed+failed.length+1)+'/'+files.length);
  try{const session=await api('upload-init',{Path:folder,RelativePath:relative,Size:file.size,Modified:file.lastModified||0,Sample:await sampleUploadFile(file)},itemRoot,transferController.signal);let offset=session.Offset;if(typeof session.ID!=='string'||!Number.isInteger(offset)||offset<0||offset>file.size)throw Error('上传响应无效，请重新选择文件续传');
   while(offset<file.size){const start=offset,chunk=file.slice(start,Math.min(file.size,start+4*1024*1024));offset=await new Promise((resolve,reject)=>{const xhr=new XMLHttpRequest();uploadRequest=xhr;xhr.open('POST',endpoint('api/upload-chunk',{id:session.ID,offset:start},itemRoot));xhr.setRequestHeader('X-Hamster-Browser','1');xhr.upload.onprogress=e=>{if(e.lengthComputable)$('uploadBar').value=(start+e.loaded)/Math.max(1,file.size)*100};xhr.onload=()=>{if(xhr.status!==200){reject(Error(xhr.responseText||'本段上传失败'));return}try{resolve(JSON.parse(xhr.responseText).Offset)}catch{reject(Error('上传响应无效'))}};xhr.onerror=()=>reject(Error('网络中断，请重新选择相同文件续传'));xhr.onabort=()=>reject(new DOMException('cancelled','AbortError'));xhr.send(chunk)})}
   const finish=await fetch(endpoint('api/upload-finish',{id:session.ID},itemRoot),{method:'POST',credentials:'same-origin',headers:{'Content-Type':'application/json','X-Hamster-Browser':'1'},body:'{}',signal:transferController.signal});if(!finish.ok)throw Error((await finish.text()).trim());
   completed++;
  }catch(e){if(e.name==='AbortError')throw e;failed.push(relative+'：'+e.message);if(/网络|Failed to fetch|NetworkError/.test(e.message))throw e}
 }}catch(e){if(e.name==='AbortError')failed.push('已取消，断点保留 7 天；重新选择相同文件或文件夹继续。');else failed.push(e.message)}
 finally{mutating=false;uploadRequest=null;transferController=null;$('uploadProgress').hidden=true;$('uploadInput').value='';$('uploadFolderInput').value='';await navigate(folder,{remember:false,force:true});say('上传完成 '+completed+' 项'+(failed.length?'；未完成 '+failed.length+' 项：'+failed.slice(0,5).join('；')+(failed.length>5?'；其余项目请重新选择后续传':''):''),Boolean(failed.length));actions()}
}
function initializeExplorerUpgrades(){
 $('copy').onclick=()=>copySelectedResources('copy');$('cut').onclick=()=>copySelectedResources('move');$('paste').onclick=()=>pasteSelectedResources();syncResourceClipboard();
 $('cancelSearch').onclick=()=>{navigationAbort?.abort();say('已取消读取；当前目录保留。')};$('searchRecursive').onchange=()=>navigate(path,{remember:false,force:true});
 $('toolboxLaunchClose').onclick=()=>$('toolboxLaunchDialog').close();
 $('copyTo').onclick=()=>openTransferPicker('copy');$('move').onclick=()=>openTransferPicker('move');
 $('moveSave').onclick=async()=>{if(!moveReady||mutating)return;const items=selected(),target=movePath;$('moveDialog').close();await transferResources(items,target,transferAction)};
 $('cancelUpload').onclick=()=>{transferController?.abort();uploadRequest?.abort()};
 $('uploadInput').onchange=()=>uploadResumableFiles([...$('uploadInput').files]);$('uploadFolder').onclick=()=>$('uploadFolderInput').click();$('uploadFolderInput').onchange=()=>uploadResumableFiles([...$('uploadFolderInput').files]);
 $('entries').addEventListener('dragover',event=>{if(writable()&&path&&!mutating)event.preventDefault()});$('entries').addEventListener('drop',async event=>{
  if(!writable()||!path||mutating)return;event.preventDefault();const originalPath=path,originalRoot=root,files=[],directories=[];
  const collect=async(entry,prefix='')=>{const relative=prefix+entry.name;if(entry.isFile){const file=await new Promise((resolve,reject)=>entry.file(resolve,reject));uploadRelativePaths.set(file,relative);files.push(file)}else if(entry.isDirectory){directories.push(relative);const reader=entry.createReader();for(;;){const entries=await new Promise((resolve,reject)=>reader.readEntries(resolve,reject));if(!entries.length)break;for(const child of entries)await collect(child,relative+'/')}}};
  try{const entries=[...(event.dataTransfer?.items||[])].map(item=>item.webkitGetAsEntry?.()).filter(Boolean);if(entries.length){for(const entry of entries)await collect(entry)}else files.push(...(event.dataTransfer?.files||[]));if(path!==originalPath||root!==originalRoot){say('目标目录已变化，请重新拖入文件。',true);return}await uploadResumableFiles(files,directories)}catch(e){say('读取拖入的文件夹失败：'+e.message,true)}
 });
}
