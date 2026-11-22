'use strict';

// Navigation controls and menus share the same actions and permission checks as the toolbar.
let contextReturnFocus=null,shellInitialized=false;
let openWithItem=null,nativeFileOpening=false;
let shortcutRevision=0,shortcutAbort=null;
let propertyRevision=0,propertyAbort=null,propertySizeAbort=null;
async function showResourceProperties(item={Path:path,Name:path.split(/[\\/]/).filter(Boolean).pop()||'根目录'}){
  propertyAbort?.abort();propertySizeAbort?.abort();propertySizeAbort=null;const controller=new AbortController(),revision=++propertyRevision,propertyRoot=root;propertyAbort=controller;
  const dialog=$('propertiesDialog'),list=$('propertiesList');list.replaceChildren();$('propertiesError').textContent='';$('propertiesStatus').textContent='正在读取属性…';if(!dialog.open)dialog.showModal();
  try{const data=await api('properties',{Path:item.Path,Version:item.Version||''},root,controller.signal);if(revision!==propertyRevision||!dialog.open)return;
    const type=data.Dir?'文件夹':({image:'图片',video:'视频',audio:'音频',text:'文本',pdf:'PDF 文档',shortcut:'Windows 快捷方式'}[data.Kind]||'文件');
    const fields=[['名称',data.Name],['类型',type],['位置',local?data.Path:(boot.Roots.find(x=>x.ID===root)?.Name||'共享目录')+(data.Path?'/'+data.Path:'')],['大小',data.Dir?'未统计（不会自动遍历目录）':bytes(data.Size)+'（'+Number(data.Size).toLocaleString()+' 字节）'],['修改时间',formatExplorerDate(data.Modified)]];
    if(data.Created)fields.push(['创建时间',formatExplorerDate(data.Created)]);if(data.Accessed)fields.push(['访问时间',formatExplorerDate(data.Accessed)]);
    if(data.Width>0&&data.Height>0)fields.push(['分辨率',data.Width+' × '+data.Height+' 像素']);
    if(data.Kind==='video'||data.Kind==='audio')fields.push(['时长',data.Duration>0?mediaTime(data.Duration):'未知']);
    if(data.ImageFormat)fields.push(['图片编码',data.ImageFormat.toUpperCase()]);
    if(data.VideoCodec)fields.push(['视频编码',data.VideoCodec]);if(data.AudioCodec)fields.push(['音频编码',data.AudioCodec]);if(data.Container)fields.push(['封装格式',data.Container]);
    if(typeof data.ReadOnly==='boolean')fields.push(['只读',data.ReadOnly?'是':'否']);if(typeof data.Hidden==='boolean')fields.push(['隐藏',data.Hidden?'是':'否']);if(boot.Platform!=='windows'&&data.Permissions)fields.push(['权限',data.Permissions]);
    for(const [label,value] of fields){const term=document.createElement('dt'),description=document.createElement('dd');term.textContent=label;description.textContent=value||'—';
      if(label==='大小'&&data.Dir){
        const size=document.createElement('span');size.textContent='未统计';description.replaceChildren(size);description.className='property-size';
        const calculate=button('计算',async()=>{
          if(propertySizeAbort){propertySizeAbort.abort();return}
          const sizeController=new AbortController();propertySizeAbort=sizeController;calculate.textContent='停止';calculate.title='停止计算目录大小';$('propertiesError').textContent='';$('propertiesStatus').textContent='正在计算目录大小… 大目录可能需要一些时间，关闭属性可取消。';
          try{const result=await api('properties-size',{Path:item.Path,Version:item.Version||''},propertyRoot,sizeController.signal);if(revision!==propertyRevision||!dialog.open)return;
            size.textContent=(result.Complete?'':'至少 ')+bytes(result.Size)+'（'+Number(result.Size).toLocaleString()+' 字节）';
            $('propertiesStatus').textContent=result.Files+' 个文件，'+result.Directories+' 个子目录'+(result.Skipped?'，跳过 '+result.Skipped+' 项':'')+'。'+(result.Note||'按文件大小统计，非磁盘占用；统计期间文件变化可能影响结果。');
          }catch(e){if(revision===propertyRevision&&dialog.open){$('propertiesStatus').textContent=e.name==='AbortError'?'已停止计算。':'';if(e.name!=='AbortError')$('propertiesError').textContent=e.message}}
          finally{if(propertySizeAbort===sizeController)propertySizeAbort=null;if(revision===propertyRevision&&dialog.open){calculate.textContent='重新计算';calculate.title='重新计算目录大小'}}
        });calculate.className='property-size-button';calculate.title='递归计算文件夹内文件大小，不读取文件内容';description.append(calculate)
      }
      list.append(term,description)}$('propertiesStatus').textContent=data.MediaError||'';
  }catch(e){if(revision===propertyRevision&&e.name!=='AbortError'){$('propertiesStatus').textContent='';$('propertiesError').textContent=e.message}}
}
async function openDirectoryShortcut(item,newTab=false){
  if(!item||item.Kind!=='shortcut'||loading||mutating)return;
  if(!item.Version){say('选择已过期，请刷新目录后重新选择',true);return}
  const revision=++shortcutRevision,tabID=activeFileTab,itemRoot=root,itemPath=path,navigationRevision=requestID;
  shortcutAbort?.abort();const controller=new AbortController();shortcutAbort=controller;
  const current=()=>revision===shortcutRevision&&tabID===activeFileTab&&itemRoot===root&&itemPath===path&&navigationRevision===requestID&&!mutating;
  stopVideoHover();say('正在读取快捷方式目标…');
  try{const result=await api('shortcut',{Path:item.Path,Version:item.Version},itemRoot,controller.signal);if(!current())return;if(newTab)await openFileTab({root:itemRoot,path:result.Path});else await navigate(result.Path,{targetRoot:itemRoot})}
  catch(e){if(current()&&e.name!=='AbortError')say(e.message,true)}
  finally{if(shortcutAbort===controller)shortcutAbort=null}
}
async function requestOpenWith(item,program=''){
  if(!canUseNativePrograms()||!item||item.Dir||loading||mutating||nativeFileOpening)return false;
  nativeFileOpening=true;$('openWithSave').disabled=$('openWithNative').disabled=true;$('openWithMessage').textContent='';$('openWithError').textContent='';
  say(program?'正在将文件交给本机程序…':'请在这台电脑的系统窗口中选择程序；可选择看图、播放或编辑软件。');
  try{const result=await api('open-with',{Path:item.Path,Version:item.Version,Program:program});const message=result.Cancelled?'已取消选择程序':program?'已交给本机程序打开：'+item.Name+'；编辑保存后可刷新目录。':'已请求系统选择应用；如果没有出现窗口，可在下方指定程序文件。';say(message);$('openWithMessage').textContent=message;return true}
  catch(e){if($('openWithDialog').open)$('openWithError').textContent=e.message;else say(e.message,true);return false}
  finally{nativeFileOpening=false;$('openWithSave').disabled=$('openWithNative').disabled=false}
}
function chooseFileApplication(item){
  if(!canUseNativePrograms()||!item||item.Dir||loading||mutating||nativeFileOpening)return;
  const native=['windows','darwin'].includes(boot.Platform);openWithItem=item;$('openWithName').textContent=item.Name;$('openWithError').textContent='';$('openWithMessage').textContent='';$('openWithNative').hidden=!native;$('openWithCustom').open=!native;
  $('openWithProgram').placeholder=boot.Platform==='windows'?'例如 C:\\Windows\\System32\\notepad.exe':boot.Platform==='darwin'?'例如 /Applications/Preview.app':'例如 /usr/bin/vlc 或 /usr/bin/gedit';$('openWithDialog').showModal();if(native)$('openWithNative').focus();else $('openWithProgram').focus();
}
const explorerDateFormatter=new Intl.DateTimeFormat('zh-CN',{year:'numeric',month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit',hour12:false});
function formatExplorerDate(value){const date=new Date(value);return Number.isFinite(date.getTime())&&date.getUTCFullYear()>1?explorerDateFormatter.format(date):''}
const explorerDocumentOutline='M6 3h8l4 4v14H6Z';
const explorerIconPaths={
  back:['m14.5 5-7 7 7 7'],forward:['m9.5 5 7 7-7 7'],up:['M12 20V4m-7 7 7-7 7 7'],refresh:['M20 7v5h-5','M20 12a8 8 0 1 0-2.3 5.7M20 7l-2.3-2.4'],
  plus:['M12 5v14M5 12h14'],sidebar:['M4 4h16v16H4Z','M9 4v16m7-11-3 3 3 3'],menu:['M4 6h16M4 12h16M4 18h16'],
  folderOpen:['M3 8V6a2 2 0 0 1 2-2h5l2 3h7a2 2 0 0 1 2 2v2','M3 11h18l-3 9H5l-2-9Z'],
  folderShortcut:['M3 7a2 2 0 0 1 2-2h5l2 2h7a2 2 0 0 1 2 2v10a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2Z','M5 18v-2a3 3 0 0 1 3-3h5m-3-3 3 3-3 3'],
  upload:['M12 16V3m-5 5 5-5 5 5','M4 15v5h16v-5'],download:['M12 3v13m-5-5 5 5 5-5','M4 16v4h16v-4'],
  info:['M12 22a10 10 0 1 0 0-20 10 10 0 0 0 0 20Z','M12 11v6M12 7h.01'],edit:['m4 16-1 5 5-1L20 8l-4-4L4 16Z','m13 7 4 4'],
  trash:['M4 7h16M9 3h6l1 4M6 7l1 14h10l1-14','M10 11v6M14 11v6'],copy:['M9 9h12v12H9Z','M15 9V3H3v12h6'],external:['M14 3h7v7M21 3 10 14','M10 5H4v15h15v-6'],
  cut:['M9 8 20 19M9 16 20 5','M8 6a3 3 0 1 1-6 0 3 3 0 0 1 6 0ZM8 18a3 3 0 1 1-6 0 3 3 0 0 1 6 0Z'],
  paste:['M9 5H5v16h14V5h-4','M9 3h6v4H9ZM9 12h6M9 16h6'],
  pin:['m8 3 8 0-1 6 4 4H5l4-4-1-6Z','M12 13v8'],unpin:['M3 3l18 18M10 3h6l-1 6 4 4h-5M5 13h4M12 16v5'],
  toolbox:['M8 7V3h8v4M3 7h18v14H3Z','M3 12h18M10 11v4h4v-4'],
  floating:['M3 4h18v16H3Z','M11 11h8v7h-8Z'],
  move:['M3 12h18m-4-4 4 4-4 4','M7 8l-4 4 4 4'],select:['M4 4h16v16H4Z','m8 12 3 3 5-6'],
  close:['M6 6l12 12M18 6 6 18'],closeOthers:['M8 5H3v14h5M11 7l10 10M21 7 11 17'],
  toTop:['M5 3h14M12 21V7m-5 5 5-5 5 5'],toBottom:['M5 21h14M12 3v14m-5-5 5 5 5-5'],down:['M12 3v18m-7-7 7 7 7-7'],
  folder:['M3 7a2 2 0 0 1 2-2h5l2 2h7a2 2 0 0 1 2 2v10a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2Z','M3 10h18'],
  shared:['M3 7a2 2 0 0 1 2-2h5l2 2h7a2 2 0 0 1 2 2v10a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2Z','M8 17v-2h8v2M12 12v3'],
  computer:['M3 4h18v13H3Z','M8 21h8M12 17v4M3 14h18'],
  drive:['M5 4h14l2 15H3L5 4Z','M3 15h18M16 18h2M6 18h.01'],
  image:['M4 4h16v16H4Z','m4 16 5-5 4 4 3-3 4 4','M8 8h.01'],
  listView:['M4 5h3v3H4ZM4 11h3v3H4ZM4 17h3v3H4Z','M11 6h9M11 12h9M11 18h9'],
  gridView:['M4 4h6v6H4ZM14 4h6v6h-6ZM4 14h6v6H4ZM14 14h6v6h-6Z'],
  video:['M4 5h16v14H4Z','m10 9 5 3-5 3Z'],
  audio:['M9 17V5l11-2v12','M9 17c0 4-6 4-6 0s6-4 6 0ZM20 15c0 4-6 4-6 0s6-4 6 0Z'],
  file:[explorerDocumentOutline,'M14 3v5h4'],
  archive:[explorerDocumentOutline,'M14 3v5h4M10 3v3h2v3h-2v3h2v3h-2v3h3v-3'],
  text:[explorerDocumentOutline,'M14 3v5h4M9 11h6M9 14h6M9 17h4'],
  document:[explorerDocumentOutline,'M14 3v5h4'],
  pdf:[explorerDocumentOutline,'M14 3v5h4'],
  spreadsheet:[explorerDocumentOutline,'M14 3v5h4M9 11h6v7H9ZM9 14h6M12 11v7'],
  presentation:[explorerDocumentOutline,'M14 3v5h4M9 11h6v6H9ZM12 17v2'],
  code:[explorerDocumentOutline,'M14 3v5h4m-8 4-2 2 2 2m4-4 2 2-2 2'],
  application:['M3 4h18v16H3Z','M3 9h18M6 6.5h.01M9 6.5h.01m-1 6 2 2-2 2m5 0h3'],
  shortcut:[explorerDocumentOutline,'M14 3v5h4M4 19v-3a3 3 0 0 1 3-3h5m-3-3 3 3-3 3'],
  font:[explorerDocumentOutline,'M14 3v5h4m-9 10 3-7 3 7m-5-2h4']
};
const explorerExtensionIcons=new Map();
for(const [kind,extensions] of Object.entries({
  archive:'7z zip rar tar gz bz2 xz zst tgz tbz2 txz cab iso',
  document:'doc docx odt rtf pages epub mobi azw azw3',
  spreadsheet:'xls xlsx xlsm xlsb ods csv tsv numbers',
  presentation:'ppt pptx pps ppsx odp key',
  code:'go py js jsx ts tsx java c h cpp hpp cs rs php rb sh bash bat cmd ps1 json xml html htm css scss sql yaml yml toml ini conf cfg',
  application:'exe msi msix app appimage apk dmg pkg deb rpm',
  shortcut:'lnk url webloc desktop',font:'ttf otf woff woff2',text:'txt log md markdown srt ass ssa vtt nfo',pdf:'pdf'
}))for(const extension of extensions.split(' '))explorerExtensionIcons.set(extension,kind);
function explorerIconKind(item){
  if(item.Kind==='drive')return 'drive';
  if(item.Dir)return 'folder';
  if(item.Kind==='shortcut')return 'folderShortcut';
  // File kinds still control preview/playback; this classification only changes artwork.
  if(['image','video','audio'].includes(item.Kind))return item.Kind;
  const name=String(item.Name||'').toLowerCase();
  if(/\.(?:7z|zip)\.\d{3,}$|\.(?:r|z)\d{2,}$/.test(name))return 'archive';
  const extension=/\.([^.]+)$/.exec(name)?.[1]||'';
  return explorerExtensionIcons.get(extension)||(Object.hasOwn(explorerIconPaths,item.Kind)?item.Kind:'file');
}
function explorerIcon(kind){
  if(!Object.hasOwn(explorerIconPaths,kind))kind='file';
  const svg=document.createElementNS('http://www.w3.org/2000/svg','svg');svg.setAttribute('viewBox','0 0 24 24');svg.setAttribute('fill','none');svg.setAttribute('stroke','currentColor');svg.setAttribute('stroke-width','1.5');svg.setAttribute('stroke-linecap','round');svg.setAttribute('stroke-linejoin','round');svg.setAttribute('aria-hidden','true');svg.setAttribute('focusable','false');svg.classList.add('file-symbol','symbol-'+kind);svg.dataset.icon=kind;
  for(const [index,d] of explorerIconPaths[kind].entries()){const path=document.createElementNS(svg.namespaceURI,'path');path.setAttribute('d',d);if(index===0&&kind!=='audio')path.classList.add('symbol-body');svg.append(path)}
  const badge={pdf:'PDF',document:'W'}[kind];
  if(badge){const text=document.createElementNS(svg.namespaceURI,'text');text.setAttribute('x','12');text.setAttribute('y','17');text.setAttribute('text-anchor','middle');text.setAttribute('stroke','none');text.setAttribute('fill','currentColor');text.setAttribute('font-size',kind==='pdf'?'4.6':'7');text.setAttribute('font-family','Segoe UI, sans-serif');text.setAttribute('font-weight','700');text.textContent=badge;svg.append(text)}
  return svg;
}
function controlIcon(kind){const svg=explorerIcon(kind);svg.classList.add('control-icon');svg.setAttribute('stroke-width','1.8');return svg}
function contextMenuIcon(label){
  if(label.includes('取消固定'))return 'unpin';if(label.includes('快速访问'))return 'pin';
  if(label.includes('解压'))return 'archive';if(label.includes('属性'))return 'info';if(label.includes('工具箱'))return 'toolbox';
  if(label.includes('剪切'))return 'cut';if(label.includes('粘贴'))return 'paste';
  if(label.includes('复制'))return 'copy';if(label.includes('下载'))return 'download';if(label.includes('删除'))return 'trash';
  if(label.includes('重命名'))return 'edit';if(label.includes('关闭其他')||label.includes('关闭右侧'))return 'closeOthers';if(label.includes('关闭'))return 'close';
  if(label.includes('顶部'))return 'toTop';if(label.includes('底部'))return 'toBottom';if(label.includes('上移'))return 'up';if(label.includes('下移'))return 'down';
  if(label.includes('向左'))return 'back';if(label.includes('向右'))return 'forward';if(label.includes('刷新'))return 'refresh';
  if(label.includes('上传'))return 'upload';if(label.includes('移动'))return 'move';if(label.includes('选择'))return 'select';
  if(label.includes('新标签')||label.includes('新建标签'))return 'plus';if(label.includes('mpv')||label.includes('原生'))return label.includes('查看')?'image':'video';if(label.includes('悬浮'))return 'floating';
  if(label.includes('打开方式')||label.includes('本地文件管理器'))return 'external';if(label.includes('文件夹')||label.includes('目录'))return 'folderOpen';
  if(label.includes('预览'))return 'image';return 'application';
}
function contextMenuButton(label,run,enabled=true,kind=contextMenuIcon(label)){const b=button(label,run);b.prepend(controlIcon(kind));b.classList.add('menu-icon-button');b.setAttribute('role','menuitem');b.disabled=!enabled;return b}
function initializeExplorerIcons(){
  for(const [id,kind] of [['back','back'],['forward','forward'],['up','up'],['refresh','refresh'],['newTab','plus'],['sidebarToggle','sidebar'],['toggleNav','menu']])$(id).replaceChildren(controlIcon(kind));
  for(const [id,kind] of [['openSelected','folderOpen'],['openLocal','external'],['newFolder','folder'],['upload','upload'],['paste','paste'],['copy','copy'],['cut','cut'],['uploadFolder','upload'],['copyTo','copy'],['move','move'],['rename','edit'],['remove','trash'],['download','download'],['openFolderTab','plus'],['selectAll','select'],['clearSelection','close'],['browserHelp','info']]){const element=$(id),text=element.textContent.replace(/^[＋↑]\s*/,'');element.replaceChildren(controlIcon(kind),document.createTextNode(text));element.classList.add('icon-label-button')}
  $('backToolbox').replaceChildren(controlIcon('back'),document.createTextNode('返回工具箱'));
}
function updateListSortHeaders(){for(const b of document.querySelectorAll('[data-sort-key]')){const active=b.dataset.sortKey===$('sort').value;b.classList.toggle('active',active);b.setAttribute('aria-label','按'+({name:'名称',size:'大小',modified:'修改时间'}[b.dataset.sortKey])+'排序'+(active?'，当前'+($('order').value==='asc'?'正序':'倒序'):''));b.querySelector('.sort-mark').textContent=active?($('order').value==='asc'?'↑':'↓'):''}}
function cacheExplorerView(){try{localStorage.setItem('hamster.files.view',JSON.stringify({view,tileSize,tileGap,pageSize,sidebarCollapsed,sort:defaultSort,order:defaultOrder,hoverPreview:hoverPreviewEnabled}))}catch{}}
function applySidebar(){
  const mobile=window.innerWidth<=760;
  document.body.classList.toggle('sidebar-collapsed',sidebarCollapsed);
  const expanded=mobile?document.body.classList.contains('nav-open'):!sidebarCollapsed;
  $('sidebar').inert=!expanded;
  const label=expanded?'收起左侧导航':'展开左侧导航';
  $('sidebarToggle').title=label;$('sidebarToggle').setAttribute('aria-label',label);$('sidebarToggle').setAttribute('aria-expanded',String(expanded));
}
function closeContextMenu(restoreFocus=false){
  const menu=$('contextMenu');if(!menu||menu.hidden)return;
  menu.hidden=true;menu.replaceChildren();
  if(restoreFocus&&contextReturnFocus?.isConnected)contextReturnFocus.focus();contextReturnFocus=null;
}
async function copyResourcePath(value){
  try{if(!navigator.clipboard?.writeText)throw Error();await navigator.clipboard.writeText(value);say('已复制路径')}
  catch{$('copyValue').value=value;$('copyDialog').querySelector('h2').textContent='复制路径';$('copyDialog').querySelector('p').textContent='可选中并复制下面的路径。';$('copyDialog').showModal();$('copyValue').select()}
}
function presentContextMenu(event,target){
  const menu=$('contextMenu');menu.hidden=false;
  const anchor=(event.target.closest?.('.entry-more,.quick-access-more')||target)?.getBoundingClientRect(),keyboard=event.type==='keydown'||event.type==='click',x=keyboard?(anchor?.left||12):event.clientX,y=keyboard?(anchor?.bottom||12):event.clientY;
  const box=menu.getBoundingClientRect();menu.style.left=Math.max(8,Math.min(x,innerWidth-box.width-8))+'px';menu.style.top=Math.max(8,Math.min(y,innerHeight-box.height-8))+'px';menu.querySelector('button:not(:disabled)')?.focus();
}
function showQuickAccessContext(event,folder,target){
  event.preventDefault();event.stopPropagation();closeContextMenu();if(!local||document.querySelector('dialog[open]'))return;
  contextReturnFocus=target;const menu=$('contextMenu');
  const index=favorites.indexOf(folder),movable=!mutating&&!quickAccessSaving&&index>=0;
  for(const [label,run,enabled] of [['打开目录',()=>navigate(folder),!loading&&!mutating],['在新标签页打开',()=>openFileTab({root:'',path:folder}),!mutating],['移到顶部',()=>moveQuickAccess(folder,favorites[0]),movable&&index>0],['上移一位',()=>moveQuickAccess(folder,favorites[index-1]),movable&&index>0],['下移一位',()=>moveQuickAccess(folder,favorites[index+1],true),movable&&index<favorites.length-1],['移到底部',()=>moveQuickAccess(folder,favorites.at(-1),true),movable&&index<favorites.length-1],['从快速访问取消固定',()=>setQuickAccess(folder,false),!quickAccessSaving]])menu.append(contextMenuButton(label,()=>{closeContextMenu();run()},enabled));
  presentContextMenu(event,target);
}

let navigationSortClickUntil=0,navigationPointerCleanup=null;
function navigationSortClickBlocked(){return Date.now()<navigationSortClickUntil}
function reorderNavigation(values,source,target,after=false,key=value=>value){
  if(source===target)return null;
  const from=values.findIndex(value=>key(value)===source);if(from<0||!values.some(value=>key(value)===target))return null;
  const next=[...values],item=next.splice(from,1)[0],to=next.findIndex(value=>key(value)===target)+(after?1:0);next.splice(to,0,item);
  return next.every((value,index)=>value===values[index])?null:next;
}
function clearNavigationSortMarks(){for(const row of document.querySelectorAll('.sort-before,.sort-after'))row.classList.remove('sort-before','sort-after')}
function bindNavigationSort(row,kind,key,axis,move,busy){
  // Pointer dragging also works on button-based tabs where native HTML drag
  // may be swallowed by the browser. Touch keeps normal scrolling + menus.
  row.draggable=false;row.dataset.sortAxis=axis;row.dataset.navigationSortKind=kind;row.navigationSortKey=key;
  row.addEventListener('click',event=>{if(navigationSortClickBlocked()){event.preventDefault();event.stopImmediatePropagation()}},{capture:true});
  row.addEventListener('pointerdown',event=>{
    if(event.button!==0||event.pointerType==='touch'||busy()||event.target.closest?.('.tab-close,.tab-more,.quick-access-more'))return;
    navigationPointerCleanup?.();const pointer=event.pointerId,startX=event.clientX,startY=event.clientY;let dragging=false,target=null,after=false;
    const finish=commit=>{
      document.removeEventListener('pointermove',onMove,true);document.removeEventListener('pointerup',onUp,true);document.removeEventListener('pointercancel',onCancel,true);document.removeEventListener('keydown',onKey,true);navigationPointerCleanup=null;
      if(row.hasPointerCapture?.(pointer))row.releasePointerCapture(pointer);
      row.classList.remove('navigation-dragging');clearNavigationSortMarks();
      if(dragging){navigationSortClickUntil=Date.now()+250;if(commit&&row.isConnected&&target?.isConnected&&!busy())move(key,target.navigationSortKey,after)}
    };
    const onMove=e=>{
      if(e.pointerId!==pointer)return;if(busy()){finish(false);return}
      if(!dragging&&Math.hypot(e.clientX-startX,e.clientY-startY)<7)return;
      if(!dragging){dragging=true;closeContextMenu();row.classList.add('navigation-dragging');row.setPointerCapture?.(pointer)}
      e.preventDefault();clearNavigationSortMarks();target=document.elementFromPoint(e.clientX,e.clientY)?.closest('[data-navigation-sort-kind]');
      if(target?.dataset.navigationSortKind!==kind||target===row){target=null;return}
      const box=target.getBoundingClientRect();after=axis==='x'?e.clientX>box.left+box.width/2:e.clientY>box.top+box.height/2;target.classList.add(after?'sort-after':'sort-before');
      const scroller=axis==='x'?$('fileTabs'):$('sidebar'),area=scroller.getBoundingClientRect(),position=axis==='x'?e.clientX:e.clientY,start=axis==='x'?area.left:area.top,end=axis==='x'?area.right:area.bottom;
      const step=position<start+28?-18:position>end-28?18:0;if(axis==='x')scroller.scrollLeft+=step;else scroller.scrollTop+=step;
    };
    const onUp=e=>{if(e.pointerId===pointer)finish(true)},onCancel=e=>{if(e.pointerId===pointer)finish(false)},onKey=e=>{if(e.key==='Escape'){e.preventDefault();finish(false)}};
    navigationPointerCleanup=()=>finish(false);document.addEventListener('pointermove',onMove,{capture:true,passive:false});document.addEventListener('pointerup',onUp,true);document.addEventListener('pointercancel',onCancel,true);document.addEventListener('keydown',onKey,true);
  });

}
async function moveQuickAccess(source,target,after=false){
  if(!local||mutating||quickAccessSaving)return;
  const next=reorderNavigation(favorites,source,target,after);if(!next)return;
  const previous=[...favorites];quickAccessSaving=true;favorites=next;side();
  const saved=await savePreferences({favorites:[...favorites]});quickAccessSaving=false;
  if(!saved){favorites=previous;side();return}
  side();try{localStorage.setItem('hamster.files.favorites',JSON.stringify(favorites))}catch{}say('快速访问顺序已保存');
}
function showFileTabContext(event,id,target){
  event.preventDefault();event.stopPropagation();closeContextMenu();if(document.querySelector('dialog[open]'))return;
  const tab=fileTabs.find(item=>item.id===id),index=fileTabs.indexOf(tab);if(!tab)return;
  contextReturnFocus=target;const menu=$('contextMenu'),ready=!mutating;
  const refresh=async()=>{if(id!==activeFileTab)await switchFileTab(id);else await navigate(path,{remember:false,page:offset})};
  for(const [label,run,enabled]of [['新建标签页',()=>openFileTab({root:tab.root,path:tab.path}),ready&&fileTabs.length<maxFileTabs],['复制标签页',()=>duplicateFileTab(id),ready&&fileTabs.length<maxFileTabs],['刷新标签页',refresh,ready&&!loading],['向左移动',()=>moveFileTab(id,fileTabs[index-1]?.id),ready&&index>0],['向右移动',()=>moveFileTab(id,fileTabs[index+1]?.id,true),ready&&index<fileTabs.length-1],['关闭标签页',()=>closeFileTab(id),ready&&fileTabs.length>1],['关闭其他标签页',()=>closeRelatedFileTabs(id),ready&&fileTabs.length>1],['关闭右侧标签页',()=>closeRelatedFileTabs(id,true),ready&&index<fileTabs.length-1]]){
    menu.append(contextMenuButton(label,()=>{closeContextMenu();run()},enabled));
  }
  presentContextMenu(event,target);
}
function showContextMenu(event,item=null){
  stopVideoHover();
  event.preventDefault();closeContextMenu();
  if(loading||mutating||document.querySelector('dialog[open]'))return;
  const target=event.target.closest?.('.entry-open')||event.target.closest?.('.entry')?.querySelector('.entry-open');
  contextReturnFocus=event.target.closest?.('button')||target||document.activeElement;
  if(item&&!selection.has(item.Path)){selection.clear();if(item.Kind!=='drive'){selection.add(item.Path);selectionAnchor=item.Path}actions()}
  const menu=$('contextMenu'),add=(label,run,enabled=true,kind=contextMenuIcon(label))=>{
    menu.append(contextMenuButton(label,()=>{closeContextMenu();run()},enabled,kind));
  };
  if(item){
    const items=selected(),one=items.length<=1;
    add(item.Dir?'打开目录':item.Kind==='shortcut'?'打开目标文件夹':'预览文件',()=>activateResource(item),one,explorerIconKind(item));
    if(local&&!item.Dir&&item.Kind==='archive'&&typeof openArchiveExtraction==='function')add('解压资源…',()=>openArchiveExtraction(item),one&&Boolean(item.Version));
    if(local&&item.Dir&&typeof showToolboxLauncher==='function')add('使用工具箱处理…',()=>showToolboxLauncher(item.Path),one);
    if(writable()&&item.Kind!=='drive'&&typeof copySelectedResources==='function'){add('复制',()=>copySelectedResources('copy'));add('剪切',()=>copySelectedResources('move'));add('复制到…',()=>openTransferPicker('copy'));}
    if(local&&item.Dir)add(inQuickAccess(item.Path)?'已在快速访问中':'添加到快速访问',()=>setQuickAccess(item.Path),one&&!quickAccessSaving&&!inQuickAccess(item.Path));
    if(item.Kind==='shortcut')add('在新标签页打开目标文件夹',()=>openDirectoryShortcut(item,true),one&&Boolean(item.Version));
    if(!item.Dir&&['image','video'].includes(item.Kind)&&canUseNativePrograms())add(item.Kind==='image'?'原生悬浮查看（mpv）':'原生悬浮播放（mpv）',()=>openNativeMedia(item),one&&Boolean(item.Version));
    if(!item.Dir&&['image','video'].includes(item.Kind))add(item.Kind==='image'?'悬浮查看':local?'浏览器悬浮播放':'悬浮播放',()=>openFloatingMedia(item,root),one);
    if(item.Dir){add('在新标签页打开',()=>openFileTab({root,path:item.Path}),one);if(canUseNativePrograms())add('在本地文件管理器打开',async()=>{try{await api('open-local',{Path:item.Path});say('已在本机文件管理器打开')}catch(e){say(e.message,true)}},one)}
    else {if(canUseNativePrograms())add('打开方式…',()=>chooseFileApplication(item),one&&!nativeFileOpening&&Boolean(item.Version));add('下载原文件',()=>download(item),one)}
    if(item.Dir&&writable())add('粘贴到此目录',()=>pasteSelectedResources(item.Path),one&&canPasteResources(item.Path));
    if(item.Kind!=='drive'&&writable())for(const [id,label]of [['rename','重命名'],['move','移动到…'],['remove','删除…']])add(label,()=>$(id).click(),!$(id).disabled);
    add(local?'复制完整路径':'复制共享内路径',()=>copyResourcePath(item.Path),one);
    add('属性',()=>showResourceProperties(item),one);
  }else{
    if(writable()){add('新建文件夹',()=>$('newFolder').click(),!$('newFolder').disabled);add('上传文件',()=>$('upload').click(),!$('upload').disabled)}
    if(local&&path&&typeof showToolboxLauncher==='function')add('使用工具箱处理当前目录…',()=>showToolboxLauncher(path));
    if(writable()&&typeof pasteSelectedResources==='function')add('粘贴',()=>pasteSelectedResources(),canPasteResources());
    add('刷新当前目录',()=>$('refresh').click());add('选择本页项目',selectPage,rows.some(row=>row.Kind!=='drive'));
    if(path||!local&&root)add('当前目录属性',()=>showResourceProperties());
  }
  presentContextMenu(event,target);
}
function initializeToolbarMenus(){
  const menus=[$('moreActions'),$('displayOptions')];
  const close=()=>{for(const details of menus)details.open=false};
  const place=details=>{
    if(!details.open)return;
    const panel=details.querySelector('.toolbar-menu'),anchor=details.querySelector('summary').getBoundingClientRect(),width=Math.min(300,innerWidth-28);
    const left=Math.max(14,Math.min(details.id==='displayOptions'?anchor.right-width:anchor.left,innerWidth-width-14));
    const below=innerHeight-anchor.bottom-14,above=anchor.top-14,up=below<180&&above>below;
    const height=Math.min(420,Math.max(80,up?above:below));
    panel.style.setProperty('--toolbar-menu-left',left+'px');panel.style.setProperty('--toolbar-menu-top',(up?Math.max(8,anchor.top-Math.min(panel.scrollHeight,height)-6):anchor.bottom+6)+'px');panel.style.setProperty('--toolbar-menu-height',height+'px');
  };
  for(const details of menus){
    details.querySelector('summary').addEventListener('click',()=>{if(!details.open)for(const other of menus)if(other!==details)other.open=false});
    details.addEventListener('toggle',()=>{if(details.open){for(const other of menus)if(other!==details)other.open=false;place(details)}});
    details.addEventListener('keydown',event=>{if(event.key==='Escape'&&details.open){event.preventDefault();event.stopPropagation();details.open=false;details.querySelector('summary').focus()}});
    details.addEventListener('click',event=>{const action=event.target.closest('button');if(action&&!action.disabled&&!action.hasAttribute('data-keep-open'))details.open=false});
  }
  $('viewOptions').addEventListener('toggle',()=>place($('displayOptions')));
  document.addEventListener('pointerdown',event=>{for(const details of menus)if(!details.contains(event.target))details.open=false});
  addEventListener('scroll',event=>{if(event.target===document||event.target===window)close()},{passive:true});
  addEventListener('resize',close);
}
function initializeExplorerShell(){
  if(shellInitialized)return;shellInitialized=true;
  initializeExplorerIcons();
  initializeToolbarMenus();
  $('propertiesClose').onclick=()=>$('propertiesDialog').close();$('propertiesDialog').addEventListener('close',()=>{propertyRevision++;propertyAbort?.abort();propertyAbort=null;propertySizeAbort?.abort();propertySizeAbort=null});
  const sideButton=event=>{
    if(![3,4].includes(event.button)||$('app').hidden)return;
    event.preventDefault();event.stopPropagation();
    const activate=event.type==='pointerup'||event.type==='mouseup'&&!window.PointerEvent;
    if(activate&&!document.querySelector('dialog[open]')&&!loading&&!mutating)(event.button===3?$('back'):$('forward')).click();
  };
  for(const type of ['pointerdown','pointerup','mousedown','mouseup','auxclick'])document.addEventListener(type,sideButton,{capture:true,passive:false});
  document.addEventListener('keydown',event=>{if(event.key==='ArrowUp'&&document.activeElement.matches('input,textarea,select,[contenteditable]'))return;if(!event.altKey||event.ctrlKey||event.metaKey||!['ArrowLeft','ArrowRight','ArrowUp'].includes(event.key)||$('app').hidden)return;event.preventDefault();event.stopPropagation();if(!document.querySelector('dialog[open]')&&!loading&&!mutating)(event.key==='ArrowLeft'?$('back'):event.key==='ArrowUp'?$('up'):$('forward')).click()},{capture:true});
  $('openWithNative').onclick=()=>requestOpenWith(openWithItem);
  $('openWithCancel').onclick=()=>$('openWithDialog').close();$('openWithSave').onclick=async()=>{const program=$('openWithProgram').value.trim();if(!program){$('openWithError').textContent='请填写本机程序的完整路径';return}if(await requestOpenWith(openWithItem,program))$('openWithDialog').close()};$('openWithProgram').onkeydown=event=>{if(event.key==='Enter'){$('openWithSave').click();event.preventDefault()}};
  $('sidebarToggle').onclick=()=>{
    if(innerWidth<=760){$('toggleNav').click();applySidebar();return}
    sidebarCollapsed=!sidebarCollapsed;applySidebar();cacheExplorerView();savePreferences({sidebarCollapsed});requestAnimationFrame(revealActiveFileTab);
  };
  $('navBackdrop').addEventListener('click',applySidebar);
  $('pageSize').onchange=()=>{const size=Number($('pageSize').value);if(![50,100,200,500,1000].includes(size))return;pageSize=size;cacheExplorerView();savePreferences({pageSize});navigate(path,{remember:false,page:Math.floor(offset/pageSize)*pageSize})};
  const sortChanged=()=>{defaultSort=$('sort').value;defaultOrder=$('order').value;cacheExplorerView();savePreferences({sort:defaultSort,order:defaultOrder});navigate(path,{remember:false})};
  $('sort').onchange=$('order').onchange=sortChanged;
  for(const b of document.querySelectorAll('[data-sort-key]'))b.onclick=()=>{if(mutating)return;const key=b.dataset.sortKey,same=$('sort').value===key;$('sort').value=key;$('order').value=same?($('order').value==='asc'?'desc':'asc'):key==='name'?'asc':'desc';sortChanged()};
  document.addEventListener('pointerdown',event=>{if(!$('viewOptions').contains(event.target))$('viewOptions').open=false});
  $('viewOptions').addEventListener('keydown',event=>{if(event.key==='Escape'){$('viewOptions').open=false;$('viewOptions').querySelector('summary').focus();event.stopPropagation()}});
  $('backTop').onclick=()=>window.scrollTo({top:0,behavior:window.matchMedia?.('(prefers-reduced-motion: reduce)').matches?'auto':'smooth'});
  $('entries').addEventListener('contextmenu',event=>{if(!event.target.closest('.entry'))showContextMenu(event)});
  document.addEventListener('pointerdown',event=>{if(!$('contextMenu').contains(event.target))closeContextMenu()});
  $('contextMenu').addEventListener('keydown',event=>{
    const buttons=[...$('contextMenu').querySelectorAll('button:not(:disabled)')],index=buttons.indexOf(document.activeElement);let next;
    if(event.key==='ArrowDown')next=buttons[(index+1)%buttons.length];if(event.key==='ArrowUp')next=buttons[(index+buttons.length-1)%buttons.length];if(event.key==='Home')next=buttons[0];if(event.key==='End')next=buttons.at(-1);
    if(next){event.preventDefault();next.focus()}if(event.key==='Escape'){event.preventDefault();event.stopPropagation();closeContextMenu(true)}if(event.key==='Tab')closeContextMenu();
  });
  addEventListener('scroll',()=>{$('backTop').hidden=window.scrollY<400;closeContextMenu()},{passive:true});
  addEventListener('resize',()=>{closeContextMenu();applySidebar()});
  const tips={back:'返回上一个已浏览目录',forward:'前进到下一个已浏览目录',up:'打开当前目录的上一级',refresh:'重新读取当前目录（F5）',go:'打开地址栏中的目录',favorite:'将当前目录添加到快速访问，重启后保留',newFolder:'在当前目录新建文件夹',upload:'上传文件到当前目录，同名文件不会覆盖',rename:'修改所选文件或文件夹的名称（F2）',move:'把所选项目移动到其他目录；同名时选择处理方式',remove:'删除所选项目，默认移入回收站，操作前需要确认（Delete）',download:'下载所选文件的原文件',openFolderTab:'在工具箱新标签页打开所选目录',selectAll:'选择当前页全部项目',clearSelection:'取消当前所有选择',view:'切换图标与列表视图',previous:'显示上一页项目',next:'显示下一页项目',sort:'选择名称、修改时间或文件大小排序',order:'选择正序或倒序，文件夹始终排在文件前面',pageSize:'选择每页显示条数；自动记忆，大图标较多时可选较小数量',backToolbox:'返回仓鼠工具箱的批处理工具',openGeneralSettings:'通用设置：播放、界面、任务提醒、共享与远程访问及启动'};
  for(const [id,text]of Object.entries(tips)){const control=$(id);if(control)control.title=text;}
  const decorate=node=>{if(node.nodeType!==1)return;for(const b of [node,...node.querySelectorAll('button,[role=button],select')])if(b.matches('button,[role=button],select')&&!b.title)b.title=b.getAttribute('aria-label')||b.textContent.trim()};
  decorate(document.body);new MutationObserver(records=>{for(const record of records)for(const node of record.addedNodes)decorate(node)}).observe(document.body,{subtree:true,childList:true});
}
