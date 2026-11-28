'use strict';

let imageViewer=null,imagePreviewMode='single',continuousImageCount=1,viewerHeaderTracking=false;
function imagePreviewHint(compatible=false){return (compatible?'兼容预览 · 下载可取得原图；':'')+(imagePreviewMode==='continuous'?'左右拼接相邻图片 · 点击两侧图片切换 · R / Shift+R 旋转':'滚轮 / 双指缩放 · 放大后拖动 · R / Shift+R 旋转 · ← → 切换')}

// Encoded bytes only, scoped to this page/session. Never persist private media.
const viewerImageCache=new Map(),viewerImagePending=new Map();
let viewerImageCacheBytes=0,viewerImageSourceCleanup=null,viewerImagePrefetch=null;
const viewerImageCacheLimit=24*1024*1024,viewerImageItemLimit=8*1024*1024;
function viewerImageKey(item,itemRoot){return JSON.stringify([itemRoot,item.Path,item.Version,item.Size,item.Modified])}
function retireViewerImage(entry){if(!entry)return;entry.retired=true;entry.decodedImage?.removeAttribute('src');entry.decodedImage=null;entry.decodedBytes=0;if(entry.url&&!entry.references){URL.revokeObjectURL(entry.url);entry.url=null}}
function desktopViewerImageURL(entry){if(!entry.url)entry.url=URL.createObjectURL(entry.blob);return entry.url}
async function predecodeDesktopImage(key,blob,signal){
  if(!window.hamsterDesktop?.mediaSettings||!blob.type.includes('jpeg')||signal.aborted||typeof blob.slice!=='function')return;
  const entry=viewerImageCache.get(key);if(!entry||entry.decodedImage)return;
  try{
    const data=new DataView(await blob.slice(0,524288).arrayBuffer());if(data.byteLength<4||data.getUint16(0)!==0xffd8)return;
    let pixels=0;for(let p=2;p+4<data.byteLength;){if(data.getUint8(p)!==0xff){p++;continue}const marker=data.getUint8(p+1);if(marker===0xff){p++;continue}if(marker===0xda||marker===0xd9)break;if(marker===0x01||marker>=0xd0&&marker<=0xd7){p+=2;continue}const length=data.getUint16(p+2);if(length<2||p+2+length>data.byteLength)break;if([0xc0,0xc1,0xc2,0xc3,0xc5,0xc6,0xc7,0xc9,0xca,0xcb,0xcd,0xce,0xcf].includes(marker)&&length>=7){pixels=data.getUint16(p+5)*data.getUint16(p+7);break}p+=2+length}
    const bytes=pixels*4,budget=48*1024*1024;if(!pixels||bytes>budget||signal.aborted||viewerImageCache.get(key)!==entry)return;
    const decoded=[...viewerImageCache.values()].filter(e=>e.decodedImage);let total=decoded.reduce((n,e)=>n+(e.decodedBytes||0),0);
    for(const previous of decoded){if(total+bytes<=budget&&decoded.filter(e=>e.decodedImage).length<2)break;previous.decodedImage.removeAttribute('src');previous.decodedImage=null;total-=previous.decodedBytes||0;previous.decodedBytes=0}
    const img=new Image();img.decoding='async';entry.decodedImage=img;entry.decodedBytes=bytes;img.src=desktopViewerImageURL(entry);await img.decode?.();if(signal.aborted||entry.retired){img.removeAttribute('src');entry.decodedImage=null;entry.decodedBytes=0}
  }catch{if(entry){entry.decodedImage?.removeAttribute('src');entry.decodedImage=null;entry.decodedBytes=0}}
}
function stopViewerImagePrefetch(except=''){if(viewerImagePrefetch&&viewerImagePrefetch.key!==except){clearTimeout(viewerImagePrefetch.timer);viewerImagePrefetch.controller.abort();viewerImagePrefetch=null}}
function clearViewerImageCache(){stopViewerImagePrefetch();viewerImageSourceCleanup?.();for(const entry of viewerImagePending.values())entry.controller.abort();viewerImagePending.clear();for(const entry of viewerImageCache.values())retireViewerImage(entry);viewerImageCache.clear();viewerImageCacheBytes=0}
async function cachedViewerImage(item,itemRoot,signal,priority='high'){
  if(!(item.Size>0&&item.Size<=viewerImageItemLimit)||signal.aborted)return null;
  const key=viewerImageKey(item,itemRoot),cached=viewerImageCache.get(key);
  if(cached&&cached.expires>Date.now()){viewerImageCache.delete(key);viewerImageCache.set(key,cached);return cached.blob}
  if(cached){viewerImageCache.delete(key);viewerImageCacheBytes-=cached.blob.size;retireViewerImage(cached)}
  let entry=viewerImagePending.get(key);
  if(!entry||entry.controller.signal.aborted){
    entry={controller:new AbortController()};const request=entry,timeout=setTimeout(()=>request.controller.abort(),15000);
    entry.promise=(async()=>{
      const response=await fetch(endpoint('raw',{path:item.Path},itemRoot),{credentials:'same-origin',signal:request.controller.signal,priority});
      if(!response.ok||!response.body?.getReader){request.controller.abort();return null}
      if(Number(response.headers.get('Content-Length'))>viewerImageItemLimit){await response.body.cancel();return null}
      const type=response.headers.get('Content-Type')||'';if(!type.startsWith('image/')){await response.body.cancel();return null}
      const reader=response.body.getReader(),parts=[];let size=0;
      try{while(true){const part=await reader.read();if(part.done)break;size+=part.value.byteLength;if(size>viewerImageItemLimit){await reader.cancel();return null}parts.push(part.value)}}finally{reader.releaseLock()}
      if(request.controller.signal.aborted||!size)return null;
      const blob=new Blob(parts,{type});
      while(viewerImageCache.size>=6||viewerImageCacheBytes+size>viewerImageCacheLimit){const oldest=viewerImageCache.keys().next().value,previous=viewerImageCache.get(oldest);viewerImageCacheBytes-=previous.blob.size;retireViewerImage(previous);viewerImageCache.delete(oldest)}
      viewerImageCache.set(key,{blob,expires:Date.now()+120000});viewerImageCacheBytes+=size;return blob;
    })().finally(()=>{clearTimeout(timeout);if(viewerImagePending.get(key)===request)viewerImagePending.delete(key)});
    viewerImagePending.set(key,entry);
  }
  const abort=()=>entry.controller.abort();signal.addEventListener('abort',abort,{once:true});
  try{return await entry.promise}finally{signal.removeEventListener('abort',abort)}
}
function installViewerImageSource(img,item,itemRoot,isCurrent){
  viewerImageSourceCleanup?.();
  const controller=new AbortController(),key=viewerImageKey(item,itemRoot);let objectURL=null,sharedEntry=null,alive=true;
  stopViewerImagePrefetch(key);
  const active=()=>alive&&!controller.signal.aborted&&isCurrent()&&!img.dataset.compatible;
  const direct=()=>{if(active())img.src=endpoint('raw',{path:item.Path},itemRoot)};
  if(item.Size>0&&item.Size<=viewerImageItemLimit){cachedViewerImage(item,itemRoot,controller.signal).then(blob=>{if(!active())return;if(blob){const entry=viewerImageCache.get(key);if(window.hamsterDesktop?.mediaSettings&&entry){sharedEntry=entry;entry.references=(entry.references||0)+1;objectURL=desktopViewerImageURL(entry)}else objectURL=URL.createObjectURL(blob);img.src=objectURL}else direct()}).catch(()=>direct())}else direct();
  const prefetch=()=>{
    if(!active()||imagePreviewMode==='continuous'||document.hidden||navigator.connection?.saveData||/^(slow-)?2g$/.test(navigator.connection?.effectiveType||''))return;
    stopViewerImagePrefetch();const next=previewNeighbor(1),neighbor=rows[next];
    if(!neighbor||neighbor.Kind!=='image'||!(neighbor.Size>0&&neighbor.Size<=(canUseNativePrograms()?4:2)*1024*1024))return;
    const pending={key:viewerImageKey(neighbor,itemRoot),controller:new AbortController()};viewerImagePrefetch=pending;
    pending.timer=setTimeout(async()=>{if(!active()||document.hidden)return;try{const blob=await cachedViewerImage(neighbor,itemRoot,pending.controller.signal,'low');if(blob)await predecodeDesktopImage(viewerImageKey(neighbor,itemRoot),blob,pending.controller.signal);if(window.hamsterDesktop?.mediaSettings&&active()&&!document.hidden&&!pending.controller.signal.aborted){const previous=rows[previewNeighbor(-1)];if(previous?.Kind==='image'&&previous.Size>0&&previous.Size<=4*1024*1024){const before=await cachedViewerImage(previous,itemRoot,pending.controller.signal,'low');if(before)await predecodeDesktopImage(viewerImageKey(previous,itemRoot),before,pending.controller.signal)}}}catch{}finally{if(viewerImagePrefetch===pending)viewerImagePrefetch=null}},250);
  };
  // Start speculative IO only after the current image has loaded.
  img.addEventListener('load',prefetch);
  viewerImageSourceCleanup=()=>{alive=false;controller.abort();img.removeEventListener('load',prefetch);if(sharedEntry){sharedEntry.references=Math.max(0,(sharedEntry.references||0)-1);if(sharedEntry.retired&&!sharedEntry.references&&sharedEntry.url){URL.revokeObjectURL(sharedEntry.url);sharedEntry.url=null}}else if(objectURL)URL.revokeObjectURL(objectURL);viewerImageSourceCleanup=null};
}
let viewerImageHelpInstalled=false;
let filmstripRows=null,filmstripKind='',filmstripRoot='',filmstripCollapsed=false,filmstripItems=[];
let videoPlayback=null;
let videoHover=null,hoverLifecycleInstalled=false,hoverPreviewEnabled=true,lastHoverTouch=0;
function stopVideoHover(){
  const active=videoHover;videoHover=null;if(!active)return;
  clearTimeout(active.delay);clearTimeout(active.timeout);active.controller?.abort();active.card.classList.remove('hover-playing');active.badge?.remove();
  if(active.media){active.media.onplaying=active.media.onerror=null;active.media.pause();active.media.removeAttribute('src');active.media.load();active.media.remove()}
  if(active.objectURL)URL.revokeObjectURL(active.objectURL);
}
function videoHoverAllowed(){
  return hoverPreviewEnabled&&!document.hidden&&!loading&&!mutating&&!selectionMode&&!document.querySelector('dialog[open]');
}
function updateHoverPreviewControl(){
  const toggle=$('hoverPreview');if(!toggle)return;
  toggle.setAttribute('aria-pressed',String(hoverPreviewEnabled));toggle.textContent='悬停预览：'+(hoverPreviewEnabled?'开':'关');toggle.title=hoverPreviewEnabled?'鼠标停留约半秒后播放静音短片；点击关闭以减少流量':'点击开启视频静音悬停预览';
}
function installVideoHover(open,icon,card,item,itemRoot){
  const enter=event=>{
    if(event.pointerType==='touch'){lastHoverTouch=Date.now();return}
    if(event.type.startsWith('pointer')&&event.pointerType!=='mouse'||event.sourceCapabilities?.firesTouchEvents||Date.now()-lastHoverTouch<1500||!videoHoverAllowed()||event.ctrlKey||event.metaKey||videoHover?.card===card)return;
    stopVideoHover();const active={card};videoHover=active;
    const badge=document.createElement('span');active.badge=badge;badge.className='hover-preview-label';badge.textContent='准备预览…';badge.setAttribute('aria-hidden','true');card.append(badge);
    active.delay=setTimeout(()=>{
      if(videoHover!==active||!open.isConnected||!videoHoverAllowed()){if(videoHover===active)stopVideoHover();return}
      const media=document.createElement('video');active.media=media;active.controller=new AbortController();
      media.className='hover-preview';media.muted=media.defaultMuted=true;media.setAttribute('muted','');media.playsInline=true;media.setAttribute('playsinline','');media.loop=true;media.preload='none';media.setAttribute('aria-hidden','true');media.tabIndex=-1;
      badge.textContent='加载预览…';
      const fail=error=>{if(videoHover!==active||active.failed)return;active.failed=true;clearTimeout(active.timeout);active.controller.abort();media.onplaying=media.onerror=null;media.pause();media.removeAttribute('src');media.load();media.remove();if(active.objectURL){URL.revokeObjectURL(active.objectURL);active.objectURL=null}card.classList.remove('hover-playing');const reason=error?.name==='NotAllowedError'?'浏览器阻止自动播放':error instanceof Error?error.message:'当前浏览器无法播放预览短片';badge.textContent='预览失败，点击播放';badge.classList.add('hover-preview-error');badge.title=reason;icon.title=reason;say('悬停预览失败：'+reason+'；可点击视频使用完整播放器。',true)};
      media.onplaying=()=>{if(videoHover===active){clearTimeout(active.timeout);card.classList.add('hover-playing');badge.textContent='静音预览'}};
      media.onerror=fail;icon.append(media);active.timeout=setTimeout(()=>fail(Error('读取预览超时，请稍后重试')),20000);
      fetch(endpoint('video-preview',{path:item.Path},itemRoot),{credentials:'same-origin',signal:active.controller.signal}).then(async response=>{
        if(!response.ok){const reason=(await response.text()).trim().split('\n')[0].slice(0,180);throw Error(reason||'无法读取预览，HTTP '+response.status)}
        const clip=await response.blob();if(videoHover!==active||active.controller.signal.aborted)return;
        if(clip.size===0||clip.size>2*1024*1024)throw Error('预览短片为空或过大');
        active.objectURL=URL.createObjectURL(clip);media.src=active.objectURL;await media.play();
      }).catch(error=>{if(error.name!=='AbortError')fail(error)});
    },450);
  };
  for(const type of ['pointerenter','pointermove','mouseenter','mousemove'])open.addEventListener(type,enter);
  for(const type of ['pointerleave','mouseleave'])open.addEventListener(type,()=>{if(videoHover?.card===card)stopVideoHover()});
  open.title+='；鼠标悬停可静音预览';
}
let viewerQueueOpen=false,viewerQueueRows=null,viewerQueueKind='',viewerQueueRoot='';
function clearViewerQueue(){viewerQueueRows=null;for(const card of $('viewerQueueItems').children)releaseListingVideo(card);$('viewerQueueItems').replaceChildren();$('viewerQueue').hidden=true}
function updateViewerQueue(){
  const kind=rows[previewIndex]?.Kind,available=['image','video'].includes(kind),items=available?rows.map((item,index)=>({item,index})).filter(({item})=>!item.Dir&&item.Kind===kind):[];
  $('viewerQueueToggle').hidden=!available;$('viewerQueue').hidden=!available||!viewerQueueOpen;$('viewerQueueToggle').setAttribute('aria-expanded',String(available&&viewerQueueOpen));$('viewerQueueToggle').setAttribute('aria-label',viewerQueueOpen?'隐藏文件列表':'显示文件列表');
  $('viewerCounter').textContent=available?(total>pageSize?'第 '+(Math.floor(offset/pageSize)+1)+' 页 · ':'')+(items.findIndex(entry=>entry.index===previewIndex)+1)+' / '+items.length:'文件预览';
  if(!available||!viewerQueueOpen){clearViewerQueue();return}
  $('viewerQueueTitle').textContent=(kind==='video'?'视频':'图片')+'列表 · '+items.length+' 项';
  if(viewerQueueRows!==rows||viewerQueueKind!==kind||viewerQueueRoot!==root){
    clearViewerQueue();$('viewerQueue').hidden=false;
    viewerQueueRows=rows;viewerQueueKind=kind;viewerQueueRoot=root;const list=$('viewerQueueItems');
    for(const [position,{item,index}]of items.entries()){
      const b=button('',()=>preview(index),'queue-item');b.dataset.index=index;b.title=item.Name;b.setAttribute('aria-label','查看 '+item.Name);
      const number=document.createElement('span');number.className='queue-number';number.textContent=position+1;
      const thumbnail=document.createElement('span');thumbnail.className='queue-thumbnail';const img=document.createElement('img');img.alt='';img.width=64;img.height=48;
      loadMediaThumbnail(img,item,root,()=>{thumbnail.replaceChildren(explorerIcon(item.Kind))});thumbnail.append(img);
      const labels=document.createElement('span');labels.className='queue-labels';const name=document.createElement('span');name.textContent=item.Name;const size=document.createElement('small');size.textContent=bytes(item.Size);labels.append(name,size);b.append(number,thumbnail,labels);
      if(kind==='video'){const duration=document.createElement('span');duration.className='queue-duration';size.append(document.createTextNode(' · '),duration);watchListingVideo(b,duration,item,root)}
      b.onkeydown=event=>{const keys={ArrowUp:position-1,ArrowDown:position+1,Home:0,End:items.length-1};if(!(event.key in keys))return;event.preventDefault();event.stopPropagation();const next=items[keys[event.key]];if(next)preview(next.index).then(()=>list.querySelector('[aria-current=true]')?.focus({preventScroll:true}))};list.append(b);
    }
  }
  for(const b of $('viewerQueueItems').children){const current=Number(b.dataset.index)===previewIndex;b.setAttribute('aria-current',String(current));b.tabIndex=current?0:-1}
  requestAnimationFrame(()=>{const list=$('viewerQueueItems'),current=list.querySelector('[aria-current=true]');if(!$('viewerQueue').hidden&&current){const box=current.getBoundingClientRect(),area=list.getBoundingClientRect();if(box.top<area.top||box.bottom>area.bottom)list.scrollTop+=box.top-area.top-(area.height-box.height)/2}});
}
function disposeVideoPlayback(){videoPlayback?.dispose();videoPlayback=null}
const mediaTime=value=>{value=Math.max(0,Math.floor(Number(value)||0));const h=Math.floor(value/3600),m=Math.floor(value/60)%60,s=value%60;return(h?h+':':'')+String(m).padStart(2,'0')+':'+String(s).padStart(2,'0')};
// Only visible/nearby videos are probed. Leaving a directory releases the queue
// and its requests; a bounded cache avoids probing the same files on return.
let listingMediaObserver=null,listingMediaQueue=[],listingMediaActive=new Map(),listingMediaGeneration=0;
const listingMediaCache=new Map();
function releaseListingVideo(card){
  const task=card.mediaInfoTask;if(!task)return;
  task.done=true;task.visible=false;task.queued=false;listingMediaObserver?.unobserve(card);listingMediaQueue=listingMediaQueue.filter(value=>value!==task);listingMediaActive.get(task)?.abort();card.mediaInfoTask=null;
}
function stopListingMedia(){
  listingMediaGeneration++;listingMediaObserver?.disconnect();listingMediaObserver=null;listingMediaQueue=[];
  for(const controller of listingMediaActive.values())controller.abort();listingMediaActive.clear();
}
function watchListingVideo(card,label,item,itemRoot){
  label.textContent='—';label.title='时长将在进入可视区域后读取';
  if(typeof IntersectionObserver!=='function'){label.title='当前浏览器不支持按需读取，可右键查看属性';return}
  if(!listingMediaObserver)listingMediaObserver=new IntersectionObserver(entries=>{
    for(const entry of entries){const task=entry.target.mediaInfoTask;if(!task)continue;
      task.visible=entry.isIntersecting;
      if(entry.isIntersecting){if(!task.queued&&!task.done&&!listingMediaActive.has(task)){task.queued=true;listingMediaQueue.push(task)}}
      else{task.queued=false;listingMediaQueue=listingMediaQueue.filter(value=>value!==task);listingMediaActive.get(task)?.abort()}
    }pumpListingMedia();
  },{rootMargin:'180px 0px'});
  const task={card,label,item,itemRoot,generation:listingMediaGeneration,key:JSON.stringify([itemRoot,item.Path,item.Size,item.Modified||'']),queued:false,done:false};card.mediaInfoTask=task;
  listingMediaObserver.observe(card);
}
function pumpListingMedia(){
  while(listingMediaActive.size<2&&listingMediaQueue.length){const task=listingMediaQueue.shift();task.queued=false;
    if(task.generation!==listingMediaGeneration||!task.card.isConnected||task.done)continue;
    if([...listingMediaActive.keys()].some(active=>active.key===task.key)){task.queued=true;listingMediaQueue.unshift(task);break}
    const apply=info=>{task.done=true;task.label.textContent=info?.Duration>0?mediaTime(info.Duration):'未知';task.label.title=info?[info.Duration>0?'时长 '+mediaTime(info.Duration):'未提供时长',info.Width&&info.Height?info.Width+' × '+info.Height:''].filter(Boolean).join(' · '):'未能读取时长，可右键查看属性或用本机播放器打开';listingMediaObserver?.unobserve(task.card)};
    const cached=listingMediaCache.get(task.key);if(cached&&cached.expires>Date.now()){listingMediaCache.delete(task.key);listingMediaCache.set(task.key,cached);apply(cached.info);continue}
    const controller=new AbortController();listingMediaActive.set(task,controller);task.label.textContent='…';
    api('media-info',{Path:task.item.Path},task.itemRoot,controller.signal).then(info=>{
      if(controller.signal.aborted||task.generation!==listingMediaGeneration)return;
      listingMediaCache.delete(task.key);listingMediaCache.set(task.key,{info,expires:Date.now()+300000});while(listingMediaCache.size>512)listingMediaCache.delete(listingMediaCache.keys().next().value);apply(info);
    }).catch(error=>{if(error.name!=='AbortError'&&!controller.signal.aborted&&task.generation===listingMediaGeneration){listingMediaCache.delete(task.key);listingMediaCache.set(task.key,{info:null,expires:Date.now()+30000});while(listingMediaCache.size>512)listingMediaCache.delete(listingMediaCache.keys().next().value);apply(null)}}).finally(()=>{listingMediaActive.delete(task);if(controller.signal.aborted&&task.visible&&!task.done&&task.generation===listingMediaGeneration&&!task.queued){task.queued=true;listingMediaQueue.push(task)}pumpListingMedia()});
  }
}
function installVideoPlayback(media,wrap,help,item,itemRoot,id){
  disposeVideoPlayback();const playback=createVideoPlayback(media,wrap,help,item,itemRoot,()=>id===previewRequest),toggle=$('viewerVideoOptions');
  const panel=document.createElement('div');panel.id='videoPreviewOptions';panel.className='video-preview-options';panel.hidden=true;panel.setAttribute('role','region');panel.setAttribute('aria-label','视频播放选项与全片进度');
  const heading=document.createElement('div');heading.className='video-options-heading';const title=document.createElement('strong');title.textContent='播放选项';const close=button('✕',()=>{show(false);toggle.focus({preventScroll:true})});close.setAttribute('aria-label','关闭播放选项');heading.append(title,close);
  const actions=wrap.querySelector('.compat-actions'),seek=wrap.querySelector('.compat-seek');panel.append(heading,seek,actions,help);wrap.append(panel);toggle.hidden=false;let disposed=false,wasCompatible=false,lastWarning='';
  function show(open){if(disposed)return;panel.hidden=!open;toggle.setAttribute('aria-expanded',String(open))}
  function sync(){if(disposed||id!==previewRequest)return;const compatible=wrap.dataset.playbackMode!=='native',message=help.textContent||'',warning=/无法|未成功|未能|不可用|阻止|限制|读取较慢/.test(message);toggle.textContent=compatible?(seek.hidden?'兼容播放':'全片进度'):'播放选项';toggle.title=(compatible?'兼容播放：完整视频进度与跳转位置。':'')+message;toggle.classList.toggle('is-compatible',compatible);toggle.classList.toggle('has-warning',warning);if(compatible&&!wasCompatible||warning&&message!==lastWarning)show(true);wasCompatible=compatible;lastWarning=warning?message:''}
  const observer=new MutationObserver(sync);observer.observe(wrap,{attributes:true,attributeFilter:['data-playback-mode']});observer.observe(help,{childList:true,subtree:true,characterData:true});toggle.onclick=()=>show(panel.hidden);show(false);sync();
  videoPlayback={...playback,dispose(){disposed=true;observer.disconnect();playback.dispose();toggle.onclick=null;toggle.setAttribute('aria-expanded','false');toggle.textContent='播放选项';toggle.classList.remove('is-compatible','has-warning');panel.remove()}};
  return videoPlayback;
}
function createVideoPlayback(media,wrap,help,item,itemRoot,isCurrent=()=>true){
  wrap.dataset.playbackMode='native';
  const controller=new AbortController();let info=null,infoPromise=null,mode='native',baseTime=0,disposed=false,seeking=false,seekTimer,slowTimer,manualOriginal=false,requested=false,wantsPlay=false,playRevision=0;
  const rememberPlay=()=>{wantsPlay=true},rememberPause=()=>{if(media.paused)wantsPlay=false};media.addEventListener('play',rememberPlay);media.addEventListener('pause',rememberPause);
  const active=()=>!disposed&&isCurrent();
  function requestPlay(){
    if(!active())return;
    wantsPlay=true;
    const revision=++playRevision,source=media.src;
    const current=()=>active()&&revision===playRevision&&media.src===source;
    try{
      const playing=media.play();
      playing?.catch?.(async error=>{
        if(!current()||!wantsPlay||['AbortError','NotSupportedError'].includes(error.name))return;
        if(error.name==='NotAllowedError'&&!media.muted){
          media.muted=true;
          try{await media.play();if(current())help.textContent+=' 浏览器限制有声自动播放，已静音播放；可在播放器中开启声音。';}
          catch{if(current())help.textContent+=' 浏览器阻止自动播放，请点击播放器的播放按钮。';}
        }else help.textContent+=' 自动播放未成功，请点击播放器的播放按钮。';
      });
    }catch{if(current())help.textContent+=' 请点击播放器的播放按钮。';}
  }
  function getInfo(){if(!infoPromise){infoPromise=api('media-info',{Path:item.Path},itemRoot,controller.signal).then(value=>{if(active())info=value;return value});infoPromise.catch(()=>{})}return infoPromise}
  const actions=document.createElement('div');actions.className='compat-actions';actions.setAttribute('role','group');actions.setAttribute('aria-label','播放兼容选项');
  const compatible=button('兼容播放',()=>startCompatible('auto',true,mode==='native'?media.currentTime:baseTime+media.currentTime));
  const original=button('原始播放',()=>{playRevision++;manualOriginal=true;mode='native';wrap.dataset.playbackMode=mode;baseTime=0;original.hidden=true;retry.hidden=true;seek.hidden=true;media.preload='metadata';media.src=endpoint('raw',{path:item.Path},itemRoot);media.load();help.textContent='原始播放；若无法播放，可切换兼容播放。';requestPlay();});original.hidden=true;
  const retry=button('强兼容播放',()=>startCompatible('convert',true,baseTime+media.currentTime));retry.hidden=true;
  actions.append(compatible,original,retry);wrap.append(actions);
  const seek=document.createElement('label');seek.className='compat-seek';seek.hidden=true;
  const caption=document.createElement('span');caption.textContent='全片进度';
  const slider=document.createElement('input');slider.type='range';slider.min='0';slider.step='1';slider.setAttribute('aria-label','完整视频进度');
  const position=document.createElement('output');seek.append(caption,slider,position);seek.title='完整视频的时间与跳转位置；播放器自带进度可能只显示当前缓冲片段';wrap.insertBefore(seek,help);
  function showTime(value){position.textContent=mediaTime(value)+' / '+mediaTime(info?.Duration)}
  slider.oninput=()=>{seeking=true;showTime(slider.value)};
  slider.onchange=()=>{seeking=false;clearTimeout(seekTimer);const target=Number(slider.value);seekTimer=setTimeout(()=>startCompatible(mode==='convert'?'convert':'auto',!media.paused,target),250)};
  media.ontimeupdate=()=>{if(mode==='native'||seeking||!info?.Duration)return;const time=Math.min(info.Duration,baseTime+(media.currentTime||0));slider.value=time;showTime(time)};
  async function startCompatible(nextMode,play,start=0,preserveIntent=false){
    if(!active())return;requested=true;
    try{if(!info)info=await getInfo();if(!active())return;play=play&&(!preserveIntent||wantsPlay);mode=nextMode;wrap.dataset.playbackMode=mode;baseTime=Math.max(0,Math.min(Number(start)||0,Math.max(0,(info.Duration||Infinity)-0.1)));clearTimeout(slowTimer);clearTimeout(seekTimer);
      original.hidden=false;retry.hidden=mode==='convert';compatible.textContent='重新兼容播放';seek.hidden=!info.Duration;slider.max=Math.max(0,(info.Duration||0)-0.1);slider.value=baseTime;showTime(baseTime);
      playRevision++;media.preload=play?'metadata':'none';media.src=endpoint('media-play',{path:item.Path,mode,start:baseTime.toFixed(3)},itemRoot);media.load();
      const copy=mode==='auto'&&info.VideoCodec==='h264'&&['yuv420p','yuvj420p'].includes(info.PixelFormat);
      help.textContent=copy?'兼容播放 · 复制画面，修复封装；原文件不变。':'兼容播放 · 按需转换，最高 720p；原文件不变。';
      if(play)requestPlay();
    }catch(e){if(active()&&e.name!=='AbortError')help.textContent='兼容播放不可用：'+e.message+'；可下载原视频查看。'}
  }
  media.onerror=()=>{if(!active())return;if(mode==='native'){help.textContent='当前设备无法直接播放，正在尝试兼容播放…';startCompatible('auto',wantsPlay,media.currentTime||0,true)}else if(mode==='auto'){help.textContent='封装修复未成功，可点击“强兼容播放”重试。';retry.hidden=false}else help.textContent='兼容播放仍未成功，请检查 FFmpeg 组件或下载原视频查看。'};
  // Most native players do not need FFprobe at all. Only read the small MP4
  // header to detect broken fragmented timelines; probe fully on compatibility use.
  if(/\.(mp4|m4v|mov|f4v)$/i.test(item.Name))api('media-info',{Path:item.Path,Lightweight:true},itemRoot,controller.signal).then(value=>{
    if(active()&&value.Fragmented&&mode==='native'&&!manualOriginal&&!requested)startCompatible('auto',wantsPlay&&!media.ended,Number(media.dataset.resumeAt)||media.currentTime||0,true);
  }).catch(()=>{});
  slowTimer=setTimeout(()=>{if(active()&&mode==='native'&&media.readyState===0)help.textContent='读取较慢，可点击“兼容播放”；原视频不会改变。'},8000);
  return {play:requestPlay,seek(value){if(!active()||!Number.isFinite(value))return;const duration=info?.Duration||media.duration||0,target=Math.max(0,duration?Math.min(value,duration-0.1):value);if(mode==='native'){try{media.currentTime=target}catch{}}else startCompatible(mode,!media.paused,target)},position:()=>({time:baseTime+(media.currentTime||0),duration:info?.Duration||media.duration||0}),dispose(){disposed=true;playRevision++;controller.abort();clearTimeout(seekTimer);clearTimeout(slowTimer);media.onerror=null;media.ontimeupdate=null;media.removeEventListener('play',rememberPlay);media.removeEventListener('pause',rememberPause)}};
}
function loadMediaThumbnail(img,item,itemRoot,onFailure){
  img.setAttribute('loading','lazy');img.decoding='async';
  img.fetchPriority='low';
  img.onerror=()=>{if(item.Kind==='image'&&!img.dataset.original){img.dataset.original='1';img.src=endpoint('raw',{path:item.Path},itemRoot);return}onFailure()};
  img.src=endpoint('thumb',{path:item.Path},itemRoot);
}
function clearViewerFilmstrip(){filmstripRows=null;filmstripKind='';filmstripItems=[];for(const img of $('viewerThumbnails').querySelectorAll('img')){img.onerror=null;img.removeAttribute('src')}$('viewerThumbnails').replaceChildren();$('viewerFilmstrip').hidden=true;$('toggleFilmstrip').hidden=true}
function centerViewerThumbnail(){
  const rail=$('viewerThumbnails'),current=rail.querySelector('[aria-current=true]');
  if(!current||!$('viewer').open||$('viewerFilmstrip').hidden)return;
  const box=current.getBoundingClientRect(),area=rail.getBoundingClientRect();
  if(area.width<=0||box.width<=0)return;
  const target=Math.max(0,Math.min(Math.max(0,rail.scrollWidth-rail.clientWidth),rail.scrollLeft+box.left-area.left-(area.width-box.width)/2));
  if(Math.abs(target-rail.scrollLeft)>.5)rail.scrollLeft=target;
}
function updateViewerFilmstrip(){
  const kind=rows[previewIndex]?.Kind,hasStrip=['image','video'].includes(kind);
  if(!hasStrip){clearViewerFilmstrip();return}
  if(filmstripRows!==rows||filmstripKind!==kind||filmstripRoot!==root){
    filmstripRows=rows;filmstripKind=kind;filmstripRoot=root;filmstripItems=rows.map((item,index)=>({item,index})).filter(({item})=>!item.Dir&&item.Kind===kind);
    const rail=$('viewerThumbnails');rail.replaceChildren();
    for(const [position,{item,index}] of filmstripItems.entries()){
      const thumb=button('',()=>preview(index),'filmstrip-thumb');thumb.dataset.index=index;thumb.title=item.Name;thumb.setAttribute('aria-label','查看第 '+(position+1)+' / '+filmstripItems.length+' 项：'+item.Name);
      const img=document.createElement('img');img.alt='';img.width=80;img.height=60;
      const number=document.createElement('span');number.className='filmstrip-number';number.textContent=position+1;
      loadMediaThumbnail(img,item,root,()=>{const fallback=document.createElement('span');fallback.className='filmstrip-placeholder';fallback.textContent=icons[kind]||'▧';img.replaceWith(fallback)});
      thumb.append(img,number);
      thumb.onkeydown=event=>{
        let next;if(event.key==='ArrowLeft')next=filmstripItems[position-1];if(event.key==='ArrowRight')next=filmstripItems[position+1];if(event.key==='Home')next=filmstripItems[0];if(event.key==='End')next=filmstripItems.at(-1);
        if(!['ArrowLeft','ArrowRight','Home','End'].includes(event.key))return;
        event.preventDefault();event.stopPropagation();if(next)preview(next.index).then(()=>$('viewerThumbnails').querySelector('[aria-current=true]')?.focus({preventScroll:true}));else if(event.key==='ArrowLeft'||event.key==='ArrowRight')advance(event.key==='ArrowRight'?1:-1);
      };
      rail.append(thumb);
    }
  }
  const position=filmstripItems.findIndex(entry=>entry.index===previewIndex),visible=filmstripItems.length>1;
  $('viewerPosition').textContent=(total>pageSize?'本页 ':'')+(position+1)+' / '+filmstripItems.length;
  $('toggleFilmstrip').hidden=!visible;$('toggleFilmstrip').textContent=filmstripCollapsed?'显示缩略图':'隐藏缩略图';$('toggleFilmstrip').setAttribute('aria-expanded',String(!filmstripCollapsed));
  $('viewerFilmstrip').hidden=!visible||filmstripCollapsed;
  for(const thumb of $('viewerThumbnails').children){const current=Number(thumb.dataset.index)===previewIndex;thumb.setAttribute('aria-current',String(current));thumb.tabIndex=current?0:-1}
  requestAnimationFrame(centerViewerThumbnail);
}
function disposeImageViewer(){viewerImageSourceCleanup?.();imageViewer?.dispose();imageViewer=null}
function installImageViewer(img){
  disposeImageViewer();
  const stage=document.createElement('div');stage.className='image-stage';stage.setAttribute('aria-label','图片缩放、拖动与旋转区域');stage.tabIndex=0;img.style.visibility='hidden';stage.append(img);$('viewerBody').append(stage);
  const neighbors=[];let neighborLimit=0;
  const pointers=new Map();let scale=1,fit=1,x=0,y=0,rotation=0,disposed=false,decoded=false,initialized=false,loadRevision=0,gesture=null,lastTap=0,lastTouch=0,smoothTimer=null,pendingSnapshot=null;
  const dimensions=()=>{const w=stage.clientWidth,h=stage.clientHeight,nw=img.naturalWidth,nh=img.naturalHeight;return decoded&&w>0&&h>0&&nw>0&&nh>0?{w,h,nw,nh,vw:rotation%180?nh:nw,vh:rotation%180?nw:nh}:null};
  function loading(){stopSmoothZoom(false);releaseNeighbors();loadRevision++;decoded=initialized=false;img.style.visibility='hidden';stage.setAttribute('aria-busy','true');$('zoomValue').textContent='…';for(const id of ['zoomIn','zoomOut','zoomFit','zoomActual','rotateLeft','rotateRight'])$(id).disabled=true;}
  function releaseNeighbor(entry){entry.node.onload=entry.node.onerror=entry.node.onclick=entry.node.onkeydown=null;entry.node.removeAttribute('src');entry.node.remove()}
  function releaseNeighbors(){for(const entry of neighbors)releaseNeighbor(entry);neighbors.length=0;neighborLimit=0}
  function updateNeighbors(){
    const continuous=imagePreviewMode==='continuous';stage.classList.toggle('continuous-images',continuous);
    if(!continuous){releaseNeighbors();return}if(!initialized)return;
    const size=dimensions();if(!size)return;const displayWidth=size.vw*scale;
    if(neighborLimit!==continuousImageCount){
      const desired=[];
      for(const direction of [-1,1]){let index=previewIndex;for(let distance=1;distance<=continuousImageCount;distance++){index+=direction;while(rows[index]&&(rows[index].Dir||rows[index].Kind!=='image'))index+=direction;if(!rows[index])break;desired.push({index,direction,distance})}}
      const wanted=new Set(desired.map(entry=>entry.index));for(let i=neighbors.length-1;i>=0;i--)if(!wanted.has(neighbors[i].index)){releaseNeighbor(neighbors[i]);neighbors.splice(i,1)}
      for(const {index,direction,distance} of desired){
        if(neighbors.some(entry=>entry.index===index))continue;const item=rows[index];
        const node=document.createElement('img'),entry={node,index,direction,distance,ready:false,failed:false,started:false,source:0};neighbors.push(entry);node.className='continuous-neighbor';node.dataset.neighborIndex=index;node.dataset.neighborDistance=distance;node.alt=item.Name;node.draggable=false;node.decoding='async';node.fetchPriority='low';node.style.visibility='hidden';node.tabIndex=0;node.setAttribute('role','button');const label=distance===1?(direction<0?'上一张':'下一张'):(direction<0?'前':'后')+distance+'张';node.setAttribute('aria-label','查看'+label+'：'+item.Name);node.title=node.getAttribute('aria-label');
        const actions=distance===1&&item.Size>0&&item.Size<=4*1024*1024?['raw','image-preview','thumb']:['image-preview','thumb'];
        const active=()=>!disposed&&imagePreviewMode==='continuous'&&neighbors.includes(entry);
        entry.load=()=>{if(!entry.started&&active()){entry.started=true;node.src=endpoint(actions[0],{path:item.Path},root)}};
        node.onload=()=>{const source=entry.source;const reveal=()=>{if(!active()||source!==entry.source||!node.naturalWidth||!node.naturalHeight)return;entry.ready=true;updateNeighbors()};if(typeof node.decode==='function')node.decode().then(reveal,reveal);else reveal()};
        node.onerror=()=>{if(!active())return;entry.ready=false;node.style.visibility='hidden';entry.source++;if(entry.source<actions.length)node.src=endpoint(actions[entry.source],{path:item.Path},root);else{entry.failed=true;node.tabIndex=-1;$('viewerHint').textContent=imagePreviewHint(Boolean(img.dataset.compatible))+' · 相邻图片暂不可用，可单独切换查看';updateNeighbors()}};
        node.onclick=event=>{if(event.detail===0&&active())preview(index)};node.onkeydown=event=>{if(event.key==='Enter'||event.key===' '){event.preventDefault();event.stopPropagation();if(active())preview(index)}};stage.append(node);
      }
      neighborLimit=continuousImageCount;
    }
    for(const direction of [-1,1]){
      let edge=displayWidth/2;
      for(const entry of neighbors.filter(entry=>entry.direction===direction).sort((a,b)=>a.distance-b.distance)){
        const boundary=stage.clientWidth/2+x+direction*edge,visible=direction<0?boundary>0:boundary<stage.clientWidth;
        // Only decode pictures that can reach the viewport; resolve the nearest first.
        if(!entry.started&&visible)entry.load();if(!entry.ready&&!entry.failed)break;
        const baseWidth=entry.failed?size.vw:size.vh*entry.node.naturalWidth/entry.node.naturalHeight;
        if(entry.ready){entry.node.style.width=baseWidth+'px';entry.node.style.height=size.vh+'px';entry.node.style.transform=`translate(-50%,-50%) translate(${x+direction*(edge+baseWidth*scale/2)}px,${y}px) scale(${scale})`;if(entry.node.style.visibility==='visible'||!stage.classList.contains('smooth-zoom'))entry.node.style.visibility='visible'}edge+=baseWidth*scale;
      }
    }
  }
  function apply(){if(disposed)return;const size=dimensions();if(!size)return;const {w,h,nw,nh,vw,vh}=size;const mx=Math.max(0,(vw*scale-w)/2),my=Math.max(0,(vh*scale-h)/2);x=Math.max(-mx,Math.min(mx,x));y=Math.max(-my,Math.min(my,y));img.style.width=nw+'px';img.style.height=nh+'px';img.style.transform=`translate(-50%,-50%) translate(${x}px,${y}px)${rotation?` rotate(${rotation}deg)`:""} scale(${scale})`;$('zoomValue').textContent=Math.round(scale*100)+'%';stage.classList.toggle('zoomed',scale>fit*1.01);$('zoomOut').disabled=scale<=Math.min(fit,0.1);$('zoomIn').disabled=scale>=8;$('zoomFit').disabled=$('zoomActual').disabled=$('rotateLeft').disabled=$('rotateRight').disabled=false;stage.dataset.rotation=String(rotation);initialized=true;stage.setAttribute('aria-busy','false');img.style.visibility='visible';updateNeighbors()}

  function stopSmoothZoom(freeze){
    if(!stage.classList.contains('smooth-zoom'))return;
    if(freeze&&initialized&&typeof window.DOMMatrixReadOnly==='function')try{const transform=getComputedStyle(img).transform;if(transform&&transform!=='none'){const matrix=new window.DOMMatrixReadOnly(transform),actual=Math.hypot(matrix.a,matrix.b);if(Number.isFinite(actual)&&actual>0){scale=actual;x=matrix.e+img.naturalWidth/2;y=matrix.f+img.naturalHeight/2}}}catch{}
    clearTimeout(smoothTimer);smoothTimer=null;stage.classList.remove('smooth-zoom');if(freeze&&!disposed)apply();
  }
  function startSmoothZoom(){
    if(window.matchMedia?.('(prefers-reduced-motion: reduce)')?.matches){stopSmoothZoom(false);return}
    clearTimeout(smoothTimer);stage.classList.add('smooth-zoom');smoothTimer=setTimeout(()=>{smoothTimer=null;stage.classList.remove('smooth-zoom');if(!disposed)apply()},180);
  }
  function reset(smooth=false){if(disposed)return;const size=dimensions();if(!size)return;if(smooth)startSmoothZoom();else stopSmoothZoom(false);const {w,h,vw,vh}=size;fit=Math.min(1,w/vw,h/vh);scale=fit;x=y=0;apply()}
  function rotate(direction){if(disposed||!initialized)return;stopSmoothZoom(false);rotation=(rotation+direction*90+360)%360;pointers.clear();gesture=null;lastTap=0;reset()}
  function zoom(value,cx,cy,smooth=true){if(!initialized||disposed)return;if(smooth)startSmoothZoom();else stopSmoothZoom(false);const box=stage.getBoundingClientRect();const ax=(cx??box.left+box.width/2)-box.left-box.width/2,ay=(cy??box.top+box.height/2)-box.top-box.height/2;const next=Math.max(Math.min(fit,0.1),Math.min(8,value));x=ax-(ax-x)*next/scale;y=ay-(ay-y)*next/scale;scale=next;apply()}
  function toggle(cx,cy){if(scale>fit*1.01)reset(true);else zoom(Math.max(1,fit*2),cx,cy)}

  stage.addEventListener('wheel',e=>{e.preventDefault();const unit=e.deltaMode===1?16:e.deltaMode===2?stage.clientHeight:1;zoom(scale*Math.exp(-Math.max(-120,Math.min(120,e.deltaY*unit))*0.0025),e.clientX,e.clientY)},{passive:false});
  stage.ondblclick=e=>{if(Date.now()-lastTouch<500)return;e.preventDefault();toggle(e.clientX,e.clientY)};
  stage.addEventListener('pointerdown',e=>{if(e.button!==0)return;stopSmoothZoom(true);stage.setPointerCapture?.(e.pointerId);pointers.set(e.pointerId,{x:e.clientX,y:e.clientY});if(pointers.size===1)gesture={startX:e.clientX,startY:e.clientY,x,y,moved:false,pinch:false,neighbor:Number(e.target.closest?.('[data-neighbor-index]')?.dataset.neighborIndex??-1)};if(pointers.size===2){const [a,b]=[...pointers.values()];gesture={distance:Math.hypot(a.x-b.x,a.y-b.y),scale,pinch:true,moved:true}}});
  stage.addEventListener('pointermove',e=>{if(!pointers.has(e.pointerId)||!gesture)return;pointers.set(e.pointerId,{x:e.clientX,y:e.clientY});if(pointers.size>=2){const [a,b]=[...pointers.values()];if(gesture.distance>0)zoom(gesture.scale*Math.hypot(a.x-b.x,a.y-b.y)/gesture.distance,(a.x+b.x)/2,(a.y+b.y)/2,false);return}if(gesture.pinch)return;const dx=e.clientX-gesture.startX,dy=e.clientY-gesture.startY;if(Math.hypot(dx,dy)>8)gesture.moved=true;if(scale>fit*1.01){x=gesture.x+dx;y=gesture.y+dy;apply()}});
  stage.addEventListener('pointerup',e=>{if(!pointers.has(e.pointerId))return;const g=gesture;pointers.delete(e.pointerId);if(!pointers.size&&g&&!g.pinch){const dx=e.clientX-g.startX,dy=e.clientY-g.startY;if(scale<=fit*1.01&&Math.abs(dx)>60&&Math.abs(dy)<80)advance(dx<0?1:-1);else if(!g.moved&&g.neighbor>=0&&imagePreviewMode==='continuous'){preview(g.neighbor)}else if(e.pointerType==='touch'&&!g.moved){lastTouch=Date.now();if(lastTouch-lastTap<330){toggle(e.clientX,e.clientY);lastTap=0}else lastTap=lastTouch}}if(!pointers.size)gesture=null});
  stage.addEventListener('pointercancel',e=>{pointers.delete(e.pointerId);if(!pointers.size)gesture=null});
  img.onload=()=>{const revision=++loadRevision;const reveal=()=>{if(disposed||revision!==loadRevision)return;decoded=true;reset();if(pendingSnapshot){const saved=pendingSnapshot;pendingSnapshot=null;imageViewer?.restore?.(saved)}$('viewerHint').textContent=imagePreviewHint(Boolean(img.dataset.compatible))};if(typeof img.decode==='function')img.decode().then(reveal,reveal);else reveal()};
  $('zoomIn').onclick=()=>zoom(scale*1.25);$('zoomOut').onclick=()=>zoom(scale/1.25);$('zoomFit').onclick=()=>reset(true);$('zoomActual').onclick=()=>zoom(1);$('rotateLeft').onclick=()=>rotate(-1);$('rotateRight').onclick=()=>rotate(1);
  const resize=()=>{stopSmoothZoom(false);if(!initialized||scale<=fit*1.01)reset();else apply();centerViewerThumbnail()};let observer;if(typeof ResizeObserver!=='undefined'){observer=new ResizeObserver(resize);observer.observe(stage)}else addEventListener('resize',resize);
  imageViewer={loading,snapshot(){return initialized&&!disposed?{zoom:scale,x,y,rotation}:null},restore(saved){if(disposed||!saved)return;if(!initialized){pendingSnapshot=saved;return}rotation=[0,90,180,270].includes(saved.rotation)?saved.rotation:0;reset();if(Number.isFinite(saved.zoom))scale=Math.max(Math.min(fit,0.1),Math.min(8,saved.zoom));x=Number.isFinite(saved.x)?saved.x:0;y=Number.isFinite(saved.y)?saved.y:0;apply()},modeChanged(){if(disposed)return;stopSmoothZoom(true);if(imagePreviewMode!=='continuous')releaseNeighbors();apply()},dispose(){disposed=true;stopSmoothZoom(false);releaseNeighbors();loadRevision++;observer?.disconnect();removeEventListener('resize',resize);img.onload=img.onerror=null;img.removeAttribute('src')},key(e){if(!initialized)return false;if(!e.ctrlKey&&!e.metaKey&&!e.altKey&&e.key.toLowerCase()==='r'){rotate(e.shiftKey?-1:1);return true}if(['+','='].includes(e.key)){zoom(scale*1.25);return true}if(e.key==='-'){zoom(scale/1.25);return true}if(e.key==='0'){reset(true);return true}if(scale>fit*1.01&&e.key.startsWith('Arrow')){stopSmoothZoom(true);if(e.key==='ArrowLeft')x+=60;if(e.key==='ArrowRight')x-=60;if(e.key==='ArrowUp')y+=60;if(e.key==='ArrowDown')y-=60;apply();return true}return false}};
  loading();if(img.complete&&img.naturalWidth>0)img.onload();
}
function handleImageKey(e){if(imageViewer?.key(e)){e.preventDefault();return true}return false}
document.addEventListener('keydown',event=>{
 if(event.defaultPrevented||event.ctrlKey||event.metaKey||event.altKey||!$('viewer')?.open||rows[previewIndex]?.Kind!=='video')return;
 const focus=document.activeElement;if(focus?.matches('input,textarea,select')||focus?.isContentEditable||focus?.closest('.video-preview-options')||focus?.tagName==='VIDEO')return;
 const media=$('viewerBody').querySelector('video');if(!media||!videoPlayback)return;const key=event.key.toLowerCase();
 if(key===' '){if(focus?.matches('button,a,[role=button]'))return;event.preventDefault();if(media.paused)videoPlayback.play();else media.pause()}
 else if(key==='arrowleft'||key==='arrowright'){event.preventDefault();videoPlayback.seek(videoPlayback.position().time+(key==='arrowright'?1:-1)*(event.shiftKey?1:5))}
 else if(key==='arrowup'||key==='arrowdown'){event.preventDefault();media.volume=Math.max(0,Math.min(1,media.volume+(key==='arrowup'?0.05:-0.05)))}
 else if(key==='m'){event.preventDefault();media.muted=!media.muted}
 else if(key==='f'){event.preventDefault();$('mediaFullscreen').click()}
});

