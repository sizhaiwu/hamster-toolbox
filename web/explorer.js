'use strict';
const $=id=>document.getElementById(id);
let local=false,boot=null,root='',path='',parent='',rows=[],total=0,offset=0,selection=new Set(),history=[],loading=false,mutating=false,view='grid',previewIndex=-1,movePath='',moveParent='',uploadRequest=null,nameAction='';
let textBytes=null,tileSize=180,tileGap=16,shareLimit=128;
let shareRoots=[],favorites=[],requestID=0,previewRequest=0,searchTimer;
let preferenceWrites=Promise.resolve(),preferenceSaveID=0,preferenceLoadError='';
let quickAccessSaving=false;
let selectionMode=false,selectionAnchor='',nativeOpening=false;
let pageSize=200,sidebarCollapsed=false,defaultSort='name',defaultOrder='asc';
const icons={image:'▧',video:'▶',audio:'♫',pdf:'PDF',text:'≡',file:'▤',drive:'▣'};
const base=location.pathname.startsWith('/files/')?'/files/':'/';
const bytes=n=>{if(n<1024)return n+' B';const units=['KiB','MiB','GiB','TiB'];let x=n/1024,i=0;while(x>=1024&&i<3){x/=1024;i++}return x.toFixed(2)+' '+units[i]};
let noticeTimer;
function say(text,bad=false,quiet=false){
  clearTimeout(noticeTimer);$('notice').textContent=text;$('notice').classList.toggle('error',bad);$('notice').setAttribute('role',bad?'alert':'status');
  $('noticeToast').hidden=quiet||!text;$('noticeToast').classList.toggle('error',bad);
  if(!bad&&!quiet&&text&&!/^(正在|请等待)/.test(text))noticeTimer=setTimeout(()=>{$('noticeToast').hidden=true},6500);
}
$('dismissNotice').onclick=()=>{clearTimeout(noticeTimer);$('noticeToast').hidden=true};
$('browserHelp').onclick=()=>say('单击选中，双击打开；也可选中后点“打开”。'+(writable()?' Ctrl/⌘ 多选，Shift 连选。':' 此共享只读。'));
function endpoint(route,extra={},requestRoot=root){const query=new URLSearchParams({...((!local&&requestRoot)?{root:requestRoot}:{}),...extra});return base+route+(query.size?'?'+query:'')}
async function api(route,data={},requestRoot=root,signal){
  const response=await fetch(endpoint('api/'+route,{},requestRoot),{method:'POST',credentials:'same-origin',headers:{'Content-Type':'application/json','X-Hamster-Browser':'1'},body:JSON.stringify(data),signal,keepalive:route==='preferences'});
  if(!response.ok){
    if(response.status===401&&!signal?.aborted){stopListingMedia();closeFloatingMedia();for(const dialog of document.querySelectorAll('dialog[open]'))dialog.close();navigationAbort?.abort();requestID++;$('app').hidden=true;$('loginPanel').hidden=false}
    const error=Error((await response.text()).trim());error.status=response.status;throw error;
  }
  return response.json();
}
function button(text,handler,cls){const b=document.createElement('button');b.type='button';b.textContent=text;b.title=text;if(cls)b.className=cls;b.onclick=handler;return b}
function closeNav(){$('navBackdrop').hidden=true;document.body.classList.remove('nav-open');$('toggleNav').setAttribute('aria-expanded','false');applySidebar()}
$('toggleNav').onclick=()=>{const open=!document.body.classList.contains('nav-open');document.body.classList.toggle('nav-open',open);$('navBackdrop').hidden=!open;$('toggleNav').setAttribute('aria-expanded',String(open))};$('navBackdrop').onclick=closeNav;
function selected(){return rows.filter(x=>selection.has(x.Path))}
function writable(){return local||Boolean(boot?.Roots?.find(x=>x.ID===root)?.Writable)}
function syncSelection(){
  syncResourceClipboard();
  $('entries').classList.toggle('selecting',selectionMode);$('listHeader').classList.toggle('selecting',selectionMode);
  $('selectMode').textContent=selectionMode?'完成选择':'选择';$('selectMode').setAttribute('aria-pressed',String(selectionMode));
  const blocked=loading||mutating,available=rows.length>0;
  $('selectMode').disabled=blocked||!available;$('selectAll').hidden=$('clearSelection').hidden=!selectionMode;
  $('selectAll').disabled=blocked||!available;$('clearSelection').disabled=blocked||!selection.size;
  for(const id of ['rename','move','remove','download'])$(id).hidden=!selectionMode&&!selection.size;
  $('openLocal').hidden=!local;$('openLocal').disabled=!local||!path||blocked||nativeOpening;
  for(const card of $('entries').querySelectorAll('.entry')){
    const checked=selection.has(card.dataset.path);card.classList.toggle('selected',checked);
    const label=card.querySelector('.entry-check'),input=label?.querySelector('input'),open=card.querySelector('.entry-open');
    if(label){label.hidden=!selectionMode;input.checked=selectionMode&&checked;input.disabled=blocked}
    open.setAttribute('aria-label',selectionMode?open.dataset.selectLabel:open.dataset.openLabel);
    if(selectionMode||checked)open.setAttribute('aria-pressed',String(checked));else open.removeAttribute('aria-pressed');
  }
}
function selectItem(item,event={},checked){
  if(loading||mutating)return;
  const anchor=rows.findIndex(row=>row.Path===selectionAnchor),index=rows.indexOf(item);
  if(event.shiftKey&&anchor>=0){if(!event.ctrlKey&&!event.metaKey)selection.clear();for(const row of rows.slice(Math.min(anchor,index),Math.max(anchor,index)+1))selection.add(row.Path)}
  else{if(checked??!selection.has(item.Path))selection.add(item.Path);else selection.delete(item.Path);selectionAnchor=item.Path}
  actions();
}
function selectPage(){if(loading||mutating)return;for(const item of rows)selection.add(item.Path);selectionAnchor=rows[0]?.Path||'';actions()}
$('selectMode').onclick=()=>{if(loading||mutating)return;selectionMode=!selectionMode;if(!selectionMode){selection.clear();selectionAnchor=''}actions()};
$('selectAll').onclick=selectPage;$('clearSelection').onclick=()=>{selection.clear();selectionAnchor='';actions()};
document.addEventListener('keydown',event=>{
  const focus=document.activeElement,editing=focus.isContentEditable||focus.closest('.floating-window')||focus.matches('textarea,select,input:not([type=checkbox]):not([type=radio])');
  if($('app').hidden||document.querySelector('dialog[open]')||editing||loading||mutating)return;
  if((event.ctrlKey||event.metaKey)&&event.key.toLowerCase()==='a'){event.preventDefault();selectPage()}
  if((event.ctrlKey||event.metaKey)&&!event.altKey){
    const key=event.key.toLowerCase();
    if(key==='v'&&resourceClipboard){event.preventDefault();closeContextMenu();pasteSelectedResources()}
    if((key==='c'||key==='x')&&selection.size&&!window.getSelection()?.toString()){event.preventDefault();closeContextMenu();copySelectedResources(key==='x'?'move':'copy')}
  }
  if(event.key==='Escape'&&(selectionMode||selection.size)){event.preventDefault();selectionMode=false;selection.clear();selectionAnchor='';actions()}
});
$('openLocal').onclick=async()=>{if(!local||!path||loading||mutating||nativeOpening)return;const folder=path;nativeOpening=true;actions();try{await api('open-local',{Path:folder});if(path===folder)say('已在本机系统文件管理器中打开当前目录')}catch(e){say(e.message,true)}finally{nativeOpening=false;actions()}};
function actions(){const items=selected(),stable=items.every(x=>x.Version),write=writable()&&Boolean(path||root)&&!loading&&!mutating;$('newFolder').disabled=$('upload').disabled=$('uploadFolder').disabled=!write;$('rename').disabled=items.length!==1||!write||!stable;$('move').disabled=$('remove').disabled=$('copyTo').disabled=!items.length||!write||!stable;$('download').disabled=items.length!==1||items[0]?.Dir||mutating;$('openSelected').hidden=items.length!==1;$('openSelected').disabled=items.length!==1||loading||mutating;$('permission').textContent=local?'本机管理':writable()?'允许修改':'只读共享';$('summary').textContent=total+' 项'+(items.length?' · 已选 '+items.length+' 项':'');updateTabActions();syncSelection()}
function activateResource(item,itemRoot=root){if(!item||loading||mutating)return;if(item.Dir)navigate(item.Path,{targetRoot:itemRoot});else if(item.Kind==='shortcut')openDirectoryShortcut(item);else preview(rows.indexOf(item))}
$('openSelected').onclick=()=>activateResource(selected()[0]);
$('entries').addEventListener('click',event=>{if(!event.target.closest('.entry')&&!loading&&!mutating){selection.clear();selectionAnchor='';actions()}});
async function loadPreferences(){
  let stored={},legacyFavorites=[],hasLegacy=false;
  preferenceLoadError=local?boot.PreferencesError||'':'';
  try{const raw=localStorage.getItem('hamster.files.view');stored=JSON.parse(raw||'{}')||{};hasLegacy=raw!==null}catch{}
  if(local)try{const raw=localStorage.getItem('hamster.files.favorites');const parsed=JSON.parse(raw||'[]');if(Array.isArray(parsed))legacyFavorites=parsed.filter(x=>typeof x==='string').slice(0,20);hasLegacy=hasLegacy||raw!==null}catch{}
  const saved=local&&boot.Preferences?boot.Preferences:stored;
  view=saved.view==='list'?'list':'grid';tileSize=Number.isFinite(saved.tileSize)?Math.max(140,Math.min(420,saved.tileSize)):180;
  tileGap=Number.isInteger(saved.tileGap)?Math.max(0,Math.min(32,saved.tileGap)):16;
  pageSize=[50,100,200,500,1000].includes(saved.pageSize)?saved.pageSize:200;sidebarCollapsed=saved.sidebarCollapsed===true;
  defaultSort=['name','size','modified'].includes(saved.sort)?saved.sort:'name';defaultOrder=['asc','desc'].includes(saved.order)?saved.order:defaultSort==='name'?'asc':'desc';
  hoverPreviewEnabled=saved.hoverPreview!==false;
  $('pageSize').value=pageSize;$('sort').value=defaultSort;$('order').value=defaultOrder;applySidebar();
  favorites=local?(boot.Preferences?boot.Preferences.favorites:legacyFavorites):[];
  if(!Array.isArray(favorites))favorites=[];
  // Migrate only on first use. An empty browser on a new port must never replace saved favorites.
  if(local&&!boot.Preferences&&!preferenceLoadError&&hasLegacy){if(!await savePreferences({view,tileSize,tileGap,favorites:[...favorites]}))preferenceLoadError='旧收藏未能保存到工具箱，请检查数据目录是否可写。'}
}
function savePreferences(change){
  if(!local)return Promise.resolve(true);
  const id=++preferenceSaveID;
  preferenceWrites=preferenceWrites.then(async()=>{try{await api('preferences',change);return true}catch(e){if(id===preferenceSaveID)say('设置保存失败，重启后可能丢失：'+e.message,true);return false}});
  return preferenceWrites;
}
function side(){
  const list=$('roots');list.replaceChildren();if(local){const b=button('此电脑',()=>navigate(''));b.prepend(explorerIcon('computer'));b.classList.toggle('active',!path);list.append(b)}else for(const share of boot.Roots||[]){const name=sharedLocationName(share.ID),b=button(name,()=>{navigate('',{targetRoot:share.ID});closeNav()});b.prepend(explorerIcon('shared'));b.title=name;b.classList.toggle('active',share.ID===root);list.append(b)}
  $('favorites').replaceChildren();const locations=favorites.filter(item=>typeof item==='string').slice(0,20).map(path=>({root:'',path}));
  for(const favorite of locations){
    const row=document.createElement('div'),b=button('',()=>{if(navigationSortClickBlocked())return;navigate(favorite.path);closeNav()},'location-button');row.className='quick-access-row';row.dataset.path=favorite.path;labelLocation(b,locationDisplay(favorite,locations));b.prepend(explorerIcon('folder'));b.setAttribute('aria-label','打开快速访问目录 '+favorite.path);b.classList.toggle('active',Boolean(path)&&sameBrowserPath(favorite.path,path));
    bindNavigationSort(row,'favorites',favorite.path,'y',moveQuickAccess,()=>!local||mutating||quickAccessSaving);
    row.oncontextmenu=event=>showQuickAccessContext(event,favorite.path,b);b.onkeydown=event=>{if(event.key==='ContextMenu'||event.shiftKey&&event.key==='F10')showQuickAccessContext(event,favorite.path,b)};
    const more=button('···',event=>showQuickAccessContext(event,favorite.path,b),'quick-access-more');more.setAttribute('aria-label','快速访问操作 '+favorite.path);more.title='快速访问操作';row.append(b,more);$('favorites').append(row)
  }
  $('quickAccess').hidden=!local;updateQuickAccessButton();
}
function inQuickAccess(folder){return favorites.some(value=>sameBrowserPath(value,folder))}
function updateQuickAccessButton(){const added=Boolean(path)&&inQuickAccess(path);$('favorite').disabled=!local||!path||quickAccessSaving||added;$('favorite').textContent=added?'已添加到快速访问':'添加到快速访问';$('favorite').title=added?'已添加；可在左侧快速访问项目上右键取消固定':'将当前目录添加到快速访问，重启后仍会保留'}
async function setQuickAccess(folder,add=true){
  if(!local||!folder||quickAccessSaving)return;
  if(add&&inQuickAccess(folder))return;
  if(add&&favorites.length>=20){say('快速访问最多添加20个目录，请先取消不需要的项目',true);return}
  const previous=[...favorites];quickAccessSaving=true;favorites=add?[folder,...favorites]:favorites.filter(value=>!sameBrowserPath(value,folder));side();
  const saved=await savePreferences({favorites:[...favorites]});quickAccessSaving=false;
  if(!saved){favorites=previous;side();return}
  side();try{localStorage.setItem('hamster.files.favorites',JSON.stringify(favorites))}catch{}say(add?'已添加到快速访问，重启后仍会保留':'已从快速访问取消固定，原目录不会删除');
}
$('favorite').onclick=()=>setQuickAccess(path);
function setView(){ $('entries').style.setProperty('--tile-width',tileSize+'px');$('entries').style.setProperty('--tile-height',Math.round(tileSize*.65)+'px');$('entries').style.setProperty('--tile-gap',tileGap+'px');$('tileGap').value=tileGap;$('tileGapValue').textContent=tileGap+'px';$('tileGapControl').hidden=view!=='grid';$('tileSize').value=tileSize;$('tileSizeValue').textContent=tileSize;$('tileSizeControl').hidden=view!=='grid';$('viewOptions').hidden=view!=='grid'; $('entries').className='entries '+view+(selectionMode?' selecting':'');$('view').replaceChildren(controlIcon(view==='grid'?'listView':'gridView'),document.createTextNode(view==='grid'?'列表视图':'图标视图'));$('listHeader').hidden=view!=='list';updateListSortHeaders();cacheExplorerView()}
$('tileSize').oninput=()=>{tileSize=Number($('tileSize').value);setView()};
$('tileSize').onchange=()=>{savePreferences({tileSize})};
$('tileGap').oninput=()=>{tileGap=Math.max(0,Math.min(32,Number($('tileGap').value)||0));setView()};
$('tileGap').onchange=()=>savePreferences({tileGap});
$('view').onclick=()=>{stopVideoHover();view=view==='grid'?'list':'grid';setView();rememberTab();persistTabs();savePreferences({view})};
function separator(){return local?(boot?.Separator||'\\'):'/'}
function breadcrumb(){const crumbs=$('crumbs');crumbs.replaceChildren();const share=boot?.Roots?.find(x=>x.ID===root);crumbs.append(button(local?'此电脑':share?.Name||'共享目录',()=>navigate('')));const parts=path.split(/[\\/]/).filter(Boolean);let current=local&&separator()==='/'?'/':'';for(let i=0;i<parts.length;i++){current=local?(separator()==='/'?current.replace(/\/$/,'')+'/'+parts[i]:i===0?parts[i]+'\\':current.replace(/[\\/]$/,'')+'\\'+parts[i]):parts.slice(0,i+1).join('/');const target=current;crumbs.append(document.createTextNode('›'),button(parts[i],()=>navigate(target)))}}

