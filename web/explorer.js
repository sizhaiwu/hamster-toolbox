'use strict';
const $=id=>document.getElementById(id);
function canUseNativePrograms(){return local&&!boot?.Remote&&boot?.NativePrograms!==false;}
let local=false,boot=null,root='',path='',parent='',rows=[],total=0,offset=0,selection=new Set(),history=[],loading=false,mutating=false,view='grid',previewIndex=-1,movePath='',moveParent='',uploadRequest=null,nameAction='';
let textBytes=null,tileSize=180,tileGap=16;
let textPreviewCode=null,textPreviewAbort=null,textPreviewValue=null;
let textPreviewWrap=innerWidth<=700;try{const savedWrap=localStorage.getItem('hamster.files.preview.wrap');if(savedWrap!==null)textPreviewWrap=savedWrap==='true'}catch{}
let favorites=[],requestID=0,previewRequest=0,searchTimer;
let preferenceWrites=Promise.resolve(),preferenceSaveID=0,preferenceLoadError='';
let quickAccessSaving=false;
let selectionMode=false,selectionAnchor='',nativeOpening=false;
let pageSize=200,sidebarCollapsed=false,defaultSort='name',defaultOrder='asc';
const icons={image:'▧',video:'▶',audio:'♫',pdf:'PDF',text:'≡',file:'▤',drive:'▣'};
const base=location.pathname.startsWith('/files/')?'/files/':'/';
const bytes=n=>{if(n<1024)return n+' B';const units=['KiB','MiB','GiB','TiB'];let x=n/1024,i=0;while(x>=1024&&i<3){x/=1024;i++}return x.toFixed(2)+' '+units[i]};
let noticeTimer;
let listingDirectorySizeJob=null;
function setListingDirectorySize(job,text,title){
  job.meta.textContent=text;job.button.textContent=text;job.button.title=title;
  job.button.setAttribute('aria-label',title+'：'+job.item.Name);
}
function stopListingDirectorySize(){
  const job=listingDirectorySizeJob;if(!job)return;listingDirectorySizeJob=null;job.controller.abort();
  if(job.button.isConnected){setListingDirectorySize(job,job.previousText,job.previousTitle);job.button.removeAttribute('aria-busy')}
}
async function calculateListingDirectorySize(item,itemRoot,calculate,meta){
  if(loading||mutating)return;
  if(listingDirectorySizeJob){if(listingDirectorySizeJob.button===calculate)stopListingDirectorySize();else say('已有目录正在计算大小，可点击“计算中…”停止后再计算其他目录。',true);return}
  const job={item,button:calculate,meta,controller:new AbortController(),revision:requestID,previousText:calculate.textContent,previousTitle:calculate.title};listingDirectorySizeJob=job;
  setListingDirectorySize(job,'计算中…','正在计算目录大小，点击停止');calculate.setAttribute('aria-busy','true');
  const current=()=>listingDirectorySizeJob===job&&job.revision===requestID&&calculate.isConnected;
  try{
    const result=await api('properties-size',{Path:item.Path,Version:item.Version||''},itemRoot,job.controller.signal);if(!current())return;
    const label=(result.Complete&&!result.Skipped?'':'至少 ')+bytes(result.Size),details=result.Files+' 个文件，'+result.Directories+' 个子目录'+(result.Skipped?'，跳过 '+result.Skipped+' 项':'');
    setListingDirectorySize(job,label,details+'；'+(result.Note||'按文件大小统计，非磁盘占用')+'。点击重新计算');
  }catch(e){if(current()&&e.name!=='AbortError'){setListingDirectorySize(job,'重试','计算失败：'+e.message+'。点击重试');say('目录大小计算失败：'+e.message,true)}}
  finally{if(listingDirectorySizeJob===job){listingDirectorySizeJob=null;calculate.removeAttribute('aria-busy')}}
}
window.addEventListener('pagehide',stopListingDirectorySize);
function say(text,bad=false,quiet=false){
  clearTimeout(noticeTimer);$('notice').textContent=text;$('notice').classList.toggle('error',bad);$('notice').setAttribute('role',bad?'alert':'status');
  $('noticeToast').hidden=quiet||!text;$('noticeToast').classList.toggle('error',bad);
  if(!bad&&!quiet&&text&&!/^(正在|请等待)/.test(text))noticeTimer=setTimeout(()=>{$('noticeToast').hidden=true},6500);
}
$('dismissNotice').onclick=()=>{clearTimeout(noticeTimer);$('noticeToast').hidden=true};
$('browserHelp').onclick=()=>say('单击选中，双击打开；F2 改名、Delete 删除确认、方向键选择、Backspace 后退。'+(writable()?' Ctrl/⌘ 多选，Shift 连选。':' 此共享只读。'));
function endpoint(route,extra={},requestRoot=root){const query=new URLSearchParams({...((!local&&requestRoot)?{root:requestRoot}:{}),...extra});return base+route+(query.size?'?'+query:'')}
async function api(route,data={},requestRoot=root,signal){
  const response=await fetch(endpoint('api/'+route,{},requestRoot),{method:'POST',credentials:'same-origin',headers:{'Content-Type':'application/json','X-Hamster-Browser':'1'},body:JSON.stringify(data),signal,keepalive:route==='preferences'});
  if(!response.ok){
    if(response.status===401&&!signal?.aborted){const browsing=!$('app').hidden;clearViewerImageCache();stopListingMedia();stopListingDirectorySize();closeFloatingMedia();if(editorMinimized)disposeFileEditorWorkspace();for(const dialog of document.querySelectorAll('dialog[open]:not(#remoteSessionExpired)'))dialog.close();navigationAbort?.abort();requestID++;if(boot?.Remote){window.toolboxRemoteSessionExpired?.()}else{$('app').hidden=true;$('loginPanel').hidden=false;if(route!=='login'&&(browsing||response.headers?.get('X-Hamster-Session')==='expired-guest'))$('loginError').textContent='共享会话已过期，请重新输入访问口令';}}
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
  $('openLocal').hidden=!canUseNativePrograms();$('openLocal').disabled=!canUseNativePrograms()||!path||blocked||nativeOpening;
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
function moveExplorerKeyboardSelection(event){
 const focus=document.activeElement,inside=focus===document.body||focus===$('entries')||Boolean(focus.closest('#entries'));
 if(!inside||!rows.length||event.altKey||event.ctrlKey||event.metaKey)return false;
 const key=event.key;if(!['ArrowUp','ArrowDown','ArrowLeft','ArrowRight','Home','End'].includes(key))return false;
 if(view==='list'&&['ArrowLeft','ArrowRight'].includes(key))return false;
 const cards=[...$('entries').querySelectorAll('.entry')],focused=focus.closest('.entry')?.dataset.path;
 let index=rows.findIndex(item=>item.Path===focused);if(index<0)index=rows.findIndex(item=>selection.has(item.Path));
 let next=index;
 if(key==='Home')next=0;else if(key==='End')next=rows.length-1;else if(index<0)next=key==='ArrowUp'||key==='ArrowLeft'?rows.length-1:0;
 else{
  let columns=1;
  if(view!=='list'&&cards.length>1){const first=cards[0].getBoundingClientRect();if(first.height>0){columns=cards.length;for(let n=1;n<cards.length;n++){if(Math.abs(cards[n].getBoundingClientRect().top-first.top)>2){columns=n;break}}}}
  if(key==='ArrowLeft')next=Math.max(0,index-1);if(key==='ArrowRight')next=Math.min(rows.length-1,index+1);
  if(key==='ArrowUp'&&index>=columns)next=index-columns;
  if(key==='ArrowDown'&&Math.floor(index/columns)<Math.floor((rows.length-1)/columns))next=Math.min(rows.length-1,index+columns);
 }
 event.preventDefault();
 if(!event.shiftKey)selection.clear();
 selectItem(rows[next],event,true);
 const open=cards[next]?.querySelector('.entry-open');open?.focus({preventScroll:true});open?.scrollIntoView?.({block:'nearest',inline:'nearest'});
 return true;
}
function explorerFileShortcut(event){
 if(event.altKey||event.ctrlKey||event.metaKey||!$('contextMenu').hidden)return;
 const focus=document.activeElement,listFocus=focus===document.body||focus===$('entries')||Boolean(focus.closest('#entries'));
 if(event.key==='Backspace'&&!event.shiftKey){event.preventDefault();if(!event.repeat)$('back').click();return}
 if(event.key==='F5'&&!event.shiftKey){event.preventDefault();if(!event.repeat)$('refresh').click();return}
 if(!listFocus)return;
 if(event.key==='F2'&&!event.shiftKey&&!$('rename').disabled){
  event.preventDefault();if(event.repeat)return;const item=selected()[0];$('rename').click();
  const dot=item?.Name.lastIndexOf('.');if(item&&!item.Dir&&dot>0)$('newName').setSelectionRange(0,dot);return;
 }
 if(event.key==='Delete'&&!event.shiftKey&&!$('remove').disabled){event.preventDefault();if(!event.repeat)$('remove').click();return}
 if(event.key==='Enter'&&!event.shiftKey&&selected().length===1&&!$('openSelected').disabled){event.preventDefault();if(!event.repeat)$('openSelected').click();return}
 moveExplorerKeyboardSelection(event);
}
document.addEventListener('keydown',event=>{
  if(event.defaultPrevented)return;
  const focus=document.activeElement,editing=focus.isContentEditable||focus.closest('[contenteditable]:not([contenteditable=false]),.floating-window,video,audio')||focus.matches('textarea,select,input:not([type=checkbox]):not([type=radio])');
  if($('app').hidden||document.querySelector('dialog[open]')||editing||loading||mutating)return;
  explorerFileShortcut(event);
  if((event.ctrlKey||event.metaKey)&&event.key.toLowerCase()==='a'){event.preventDefault();selectPage()}
  if((event.ctrlKey||event.metaKey)&&!event.altKey){
    const key=event.key.toLowerCase();
    if(key==='v'&&resourceClipboard){event.preventDefault();closeContextMenu();pasteSelectedResources()}
    if((key==='c'||key==='x')&&selection.size&&!window.getSelection()?.toString()){event.preventDefault();closeContextMenu();copySelectedResources(key==='x'?'move':'copy')}
  }
  if(event.key==='Escape'&&(selectionMode||selection.size)){event.preventDefault();selectionMode=false;selection.clear();selectionAnchor='';actions()}
});
$('openLocal').onclick=async()=>{if(!canUseNativePrograms()||!path||loading||mutating||nativeOpening)return;const folder=path;nativeOpening=true;actions();try{await api('open-local',{Path:folder});if(path===folder)say('已在本机系统文件管理器中打开当前目录')}catch(e){say(e.message,true)}finally{nativeOpening=false;actions()}};
function actions(){const items=selected(),stable=items.every(x=>x.Version),write=writable()&&Boolean(path||root)&&!loading&&!mutating;$('newFile').disabled=$('newFolder').disabled=$('upload').disabled=$('uploadFolder').disabled=!write;$('rename').disabled=items.length!==1||!write||!stable;$('move').disabled=$('remove').disabled=$('copyTo').disabled=!items.length||!write||!stable;$('download').disabled=items.length!==1||items[0]?.Dir||mutating;$('openSelected').hidden=items.length!==1;$('openSelected').disabled=items.length!==1||loading||mutating;$('permission').textContent=local?'本机管理':writable()?'允许修改':'只读共享';$('summary').textContent=total+' 项'+(items.length?' · 已选 '+items.length+' 项':'');updateTabActions();syncSelection()}
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

async function navigate(next,{remember=true,page=0,targetRoot=root,mode='',scroll,force=false,selectionSnapshot=null}={}){
  if(mutating){say('请等待当前文件操作结束',true);return false}
  stopVideoHover();stopListingMedia();stopListingDirectorySize();closeContextMenu();
  const tabID=activeFileTab,id=++requestID,previousLocation={root,path,offset,scroll:scrollRestorePending?.value??Math.max(0,window.scrollY||0),search:$('search').value,sort:$('sort').value,order:$('order').value,view,pageSize};
  captureDirectorySelection(previousLocation);
  const destination=mode==='back'?history.at(-1):mode==='forward'?future.at(-1):remember&&next===parent?history.findLast(item=>item.root===targetRoot&&sameBrowserPath(item.path,next)):null;
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
    const restoredFocus=restoreDirectorySelection(destination||selectionSnapshot||(!changed&&offset===previousLocation.offset?previousLocation:null));
    render();breadcrumb();side();
    const tab=activeTab();if(tab)Object.assign(tab,{root,path,back:[...history],forward:[...future],search,sort,order,view,offset,scroll});
    renderTabs();persistTabs();
    say((data.Truncated?'目录超过20000项，仅展示前20000项，请进入更具体的子目录。':'单击选中，双击打开；也可选中后点“打开”。'+(writable()?' Ctrl/⌘ 多选，Shift 连选。':' 此共享只读。'))+(!workspaceStorageOK?' 浏览器未允许保存标签，刷新后无法恢复。':''),false,!data.Truncated&&workspaceStorageOK);
    $('previous').disabled=offset===0;$('next').disabled=offset+rows.length>=total;
    $('page').textContent=total?Math.floor(offset/pageSize)+1+' / '+Math.ceil(total/pageSize):'0 / 0';closeNav();
    scrollRestorePending={tab:tabID,value:scroll};
    requestAnimationFrame(()=>{if(id===requestID&&tabID===activeFileTab){window.scrollTo({top:scroll,behavior:'auto'});scrollRestorePending=null;if(restoredFocus&&!document.activeElement.matches('input,textarea,select,[contenteditable]')){const card=[...$('entries').querySelectorAll('.entry')].find(item=>item.dataset.path===restoredFocus);card?.querySelector('.entry-open')?.focus({preventScroll:true})}}});
    return true;
  }catch(e){if(id===requestID&&tabID===activeFileTab&&e.name!=='AbortError')say(e.message,true);return false}
  finally{if(id===requestID&&tabID===activeFileTab){loading=false;navigationAbort=null;$('cancelSearch').hidden=true;actions()}}
}
function render(){
  stopVideoHover();stopListingMedia();stopListingDirectorySize();
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
    if(item.Dir&&item.Kind!=='drive'){
      const calculate=button('计算',event=>{event.preventDefault();event.stopPropagation();calculateListingDirectorySize(item,itemRoot,calculate,meta)},'entry-size');
      calculate.title='计算目录大小';calculate.setAttribute('aria-label','计算目录大小：'+item.Name);calculate.ondblclick=event=>{event.preventDefault();event.stopPropagation()};
      meta.textContent='计算';meta.classList.add('entry-size-placeholder');card.append(calculate);
    }
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
  const item=rows[previewIndex];if(!canUseNativePrograms()||!['image','video'].includes(item?.Kind)||$('viewerNative').disabled)return;
  const id=previewRequest,source={...item};$('viewerNative').disabled=true;$('viewerBody').querySelector('video')?.pause();
  const opened=await openNativeMedia(source);
  if(id!==previewRequest)return;
  if(opened&&$('viewer').open)$('viewer').close();else $('viewerNative').disabled=false;
};
function clearViewer(keepFilmstrip=false){if(!keepFilmstrip)stopViewerImagePrefetch();clearTextPreview();previewPagingAbort?.abort();previewPagingAbort=null;disposeVideoPlayback();if(!keepFilmstrip){clearViewerFilmstrip();clearViewerQueue()}disposeImageViewer();$('viewerTools').hidden=true;$('imageTools').hidden=true;$('videoTools').hidden=true;configureImagePreview(null);$('viewerNative').hidden=true;$('viewerNative').disabled=false;document.body.classList.remove('viewer-open');textBytes=null;previewRequest++;for(const media of $('viewerBody').querySelectorAll('video,audio')){media.pause();media.removeAttribute('src');media.load()}$('viewerBody').replaceChildren()}
async function preview(index){
  stopVideoHover();
  const item=rows[index];if(!item||item.Dir)return;
  if(item.Kind==='video'&&canUseNativePrograms()&&window.toolboxSettings?.DefaultPlayer==='mpv'&&await openNativeMedia(item))return;
  clearViewer(true);const id=previewRequest;previewIndex=index;
  $('viewerNative').hidden=!canUseNativePrograms()||!['image','video'].includes(item.Kind);$('viewerNative').textContent=item.Kind==='image'?'原生查看':'mpv 播放';
  $('viewer').dataset.kind=item.Kind;configureImagePreview(item);configureTextPreview(item);$('textEncoding').hidden=item.Kind!=='text';$('textEncoding').value='auto';
  $('viewerHint').textContent=item.Kind==='image'?'滚轮 / 双指缩放 · 放大后拖动 · ← → 切换':'当前目录同类文件，可用两侧按钮切换';
  $('viewerPrevious').disabled=previewNeighbor(-1)<0&&!canPreviewPage(-1);$('viewerNext').disabled=previewNeighbor(1)<0&&!canPreviewPage(1);
  $('viewerTitle').textContent=item.Name;$('viewerTitle').title=item.Name;
  $('viewerInfo').textContent=bytes(item.Size)+(item.Modified?' · '+formatExplorerDate(item.Modified):'');
  const body=$('viewerBody'),url=endpoint('raw',{path:item.Path});
  document.body.classList.add('viewer-open');if(!$('viewer').open)$('viewer').showModal();
  updateViewerFilmstrip();updateViewerQueue();
  if(item.Kind==='image'){
    const img=document.createElement('img');img.alt=item.Name;img.draggable=false;img.decoding='async';
    img.onerror=()=>{
      if(id!==previewRequest)return;
      if(!img.dataset.compatible){imageViewer?.loading();img.dataset.compatible='1';img.src=endpoint('image-preview',{path:item.Path});$('viewerHint').textContent='正在生成兼容预览；原图不变。';return}
      disposeImageViewer();$('viewerTools').hidden=true;body.replaceChildren();const p=document.createElement('p');p.textContent='设备无法预览此图片，可下载原文件查看。';body.append(p);
    };
    $('viewerTools').hidden=false;$('imageTools').hidden=false;installImageViewer(img);installViewerImageSource(img,item,root,()=>id===previewRequest);
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
    const panel=document.createElement('div');panel.className='text-preview-panel';const mount=document.createElement('div');mount.id='textPreviewMount';mount.className='text-preview-code';mount.hidden=true;
    const pre=document.createElement('pre');pre.className='text-preview-fallback';pre.textContent='正在读取…';panel.append(mount,pre);body.append(panel);
    const controller=new AbortController();textPreviewAbort=controller;$('viewerHint').textContent='正在读取文本…';
    try{const response=await fetch(endpoint('text',{path:item.Path}),{signal:controller.signal});if(!response.ok){const value=await response.text();if(id===previewRequest){pre.textContent='无法预览：'+value;$('viewerHint').textContent='预览未能加载，可下载原文件查看'}}else{const value=await response.arrayBuffer();if(id===previewRequest&&!controller.signal.aborted){textBytes=new Uint8Array(value);decodeText()}}}catch(e){if(id===previewRequest&&!controller.signal.aborted){pre.textContent='无法预览：'+e.message;$('viewerHint').textContent='文本读取失败'}}

  }else{
    const p=document.createElement('p');p.textContent='此文件没有内置预览，可点击“下载原文件”后用对应软件打开。Office 文档和压缩包不会上传到第三方服务。';body.append(p);
  }
}

function clearTextPreview(){
 textPreviewAbort?.abort();textPreviewAbort=null;textPreviewCode?.destroy();textPreviewCode=null;textPreviewValue=null;
 $('viewerTextActions').hidden=true;$('viewerTextCopy').disabled=true;$('viewerTextCopy').textContent='复制';$('viewerEdit').hidden=true;
 const main=$('viewerBody').parentElement;for(const id of ['viewerPrevious','viewerNext'])main.append($(id));
}
function configureTextPreview(item){
 if(item.Kind!=='text')return;
 $('viewerTextActions').hidden=false;$('viewerEdit').hidden=!writable();$('viewerTextActions').append($('viewerPrevious'),$('viewerNext'));
 $('viewerTextWrap').setAttribute('aria-pressed',String(textPreviewWrap));$('viewer').dataset.textWrap=String(textPreviewWrap);
 $('viewerTools').hidden=false;
}
function decodeText(){
 if(!textBytes||rows[previewIndex]?.Kind!=='text')return;let encoding=$('textEncoding').value,decoded;
 if(encoding==='auto'){if(textBytes[0]===255&&textBytes[1]===254)encoding='utf-16le';else if(textBytes[0]===254&&textBytes[1]===255)encoding='utf-16be';else{try{decoded=new TextDecoder('utf-8',{fatal:true}).decode(textBytes);encoding='utf-8'}catch{encoding='gb18030'}}}
 const pre=$('viewerBody').querySelector('pre'),mount=$('textPreviewMount');if(!pre||!mount)return;
 try{
  const value=decoded??new TextDecoder(encoding).decode(textBytes);textPreviewValue=value;
  if(window.hamsterCodeEditor?.preview){try{if(textPreviewCode)textPreviewCode.setText(value);else textPreviewCode=window.hamsterCodeEditor.preview({parent:mount,text:value,name:rows[previewIndex].Name,nonce:document.querySelector('meta[name="file-editor-nonce"]')?.content,wrap:textPreviewWrap});mount.hidden=false;pre.hidden=true;pre.textContent=''}catch{textPreviewCode?.destroy();textPreviewCode=null;mount.replaceChildren();mount.hidden=true;pre.hidden=false;pre.textContent=value}}else pre.textContent=value;
  let count=textPreviewCode?.view.state.doc.lines;if(!count){count=1;for(let i=0;i<value.length;i++){if(value.charCodeAt(i)===13){count++;if(value.charCodeAt(i+1)===10)i++}else if(value.charCodeAt(i)===10)count++}}$('viewerHint').textContent='只读预览 · '+encoding.toUpperCase()+' · '+count.toLocaleString()+' 行';$('viewerTextCopy').disabled=false;
 }catch{pre.textContent='当前设备不支持此编码，请下载查看';pre.hidden=false;mount.hidden=true;textPreviewValue=null;$('viewerTextCopy').disabled=true;$('viewerHint').textContent='编码解码失败'}
}
$('viewerEdit').onclick=()=>{const item=rows[previewIndex];if(item?.Kind==='text'&&writable())openTextEditor(item)};
$('viewerTextWrap').onclick=()=>{textPreviewWrap=!textPreviewWrap;$('viewer').dataset.textWrap=String(textPreviewWrap);$('viewerTextWrap').setAttribute('aria-pressed',String(textPreviewWrap));textPreviewCode?.setWrap(textPreviewWrap);try{localStorage.setItem('hamster.files.preview.wrap',String(textPreviewWrap))}catch{}};
$('viewerTextCopy').onclick=async()=>{if(textPreviewValue===null)return;const id=previewRequest,value=textPreviewValue;try{await navigator.clipboard.writeText(value);if(id===previewRequest)$('viewerTextCopy').textContent='已复制'}catch{if(id===previewRequest)$('viewerHint').textContent='复制失败，请选中文字后手动复制'}};
$('textEncoding').onchange=decodeText;
function previewNeighbor(step){let next=previewIndex+step;const kind=rows[previewIndex]?.Kind;const media=['image','video','audio'].includes(kind);while(rows[next]&&(rows[next].Dir||(media&&rows[next].Kind!==kind)))next+=step;return rows[next]?next:-1}
function advance(step){const next=previewNeighbor(step);if(next>=0)return preview(next);return previewBeyondPage(step)}$('viewerPrevious').onclick=()=>advance(-1);$('viewerNext').onclick=()=>advance(1);$('viewerClose').onclick=()=>$('viewer').close();$('viewer').addEventListener('close',()=>{if(document.fullscreenElement===$('viewerSurface')){const exiting=document.exitFullscreen?.();exiting?.catch?.(()=>{})}clearViewer()});document.addEventListener('keydown',e=>{if(!$('viewer').open||rows[previewIndex]?.Kind!=='image'||['INPUT','TEXTAREA'].includes(document.activeElement.tagName))return;if(handleImageKey(e))return;if(e.key==='ArrowLeft')advance(-1);if(e.key==='ArrowRight')advance(1)});
function join(dir,name){return local?dir.replace(/[\\/]$/,'')+separator()+name:(dir?dir.replace(/\/$/,'')+'/':'')+name}
function openName(action){nameAction=action;$('nameTitle').textContent=action==='mkdir'?'新建文件夹':action==='create'?'新建文件':'修改名称';$('newName').value=action==='mkdir'?'新建文件夹':action==='create'?'新建文件.txt':selected()[0]?.Name||'';$('nameError').textContent='';$('nameDialog').showModal();$('newName').focus();$('newName').select()}
$('newFile').onclick=()=>openName('create');$('newFolder').onclick=()=>openName('mkdir');$('rename').onclick=()=>openName('rename');$('nameCancel').onclick=()=>$('nameDialog').close();
$('newName').onkeydown=event=>{if(event.key==='Enter'&&!event.isComposing){event.preventDefault();if(!$('nameSave').disabled)$('nameSave').click()}};
$('nameSave').onclick=async()=>{
 if(mutating)return;const action=nameAction,creating=['mkdir','create'].includes(action),folder=path,itemRoot=root,name=$('newName').value;
 mutating=true;actions();$('nameSave').disabled=true;
 try{
  const result=await api('mutate',{Action:action,Path:creating?folder:selected()[0].Path,Name:name,Version:creating?'':selected()[0].Version},itemRoot);
  $('nameDialog').close();mutating=false;
  await navigate(path,{remember:false,force:true});
  if(action==='create'){
   say('已新建文件：'+name);
   if(root===itemRoot&&path===folder&&result.Path&&['text','file'].includes(result.Kind))await openTextEditor({Name:name,Path:result.Path,Kind:result.Kind});
  }
 }catch(e){$('nameError').textContent=e.message;if(!$('nameDialog').open)say(e.message,true)}
 finally{mutating=false;$('nameSave').disabled=false;actions()}
};
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
$('toggleCode').onclick=()=>{const hidden=$('accessCode').dataset.hidden==='true';$('accessCode').dataset.hidden=String(!hidden);$('toggleCode').textContent=hidden?'隐藏':'显示'};$('login').onclick=async()=>{$('login').disabled=true;$('loginError').textContent='';try{await api('login',{Password:$('accessCode').value});$('accessCode').value='';await initializeExplorer(true)}catch(e){$('loginError').textContent=e.message}finally{$('login').disabled=false}};$('accessCode').onkeydown=e=>{if(e.key==='Enter')$('login').click()};$('logout').onclick=async()=>{if(editorTabs.some(tabDirty)||savingEditor()){restoreEditorWindow();editorBatchMessage='请先保存或关闭编辑器，再退出共享';updateFileEditor();return}if(editorTabs.length)disposeFileEditorWorkspace();try{forgetTabs();clearViewerImageCache();await api('logout')}finally{location.reload()}};
const initializeExplorer=async(afterLogin=false)=>{try{boot=await api('boot');local=boot.Local;$('app').hidden=false;$('loginPanel').hidden=true;$('favorite').hidden=!local;$('logout').hidden=local;$('modeLabel').textContent=boot.Remote?'远程文件':local?'本机文件':'内网共享';if(boot.Remote)$('permission').textContent='远程管理';$('rootLabel').textContent=local?'位置':'共享目录';$('location').readOnly=!local;$('go').hidden=!local;if(local){$('backToolbox').href=boot.ToolboxURL;$('backToolbox').hidden=false}else{$('backToolbox').removeAttribute('href');$('backToolbox').hidden=true;root=boot.Roots[0]?.ID||''}if(local&&window.toolboxFeedback?.connect){try{await window.toolboxFeedback.connect(boot.ToolboxURL)}catch(e){say('通用设置暂不可用：'+e.message,true)}}await loadPreferences();initializeExplorerShell();if(typeof initializeExplorerUpgrades==='function')initializeExplorerUpgrades();initializeMediaTools();side();if(!local&&!root){say('当前没有共享目录，请联系这台电脑的使用者');actions();return}initializeTabs();await switchFileTab(activeFileTab,{remember:false});if(preferenceLoadError)say(preferenceLoadError,true)}catch(e){if(!local&&base==='/'){$('loginPanel').hidden=false;$('app').hidden=true;if(e.status!==401)$('loginError').textContent='共享页面加载失败：'+e.message;else if(afterLogin)$('loginError').textContent='登录状态未保存，请允许本站 Cookie 后重试。'}else{$('app').hidden=false;say(e.message,true)}}};
initializeExplorer();

let fileEdit=null,fileCode=null,pendingEditorAction=null,pendingEditorTabs=[],editorTree=null;
let editorTabs=[],editorSavingAll=false,editorTabCounter=0,editorBatchMessage='';
function editorTabKey(item,itemRoot){const value=item.Path.replace(/\\/g,'/');return itemRoot+'\0'+(boot?.Separator==='\\'?value.toLowerCase():value)}
function editorDirty(){return Boolean(fileEdit&&(fileCode?!fileCode.isClean(fileEdit.saved):$('fileEditorContent').value!==fileEdit.saved))}
function tabDirty(tab){return tab===fileEdit?editorDirty():tab.dirty??tab.text!==tab.saved}
function savingEditor(){return editorSavingAll||editorTabs.some(tab=>tab.busy&&!tab.loading)}
function syncEditorText(){const value=fileCode?fileCode.getText():$('fileEditorContent').value;if($('fileEditorContent').value!==value)$('fileEditorContent').value=value;if(fileEdit)fileEdit.text=value;return value}
function captureEditorTab(){if(!fileEdit)return;syncEditorText();fileEdit.dirty=editorDirty();fileEdit.language=$('fileEditorLanguage').value;fileEdit.session=fileCode?.snapshot()||null;fileEdit.selection=editorSelection();fileEdit.scrollTop=$('fileEditorContent').scrollTop;fileEdit.scrollLeft=$('fileEditorContent').scrollLeft}
function setEditorText(value,history=false){$('fileEditorContent').value=value;fileCode?.setText(value,history);if(fileEdit)fileEdit.text=value}
function editorSelection(){return fileCode?.selection()||{start:$('fileEditorContent').selectionStart,end:$('fileEditorContent').selectionEnd}}
function selectEditorText(start,end){if(fileCode)fileCode.select(start,end);else{$('fileEditorContent').focus();$('fileEditorContent').setSelectionRange(start,end)}updateEditorCursor()}
function replaceEditorText(value){if(fileCode)fileCode.replace(value);else{const input=$('fileEditorContent');input.setRangeText(value,input.selectionStart,input.selectionEnd,'end')}updateFileEditor()}
function setEditorLabel(node,value){if(node.textContent!==value)node.textContent=value}
function setEditorAttribute(node,name,value){value=String(value);if(node.getAttribute(name)!==value)node.setAttribute(name,value)}
function renderEditorTabs(){
 const list=$('fileEditorTabs'),names=new Map();for(const tab of editorTabs)names.set(tab.item.Name,(names.get(tab.item.Name)||0)+1);
 for(const tab of editorTabs){
  if(!tab.ui){
   const wrap=document.createElement('div');wrap.className='file-editor-tab';
   const label=document.createElement('span'),trigger=button('',()=>activateEditorTab(tab));trigger.className='file-editor-tab-open';trigger.setAttribute('role','tab');trigger.setAttribute('aria-controls','fileCodeMount');trigger.dataset.editorTab=tab.id;trigger.append(controlIcon('text'),label);
   const close=button('×',()=>closeEditorTab(tab));close.className='file-editor-tab-close';wrap.append(trigger,close);tab.ui={wrap,trigger,label,close};list.append(wrap);
  }
  const {wrap,trigger,label,close}=tab.ui,active=tab===fileEdit,scope=local?'':(boot?.Roots?.find(entry=>entry.ID===tab.root)?.Name||'共享目录');
  const name=tab.item.Name+(names.get(tab.item.Name)>1?' · '+[scope,editorParentPath(tab.item.Path)].filter(Boolean).join('/'):'');
  wrap.classList.toggle('active',active);setEditorLabel(label,name+(tabDirty(tab)?' ●':'')+(tab.error?' !':''));
  setEditorAttribute(trigger,'aria-selected',active);setEditorAttribute(trigger,'title',tab.item.Path+(tab.error?'\n'+tab.error:''));if(trigger.tabIndex!==(active?0:-1))trigger.tabIndex=active?0:-1;
  setEditorAttribute(close,'title','关闭 '+tab.item.Name);setEditorAttribute(close,'aria-label','关闭文件 '+name);if(close.disabled!==(tab.busy&&!tab.loading))close.disabled=tab.busy&&!tab.loading;
 }
}
function updateEditorCursor(){
 let position;if(fileCode)position=fileCode.cursor();else{const value=$('fileEditorContent').value,at=$('fileEditorContent').selectionStart,prefix=value.slice(0,at);position={line:prefix.split('\n').length,column:at-(prefix.lastIndexOf('\n')+1)+1}}
 setEditorLabel($('fileEditorCursor'),'行 '+position.line+'，列 '+position.column);
}
function updateFileEditor(){
 if(fileEdit&&!fileCode)fileEdit.text=$('fileEditorContent').value;
 const dirty=editorDirty(),disabled=!fileEdit||fileEdit.loading||!fileEdit.revision;
 $('fileEditorSave').disabled=disabled||editorSavingAll||Boolean(fileEdit?.busy)||!dirty;$('fileEditorSaveAll').disabled=savingEditor()||!editorTabs.some(tab=>tabDirty(tab)&&!tab.busy&&tab.revision);
 $('fileEditorContent').disabled=disabled;fileCode?.setDisabled(disabled);$('fileEditorComplete').disabled=disabled||!fileCode;
 $('fileEditorClose').disabled=savingEditor();$('fileEditorReload').disabled=!fileEdit||fileEdit.busy;
 for(const id of ['fileEditorKeep','fileEditorSaveContinue','fileEditorDiscardConfirm'])$(id).disabled=savingEditor();
 const state=fileEdit?.busy?'正在处理…':dirty?'未保存':fileEdit?.savedAt?'已保存 · '+fileEdit.savedAt:'已载入';if($('fileEditorStatus').textContent!==state)$('fileEditorStatus').textContent=state;
 setEditorLabel($('fileEditorName'),fileEdit?.item.Name||'');setEditorLabel($('fileEditorPath'),fileEdit?.item.Path||'');setEditorAttribute($('fileEditorPath'),'title',fileEdit?.item.Path||'');
 setEditorLabel($('fileEditorFormat'),fileEdit?.format||'');setEditorAttribute($('fileEditorFormat'),'title',fileEdit?.backup?'备份位置：'+fileEdit.backup:'');
 setEditorLabel($('fileEditorError'),fileEdit?.error||'');setEditorLabel($('fileEditorBatchStatus'),editorBatchMessage);
 renderEditorTabs();updateEditorCursor();updateEditorDock();
}
function ensureEditorView(name){
 if(fileCode||!window.hamsterCodeEditor)return;
 try{fileCode=window.hamsterCodeEditor.mount({parent:$('fileCodeMount'),source:$('fileEditorContent'),name,syncSource:false,themeMode:$('fileEditorTheme').value,indentSize:Number($('fileEditorIndent').value),nonce:document.querySelector('meta[name="file-editor-nonce"]')?.content,onChange:updateFileEditor,onSelection:updateEditorCursor,onSave:saveFileEditor,onSaveAll:saveAllFileEditors,onSearch:openEditorSearch})}catch{if(fileEdit)fileEdit.error='代码编辑器未能启动，当前使用纯文本模式。'}
}
function activateEditorTab(tab){
 if(!editorTabs.includes(tab))return;captureEditorTab();fileEdit=tab;
 $('fileEditorContent').value=tab.text;$('fileEditorLanguage').value=tab.language||'auto';ensureEditorView(tab.item.Name);
 if(fileCode){if(tab.session)fileCode.restore(tab.session);else fileCode.reset(tab.text,tab.item.Name,tab.language||'auto');fileCode.setTheme($('fileEditorTheme').value);fileCode.setIndent($('fileEditorIndent').value)}
 else{const selection=tab.selection||{start:0,end:0};$('fileEditorContent').setSelectionRange(selection.start,selection.end);$('fileEditorContent').scrollTop=tab.scrollTop||0;$('fileEditorContent').scrollLeft=tab.scrollLeft||0}
 if(editorTree&&editorTree.root!==tab.root)initializeEditorTree(tab.item);
 if(innerWidth<=700){$('fileEditor').classList.remove('tree-visible');$('fileEditorTreeToggle').setAttribute('aria-expanded','false')}
 updateFileEditor();markEditorTreeSelection();$('fileEditorTabs').querySelector('.active')?.scrollIntoView?.({block:'nearest',inline:'nearest'});fileCode?.resize();
}
async function readEditorTab(tab){
 tab.controller?.abort();const controller=new AbortController();tab.controller=controller;tab.busy=true;tab.loading=true;tab.error='';if(tab===fileEdit)updateFileEditor();
 try{
  const data=await api('text-open',{Path:tab.item.Path},tab.root,controller.signal);
  if(!editorTabs.includes(tab)||tab.controller!==controller)return;
  tab.revision=data.Revision;tab.saved=data.Text;tab.text=data.Text;tab.dirty=false;tab.session=null;tab.selection={start:0,end:0};tab.savedAt='';tab.backup='';tab.format=data.Encoding+(data.BOM?' · BOM':'')+' · '+data.Newline;
  if(tab===fileEdit){$('fileEditorContent').value=data.Text;if(fileCode)fileCode.reset(data.Text,tab.item.Name,tab.language||'auto')}
 }catch(error){if(editorTabs.includes(tab)&&tab.controller===controller&&error.name!=='AbortError')tab.error=error.message}
 finally{if(editorTabs.includes(tab)&&tab.controller===controller){tab.busy=false;tab.loading=false;updateFileEditor()}}
}
async function openTextEditor(item,keepTree=false,itemRoot=root){
 if(!(local||boot?.Roots?.find(entry=>entry.ID===itemRoot)?.Writable)||!item||item.Dir)return;
 if($('viewer').open)$('viewer').close();restoreEditorWindow();
 const key=editorTabKey(item,itemRoot),existing=editorTabs.find(tab=>tab.key===key);
 if(existing){activateEditorTab(existing);return existing.pending}
 const first=!$('fileEditor').open;
 if(first){editorBatchMessage='';try{const font=localStorage.getItem('hamster.files.editor.font');if(['14','16','18','20'].includes(font))$('fileEditorFont').value=font;const indent=localStorage.getItem('hamster.files.editor.indent');if(['2','4'].includes(indent))$('fileEditorIndent').value=indent;const theme=localStorage.getItem('hamster.files.editor.theme');if(['light','dark'].includes(theme))$('fileEditorTheme').value=theme}catch{}$('fileEditor').dataset.editorTheme=$('fileEditorTheme').value;$('fileEditor').style.setProperty('--editor-font',$('fileEditorFont').value+'px');$('fileEditor').classList.add('editor-expanded');$('fileEditorFullscreen').textContent='还原';$('fileEditor').showModal()}
 const tab={id:'edit-'+(++editorTabCounter),key,item:{...item},root:itemRoot,text:'',saved:'',busy:true,loading:true,revision:'',error:'',format:'',language:'auto'};editorTabs.push(tab);activateEditorTab(tab);
 if(first||!editorTree)initializeEditorTree(item);resizeFileEditor();
 tab.pending=readEditorTab(tab);await tab.pending;
}
async function saveEditorTab(tab){
 if(tab===fileEdit)captureEditorTab();if(!editorTabs.includes(tab)||tab.busy||!tab.revision||!tabDirty(tab))return !tabDirty(tab);
 const value=tab.text;tab.busy=true;tab.error='';updateFileEditor();
 try{
  const data=await api('text-save',{Path:tab.item.Path,Revision:tab.revision,Text:value},tab.root);
  if(!editorTabs.includes(tab))return false;
  tab.revision=data.Revision;tab.saved=value;tab.dirty=tab.text!==value;tab.savedAt=new Date().toLocaleTimeString();tab.backup=data.Backup||'';tab.format=data.Encoding+(data.BOM?' · BOM':'')+' · '+data.Newline;
  return true;
 }catch(error){if(editorTabs.includes(tab))tab.error=error.message+'；当前编辑内容仍保留，可复制后重新读取对比。';return false}
 finally{if(editorTabs.includes(tab)){tab.busy=false;updateFileEditor()}}
}
async function refreshEditorListing(){try{await navigate(path,{remember:false,force:true,page:offset})}catch{editorBatchMessage='文件已保存；目录刷新失败，可稍后刷新。';updateFileEditor()}}
async function saveFileEditor(){const tab=fileEdit;if(!tab||editorSavingAll)return false;const result=await saveEditorTab(tab);if(result)await refreshEditorListing();return result}
async function saveAllFileEditors(){
 if(savingEditor())return false;captureEditorTab();editorSavingAll=true;editorBatchMessage='正在保存全部文件…';updateFileEditor();let saved=0,failed=0;
 try{for(const tab of [...editorTabs])if(tabDirty(tab)){if(await saveEditorTab(tab))saved++;else failed++}}
 finally{editorSavingAll=false;const remaining=editorTabs.filter(tabDirty).length;editorBatchMessage='已保存 '+saved+' 个'+(remaining?'；'+remaining+' 个仍未保存':'')+(failed?'（'+failed+' 个保存失败）':'');updateFileEditor()}
 if(saved)await refreshEditorListing();return !editorTabs.some(tab=>tabDirty(tab)||tab.busy&&!tab.loading);
}
function removeEditorTab(tab){
 if(tab.busy&&!tab.loading)return;const index=editorTabs.indexOf(tab);if(index<0)return;
 tab.controller?.abort();const active=tab===fileEdit;if(active){captureEditorTab();fileEdit=null}editorTabs.splice(index,1);tab.ui?.wrap.remove();tab.ui=null;tab.session=null;tab.text='';tab.saved='';
 if(!editorTabs.length){$('fileEditor').close();return}
 if(active)activateEditorTab(editorTabs[Math.min(index,editorTabs.length-1)]);else updateFileEditor();
}
function showEditorDecision(action,tabs,description){pendingEditorAction=action;pendingEditorTabs=tabs;$('fileEditorDiscardMessage').textContent=description||'有未保存的修改。';$('fileEditorSaveContinue').textContent='保存后继续';$('fileEditorDiscardConfirm').textContent='放弃修改并继续';if(!$('fileEditorDiscard').open)$('fileEditorDiscard').showModal()}
function closeEditorTab(tab){if(tab.busy&&!tab.loading)return;captureEditorTab();if(tabDirty(tab))showEditorDecision(()=>removeEditorTab(tab),[tab],'关闭 '+tab.item.Name+' 前，请先保存或确认放弃此文件的修改。');else removeEditorTab(tab)}
function closeFileEditor(){if(savingEditor())return;captureEditorTab();const dirty=editorTabs.filter(tabDirty);if(dirty.length)showEditorDecision(()=>$('fileEditor').close(),dirty,'有 '+dirty.length+' 个文件未保存，关闭编辑器将丢弃这些修改。');else $('fileEditor').close()}
function reloadEditorFile(){const tab=fileEdit;if(!tab||tab.busy)return;captureEditorTab();const load=()=>readEditorTab(tab);if(tabDirty(tab))showEditorDecision(load,[tab],'重新读取 '+tab.item.Name+' 会替换当前草稿。');else load()}
function findInFileEditor(){syncEditorText();const input=$('fileEditorContent'),term=$('fileEditorFind').value;if(!term||!fileEdit)return false;let index=input.value.indexOf(term,editorSelection().end);if(index<0)index=input.value.indexOf(term);if(index<0){fileEdit.error='没有找到匹配文字';updateFileEditor();return false}fileEdit.error='';$('fileEditorError').textContent='';selectEditorText(index,index+term.length);return true}
function openEditorSearch(){ $('fileEditorSearchPanel').hidden=false;$('fileEditorSearchToggle').setAttribute('aria-expanded','true');resizeFileEditor();$('fileEditorFind').focus() }
function closeEditorSearch(){ $('fileEditorSearchPanel').hidden=true;$('fileEditorSearchToggle').setAttribute('aria-expanded','false');resizeFileEditor() }
$('fileEditorContent').oninput=updateFileEditor;$('fileEditorContent').addEventListener('keyup',updateEditorCursor);$('fileEditorContent').addEventListener('click',updateEditorCursor);
$('fileEditorSave').onclick=saveFileEditor;$('fileEditorSaveAll').onclick=saveAllFileEditors;$('fileEditorClose').onclick=closeFileEditor;$('fileEditorReload').onclick=reloadEditorFile;
$('fileEditor').addEventListener('cancel',event=>{event.preventDefault();closeFileEditor()});
function disposeFileEditorWorkspace(){disposeEditorWindow();for(const tab of editorTabs){tab.controller?.abort();tab.ui=null;tab.session=null;tab.text='';tab.saved=''}editorTabs=[];fileEdit=null;fileCode?.destroy();fileCode=null;disposeEditorTree();pendingEditorAction=null;pendingEditorTabs=[];editorBatchMessage='';$('fileEditorContent').value='';$('fileEditorFind').value='';$('fileEditorReplacement').value='';$('fileEditorTabs').replaceChildren();$('fileEditorFormat').title='';$('fileEditor').classList.remove('editor-expanded');closeEditorSearch()}
$('fileEditor').addEventListener('close',()=>{if(editorSuspendedCloses){editorSuspendedCloses--;return}disposeFileEditorWorkspace()});
$('fileEditorKeep').onclick=()=>{pendingEditorAction=null;pendingEditorTabs=[];$('fileEditorDiscard').close()};
$('fileEditorDiscard').addEventListener('cancel',event=>{if(savingEditor())event.preventDefault()});
$('fileEditorSaveContinue').onclick=async()=>{if(!pendingEditorAction||savingEditor())return;const action=pendingEditorAction,targets=[...pendingEditorTabs];for(const tab of targets)if(!await saveEditorTab(tab)){$('fileEditorDiscardMessage').textContent='未能保存 '+tab.item.Name+'，草稿仍保留。'+tab.error;return}if(targets.some(tabDirty))return;pendingEditorAction=null;pendingEditorTabs=[];$('fileEditorDiscard').close();action()};
$('fileEditorDiscardConfirm').onclick=()=>{if(savingEditor())return;const action=pendingEditorAction;pendingEditorAction=null;pendingEditorTabs=[];$('fileEditorDiscard').close();if(action)action();else $('fileEditor').close()};
$('fileEditorFindNext').onclick=findInFileEditor;$('fileEditorFind').onkeydown=event=>{if(event.key==='Enter'){event.preventDefault();findInFileEditor()}};
$('fileEditorSearchToggle').onclick=()=>{$('fileEditorSearchPanel').hidden?openEditorSearch():closeEditorSearch()};$('fileEditorSearchClose').onclick=closeEditorSearch;
$('fileEditorReplace').onclick=()=>{if(!fileEdit||fileEdit.loading||!fileEdit.revision)return;syncEditorText();const input=$('fileEditorContent'),term=$('fileEditorFind').value;if(!term)return;const selection=editorSelection();if(input.value.slice(selection.start,selection.end)!==term&&!findInFileEditor())return;replaceEditorText($('fileEditorReplacement').value)};
$('fileEditorReplaceAll').onclick=()=>{if(!fileEdit||fileEdit.loading||!fileEdit.revision)return;const term=$('fileEditorFind').value;if(!term)return;setEditorText(syncEditorText().split(term).join($('fileEditorReplacement').value),true);updateFileEditor()};
$('fileEditorFont').onchange=()=>{$('fileEditor').style.setProperty('--editor-font',$('fileEditorFont').value+'px');fileCode?.resize();try{localStorage.setItem('hamster.files.editor.font',$('fileEditorFont').value)}catch{}};
$('fileEditorComplete').onclick=()=>fileCode?.complete();
$('fileEditorIndent').onchange=()=>{fileCode?.setIndent($('fileEditorIndent').value);try{localStorage.setItem('hamster.files.editor.indent',$('fileEditorIndent').value)}catch{}};
$('fileEditorLanguage').onchange=()=>{if(fileEdit)fileEdit.language=$('fileEditorLanguage').value;fileCode?.setLanguage($('fileEditorLanguage').value)};
$('fileEditorTheme').onchange=()=>{$('fileEditor').dataset.editorTheme=$('fileEditorTheme').value;fileCode?.setTheme($('fileEditorTheme').value);try{localStorage.setItem('hamster.files.editor.theme',$('fileEditorTheme').value)}catch{}};
$('fileEditorFullscreen').onclick=toggleEditorMaximize;
$('fileEditor').addEventListener('keydown',event=>{if((event.ctrlKey||event.metaKey)&&event.key.toLowerCase()==='s'){event.preventDefault();event.shiftKey?saveAllFileEditors():saveFileEditor()}if((event.ctrlKey||event.metaKey)&&event.key.toLowerCase()==='f'){event.preventDefault();openEditorSearch()}if(event.key==='Tab'&&event.target===$('fileEditorContent')&&!event.shiftKey&&fileEdit?.revision&&!fileEdit.loading){event.preventDefault();event.target.setRangeText(' '.repeat(Number($('fileEditorIndent').value)),event.target.selectionStart,event.target.selectionEnd,'end');updateFileEditor()}});
$('fileEditorTabs').addEventListener('keydown',event=>{if(!['ArrowLeft','ArrowRight','Home','End'].includes(event.key))return;const index=editorTabs.indexOf(fileEdit);let next=event.key==='Home'?0:event.key==='End'?editorTabs.length-1:(index+(event.key==='ArrowLeft'?-1:1)+editorTabs.length)%editorTabs.length;event.preventDefault();activateEditorTab(editorTabs[next]);$('fileEditorTabs').querySelector('[aria-selected=true]')?.focus()});
window.addEventListener('beforeunload',event=>{if(editorTabs.some(tabDirty)||savingEditor()){event.preventDefault();event.returnValue=''}});
function resizeFileEditor(){resizeEditorWindow()}
window.visualViewport?.addEventListener('resize',resizeFileEditor);window.visualViewport?.addEventListener('scroll',resizeFileEditor);window.addEventListener('resize',resizeFileEditor);

function editorParentPath(value){const index=Math.max(value.lastIndexOf('/'),value.lastIndexOf('\\'));if(index<0)return '';let parent=value.slice(0,index);if(local&&/^[a-z]:$/i.test(parent))parent+='\\';if(local&&!parent&&value.startsWith('/'))parent='/';return parent}
function disposeEditorTree(){if(editorTree)for(const controller of editorTree.controllers)controller.abort();editorTree=null;$('fileEditorTree').replaceChildren()}
function markEditorTreeSelection(){for(const row of $('fileEditorTree').querySelectorAll('[data-file-path]')){const active=Boolean(fileEdit&&sameBrowserPath(row.dataset.filePath,fileEdit.item.Path));row.classList.toggle('active',active);row.setAttribute('aria-current',active?'true':'false')}}
function requestEditorFile(item){return openTextEditor(item,true,editorTree?.root||root)}
function initializeEditorTree(item){
 const initial=editorParentPath(item.Path);loadEditorRoot(initial);
 $('fileEditor').classList.toggle('tree-visible',innerWidth>700);
 $('fileEditorTreeToggle').setAttribute('aria-expanded',String(innerWidth>700));
}
function loadEditorRoot(folder){
 if(!fileEdit)return;const shareRoot=fileEdit.root;disposeEditorTree();
 editorTree={root:shareRoot,folder,controllers:new Set()};
 $('fileEditorDirectory').textContent=folder.split(/[\\/]/).filter(Boolean).at(-1)||'共享目录';$('fileEditorDirectory').title=folder||'共享目录';$('fileEditorTreeUp').disabled=true;
 loadEditorDirectory(editorTree,$('fileEditorTree'),folder,0,true);
}
async function loadEditorDirectory(tree,container,folder,offset=0,isRoot=false){
 if(editorTree!==tree)return;
 const controller=new AbortController();tree.controllers.add(controller);
 const pending=document.createElement('p');pending.className='file-tree-message';pending.textContent='正在读取…';container.append(pending);
 try{
  const data=await api('list',{Path:folder,Offset:offset,PageSize:200,Sort:'name',Order:'asc'},tree.root,controller.signal);
  if(editorTree!==tree||!$('fileEditor').open)return;
  if(isRoot){tree.parent=data.Parent;$('fileEditorTreeUp').disabled=local?!data.Parent:!folder}
  for(const item of data.Entries||[]){
   if(item.Kind==='drive')continue;
   if(item.Dir){
    const details=document.createElement('details'),summary=document.createElement('summary'),children=document.createElement('div');details.className='file-tree-folder';children.className='file-tree-children';summary.append(controlIcon('folder'),document.createTextNode(item.Name));summary.title=item.Name;details.append(summary,children);let loaded=false;
    details.addEventListener('toggle',()=>{if(details.open&&!loaded){loaded=true;loadEditorDirectory(tree,children,item.Path)}});container.append(details);
   }else{
    const editable=['text','file'].includes(item.Kind),row=button(item.Name,()=>requestEditorFile(item));row.className='file-tree-file';row.dataset.filePath=item.Path;row.title=editable?item.Name:'此类型不能作为文本编辑';row.disabled=!editable;row.prepend(controlIcon(explorerIconKind(item)));container.append(row);
   }
  }
  const next=offset+200;
  if(next<data.Total&&next>offset){const more=button('加载更多（'+next+' / '+data.Total+'）',()=>{more.remove();loadEditorDirectory(tree,container,folder,next,isRoot)});more.className='file-tree-more';container.append(more)}
  if(!offset&&!data.Entries?.length){pending.textContent='此目录为空';return}
  markEditorTreeSelection();
 }catch(error){if(editorTree===tree&&error.name!=='AbortError'){pending.textContent='读取失败：'+error.message;const retry=button('重试',()=>{pending.remove();retry.remove();loadEditorDirectory(tree,container,folder,offset,isRoot)});retry.className='file-tree-more';container.append(retry);return}}
 finally{tree.controllers.delete(controller)}
 pending.remove();
}
$('fileEditorTreeToggle').onclick=()=>{const visible=!$('fileEditor').classList.contains('tree-visible');$('fileEditor').classList.toggle('tree-visible',visible);$('fileEditorTreeToggle').setAttribute('aria-expanded',String(visible));fileCode?.resize()};
$('fileEditorTreeRefresh').onclick=()=>{if(editorTree)loadEditorRoot(editorTree.folder)};
$('fileEditorTreeUp').onclick=()=>{if(editorTree&&!$('fileEditorTreeUp').disabled)loadEditorRoot(editorTree.parent)};
