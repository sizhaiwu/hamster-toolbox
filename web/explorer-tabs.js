'use strict';

// A workspace belongs to this browser tab, not the server or other devices.
// Store navigation only: never cache credentials, file versions or mutations.
let fileTabs=[],activeFileTab='',future=[],navigationAbort=null,workspaceStorageOK=true,scrollRestorePending=null;
const maxFileTabs=20,maxTabHistory=40;

function tabStorage(){return local&&window.toolboxSettings?.RestoreTabs?localStorage:sessionStorage}
function activeTab(){return fileTabs.find(tab=>tab.id===activeFileTab)}
function workspaceKey(){return 'hamster.files.tabs.v1.'+(local?'local':'guest')}
function forgetTabs(){try{tabStorage().removeItem(workspaceKey())}catch{}fileTabs=[];activeFileTab=''}
function tabID(){return 'tab-'+(globalThis.crypto?.randomUUID?.()||Date.now().toString(36)+'-'+Math.random().toString(36).slice(2))}
function allowedLocation(location){
  if(!location||typeof location.path!=='string'||location.path.length>32768||typeof location.root!=='string')return false;
  if(local)return location.root==='';
  return !/^[\\/]|[:\x00]/.test(location.path)&&!location.path.split(/[\\/]/).includes('..')&&Boolean(boot?.Roots?.some(item=>item.ID===location.root));
}
function restoreHistoryLocation(value){
  const result={root:value.root,path:value.path,offset:Number.isInteger(value.offset)?Math.max(0,Math.min(Number.MAX_SAFE_INTEGER,value.offset)):0,scroll:Number.isFinite(value.scroll)?Math.max(0,Math.min(20000000,value.scroll)):0};
  if(typeof value.search==='string')result.search=value.search.slice(0,256);
  if(['name','modified','size'].includes(value.sort))result.sort=value.sort;
  if(['asc','desc'].includes(value.order))result.order=value.order;
  if(['grid','list'].includes(value.view))result.view=value.view;
  if([50,100,200,500,1000].includes(value.pageSize))result.pageSize=value.pageSize;
  return result;
}
function newTabState(location={root,path}){
  return {id:tabID(),root:location.root,path:location.path,back:[],forward:[],search:'',sort:defaultSort,order:defaultOrder,view,offset:0,scroll:0};
}
function rememberTab(){
  const tab=activeTab();if(!tab)return;
  const scroll=scrollRestorePending?.tab===activeFileTab?scrollRestorePending.value:loading?tab.scroll:Math.max(0,window.scrollY||0);
  Object.assign(tab,{root,path,back:history.slice(-maxTabHistory),forward:future.slice(-maxTabHistory),search:$('search').value.slice(0,256),sort:$('sort').value,order:$('order').value,view,offset,scroll});
}
let serverWorkspaceTimer;
function persistTabs(flush=false){
  if(!activeFileTab)return;
  try{
    const saved=fileTabs.map(({id,root,path,back,forward,search,sort,order,view,offset,scroll})=>({id,root,path,back,forward,search,sort,order,view,offset,scroll}));
    let value=JSON.stringify({version:1,active:activeFileTab,tabs:saved});
    if(value.length>1024*1024)value=JSON.stringify({version:1,active:activeFileTab,tabs:saved.map(tab=>({...tab,back:[],forward:[]}))});
    tabStorage().setItem(workspaceKey(),value);workspaceStorageOK=true;
    if(local&&window.toolboxSettings?.RestoreTabs){
      const workspace={version:1,active:activeFileTab,tabs:saved.map(({id,root,path,search,sort,order,view,offset,scroll})=>({id,root,path,search,sort,order,view,offset,scroll}))};clearTimeout(serverWorkspaceTimer);
      if(flush){fetch(endpoint('api/preferences'),{method:'POST',credentials:'same-origin',headers:{'Content-Type':'application/json','X-Hamster-Browser':'1'},body:JSON.stringify({workspace}),keepalive:true}).catch(()=>{})}
      else serverWorkspaceTimer=setTimeout(()=>savePreferences({workspace}),300);
    }

  }catch{workspaceStorageOK=false}
}
function restoreTabs(){
  fileTabs=[];activeFileTab='';
  try{
    const text=tabStorage().getItem(workspaceKey())||(local&&window.toolboxSettings?.RestoreTabs&&boot.Preferences?.workspace?JSON.stringify(boot.Preferences.workspace):null);
    if(text&&text.length<=1024*1024){
      const saved=JSON.parse(text);
      if(saved.version===1&&Array.isArray(saved.tabs)){
        const ids=new Set();
        for(const item of saved.tabs.slice(0,maxFileTabs)){
          if(!allowedLocation(item))continue;
          const tab=newTabState(item);
          if(typeof item.id==='string'&&/^[a-z0-9_-]{1,80}$/i.test(item.id)&&!ids.has(item.id))tab.id=item.id;
          ids.add(tab.id);
          tab.back=Array.isArray(item.back)?item.back.filter(allowedLocation).slice(-maxTabHistory).map(restoreHistoryLocation):[];
          tab.forward=Array.isArray(item.forward)?item.forward.filter(allowedLocation).slice(-maxTabHistory).map(restoreHistoryLocation):[];
          tab.search=typeof item.search==='string'?item.search.slice(0,256):'';
          tab.sort=['name','modified','size'].includes(item.sort)?item.sort:'name';
          tab.order=['asc','desc'].includes(item.order)?item.order:tab.sort==='name'?'asc':'desc';
          tab.view=item.view==='list'?'list':'grid';
          tab.offset=Number.isInteger(item.offset)?Math.max(0,Math.min(Number.MAX_SAFE_INTEGER,item.offset)):0;
          tab.scroll=Number.isFinite(item.scroll)?Math.max(0,Math.min(20000000,item.scroll)):0;
          fileTabs.push(tab);
        }
        if(fileTabs.some(tab=>tab.id===saved.active))activeFileTab=saved.active;
      }
    }
  }catch{workspaceStorageOK=false}
  if(!fileTabs.length)fileTabs.push(newTabState({root:local?'':boot.Roots[0]?.ID||'',path:''}));
  if(!activeFileTab)activeFileTab=fileTabs[0].id;
}
function sharedLocationName(id){
  const share=boot?.Roots?.find(item=>item.ID===id);if(!share)return '共享目录';
  const same=boot.Roots.filter(item=>item.Name===share.Name);
  return share.Name+(same.length>1?' '+(same.findIndex(item=>item.ID===id)+1):'');
}
function locationDetails(location){
  const windows=local&&separator()==='\\',split=windows?/[\\/]+/:/\/+/,parts=location.path.split(split).filter(Boolean);
  let anchor=local?'/':sharedLocationName(location.root),folders=parts;
  if(windows){
    if(/^[\\/]{2}/.test(location.path)){anchor='\\\\'+parts.slice(0,2).join('\\');folders=parts.slice(2)}
    else{anchor=parts[0]?.toUpperCase()||'';folders=parts.slice(1)}
  }
  const name=folders.at(-1)||(local?(location.path?(windows?anchor+'\\':'/'):'此电脑'):anchor);
  const normalized=windows?location.path.replace(/[\\/]+/g,'/').replace(/\/+$/,'').toLowerCase():location.path.replace(/\/+$/,'')||'/';
  return {name,anchor,parents:folders.slice(0,-1),key:JSON.stringify([location.root||'',normalized]),title:local?(location.path||'此电脑'):anchor+(location.path?'/'+location.path:''),windows};
}
function locationDisplay(location,peers){
  const current=locationDetails(location),nameKey=name=>current.windows?name.toLowerCase():name;
  const matches=peers.map(locationDetails).filter(item=>nameKey(item.name)===nameKey(current.name));
  if(matches.every(item=>item.key===current.key))return {...current,context:''};
  const context=(item,depth)=>{
    const parents=item.parents.slice(-depth),tail=depth?parents.join(item.windows?'\\':'/'):'';
    if(!tail)return item.anchor||'此电脑';
    const omitted=depth<item.parents.length;
    return item.windows?item.anchor+'\\'+(omitted?'…\\':'')+tail:local?(omitted?'…/':'/')+tail:item.anchor+'/'+(omitted?'…/':'')+tail;
  };
  for(let depth=0;depth<=current.parents.length;depth++){
    const text=context(current,depth);
    if(matches.every(item=>item.key===current.key||nameKey(context(item,depth))!==nameKey(text)))return {...current,context:text};
  }
  return {...current,context:current.title};
}
function labelLocation(target,display,prefix=''){
  const name=document.createElement('span');name.className='location-name';name.textContent=prefix+display.name;target.replaceChildren(name);
  if(display.context){const context=document.createElement('span');context.className='location-context';context.textContent=display.context;target.append(context)}
  target.title=display.title;
}
function tabTitle(tab){const display=locationDisplay(tab,fileTabs);return display.name+(display.context?' · '+display.context:'')}
function revealActiveFileTab(){
  const strip=$('fileTabs'),current=document.getElementById(activeFileTab);if(!current)return;
  const box=current.parentElement.getBoundingClientRect(),area=strip.getBoundingClientRect();if(box.left<area.left)strip.scrollLeft-=area.left-box.left;else if(box.right>area.right)strip.scrollLeft+=box.right-area.right;
}
function renderTabs(){
  const strip=$('fileTabs');strip.replaceChildren();
  for(const tab of fileTabs){
    const wrap=document.createElement('div');wrap.className='browser-tab'+(tab.id===activeFileTab?' active':'');wrap.setAttribute('role','presentation');
    const display=locationDisplay(tab,fileTabs),label=button('',()=>{if(!navigationSortClickBlocked())switchFileTab(tab.id)},'tab-label');labelLocation(label,display);wrap.classList.toggle('has-location',Boolean(display.context));
    label.id=tab.id;label.setAttribute('role','tab');label.setAttribute('aria-selected',String(tab.id===activeFileTab));label.setAttribute('aria-controls','entries');label.tabIndex=tab.id===activeFileTab?0:-1;
    label.setAttribute('aria-label',tabTitle(tab));
    label.onauxclick=e=>{if(e.button===1){e.preventDefault();closeFileTab(tab.id)}};
    label.onkeydown=e=>{
      if(e.key==='ContextMenu'||e.shiftKey&&e.key==='F10'){showFileTabContext(e,tab.id,label);return}
      const index=fileTabs.indexOf(tab);let target;
      if(e.key==='ArrowRight')target=fileTabs[(index+1)%fileTabs.length];
      if(e.key==='ArrowLeft')target=fileTabs[(index+fileTabs.length-1)%fileTabs.length];
      if(e.key==='Home')target=fileTabs[0];if(e.key==='End')target=fileTabs.at(-1);
      if(target){e.preventDefault();switchFileTab(target.id).then(()=>document.getElementById(target.id)?.focus())}
    };
    const close=button('×',()=>closeFileTab(tab.id),'tab-close');close.setAttribute('aria-label','关闭标签页 '+tabTitle(tab));close.title=fileTabs.length===1?'至少保留一个标签页':'关闭标签页';close.disabled=mutating||fileTabs.length===1;label.disabled=mutating;
    const more=button('···',event=>showFileTabContext(event,tab.id,label),'tab-more');more.title='标签页操作';more.setAttribute('aria-label','标签页操作 '+tabTitle(tab));more.disabled=mutating;
    wrap.oncontextmenu=event=>showFileTabContext(event,tab.id,label);
    bindNavigationSort(wrap,'tabs',tab.id,'x',moveFileTab,()=>mutating);
    wrap.append(label,more,close);strip.append(wrap);
  }
  $('newTab').disabled=mutating||fileTabs.length>=maxFileTabs;
  requestAnimationFrame(revealActiveFileTab);
}
function moveFileTab(source,target,after=false){
  if(mutating)return;
  const next=reorderNavigation(fileTabs,source,target,after,tab=>tab.id);if(!next)return;
  rememberTab();fileTabs=next;renderTabs();persistTabs();
}
async function duplicateFileTab(id){
  if(mutating||fileTabs.length>=maxFileTabs)return;
  rememberTab();const source=fileTabs.find(tab=>tab.id===id);if(!source||!allowedLocation(source))return;
  const copy={...source,id:tabID(),back:source.back.map(item=>({...item})),forward:source.forward.map(item=>({...item}))};
  fileTabs.splice(fileTabs.indexOf(source)+1,0,copy);await switchFileTab(copy.id,{remember:false});
}
async function closeRelatedFileTabs(id,rightOnly=false){
  if(mutating)return;const index=fileTabs.findIndex(tab=>tab.id===id);if(index<0)return;
  rememberTab();fileTabs=rightOnly?fileTabs.slice(0,index+1):[fileTabs[index]];
  if(!fileTabs.some(tab=>tab.id===activeFileTab))await switchFileTab(id,{remember:false});else{renderTabs();persistTabs()}
}
async function switchFileTab(id,{remember=true}={}){
  if(mutating){say('请等待当前文件操作结束后再切换标签。',true);return}
  const next=fileTabs.find(tab=>tab.id===id);if(!next)return;
  if(remember)rememberTab();
  clearTimeout(searchTimer);navigationAbort?.abort();requestID++;loading=false;
  for(const dialog of document.querySelectorAll('dialog[open]:not(#taskResultDialog):not(#generalSettingsDialog)'))dialog.close();
  clearViewer();closeNav();selection.clear();selectionMode=false;selectionAnchor='';
  activeFileTab=id;root=next.root;path=next.path;history=[...next.back];future=[...next.forward];view=next.view;offset=next.offset;rows=[];total=0;parent='';
  $('search').value=next.search;$('sort').value=next.sort;$('order').value=next.order;
  $('location').value=local?path:(boot.Roots.find(item=>item.ID===root)?.Name||'共享目录')+(path?'/'+path:'');
  renderTabs();render();breadcrumb();side();persistTabs();
  const navigationRevision=requestID+1;
  const restored=await navigate(path,{remember:false,page:next.offset,scroll:next.scroll});
  if(!restored&&requestID===navigationRevision&&activeFileTab===id&&$('loginPanel').hidden){
    history=[];future=[];next.back=[];next.forward=[];
    const available=await navigate('',{remember:false,scroll:0});
    if(available)say('上次的目录已不存在或无法访问，已返回可用的根目录。',true);
  }
}
async function openFileTab(location={root,path}){
  if(mutating){say('请等待当前文件操作结束后再新建标签。',true);return}
  if(fileTabs.length>=maxFileTabs){say('最多打开20个标签页，请先关闭不需要的标签。',true);return}
  if(!allowedLocation(location))return;
  rememberTab();const tab=newTabState(location);fileTabs.push(tab);await switchFileTab(tab.id,{remember:false});
}
async function closeFileTab(id){
  if(mutating||fileTabs.length<=1)return;
  rememberTab();const index=fileTabs.findIndex(tab=>tab.id===id);if(index<0)return;
  const wasActive=id===activeFileTab;fileTabs.splice(index,1);
  if(wasActive)await switchFileTab(fileTabs[Math.min(index,fileTabs.length-1)].id,{remember:false});else{renderTabs();persistTabs()}
}
function updateTabActions(){
  $('back').disabled=loading||mutating||history.length===0;
  $('forward').disabled=loading||mutating||future.length===0;
  const selectedFolders=selected();$('openFolderTab').hidden=selectedFolders.length!==1||!(selectedFolders[0]?.Dir||selectedFolders[0]?.Kind==='shortcut');
  $('openFolderTab').disabled=loading||mutating;
  $('newTab').disabled=mutating||fileTabs.length>=maxFileTabs;
  for(const tab of $('fileTabs').querySelectorAll('button'))tab.disabled=mutating||(tab.classList.contains('tab-close')&&fileTabs.length===1);
}
function initializeTabs(){
  restoreTabs();renderTabs();
  bindWorkspaceEvents();
  $('newTab').onclick=()=>openFileTab();
  $('openFolderTab').onclick=()=>{const folder=selected()[0];if(folder?.Dir)openFileTab({root,path:folder.Path});else if(folder?.Kind==='shortcut')openDirectoryShortcut(folder,true)};
  $('back').onclick=()=>{if(history.length){const previous=history.at(-1);navigate(previous.path,{targetRoot:previous.root,mode:'back'})}};
  $('forward').onclick=()=>{if(future.length){const next=future.at(-1);navigate(next.path,{targetRoot:next.root,mode:'forward'})}};
}
let workspaceScrollTimer,workspaceEventsBound=false;
function bindWorkspaceEvents(){
  if(workspaceEventsBound)return;workspaceEventsBound=true;
  addEventListener('scroll',()=>{clearTimeout(workspaceScrollTimer);workspaceScrollTimer=setTimeout(()=>{if(!loading&&!mutating){rememberTab();persistTabs()}},150)},{passive:true});
  addEventListener('pagehide',()=>{rememberTab();persistTabs(true)});
  addEventListener('resize',()=>requestAnimationFrame(revealActiveFileTab));
}
