'use strict';

let imageViewer=null;
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
  disposeVideoPlayback();videoPlayback=createVideoPlayback(media,wrap,help,item,itemRoot,()=>id===previewRequest);
  return videoPlayback;
}
function createVideoPlayback(media,wrap,help,item,itemRoot,isCurrent=()=>true){
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
  const original=button('原始播放',()=>{playRevision++;manualOriginal=true;mode='native';baseTime=0;original.hidden=true;retry.hidden=true;seek.hidden=true;media.preload='metadata';media.src=endpoint('raw',{path:item.Path},itemRoot);media.load();help.textContent='原始播放；若无法播放，可切换兼容播放。';requestPlay();});original.hidden=true;
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
    try{if(!info)info=await getInfo();if(!active())return;play=play&&(!preserveIntent||wantsPlay);mode=nextMode;baseTime=Math.max(0,Math.min(Number(start)||0,Math.max(0,(info.Duration||Infinity)-0.1)));clearTimeout(slowTimer);clearTimeout(seekTimer);
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
  return {play:requestPlay,position:()=>({time:baseTime+(media.currentTime||0),duration:info?.Duration||media.duration||0}),dispose(){disposed=true;playRevision++;controller.abort();clearTimeout(seekTimer);clearTimeout(slowTimer);media.onerror=null;media.ontimeupdate=null;media.removeEventListener('play',rememberPlay);media.removeEventListener('pause',rememberPause)}};
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
  if(!current||$('viewerFilmstrip').hidden)return;
  const box=current.getBoundingClientRect(),area=rail.getBoundingClientRect();
  if(box.left<area.left||box.right>area.right)rail.scrollLeft+=box.left-area.left-(area.width-box.width)/2;
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
function disposeImageViewer(){imageViewer?.dispose();imageViewer=null}
function installImageViewer(img){
  disposeImageViewer();
  const stage=document.createElement('div');stage.className='image-stage';stage.setAttribute('aria-label','图片缩放与拖动区域');stage.tabIndex=0;stage.append(img);$('viewerBody').append(stage);
  const pointers=new Map();let scale=1,fit=1,x=0,y=0,disposed=false,gesture=null,lastTap=0,lastTouch=0;
  const dimensions=()=>({w:stage.clientWidth||640,h:stage.clientHeight||480,nw:img.naturalWidth||640,nh:img.naturalHeight||480});
  function apply(){if(disposed)return;const {w,h,nw,nh}=dimensions();const mx=Math.max(0,(nw*scale-w)/2),my=Math.max(0,(nh*scale-h)/2);x=Math.max(-mx,Math.min(mx,x));y=Math.max(-my,Math.min(my,y));img.style.width=nw+'px';img.style.height=nh+'px';img.style.transform=`translate(-50%,-50%) translate(${x}px,${y}px) scale(${scale})`;$('zoomValue').textContent=Math.round(scale*100)+'%';stage.classList.toggle('zoomed',scale>fit*1.01);$('zoomOut').disabled=scale<=Math.min(fit,0.1);$('zoomIn').disabled=scale>=8;}
  function reset(){if(disposed)return;const {w,h,nw,nh}=dimensions();fit=Math.min(1,w/nw,h/nh);scale=fit;x=y=0;apply()}
  function zoom(value,cx,cy){const box=stage.getBoundingClientRect();const ax=(cx??box.left+box.width/2)-box.left-box.width/2,ay=(cy??box.top+box.height/2)-box.top-box.height/2;const next=Math.max(Math.min(fit,0.1),Math.min(8,value));x=ax-(ax-x)*next/scale;y=ay-(ay-y)*next/scale;scale=next;apply()}
  function toggle(cx,cy){if(scale>fit*1.01)reset();else zoom(Math.max(1,fit*2),cx,cy)}
  stage.addEventListener('wheel',e=>{e.preventDefault();zoom(scale*Math.exp(-Math.max(-150,Math.min(150,e.deltaY))*0.005),e.clientX,e.clientY)},{passive:false});
  stage.ondblclick=e=>{if(Date.now()-lastTouch<500)return;e.preventDefault();toggle(e.clientX,e.clientY)};
  stage.addEventListener('pointerdown',e=>{if(e.button!==0)return;stage.setPointerCapture?.(e.pointerId);pointers.set(e.pointerId,{x:e.clientX,y:e.clientY});if(pointers.size===1)gesture={startX:e.clientX,startY:e.clientY,x,y,moved:false,pinch:false};if(pointers.size===2){const [a,b]=[...pointers.values()];gesture={distance:Math.hypot(a.x-b.x,a.y-b.y),scale,pinch:true,moved:true}}});
  stage.addEventListener('pointermove',e=>{if(!pointers.has(e.pointerId)||!gesture)return;pointers.set(e.pointerId,{x:e.clientX,y:e.clientY});if(pointers.size>=2){const [a,b]=[...pointers.values()];if(gesture.distance>0)zoom(gesture.scale*Math.hypot(a.x-b.x,a.y-b.y)/gesture.distance,(a.x+b.x)/2,(a.y+b.y)/2);return}if(gesture.pinch)return;const dx=e.clientX-gesture.startX,dy=e.clientY-gesture.startY;if(Math.hypot(dx,dy)>8)gesture.moved=true;if(scale>fit*1.01){x=gesture.x+dx;y=gesture.y+dy;apply()}});
  stage.addEventListener('pointerup',e=>{if(!pointers.has(e.pointerId))return;const g=gesture;pointers.delete(e.pointerId);if(!pointers.size&&g&&!g.pinch){const dx=e.clientX-g.startX,dy=e.clientY-g.startY;if(scale<=fit*1.01&&Math.abs(dx)>60&&Math.abs(dy)<80)advance(dx<0?1:-1);else if(e.pointerType==='touch'&&!g.moved){lastTouch=Date.now();if(lastTouch-lastTap<330){toggle(e.clientX,e.clientY);lastTap=0}else lastTap=lastTouch}}if(!pointers.size)gesture=null});
  stage.addEventListener('pointercancel',e=>{pointers.delete(e.pointerId);if(!pointers.size)gesture=null});
  img.onload=()=>{if(!disposed){reset();if(img.dataset.compatible)$('viewerHint').textContent='兼容预览 · 下载可取得原图；支持缩放和拖动'}};
  $('zoomIn').onclick=()=>zoom(scale*1.25);$('zoomOut').onclick=()=>zoom(scale/1.25);$('zoomFit').onclick=reset;$('zoomActual').onclick=()=>zoom(1);
  const resize=()=>{if(scale<=fit*1.01)reset();else apply();centerViewerThumbnail()};let observer;if(typeof ResizeObserver!=='undefined'){observer=new ResizeObserver(resize);observer.observe(stage)}else addEventListener('resize',resize);
  imageViewer={dispose(){disposed=true;observer?.disconnect();removeEventListener('resize',resize)},key(e){if(['+','='].includes(e.key)){zoom(scale*1.25);return true}if(e.key==='-'){zoom(scale/1.25);return true}if(e.key==='0'){reset();return true}if(scale>fit*1.01&&e.key.startsWith('Arrow')){if(e.key==='ArrowLeft')x+=60;if(e.key==='ArrowRight')x-=60;if(e.key==='ArrowUp')y+=60;if(e.key==='ArrowDown')y-=60;apply();return true}return false}};
  reset();
}
function handleImageKey(e){if(imageViewer?.key(e)){e.preventDefault();return true}return false}

function sameBrowserPath(a,b){const normalize=p=>p.replace(/[\\/]+$/,'')||'/';return separator()==='\\'?normalize(a).toLowerCase()===normalize(b).toLowerCase():normalize(a)===normalize(b)}
function initializeMediaTools(){
  initializeNativePlayer();
  updateHoverPreviewControl();$('hoverPreview').onclick=async()=>{stopVideoHover();const previous=hoverPreviewEnabled;hoverPreviewEnabled=!previous;updateHoverPreviewControl();cacheExplorerView();const saved=await savePreferences({hoverPreview:hoverPreviewEnabled});if(!saved){hoverPreviewEnabled=previous;updateHoverPreviewControl();cacheExplorerView()}};
  if(!hoverLifecycleInstalled){hoverLifecycleInstalled=true;window.addEventListener('scroll',stopVideoHover,{passive:true,capture:true});window.addEventListener('blur',stopVideoHover);window.addEventListener('pagehide',stopVideoHover);document.addEventListener('visibilitychange',()=>{if(document.hidden)stopVideoHover()});document.addEventListener('pointerdown',event=>{if(event.pointerType==='touch')lastHoverTouch=Date.now();stopVideoHover()},{capture:true});document.addEventListener('touchstart',()=>{lastHoverTouch=Date.now();stopVideoHover()},{passive:true,capture:true})}
  $('viewerQueueToggle').onclick=()=>{viewerQueueOpen=!viewerQueueOpen;updateViewerQueue()};$('viewerQueueClose').onclick=()=>{viewerQueueOpen=false;updateViewerQueue();$('viewerQueueToggle').focus()};
  $('toggleFilmstrip').onclick=()=>{filmstripCollapsed=!filmstripCollapsed;updateViewerFilmstrip()};
  $('viewerThumbnails').onwheel=event=>{const rail=$('viewerThumbnails');if(event.ctrlKey||event.metaKey||Math.abs(event.deltaX)>=Math.abs(event.deltaY))return;const factor=event.deltaMode===1?32:event.deltaMode===2?rail.clientWidth:1;const next=Math.max(0,Math.min(rail.scrollWidth-rail.clientWidth,rail.scrollLeft+event.deltaY*factor));if(next!==rail.scrollLeft){event.preventDefault();rail.scrollLeft=next}};
  $('mediaFullscreen').onclick=async()=>{try{if(document.fullscreenElement)await document.exitFullscreen();else if($('viewerSurface').requestFullscreen)await $('viewerSurface').requestFullscreen();else{const video=$('viewerBody').querySelector('video');if(video?.webkitEnterFullscreen)video.webkitEnterFullscreen();else $('viewerHint').textContent='当前浏览器不支持全屏，可横屏查看'}}catch{$('viewerHint').textContent='当前设备未允许全屏'}};

}