async function navigate(next,{remember=true,page=0,targetRoot=root,mode='',scroll,force=false}={}){
  if(mutating){say('请等待当前文件操作结束',true);return false}
  stopVideoHover();stopListingMedia();closeContextMenu();
  const tabID=activeFileTab,id=++requestID,previousLocation={root,path,offset,scroll:scrollRestorePending?.value??Math.max(0,window.scrollY||0),search:$('search').value,sort:$('sort').value,order:$('order').value,view,pageSize};
  const destination=mode==='back'?history.at(-1):mode==='forward'?future.at(-1):null;
  if(destination){page=destination.offset||0;scroll=destination.scroll||0}
  scroll??=next===path&&targetRoot===root&&page===offset&&!remember?previousLocation.scroll:0;
  scrollRestorePending=null;
  navigationAbort?.abort();const controller=new AbortController();navigationAbort=controller;
  const search=destination?.search??$('search').value,sort=destination?.sort||$('sort').value,order=destination?.order||$('order').value;
  const requestedPageSize=destination?.pageSize||pageSize;
  loading=true;actions();$('cancelSearch').hidden=false;say('正在读取目录…');
  try{
    const data=await api('list',{Path:next,Search:search,Sort:sort,Order:order,PageSize:requestedPageSize,Offset:page,Recursive:$('searchRecursive').checked,Force:force},targetRoot,controller.signal);
    if(id!==requestID||tabID!==activeFileTab)return false;
    const changed=targetRoot!==root||data.Path!==path;
    if(mode==='back'){history.pop();future.push(previousLocation)}
    else if(mode==='forward'){future.pop();history.push(previousLocation)}
    else if(remember&&changed){history.push(previousLocation);future=[]}
    history=history.slice(-maxTabHistory);future=future.slice(-maxTabHistory);
    root=targetRoot;path=data.Path;parent=data.Parent;rows=data.Entries||[];total=data.Total;offset=data.Offset||0;selection.clear();selectionMode=false;selectionAnchor='';
    if(destination){view=destination.view||view;pageSize=requestedPageSize;$('pageSize').value=pageSize;$('search').value=search;$('sort').value=sort;$('order').value=order}
    $('location').value=local?path:(boot.Roots.find(item=>item.ID===root)?.Name||'共享目录')+(path?'/'+path:'');
    $('heading').textContent=path.split(/[\\/]/).filter(Boolean).pop()||(local?'此电脑':'共享目录');
    render();breadcrumb();side();
    const tab=activeTab();if(tab)Object.assign(tab,{root,path,back:[...history],forward:[...future],search,sort,order,view,offset,scroll});
    renderTabs();persistTabs();
    say((data.Truncated?'目录超过20000项，仅展示前20000项，请进入更具体的子目录。':'单击选中，双击打开；也可选中后点“打开”。'+(writable()?' Ctrl/⌘ 多选，Shift 连选。':' 此共享只读。'))+(!workspaceStorageOK?' 浏览器未允许保存标签，刷新后无法恢复。':''),false,!data.Truncated&&workspaceStorageOK);
    $('previous').disabled=offset===0;$('next').disabled=offset+rows.length>=total;
    $('page').textContent=total?Math.floor(offset/pageSize)+1+' / '+Math.ceil(total/pageSize):'0 / 0';closeNav();
    scrollRestorePending={tab:tabID,value:scroll};
    requestAnimationFrame(()=>{if(id===requestID&&tabID===activeFileTab){window.scrollTo({top:scroll,behavior:'auto'});scrollRestorePending=null}});
    return true;
  }catch(e){if(id===requestID&&tabID===activeFileTab&&e.name!=='AbortError')say(e.message,true);return false}
  finally{if(id===requestID&&tabID===activeFileTab){loading=false;navigationAbort=null;$('cancelSearch').hidden=true;actions()}}
}
function render(){
  stopVideoHover();stopListingMedia();
  const entries=$('entries');entries.replaceChildren();setView();
  if(!rows.length){const empty=document.createElement('p');empty.className='empty';empty.textContent=local&&!path?'没有找到可读取的磁盘，可在地址栏粘贴目录路径。':'目录为空，或没有匹配项目。';entries.append(empty);syncSelection();return}
  for(const item of rows){
    const card=document.createElement('article');card.className='entry';card.dataset.path=item.Path;const itemRoot=root;
    card.addEventListener('dragstart',event=>event.preventDefault());
    let touchInput=false,lastTap=0;
    const open=button('',event=>{
      if(loading||mutating)return;
      if(selectionMode||event.ctrlKey||event.metaKey||event.shiftKey){event.preventDefault();lastTap=0;selectItem(item,event);return}
      selection.clear();selectItem(item,{},true);
      if(touchInput||event.pointerType==='touch'){const now=Date.now();if(lastTap&&now-lastTap<400){lastTap=0;activateResource(item,itemRoot)}else lastTap=now}
    },'entry-open');
    open.addEventListener('pointerdown',event=>{touchInput=event.pointerType==='touch'});
    open.ondblclick=event=>{event.preventDefault();if(!touchInput&&!selectionMode&&!event.ctrlKey&&!event.metaKey&&!event.shiftKey)activateResource(item,itemRoot)};
    open.onauxclick=event=>{if(event.button===1&&!loading&&!mutating){if(item.Dir){event.preventDefault();openFileTab({root:itemRoot,path:item.Path})}else if(item.Kind==='shortcut'){event.preventDefault();openDirectoryShortcut(item,true)}}};
    open.dataset.openLabel='选择 '+item.Name+'，双击打开';open.dataset.selectLabel='选择 '+item.Name;open.setAttribute('aria-label',open.dataset.openLabel);
    open.title=open.dataset.openLabel;card.oncontextmenu=event=>showContextMenu(event,item);open.onkeydown=event=>{if(event.key==='ContextMenu'||(event.shiftKey&&event.key==='F10'))showContextMenu(event,item);else if(event.key==='Enter'&&!selectionMode&&!event.ctrlKey&&!event.metaKey&&!event.shiftKey){event.preventDefault();activateResource(item,itemRoot)}};
    const icon=document.createElement('span');icon.className='entry-icon';const iconKind=explorerIconKind(item);icon.append(explorerIcon(iconKind));card.dataset.kind=item.Dir?'folder':item.Kind;card.dataset.iconKind=iconKind;
    if(!item.Dir&&['image','video'].includes(item.Kind)){
      const img=document.createElement('img');img.alt='';img.draggable=false;
      loadMediaThumbnail(img,item,itemRoot,()=>{img.replaceWith(explorerIcon(item.Kind));if(item.Kind==='video'){icon.title='缩略图暂不可用，可打开视频或刷新重试';const hint=document.createElement('small');hint.textContent='暂无缩略图';icon.append(hint)}});
      icon.replaceChildren(img);
    }
    const title=document.createElement('span');title.className='entry-title';title.textContent=item.Name;
    const meta=document.createElement('span');meta.className='entry-meta';meta.textContent=item.Kind==='drive'?'磁盘':item.Dir?'文件夹':item.Kind==='shortcut'?'快捷方式':bytes(item.Size);
    const date=document.createElement('span');date.className='entry-date';date.textContent=item.Modified?formatExplorerDate(item.Modified):'';
    const duration=document.createElement('span');duration.className='entry-duration';duration.textContent='';
    open.append(icon,title,duration,meta,date);card.append(open);
    if(item.Kind==='video'){const play=document.createElement('span');play.className='entry-play';play.textContent='▶';play.setAttribute('aria-hidden','true');icon.append(play);installVideoHover(open,icon,card,item,itemRoot)}
    const more=button('···',event=>showContextMenu(event,item),'entry-more');more.title='更多操作：'+item.Name;more.setAttribute('aria-label',more.title);card.append(more);
    {
      const label=document.createElement('label');label.className='entry-check';label.hidden=!selectionMode;
      const check=document.createElement('input');check.type='checkbox';check.setAttribute('aria-label','选择 '+item.Name);
      check.onchange=()=>{if(selectionMode)selectItem(item,{},check.checked);else check.checked=selection.has(item.Path)};
      label.append(check);card.append(label);
    }
    entries.append(card);
    if(item.Kind==='video')watchListingVideo(card,duration,item,itemRoot);
  }
  syncSelection();
}
$('up').onclick=()=>navigate(parent);$('refresh').onclick=()=>navigate(path,{remember:false,page:offset,force:true});$('go').onclick=()=>{if(local)navigate($('location').value)};$('location').onkeydown=e=>{if(e.key==='Enter'&&local)navigate(e.target.value)};$('search').oninput=()=>{clearTimeout(searchTimer);rememberTab();persistTabs();searchTimer=setTimeout(()=>navigate(path,{remember:false}),250)};$('previous').onclick=()=>navigate(path,{remember:false,page:Math.max(0,offset-pageSize)});$('next').onclick=()=>navigate(path,{remember:false,page:offset+pageSize});
function download(item){if(!item||item.Dir)return;const a=document.createElement('a');a.href=endpoint('raw',{path:item.Path,download:'1'});a.download=item.Name;a.rel='noreferrer';a.click()}
$('download').onclick=()=>download(selected()[0]);$('viewerDownload').onclick=()=>download(rows[previewIndex]);
$('viewerNative').onclick=async()=>{
  const item=rows[previewIndex];if(!local||item?.Kind!=='video'||$('viewerNative').disabled)return;
  const id=previewRequest,source={...item};$('viewerNative').disabled=true;$('viewerBody').querySelector('video')?.pause();
  const opened=await openNativeMedia(source);
  if(id!==previewRequest)return;
  if(opened&&$('viewer').open)$('viewer').close();else $('viewerNative').disabled=false;
};
function clearViewer(keepFilmstrip=false){previewPagingAbort?.abort();previewPagingAbort=null;disposeVideoPlayback();if(!keepFilmstrip){clearViewerFilmstrip();clearViewerQueue()}disposeImageViewer();$('viewerTools').hidden=true;$('imageTools').hidden=true;$('videoTools').hidden=true;$('viewerNative').hidden=true;$('viewerNative').disabled=false;document.body.classList.remove('viewer-open');textBytes=null;previewRequest++;for(const media of $('viewerBody').querySelectorAll('video,audio')){media.pause();media.removeAttribute('src');media.load()}$('viewerBody').replaceChildren()}
async function preview(index){
  stopVideoHover();
  const item=rows[index];if(!item||item.Dir)return;
  if(item.Kind==='video'&&local&&window.toolboxSettings?.DefaultPlayer==='mpv'&&await openNativeMedia(item))return;
  clearViewer(true);const id=previewRequest;previewIndex=index;
  $('viewerNative').hidden=!local||item.Kind!=='video';
  $('viewer').dataset.kind=item.Kind;$('textEncoding').hidden=item.Kind!=='text';$('textEncoding').value='auto';
  $('viewerHint').textContent=item.Kind==='image'?'滚轮 / 双指缩放 · 放大后拖动 · ← → 切换':'当前目录同类文件，可用两侧按钮切换';
  $('viewerPrevious').disabled=previewNeighbor(-1)<0&&!canPreviewPage(-1);$('viewerNext').disabled=previewNeighbor(1)<0&&!canPreviewPage(1);
  $('viewerTitle').textContent=item.Name;$('viewerTitle').title=item.Name;
  $('viewerInfo').textContent=bytes(item.Size)+(item.Modified?' · '+formatExplorerDate(item.Modified):'');
  const body=$('viewerBody'),url=endpoint('raw',{path:item.Path});
  document.body.classList.add('viewer-open');if(!$('viewer').open)$('viewer').showModal();
  updateViewerFilmstrip();updateViewerQueue();
  if(item.Kind==='image'){
    const img=document.createElement('img');img.alt=item.Name;img.draggable=false;img.src=url;
    img.onerror=()=>{
      if(id!==previewRequest)return;
      if(!img.dataset.compatible){img.dataset.compatible='1';img.src=endpoint('image-preview',{path:item.Path});$('viewerHint').textContent='正在生成兼容预览；原图不变。';return}
      disposeImageViewer();$('viewerTools').hidden=true;body.replaceChildren();const p=document.createElement('p');p.textContent='设备无法预览此图片，可下载原文件查看。';body.append(p);
    };
    installImageViewer(img);$('viewerTools').hidden=false;$('imageTools').hidden=false;
  }else if(['video','audio'].includes(item.Kind)){
    const wrap=document.createElement('div');wrap.className='media-wrap';const media=document.createElement(item.Kind);media.controls=true;
    $('viewerTools').hidden=false;$('videoTools').hidden=false;$('videoSpeed').value='1';$('videoSpeed').onchange=()=>{media.playbackRate=Number($('videoSpeed').value)};
    if(item.Kind==='video')media.poster=endpoint('thumb',{path:item.Path});
    media.preload='metadata';media.setAttribute('playsinline','');media.muted=Boolean(window.toolboxSettings?.BrowserMuted);media.src=url;
    const help=document.createElement('p');help.textContent='使用原始画质播放；打不开时可尝试兼容播放。';
    media.onerror=()=>help.textContent='当前浏览器无法播放此编码，请下载原文件用播放器打开。';
    wrap.append(media,help);body.append(wrap);if(item.Kind==='video'){const playback=installVideoPlayback(media,wrap,help,item,root,id);installPlaybackBookmark(media,playback,item,root,id);if(window.toolboxSettings?.BrowserAutoPlay!==false)playback.play();}
  }else if(item.Kind==='pdf'){
    const frame=document.createElement('iframe');frame.title=item.Name;frame.src=url;body.append(frame);
  }else if(item.Kind==='text'){
    const pre=document.createElement('pre');pre.textContent='正在读取…';body.append(pre);
    try{const response=await fetch(endpoint('text',{path:item.Path}));if(!response.ok){const value=await response.text();if(id===previewRequest)pre.textContent='无法预览：'+value}else{const value=await response.arrayBuffer();if(id===previewRequest){textBytes=new Uint8Array(value);decodeText()}}}catch(e){if(id===previewRequest)pre.textContent=e.message}
  }else{
    const p=document.createElement('p');p.textContent='此文件没有内置预览，可点击“下载原文件”后用对应软件打开。Office 文档和压缩包不会上传到第三方服务。';body.append(p);
  }
}

