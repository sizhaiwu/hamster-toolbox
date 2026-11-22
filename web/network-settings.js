// Shared by Toolbox, Explorer and Notes. No guest credentials grant configuration access.
(()=>{
 const $=id=>document.getElementById(id);
 const button=(text,onclick,className='')=>{const b=document.createElement('button');b.type='button';b.textContent=text;b.onclick=onclick;b.className=className;return b};
 const sameBrowserPath=(a,b)=>{const key=p=>{let s=p.replace(/\\/g,'/').replace(/\/+$/,'');return /^[a-z]:|^\/\//i.test(s)?s.toLowerCase():s};return key(a)===key(b)};
 let local=false,access=null,path='',shareRoots=[],shareLimit=128,openRevision=0;
 let sharePickerState={path:'',parent:'',offset:0,total:0,selected:new Set(),revision:0};
 const api=async(route,body={})=>{if(!access)throw Error('设置连接尚未就绪');if(route==='boot')return {Sharing:await access.read()};if(route==='sharing')return access.save(body);if(route==='list')return access.list(body);throw Error('未知设置请求')};
function renderShare(status){
  $('copyDialog').querySelector('h2').textContent='访问地址';$('copyDialog').querySelector('p').textContent='复制此地址到同一内网设备的浏览器。';
  shareRoots=structuredClone(status.Roots||[]);
  shareLimit=status.MaxRoots||128;$('shareEnabled').checked=status.StartupEnabled??status.Enabled;
  $('sharePort').value=status.Port;$('shareCode').value='';$('workspaceEnabled').checked=Boolean(status.WorkspaceEnabled);$('workspaceHTTP').checked=Boolean(status.WorkspaceHTTP);$('workspacePassword').value='';$('workspacePasswordState').textContent=status.WorkspacePasswordSet?'已设置管理员口令；留空保留。':'首次开启需要设置独立管理员口令。';
  $('shareOverlay').checked=Boolean(status.OverlayEnabled);$('sharePublicURL').value=status.PublicURL||'';$('shareProxies').value=(status.TrustedProxies||[]).join(', ');
  $('passwordState').textContent=status.PasswordSet?'已设置口令；留空保留。修改共享后，已登录设备需要重新登录。':'尚未设置访问口令。';
  drawShareRoots(status.Unavailable||{});$('shareURLs').replaceChildren();
  $('shareStatus').textContent=status.Enabled?'共享运行中 · 下次启动工具箱会自动开启':status.StartupEnabled?'自动共享已保存 · 当前未运行':'共享已关闭 · 下次启动保持关闭';
  $('shareStatus').classList.toggle('error',Boolean(status.Error));
  $('shareError').textContent=status.Error||'';
  if(status.Enabled&&!(status.URLs||[]).length){const p=document.createElement('p');p.textContent='共享已开启，暂未发现内网IPv4地址，请检查网卡或Wi-Fi。';$('shareURLs').append(p)}
  for(const url of [...new Set([...(status.URLs||[]),...(status.Enabled&&status.PublicURL?[status.PublicURL+'/']:[])])]){const row=document.createElement('div');row.className='share-url';const link=document.createElement('a');link.href=url;link.textContent=url;link.target='_blank';link.rel='noopener noreferrer';if(status.WorkspaceEnabled){const admin=document.createElement('a');admin.href=new URL('/admin/',url).href;admin.textContent='完整工具箱登录';admin.target='_blank';admin.rel='noopener noreferrer';row.append(admin)}row.append(link,button('复制地址',async()=>{try{await navigator.clipboard.writeText(url)}catch{$('copyValue').value=url;$('copyDialog').showModal();$('copyValue').select()}}));$('shareURLs').append(row)}
}
function drawShareRoots(unavailable={}){const container=$('shareRoots');container.replaceChildren();for(const [index,item]of shareRoots.entries()){const row=document.createElement('div');row.className='share-root';const body=document.createElement('div');const label=document.createElement('label');label.textContent='显示名称';const input=document.createElement('input');input.type='text';input.value=item.Name||'';input.maxLength=80;input.oninput=()=>item.Name=input.value;label.append(input);const p=document.createElement('p');p.className='share-root-path';p.title=item.Path;p.textContent=item.Path;const writable=document.createElement('label');writable.className='check share-permission';const checkbox=document.createElement('input');checkbox.type='checkbox';checkbox.checked=item.Writable;const permissionText=document.createElement('span');permissionText.className='share-permission-text';const permissionBadge=document.createElement('small');permissionBadge.className='share-permission-badge';const permissionLabel=document.createElement('span');permissionLabel.textContent='允许修改';permissionText.append(permissionLabel,permissionBadge);const updatePermission=()=>{item.Writable=checkbox.checked;permissionBadge.textContent=item.Writable?'可修改':'只读';permissionBadge.classList.toggle('writable',item.Writable);writable.title=item.Writable?'可上传、新建、移动、改名和删除':'仅可查看、播放和下载';};checkbox.onchange=updatePermission;updatePermission();writable.append(checkbox,permissionText);body.append(label,p,writable);if(unavailable[item.ID]){const warning=document.createElement('p');warning.className='error';warning.textContent=unavailable[item.ID];body.append(warning)}const remove=button('移除',()=>{shareRoots.splice(index,1);drawShareRoots(unavailable)},'share-root-remove');remove.title='从共享列表移除，不会删除本机目录';row.append(body,remove);container.append(row)}$('shareCount').textContent=shareRoots.length+' / '+shareLimit+' 个目录';$('shareAdd').disabled=shareRoots.length>=shareLimit;}
async function openNetworkSettings(){
 if(!local)throw Error('请从工具箱的通用设置打开');
 const revision=++openRevision;path=access.initialPath?.()||'';
 $('shareError').textContent='';$('shareStatus').textContent='正在读取已保存的共享设置…';$('shareSave').disabled=true;$('shareAddPath').value=path;$('sharingDialog').showModal();$('sharingDialog').scrollTop=0;$('sharingDialog').querySelector('.sharing-settings-body').scrollTop=0;
 try{const value=await api('boot');if(revision!==openRevision||!$('sharingDialog').open)return;renderShare(value.Sharing);$('shareSave').disabled=false}catch(e){if(revision===openRevision)$('shareError').textContent=e.message}
};$('sharingClose').onclick=()=>$('sharingDialog').close();$('workspaceShowPassword').onclick=()=>{const input=$('workspacePassword'),hidden=input.dataset.hidden==='true';input.dataset.hidden=String(!hidden);$('workspaceShowPassword').textContent=hidden?'隐藏':'显示'};$('shareShowCode').onclick=()=>{const hidden=$('shareCode').dataset.hidden==='true';$('shareCode').dataset.hidden=String(!hidden);$('shareShowCode').textContent=hidden?'隐藏口令':'显示口令'};$('shareAdd').onclick=()=>{const value=$('shareAddPath').value.trim();if(!value)return;if(shareRoots.length>=shareLimit){$('shareError').textContent='最多共享'+shareLimit+'个资源目录';return}if(shareRoots.some(item=>item.Path.replace(/[\\/]+$/,'').toLowerCase()===value.replace(/[\\/]+$/,'').toLowerCase())){$('shareError').textContent='这个目录已经添加，请直接修改列表中的设置';return}shareRoots.push({Name:value.replace(/[\\/]$/,'').split(/[\\/]/).pop(),Path:value,Writable:false});$('shareAddPath').value='';drawShareRoots()};$('shareSave').onclick=saveSharingSettings;$('copyClose').onclick=()=>$('copyDialog').close();

let shareSaving=false;
function showSharingResult(kind,title,message){
  if(!$('sharingDialog').open)return;
  const dialog=$('shareResultDialog');dialog.dataset.result=kind;$('shareResultTitle').textContent=title;$('shareResultMessage').textContent=message;$('shareResultIcon').textContent=kind==='error'?'!':kind==='warning'?'!':'✓';
  if(!dialog.open)dialog.showModal();
}
$('shareResultClose').onclick=()=>$('shareResultDialog').close();
$('shareResultDialog').addEventListener('close',()=>{if($('sharingDialog').open&&!$('shareSave').disabled)$('shareSave').focus({preventScroll:true})});
async function saveSharingSettings(){
  if(!local||shareSaving||!$('sharingDialog').open)return;
  shareSaving=true;$('shareSave').disabled=true;$('shareSave').textContent='正在保存…';$('shareError').textContent='';
  try{
    const status=await api('sharing',{Enabled:$('shareEnabled').checked,WorkspaceEnabled:$('workspaceEnabled').checked,WorkspaceHTTP:$('workspaceHTTP').checked,WorkspacePassword:$('workspacePassword').value,OverlayEnabled:$('shareOverlay').checked,PublicURL:$('sharePublicURL').value.trim(),TrustedProxies:$('shareProxies').value.split(/[,，\n]/).map(value=>value.trim()).filter(Boolean),Port:Number($('sharePort').value),Password:$('shareCode').value,Roots:shareRoots});
    renderShare(status);
    const missing=Object.keys(status.Unavailable||{}).length;
    let kind='success',title='保存成功',message=status.Enabled?'内网共享已开启，可以使用共享设置中的访问地址连接。\n修改共享后，已登录设备需要重新登录。':'设置已保存，内网共享已关闭，下次启动也保持关闭。';
    if(status.Error){kind='warning';title='共享状态需要核对';message=status.Error}
    else if(missing){kind='warning';title='已保存，部分目录不可用';message=(status.Enabled?'共享已开启。':'共享已关闭。')+'有 '+missing+' 个目录暂不可用，请检查磁盘或路径；配置已保留。'}
    $('shareError').classList.toggle('error',kind!=='success');$('shareError').textContent=message;showSharingResult(kind,title,message);
  }catch(e){$('shareCode').value='';$('workspacePassword').value='';$('shareError').classList.add('error');$('shareError').textContent=e.message;showSharingResult('error','保存失败',e.message)}
  finally{shareSaving=false;$('shareSave').disabled=false;$('shareSave').textContent='保存并应用'}
}

function addSharedPaths(paths){
  const fresh=[];for(const p of paths){if(!p||shareRoots.some(item=>sameBrowserPath(item.Path,p))||fresh.some(other=>sameBrowserPath(other,p)))continue;fresh.push(p)}
  if(shareRoots.length+fresh.length>shareLimit)throw Error('最多共享'+shareLimit+'个目录，请减少本次选择');
  for(const p of fresh)shareRoots.push({Name:p.replace(/[\\/]+$/,'').split(/[\\/]/).pop()||p,Path:p,Writable:false});
  drawShareRoots();return fresh.length;
}
async function browseSharePicker(next,page=0){
  const id=++sharePickerState.revision;$('sharePickerError').textContent='正在读取目录…';$('sharePickerCurrent').disabled=true;
  try{const data=await api('list',{Path:next,Sort:'name',Offset:page,DirectoriesOnly:true},'');if(id!==sharePickerState.revision||!$('sharePicker').open)return;Object.assign(sharePickerState,{path:data.Path,parent:data.Parent,offset:data.Offset||0,total:data.Total||0});$('sharePickerPath').value=data.Path;$('sharePickerEntries').replaceChildren();
    for(const item of data.Entries||[]){if(!item.Dir)continue;const row=document.createElement('div');row.className='picker-folder';const check=document.createElement('input');check.type='checkbox';check.checked=sharePickerState.selected.has(item.Path);check.setAttribute('aria-label','选择共享目录 '+item.Name);check.onchange=()=>{if(check.checked)sharePickerState.selected.add(item.Path);else sharePickerState.selected.delete(item.Path);updateSharePickerSelection()};row.append(check,button('📁 '+item.Name,()=>browseSharePicker(item.Path)));$('sharePickerEntries').append(row)}
    $('sharePickerCurrent').disabled=!data.Path;$('sharePickerPrevious').disabled=!sharePickerState.offset;$('sharePickerNext').disabled=sharePickerState.offset+200>=sharePickerState.total;$('sharePickerPage').textContent=(Math.floor(sharePickerState.offset/200)+1)+' / '+Math.max(1,Math.ceil(sharePickerState.total/200));$('sharePickerError').textContent=data.Truncated?'目录过大，仅展示前20000项，可输入具体子目录路径。':'';updateSharePickerSelection();
  }catch(e){if(id===sharePickerState.revision)$('sharePickerError').textContent=e.message}
}
function updateSharePickerSelection(){$('sharePickerSelected').disabled=!sharePickerState.selected.size;$('sharePickerSelected').textContent='添加所选目录（'+sharePickerState.selected.size+'）'}

  $('shareBrowse').onclick=()=>{sharePickerState.selected.clear();$('sharePicker').showModal();updateSharePickerSelection();browseSharePicker($('shareAddPath').value.trim()||'')};
  $('sharePickerHome').onclick=()=>browseSharePicker('');$('sharePickerUp').onclick=()=>browseSharePicker(sharePickerState.parent);$('sharePickerGo').onclick=()=>browseSharePicker($('sharePickerPath').value.trim());$('sharePickerPath').onkeydown=e=>{if(e.key==='Enter')$('sharePickerGo').click()};
  $('sharePickerPath').oninput=()=>{sharePickerState.revision++;$('sharePickerCurrent').disabled=true;$('sharePickerError').textContent='请点击前往或按 Enter，确认共享目录。'};
  $('sharePickerPrevious').onclick=()=>browseSharePicker(sharePickerState.path,Math.max(0,sharePickerState.offset-200));$('sharePickerNext').onclick=()=>browseSharePicker(sharePickerState.path,sharePickerState.offset+200);
  const choose=paths=>{try{addSharedPaths(paths);$('shareAddPath').value='';$('sharePicker').close()}catch(e){$('sharePickerError').textContent=e.message}};
  $('sharePickerCurrent').onclick=()=>choose([sharePickerState.path]);$('sharePickerSelected').onclick=()=>choose([...sharePickerState.selected]);$('sharePickerCancel').onclick=()=>$('sharePicker').close();
  $('shareAdd').onclick=()=>{try{const p=$('shareAddPath').value.trim();if(!p)return;const added=addSharedPaths([p]);$('shareError').textContent=added?'':'这个目录已经添加，请直接修改列表中的设置';if(added)$('shareAddPath').value=''}catch(e){$('shareError').textContent=e.message}};
 $('sharingDialog').addEventListener('close',()=>{openRevision++;$('shareCode').value='';$('workspacePassword').value='';document.getElementById('openNetworkSettings')?.focus()});
 window.toolboxNetworkSettings={configure(value){access=value;local=Boolean(value)},open:openNetworkSettings};
})();
