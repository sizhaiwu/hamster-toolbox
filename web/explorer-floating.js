'use strict';

// Floating media owns its source/root and playback controller, independent of directory tabs.
const floatingWindows=new Map();
let floatingLayer=null,floatingDock=null,floatingSerial=0,floatingOrder=[];
function floatingViewport(){return {w:Math.max(240,innerWidth),h:Math.max(180,innerHeight)}}
function floatingRect(win,rect){
  const viewport=floatingViewport(),minWidth=Math.min(240,viewport.w-16),minHeight=Math.min(210,viewport.h-16);
  win.rect={w:Math.min(viewport.w-16,Math.max(minWidth,rect.w)),h:Math.min(viewport.h-16,Math.max(minHeight,rect.h)),x:rect.x,y:rect.y};
  win.rect.x=Math.max(8,Math.min(win.rect.x,viewport.w-win.rect.w-8));win.rect.y=Math.max(8,Math.min(win.rect.y,viewport.h-(win.minimized?42:win.rect.h)-8));
  Object.assign(win.node.style,{left:win.rect.x+'px',top:win.rect.y+'px',width:win.rect.w+'px',height:(win.minimized?42:win.rect.h)+'px'});
}
function focusFloating(win){
  floatingOrder=floatingOrder.filter(id=>id!==win.id);floatingOrder.push(win.id);
  const ordered=floatingOrder.map(id=>floatingWindows.get(id)).filter(Boolean);
  [...ordered.filter(item=>!item.pinned),...ordered.filter(item=>item.pinned)].forEach((item,index)=>{item.node.style.zIndex=index+1;item.node.classList.toggle('floating-active',item===win)});
}
function updateFloatingDock(){if(!floatingDock)return;floatingDock.hidden=floatingWindows.size===0;floatingDock.querySelector('.floating-count').textContent='悬浮窗口 '+floatingWindows.size}
function closeFloating(win){
  if(win.closed)return;win.closed=true;win.dispose?.();
  if(win.media){if(document.pictureInPictureElement===win.media)document.exitPictureInPicture?.().catch(()=>{});win.media.pause();win.media.removeAttribute('src');win.media.removeAttribute('poster');win.media.load()}
  if(win.image){win.image.onerror=null;win.image.removeAttribute('src')}
  win.node.remove();floatingWindows.delete(win.id);floatingOrder=floatingOrder.filter(id=>id!==win.id);updateFloatingDock();
}
function closeFloatingMedia(){for(const win of [...floatingWindows.values()])closeFloating(win)}
function arrangeFloating(){
  const viewport=floatingViewport(),top=Math.min(190,viewport.h*.24),gap=12;
  let x=12,y=top,rowHeight=0,overflow=false,overflowIndex=0;
  for(const win of floatingWindows.values()){
    const width=win.rect.w,height=win.minimized?42:win.rect.h;
    if(!overflow&&x>12&&x+width>viewport.w-12){x=12;y+=rowHeight+gap;rowHeight=0}
    if(y+height>viewport.h-60)overflow=true;
    const shift=(overflowIndex%10)*24;
    const position=overflow?{x:Math.min(12+shift,viewport.w-width-8),y:Math.min(top+shift,viewport.h-height-60)}:{x,y};
    floatingRect(win,{...win.rect,...position});
    if(overflow)overflowIndex++;else{x+=width+gap;rowHeight=Math.max(rowHeight,height)}
  }
}
function ensureFloatingLayer(){
  if(floatingLayer)return;
  floatingLayer=document.createElement('div');floatingLayer.className='floating-layer';floatingLayer.setAttribute('aria-label','悬浮媒体窗口');document.body.append(floatingLayer);
  floatingDock=document.createElement('div');floatingDock.className='floating-dock';floatingDock.setAttribute('role','group');floatingDock.setAttribute('aria-label','悬浮窗口管理');
  const count=document.createElement('span');count.className='floating-count';
  const arrange=button('整理',arrangeFloating);arrange.title='只整理位置，保留当前大小和收起状态';
  const pause=button('全部暂停',()=>{for(const win of floatingWindows.values())win.media?.pause()});pause.title='暂停所有悬浮视频，图片不受影响';
  const close=button('全部关闭',closeFloatingMedia);close.title='关闭所有悬浮窗口，不修改原文件';floatingDock.append(count,arrange,pause,close);document.body.append(floatingDock);
  window.addEventListener('resize',()=>{for(const win of floatingWindows.values())floatingRect(win,win.rect)});
  document.addEventListener('pointerdown',event=>{for(const options of floatingLayer.querySelectorAll('.floating-options[open]'))if(!options.contains(event.target))options.open=false},{capture:true});
  window.addEventListener('pagehide',closeFloatingMedia);
}
function installFloatingGesture(win,handle,resizing=false){
  let gesture=null;
  const finish=event=>{if(!gesture||event.pointerId!==gesture.id)return;gesture=null;win.node.classList.remove('floating-dragging');if(handle.hasPointerCapture?.(event.pointerId))handle.releasePointerCapture(event.pointerId)};
  handle.addEventListener('pointerdown',event=>{
    if(event.button!==0||event.target.closest('button,summary,input,select,details')&&event.target!==handle)return;
    event.preventDefault();focusFloating(win);gesture={id:event.pointerId,x:event.clientX,y:event.clientY,rect:{...win.rect}};handle.setPointerCapture?.(event.pointerId);win.node.classList.add('floating-dragging');
  });
  handle.addEventListener('pointermove',event=>{if(!gesture||event.pointerId!==gesture.id)return;const dx=event.clientX-gesture.x,dy=event.clientY-gesture.y,rect=gesture.rect;floatingRect(win,resizing?{...rect,w:rect.w+dx,h:rect.h+dy}:{...rect,x:rect.x+dx,y:rect.y+dy})});
  for(const type of ['pointerup','pointercancel','lostpointercapture'])handle.addEventListener(type,finish);
  handle.addEventListener('keydown',event=>{if(!['ArrowLeft','ArrowRight','ArrowUp','ArrowDown'].includes(event.key)||event.target!==handle)return;event.preventDefault();event.stopPropagation();const dx=event.key==='ArrowRight'?20:event.key==='ArrowLeft'?-20:0,dy=event.key==='ArrowDown'?20:event.key==='ArrowUp'?-20:0;floatingRect(win,resizing?{...win.rect,w:win.rect.w+dx,h:win.rect.h+dy}:{...win.rect,x:win.rect.x+dx,y:win.rect.y+dy})});
}
function installFloatingImage(win,body){
  const stage=document.createElement('div');stage.className='floating-image-stage';const img=document.createElement('img');win.image=img;img.alt=win.item.Name;img.draggable=false;img.src=endpoint('raw',{path:win.item.Path},win.root);stage.append(img);body.append(stage);
  let scale=1,x=0,y=0,drag=null,pinch=null;const pointers=new Map();
  stage.tabIndex=0;stage.title='滚轮或双指缩放，放大后拖动；双击或按 0 恢复适应窗口';stage.setAttribute('aria-label','悬浮图片：滚轮或双指缩放，双击恢复');
  const paint=()=>{const box=stage.getBoundingClientRect();x=Math.max(-box.width*(scale-1)/2,Math.min(x,box.width*(scale-1)/2));y=Math.max(-box.height*(scale-1)/2,Math.min(y,box.height*(scale-1)/2));img.style.transform=`translate(${x}px,${y}px) scale(${scale})`;stage.classList.toggle('zoomed',scale>1)};
  const zoom=amount=>{scale=Math.max(1,Math.min(6,amount));if(scale===1)x=y=0;paint()};
  stage.addEventListener('wheel',event=>{event.preventDefault();zoom(scale*(event.deltaY<0?1.15:1/1.15))},{passive:false});
  const distance=()=>{const [a,b]=[...pointers.values()];return Math.hypot(a.x-b.x,a.y-b.y)};
  stage.addEventListener('pointerdown',event=>{if(event.button!==0||scale<=1&&event.pointerType!=='touch')return;event.preventDefault();pointers.set(event.pointerId,{x:event.clientX,y:event.clientY});stage.setPointerCapture?.(event.pointerId);if(pointers.size===2){pinch={distance:Math.max(1,distance()),scale};drag=null}else if(pointers.size===1)drag={id:event.pointerId,x:event.clientX,y:event.clientY,px:x,py:y}});
  stage.addEventListener('pointermove',event=>{if(!pointers.has(event.pointerId))return;pointers.set(event.pointerId,{x:event.clientX,y:event.clientY});if(pinch&&pointers.size===2)zoom(pinch.scale*distance()/pinch.distance);else if(drag?.id===event.pointerId&&scale>1){x=drag.px+event.clientX-drag.x;y=drag.py+event.clientY-drag.y;paint()}});
  for(const type of ['pointerup','pointercancel','lostpointercapture'])stage.addEventListener(type,event=>{pointers.delete(event.pointerId);pinch=null;drag=null;if(pointers.size===1){const [id,p]=pointers.entries().next().value;drag={id,x:p.x,y:p.y,px:x,py:y}}});
  stage.addEventListener('dblclick',()=>zoom(1));stage.addEventListener('keydown',event=>{if(['+','=','-','0'].includes(event.key)){event.preventDefault();event.stopPropagation();zoom(event.key==='0'?1:event.key==='-'?scale/1.15:scale*1.15)}});
  img.onerror=()=>{if(win.closed)return;if(!img.dataset.compatible){img.dataset.compatible='1';img.src=endpoint('image-preview',{path:win.item.Path},win.root);return}img.remove();const message=document.createElement('p');message.className='floating-error';message.textContent='此图片无法预览，可下载原文件查看。';stage.append(message)};
}
function installFloatingVideo(win,body){
  const wrap=document.createElement('div');wrap.className='floating-video-wrap';const media=document.createElement('video');win.media=media;media.controls=true;media.muted=media.defaultMuted=true;media.setAttribute('playsinline','');media.preload='metadata';media.src=endpoint('raw',{path:win.item.Path},win.root);
  const help=document.createElement('p');help.className='floating-video-help';help.textContent='静音启动；每个窗口可单独调进度和声音。';wrap.append(media,help);body.append(wrap);
  win.dispose=createVideoPlayback(media,wrap,help,win.item,win.root,()=>!win.closed).dispose;
  // Native video controls handle sound, time and speed. Extra compatibility tools stay in the title menu.
  const options=document.createElement('details');options.className='floating-options';const summary=document.createElement('summary');summary.textContent='···';summary.title='播放选项；兼容播放时可在这里调整全片进度';summary.setAttribute('aria-label','悬浮视频播放选项');const panel=document.createElement('div');panel.className='floating-options-panel';options.append(summary,panel);win.header.insertBefore(options,win.header.children[1]);
  panel.append(wrap.querySelector('.compat-actions'),wrap.querySelector('.compat-seek'),help);
  options.addEventListener('keydown',event=>{if(event.key==='Escape'&&options.open){options.open=false;summary.focus();event.stopPropagation()}});
  if(document.pictureInPictureEnabled&&typeof media.requestPictureInPicture==='function'){
    const pip=button('系统画中画',async()=>{try{if(document.pictureInPictureElement===media)await document.exitPictureInPicture();else await media.requestPictureInPicture();options.open=false}catch{help.textContent='当前浏览器未允许系统画中画，可继续使用页面内悬浮窗口。'}});pip.className='floating-pip';pip.title='系统画中画，可显示在其他软件上方；数量受浏览器限制';panel.append(pip);
  }
  const playing=media.play();playing?.catch?.(()=>{if(!win.closed)help.textContent='点击播放器开始；可单独调节进度和声音。'});
}
function openFloatingMedia(item,itemRoot=root){
  if(!item||item.Dir||!['image','video'].includes(item.Kind))return;
  stopVideoHover();ensureFloatingLayer();const id='floating-media-'+(++floatingSerial),node=document.createElement('section'),win={id,node,item:{...item},root:itemRoot,pinned:false,minimized:false,closed:false};
  node.className='floating-window';node.dataset.kind=item.Kind;node.id=id;node.setAttribute('role','dialog');node.setAttribute('aria-modal','false');node.setAttribute('aria-labelledby',id+'-title');node.tabIndex=-1;
  const header=document.createElement('div');win.header=header;header.className='floating-header';header.tabIndex=0;header.setAttribute('aria-label','拖动悬浮窗口，方向键可移动');const title=document.createElement('strong');title.id=id+'-title';title.textContent=item.Name;title.title=item.Name;
  const pin=button('置顶',()=>{win.pinned=!win.pinned;pin.setAttribute('aria-pressed',String(win.pinned));pin.title=win.pinned?'取消页面内置顶':'置于其他页面悬浮窗口上方';focusFloating(win)});pin.title='置于其他页面悬浮窗口上方';pin.setAttribute('aria-pressed','false');
  const collapse=button('−',()=>{win.minimized=!win.minimized;node.classList.toggle('floating-minimized',win.minimized);collapse.title=win.minimized?'展开窗口':'收起窗口';collapse.textContent=win.minimized?'＋':'−';collapse.setAttribute('aria-expanded',String(!win.minimized));floatingRect(win,win.rect)});win.collapse=collapse;collapse.title='收起窗口';collapse.setAttribute('aria-label','收起或展开悬浮窗口');collapse.setAttribute('aria-expanded','true');
  const close=button('×',()=>closeFloating(win));close.className='floating-close';close.title='关闭悬浮窗口';close.setAttribute('aria-label','关闭悬浮窗口');header.append(title,pin,collapse,close);
  const body=document.createElement('div');body.className='floating-body';
  const resize=button('◢',()=>{});resize.className='floating-resize';resize.title='拖动调整大小，方向键可调整';resize.setAttribute('aria-label','调整悬浮窗口大小');node.append(header,body,resize);floatingLayer.append(node);floatingWindows.set(id,win);
  node.addEventListener('pointerdown',()=>focusFloating(win),{capture:true});node.addEventListener('focusin',()=>focusFloating(win));node.addEventListener('keydown',event=>{if(event.key==='Escape'){event.stopPropagation();closeFloating(win)}});
  installFloatingGesture(win,header);installFloatingGesture(win,resize,true);
  const viewport=floatingViewport(),step=((floatingWindows.size-1)%10)*30;
  floatingRect(win,{x:Math.max(16,Math.min(260,viewport.w*.2))+step,y:180+step,w:item.Kind==='video'?480:360,h:item.Kind==='video'?380:440});focusFloating(win);updateFloatingDock();
  if(item.Kind==='image')installFloatingImage(win,body);else installFloatingVideo(win,body);
  return win;
}