function decodeText(){if(!textBytes)return;let encoding=$('textEncoding').value;if(encoding==='auto'){if(textBytes[0]===255&&textBytes[1]===254)encoding='utf-16le';else if(textBytes[0]===254&&textBytes[1]===255)encoding='utf-16be';else{try{new TextDecoder('utf-8',{fatal:true}).decode(textBytes);encoding='utf-8'}catch{encoding='gb18030'}}}try{$('viewerBody').querySelector('pre').textContent=new TextDecoder(encoding).decode(textBytes)}catch{$('viewerBody').querySelector('pre').textContent='当前设备不支持此编码，请下载查看'}}
$('textEncoding').onchange=decodeText;
function previewNeighbor(step){let next=previewIndex+step;const kind=rows[previewIndex]?.Kind;const media=['image','video','audio'].includes(kind);while(rows[next]&&(rows[next].Dir||(media&&rows[next].Kind!==kind)))next+=step;return rows[next]?next:-1}
function advance(step){const next=previewNeighbor(step);if(next>=0)return preview(next);return previewBeyondPage(step)}$('viewerPrevious').onclick=()=>advance(-1);$('viewerNext').onclick=()=>advance(1);$('viewerClose').onclick=()=>$('viewer').close();$('viewer').addEventListener('close',()=>{if(document.fullscreenElement===$('viewerSurface')){const exiting=document.exitFullscreen?.();exiting?.catch?.(()=>{})}clearViewer()});document.addEventListener('keydown',e=>{if(!$('viewer').open||rows[previewIndex]?.Kind!=='image'||['INPUT','TEXTAREA'].includes(document.activeElement.tagName))return;if(handleImageKey(e))return;if(e.key==='ArrowLeft')advance(-1);if(e.key==='ArrowRight')advance(1)});
function join(dir,name){return local?dir.replace(/[\\/]$/,'')+separator()+name:(dir?dir.replace(/\/$/,'')+'/':'')+name}
function openName(action){nameAction=action;$('nameTitle').textContent=action==='mkdir'?'新建文件夹':'修改名称';$('newName').value=action==='mkdir'?'新建文件夹':selected()[0]?.Name||'';$('nameError').textContent='';$('nameDialog').showModal();$('newName').focus();$('newName').select()}
$('newFolder').onclick=()=>openName('mkdir');$('rename').onclick=()=>openName('rename');$('nameCancel').onclick=()=>$('nameDialog').close();$('nameSave').onclick=async()=>{if(mutating)return;mutating=true;actions();$('nameSave').disabled=true;try{await api('mutate',{Action:nameAction,Path:nameAction==='mkdir'?path:selected()[0].Path,Name:$('newName').value,Version:nameAction==='mkdir'?'':selected()[0].Version});$('nameDialog').close();mutating=false;await navigate(path,{remember:false})}catch(e){$('nameError').textContent=e.message}finally{mutating=false;$('nameSave').disabled=false;actions()}};
let moveRequest=0,moveAbort=null,moveOffset=0,moveTotal=0,moveReady=false;
async function browseMove(next,page=0){
 const id=++moveRequest,currentRoot=root;moveAbort?.abort();const controller=new AbortController();moveAbort=controller;moveReady=false;$('moveSave').disabled=true;$('moveError').textContent='正在读取目录…';
 try{
  const data=await api('list',{Path:next,Sort:'name',Offset:page,DirectoriesOnly:true},currentRoot,controller.signal);
  if(id!==moveRequest||currentRoot!==root||!$('moveDialog').open)return;
  movePath=data.Path;moveParent=data.Parent;moveOffset=data.Offset||0;moveTotal=data.Total||0;$('movePath').value=local?movePath:'/'+movePath;$('moveFolders').replaceChildren();
  for(const item of data.Entries||[])if(item.Dir)$('moveFolders').append(button('📁 '+item.Name,()=>browseMove(item.Path)));
  moveReady=true;$('moveSave').disabled=local&&!movePath;$('movePrevious').disabled=moveOffset===0;$('moveNext').disabled=moveOffset+200>=moveTotal;$('movePage').textContent=Math.floor(moveOffset/200)+1+' / '+Math.max(1,Math.ceil(moveTotal/200));$('moveError').textContent=data.Truncated?'目录超过20000项，可输入具体子目录路径。':'';
 }catch(e){if(id===moveRequest&&e.name!=='AbortError')$('moveError').textContent=e.message}
 finally{if(id===moveRequest)moveAbort=null}
}
$('movePath').oninput=()=>{moveRequest++;moveAbort?.abort();moveAbort=null;moveReady=false;$('moveSave').disabled=true;$('moveError').textContent='请点击前往或按 Enter，确认目标目录。'};
$('movePrevious').onclick=()=>browseMove(movePath,Math.max(0,moveOffset-200));$('moveNext').onclick=()=>browseMove(movePath,moveOffset+200);
$('moveDialog').addEventListener('close',()=>{moveRequest++;moveAbort?.abort();moveAbort=null;moveReady=false});
$('move').onclick=()=>{$('moveDialog').showModal();browseMove(path)};$('moveHome').onclick=()=>browseMove('');$('moveGo').onclick=()=>browseMove(local?$('movePath').value:$('movePath').value.replace(/^\//,''));$('movePath').onkeydown=e=>{if(e.key==='Enter')$('moveGo').click()};$('moveUp').onclick=()=>browseMove(moveParent);$('moveCancel').onclick=()=>$('moveDialog').close();$('moveSave').onclick=async()=>{if(mutating||!moveReady)return;const items=selected();mutating=true;actions();$('moveSave').disabled=true;try{for(const item of items)await api('mutate',{Action:'move',Path:item.Path,Target:join(movePath,item.Name),Version:item.Version});$('moveDialog').close();mutating=false;await navigate(path,{remember:false})}catch(e){$('moveDialog').close();mutating=false;await navigate(path,{remember:false});say('移动未全部完成：'+e.message+'。目录已刷新，请核对后重新选择剩余项目。',true)}finally{mutating=false;$('moveSave').disabled=false;actions()}};
$('remove').onclick=()=>{$('deleteNames').textContent=selected().map(x=>x.Name).join('、');$('permanent').checked=false;$('deleteError').textContent='';$('deleteDialog').showModal()};$('deleteCancel').onclick=()=>$('deleteDialog').close();$('deleteSave').onclick=async()=>{if(mutating)return;const items=selected();mutating=true;actions();$('deleteSave').disabled=true;try{for(const item of items)await api('mutate',{Action:'delete',Path:item.Path,Permanent:$('permanent').checked,Version:item.Version});$('deleteDialog').close();mutating=false;await navigate(path,{remember:false})}catch(e){$('deleteDialog').close();mutating=false;await navigate(path,{remember:false});say('删除未全部完成：'+e.message+'。目录已刷新，请核对后重新选择剩余项目。',true)}finally{mutating=false;$('deleteSave').disabled=false;actions()}};
$('upload').onclick=()=>$('uploadInput').click();$('cancelUpload').onclick=()=>uploadRequest?.abort();$('uploadInput').onchange=async()=>{if(mutating)return;const files=[...$('uploadInput').files];if(!files.length)return;mutating=true;actions();$('uploadProgress').hidden=false;try{for(const file of files){if(file.size>32*1024**3)throw Error('单文件不能超过32GiB：'+file.name);say('正在上传 '+file.name+'；同名文件不会覆盖。');await new Promise((resolve,reject)=>{const xhr=new XMLHttpRequest();uploadRequest=xhr;xhr.open('POST',endpoint('api/upload',{path,name:file.name}));xhr.setRequestHeader('X-Hamster-Browser','1');xhr.upload.onprogress=e=>{if(e.lengthComputable)$('uploadBar').value=e.loaded/e.total*100};xhr.onload=()=>xhr.status===200?resolve():reject(Error(xhr.responseText||'上传失败'));xhr.onerror=()=>reject(Error('网络连接失败'));xhr.onabort=()=>reject(Error('上传已取消，已完成文件保留'));xhr.send(file)})}mutating=false;await navigate(path,{remember:false});say('上传完成')}catch(e){mutating=false;await navigate(path,{remember:false});say(e.message,true)}finally{mutating=false;uploadRequest=null;$('uploadProgress').hidden=true;$('uploadInput').value='';actions()}};
function renderShare(status){
  $('copyDialog').querySelector('h2').textContent='访问地址';$('copyDialog').querySelector('p').textContent='复制此地址到同一内网设备的浏览器。';
  shareRoots=structuredClone(status.Roots||[]);
  shareLimit=status.MaxRoots||128;$('shareEnabled').checked=status.StartupEnabled??status.Enabled;
  $('sharePort').value=status.Port;$('shareCode').value='';
  $('shareOverlay').checked=Boolean(status.OverlayEnabled);$('sharePublicURL').value=status.PublicURL||'';$('shareProxies').value=(status.TrustedProxies||[]).join(', ');
  $('passwordState').textContent=status.PasswordSet?'已设置口令；留空保留。修改共享后，已登录设备需要重新登录。':'尚未设置访问口令。';
  drawShareRoots(status.Unavailable||{});$('shareURLs').replaceChildren();
  $('shareStatus').textContent=status.Enabled?'共享运行中 · 下次启动工具箱会自动开启':status.StartupEnabled?'自动共享已保存 · 当前未运行':'共享已关闭 · 下次启动保持关闭';
  $('shareStatus').classList.toggle('error',Boolean(status.Error));
  $('shareError').textContent=status.Error||'';
  if(status.Enabled&&!(status.URLs||[]).length){const p=document.createElement('p');p.textContent='共享已开启，暂未发现内网IPv4地址，请检查网卡或Wi-Fi。';$('shareURLs').append(p)}
  for(const url of [...new Set([...(status.URLs||[]),...(status.Enabled&&status.PublicURL?[status.PublicURL+'/']:[])])]){const row=document.createElement('div');row.className='share-url';const link=document.createElement('a');link.href=url;link.textContent=url;link.target='_blank';link.rel='noopener noreferrer';row.append(link,button('复制地址',async()=>{try{await navigator.clipboard.writeText(url)}catch{$('copyValue').value=url;$('copyDialog').showModal();$('copyValue').select()}}));$('shareURLs').append(row)}
}
function drawShareRoots(unavailable={}){const container=$('shareRoots');container.replaceChildren();for(const [index,item]of shareRoots.entries()){const row=document.createElement('div');row.className='share-root';const body=document.createElement('div');const label=document.createElement('label');label.textContent='显示名称';const input=document.createElement('input');input.type='text';input.value=item.Name||'';input.maxLength=80;input.oninput=()=>item.Name=input.value;label.append(input);const p=document.createElement('p');p.className='share-root-path';p.title=item.Path;p.textContent=item.Path;const writable=document.createElement('label');writable.className='check share-permission';const checkbox=document.createElement('input');checkbox.type='checkbox';checkbox.checked=item.Writable;const permissionText=document.createElement('span');permissionText.className='share-permission-text';const permissionBadge=document.createElement('small');permissionBadge.className='share-permission-badge';const permissionLabel=document.createElement('span');permissionLabel.textContent='允许修改';permissionText.append(permissionLabel,permissionBadge);const updatePermission=()=>{item.Writable=checkbox.checked;permissionBadge.textContent=item.Writable?'可修改':'只读';permissionBadge.classList.toggle('writable',item.Writable);writable.title=item.Writable?'可上传、新建、移动、改名和删除':'仅可查看、播放和下载';};checkbox.onchange=updatePermission;updatePermission();writable.append(checkbox,permissionText);body.append(label,p,writable);if(unavailable[item.ID]){const warning=document.createElement('p');warning.className='error';warning.textContent=unavailable[item.ID];body.append(warning)}const remove=button('移除',()=>{shareRoots.splice(index,1);drawShareRoots(unavailable)},'share-root-remove');remove.title='从共享列表移除，不会删除本机目录';row.append(body,remove);container.append(row)}$('shareCount').textContent=shareRoots.length+' / '+shareLimit+' 个目录';$('shareAdd').disabled=shareRoots.length>=shareLimit;}
$('sharing').onclick=async()=>{$('shareError').textContent='';$('shareStatus').textContent='正在读取已保存的共享设置…';$('shareSave').disabled=true;$('shareAddPath').value=path;$('sharingDialog').showModal();$('sharingDialog').scrollTop=0;$('sharingDialog').querySelector('.sharing-settings-body').scrollTop=0;try{boot=await api('boot');renderShare(boot.Sharing);$('shareSave').disabled=false}catch(e){$('shareError').textContent=e.message}};$('sharingClose').onclick=()=>$('sharingDialog').close();$('shareShowCode').onclick=()=>{const hidden=$('shareCode').dataset.hidden==='true';$('shareCode').dataset.hidden=String(!hidden);$('shareShowCode').textContent=hidden?'隐藏口令':'显示口令'};$('shareAdd').onclick=()=>{const value=$('shareAddPath').value.trim();if(!value)return;if(shareRoots.length>=shareLimit){$('shareError').textContent='最多共享'+shareLimit+'个资源目录';return}if(shareRoots.some(item=>item.Path.replace(/[\\/]+$/,'').toLowerCase()===value.replace(/[\\/]+$/,'').toLowerCase())){$('shareError').textContent='这个目录已经添加，请直接修改列表中的设置';return}shareRoots.push({Name:value.replace(/[\\/]$/,'').split(/[\\/]/).pop(),Path:value,Writable:false});$('shareAddPath').value='';drawShareRoots()};$('shareSave').onclick=saveSharingSettings;$('copyClose').onclick=()=>$('copyDialog').close();
$('toggleCode').onclick=()=>{const hidden=$('accessCode').dataset.hidden==='true';$('accessCode').dataset.hidden=String(!hidden);$('toggleCode').textContent=hidden?'隐藏':'显示'};$('login').onclick=async()=>{$('login').disabled=true;$('loginError').textContent='';try{await api('login',{Password:$('accessCode').value});$('accessCode').value='';await start(true)}catch(e){$('loginError').textContent=e.message}finally{$('login').disabled=false}};$('accessCode').onkeydown=e=>{if(e.key==='Enter')$('login').click()};$('logout').onclick=async()=>{try{forgetTabs();await api('logout')}finally{location.reload()}};
async function start(afterLogin=false){try{boot=await api('boot');local=boot.Local;$('app').hidden=false;$('loginPanel').hidden=true;$('sharing').hidden=!local;$('favorite').hidden=!local;$('logout').hidden=local;$('modeLabel').textContent=local?'本机文件':'内网共享';$('rootLabel').textContent=local?'位置':'共享目录';$('location').readOnly=!local;$('go').hidden=!local;if(local){$('backToolbox').href=boot.ToolboxURL;$('backToolbox').hidden=false}else{$('backToolbox').removeAttribute('href');$('backToolbox').hidden=true;root=boot.Roots[0]?.ID||''}if(local&&window.toolboxFeedback?.connect){try{await window.toolboxFeedback.connect(boot.ToolboxURL)}catch(e){say('通用设置暂不可用：'+e.message,true)}}await loadPreferences();initializeExplorerShell();if(typeof initializeExplorerUpgrades==='function')initializeExplorerUpgrades();initializeMediaTools();side();if(!local&&!root){say('当前没有共享目录，请联系这台电脑的使用者');actions();return}initializeTabs();await switchFileTab(activeFileTab,{remember:false});if(preferenceLoadError)say(preferenceLoadError,true)}catch(e){if(!local&&base==='/'){$('loginPanel').hidden=false;$('app').hidden=true;if(e.status!==401)$('loginError').textContent='共享页面加载失败：'+e.message;else if(afterLogin)$('loginError').textContent='登录状态未保存，请允许本站 Cookie 后重试。'}else{$('app').hidden=false;say(e.message,true)}}}
start();


let shareSaving=false;
function showSharingResult(kind,title,message){
  if($('app').hidden||!$('sharingDialog').open)return;
  const dialog=$('shareResultDialog');dialog.dataset.result=kind;$('shareResultTitle').textContent=title;$('shareResultMessage').textContent=message;$('shareResultIcon').textContent=kind==='error'?'!':kind==='warning'?'!':'✓';
  if(!dialog.open)dialog.showModal();
}
$('shareResultClose').onclick=()=>$('shareResultDialog').close();
$('shareResultDialog').addEventListener('close',()=>{if($('sharingDialog').open&&!$('shareSave').disabled)$('shareSave').focus({preventScroll:true})});
async function saveSharingSettings(){
  if(!local||shareSaving||!$('sharingDialog').open)return;
  shareSaving=true;$('shareSave').disabled=true;$('shareSave').textContent='正在保存…';$('shareError').textContent='';
  try{
    const status=await api('sharing',{Enabled:$('shareEnabled').checked,OverlayEnabled:$('shareOverlay').checked,PublicURL:$('sharePublicURL').value.trim(),TrustedProxies:$('shareProxies').value.split(/[,，\n]/).map(value=>value.trim()).filter(Boolean),Port:Number($('sharePort').value),Password:$('shareCode').value,Roots:shareRoots});
    renderShare(status);
    const missing=Object.keys(status.Unavailable||{}).length;
    let kind='success',title='保存成功',message=status.Enabled?'内网共享已开启，可以使用共享设置中的访问地址连接。\n修改共享后，已登录设备需要重新登录。':'设置已保存，内网共享已关闭，下次启动也保持关闭。';
    if(status.Error){kind='warning';title='共享状态需要核对';message=status.Error}
    else if(missing){kind='warning';title='已保存，部分目录不可用';message=(status.Enabled?'共享已开启。':'共享已关闭。')+'有 '+missing+' 个目录暂不可用，请检查磁盘或路径；配置已保留。'}
    $('shareError').classList.toggle('error',kind!=='success');$('shareError').textContent=message;showSharingResult(kind,title,message);
  }catch(e){$('shareCode').value='';$('shareError').classList.add('error');$('shareError').textContent=e.message;showSharingResult('error','保存失败',e.message)}
  finally{shareSaving=false;$('shareSave').disabled=false;$('shareSave').textContent='保存并应用'}
}