function configureImagePreview(item){
  const image=item?.Kind==='image',video=item?.Kind==='video',media=image||video,dock=$('viewerImageActions'),footer=$('viewer').querySelector('.viewer-footer'),tools=$('viewerTools'),hint=$('viewerHint');
  $('toggleFilmstrip').title=image?'显示或隐藏缩略图；电脑端将鼠标移到画面底部可显示':'显示或隐藏画面下方的缩略图栏';
  $('viewerImageHelp').open=false;$('viewerImageHelp').hidden=video;$('viewerVideoOptions').hidden=!video;$('continuousCountControl').hidden=imagePreviewMode!=='continuous';$('viewer').classList.toggle('continuous-preview',image&&imagePreviewMode==='continuous');
  const strip=$('viewerFilmstrip'),imageHint=$('viewerImageHint'),stripParent=media?$('viewerBody').parentElement:$('viewerSurface');
  if(media){if(tools.parentElement!==dock)dock.prepend(tools);if(hint.parentElement!==imageHint)imageHint.append(hint)}
  else{if(hint.parentElement!==footer)footer.prepend(hint);if(tools.parentElement!==footer)footer.append(tools)}
  if(strip.parentElement!==stripParent)stripParent.append(strip);
  if(dock.hidden!==!media)dock.hidden=!media;if(footer.hidden!==media)footer.hidden=media;
}
function sameBrowserPath(a,b){const normalize=p=>p.replace(/[\\/]+$/,'')||'/';return separator()==='\\'?normalize(a).toLowerCase()===normalize(b).toLowerCase():normalize(a)===normalize(b)}
let compactViewerSlots=null,compactViewerInstalled=false;
function configureCompactViewer(){
  const menu=$('viewerMore'),panel=$('viewerMorePanel');if(!menu||!panel)return;
  if(!compactViewerSlots){compactViewerSlots=['viewerInfo','viewerTextActions','viewerQueueToggle','viewerNative','viewerDownload','viewerVideoOptions','viewerImageActions'].map(id=>{const node=$(id),slot=document.createComment('preview-action');node.before(slot);return{node,slot}})}
  const compact=window.innerWidth<=760||Boolean(window.matchMedia?.('(max-height:500px) and (pointer:coarse)').matches);
  // iOS Chrome's normal browser view already reserves the top safe area.
  // Keep the inset for Safari, standalone launches and actual fullscreen.
  const immersive=Boolean(document.fullscreenElement||document.webkitFullscreenElement||navigator.standalone||window.matchMedia?.('(display-mode: standalone), (display-mode: fullscreen)').matches);
  $('viewer').classList.toggle('ios-chrome-browser',/\bCriOS\//.test(navigator.userAgent)&&!immersive);
  const files=$('viewerMoreFiles');
  for(const {node,slot} of compactViewerSlots){if(compact){const target=['viewerQueueToggle','viewerNative','viewerDownload','viewerVideoOptions'].includes(node.id)?files:panel;if(node.parentElement!==target)target.append(node)}else if(node.parentNode!==slot.parentNode)slot.after(node)}
  if(compact)panel.insertBefore(files,$('viewerTextActions'));
  $('viewerMoreName').textContent=$('viewerTitle').textContent;
  if(!compact)menu.open=false;
  updateViewerHeaderGeometry();
}
function updateViewerHeaderGeometry(){
  if(!$('viewer').open)return;
  const height=Math.ceil($('viewer').querySelector('.viewer-header').getBoundingClientRect().height),surface=$('viewerSurface');
  if(height>0&&surface.style.getPropertyValue('--viewer-header-height')!==height+'px')surface.style.setProperty('--viewer-header-height',height+'px');
}
function initializeMediaTools(){
  configureCompactViewer();
  if(!compactViewerInstalled){
  compactViewerInstalled=true;
  window.addEventListener('resize',configureCompactViewer);
  document.addEventListener('fullscreenchange',configureCompactViewer);
  document.addEventListener('webkitfullscreenchange',configureCompactViewer);
  window.matchMedia?.('(display-mode: standalone), (display-mode: fullscreen)').addEventListener?.('change',configureCompactViewer);
  const more=$('viewerMore');
  document.addEventListener('pointerdown',event=>{if(more.open&&!more.contains(event.target))more.open=false});
  more.addEventListener('keydown',event=>{if(event.key==='Escape'&&more.open){event.preventDefault();event.stopPropagation();more.open=false;more.querySelector('summary').focus({preventScroll:true})}});
  more.addEventListener('click',event=>{if(['viewerQueueToggle','viewerDownload','viewerNative','viewerVideoOptions','mediaFullscreen'].includes(event.target.closest('button')?.id))more.open=false});
  $('viewer').addEventListener('close',()=>{more.open=false});
  }
  // One persistent observer handles wrapped tools, fullscreen and UI scaling.
  // Only overlay panel offsets change; the image/video viewport stays stable.
  if(!viewerHeaderTracking){
    viewerHeaderTracking=true;
    if(typeof ResizeObserver!=='undefined')new ResizeObserver(updateViewerHeaderGeometry).observe($('viewer').querySelector('.viewer-header'));
    window.addEventListener('resize',updateViewerHeaderGeometry);
  }
  try{const saved=localStorage.getItem('hamster.files.video-speed.v1');if([...$('videoSpeed').options].some(option=>option.value===saved))$('videoSpeed').value=saved}catch{}
  document.addEventListener('pointerdown',event=>{const panel=$('videoPreviewOptions');if(panel&&!panel.hidden&&!panel.contains(event.target)&&!$('viewerVideoOptions').contains(event.target)){panel.hidden=true;$('viewerVideoOptions').setAttribute('aria-expanded','false')}});
  document.addEventListener('keydown',event=>{const panel=$('videoPreviewOptions');if(event.key==='Escape'&&panel&&!panel.hidden&&event.target.closest?.('dialog')===$('viewer')){event.preventDefault();event.stopPropagation();panel.hidden=true;$('viewerVideoOptions').setAttribute('aria-expanded','false');$('viewerVideoOptions').focus({preventScroll:true})}},true);

  try{imagePreviewMode=localStorage.getItem('hamster.files.image-preview-mode.v1')==='continuous'?'continuous':'single'}catch{}
  try{const saved=Number(localStorage.getItem('hamster.files.image-preview-count.v1'));if(Number.isInteger(saved)&&saved>=1&&saved<=10)continuousImageCount=saved}catch{}
  $('continuousImageCount').value=continuousImageCount;$('continuousCountControl').hidden=imagePreviewMode!=='continuous';
  const changeContinuousCount=normalize=>{const raw=Number($('continuousImageCount').value);if(!Number.isInteger(raw)||raw<1||raw>10){if(normalize)$('continuousImageCount').value=continuousImageCount;return}if(raw===continuousImageCount)return;continuousImageCount=raw;try{localStorage.setItem('hamster.files.image-preview-count.v1',String(raw))}catch{}imageViewer?.modeChanged()};$('continuousImageCount').oninput=()=>changeContinuousCount(false);$('continuousImageCount').onchange=()=>changeContinuousCount(true);
  $('imagePreviewMode').value=imagePreviewMode;$('imagePreviewMode').onchange=()=>{imagePreviewMode=$('imagePreviewMode').value==='continuous'?'continuous':'single';try{localStorage.setItem('hamster.files.image-preview-mode.v1',imagePreviewMode)}catch{}$('continuousCountControl').hidden=imagePreviewMode!=='continuous';if(imagePreviewMode==='continuous')stopViewerImagePrefetch();$('viewer').classList.toggle('continuous-preview',rows[previewIndex]?.Kind==='image'&&imagePreviewMode==='continuous');$('viewerHint').textContent=imagePreviewHint(Boolean($('viewerBody').querySelector('.image-stage>img:not(.continuous-neighbor)')?.dataset.compatible));imageViewer?.modeChanged()};
  initializeNativePlayer();
  updateHoverPreviewControl();$('hoverPreview').onclick=async()=>{stopVideoHover();const previous=hoverPreviewEnabled;hoverPreviewEnabled=!previous;updateHoverPreviewControl();cacheExplorerView();const saved=await savePreferences({hoverPreview:hoverPreviewEnabled});if(!saved){hoverPreviewEnabled=previous;updateHoverPreviewControl();cacheExplorerView()}};
  if(!hoverLifecycleInstalled){hoverLifecycleInstalled=true;window.addEventListener('scroll',stopVideoHover,{passive:true,capture:true});window.addEventListener('blur',stopVideoHover);window.addEventListener('pagehide',()=>{stopVideoHover();clearViewerImageCache()});document.addEventListener('visibilitychange',()=>{if(document.hidden){stopVideoHover();stopViewerImagePrefetch()}});document.addEventListener('pointerdown',event=>{if(event.pointerType==='touch')lastHoverTouch=Date.now();stopVideoHover()},{capture:true});document.addEventListener('touchstart',()=>{lastHoverTouch=Date.now();stopVideoHover()},{passive:true,capture:true})}
  $('viewerQueueToggle').onclick=()=>{viewerQueueOpen=!viewerQueueOpen;updateViewerQueue()};$('viewerQueueClose').onclick=()=>{viewerQueueOpen=false;updateViewerQueue();$('viewerQueueToggle').focus()};
  $('toggleFilmstrip').onclick=()=>{filmstripCollapsed=!filmstripCollapsed;updateViewerFilmstrip()};
  if(!viewerImageHelpInstalled){viewerImageHelpInstalled=true;const imageHelp=$('viewerImageHelp');
    document.addEventListener('pointerdown',event=>{if(imageHelp.open&&!imageHelp.contains(event.target))imageHelp.open=false});
    imageHelp.addEventListener('keydown',event=>{if(event.key==='Escape'&&imageHelp.open){event.preventDefault();event.stopPropagation();imageHelp.open=false;imageHelp.querySelector('summary').focus()}});
  }
  $('viewerThumbnails').onwheel=event=>{const rail=$('viewerThumbnails');if(event.ctrlKey||event.metaKey||Math.abs(event.deltaX)>=Math.abs(event.deltaY))return;const factor=event.deltaMode===1?32:event.deltaMode===2?rail.clientWidth:1;const next=Math.max(0,Math.min(rail.scrollWidth-rail.clientWidth,rail.scrollLeft+event.deltaY*factor));if(next!==rail.scrollLeft){event.preventDefault();rail.scrollLeft=next}};
  $('mediaFullscreen').onclick=async()=>{try{if(document.fullscreenElement)await document.exitFullscreen();else if($('viewerSurface').requestFullscreen)await $('viewerSurface').requestFullscreen();else{const video=$('viewerBody').querySelector('video');if(video?.webkitEnterFullscreen)video.webkitEnterFullscreen();else $('viewerHint').textContent='当前浏览器不支持全屏，可横屏查看'}}catch{$('viewerHint').textContent='当前设备未允许全屏'}};

}
