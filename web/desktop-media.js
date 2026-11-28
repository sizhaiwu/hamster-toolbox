(()=>{
 'use strict';const bridge=window.hamsterDesktop;if(!bridge?.mediaSettings)return;
 const params=new URLSearchParams(location.search),standalone=params.get('desktop-media')==='1';
 let preferences={remember:false,focusAudio:false},revision=0,controls={},lastImageSave=0,activeBookmark=null;
 const temporarilyMuted=new WeakMap();
 const node=(tag,text)=>{const n=document.createElement(tag);if(text)n.textContent=text;return n};
 // Hide the reused Explorer shell before any asynchronous startup work. The
 // desktop host reveals this window only after the standalone view is painted.
 let loading=null,startupTimer,startupFailed=false;
 function startupError(error){if(!standalone)return;startupFailed=true;clearTimeout(startupTimer);loading?.remove();loading=null;if(document.querySelector('.desktop-media-error'))return;const panel=node('div');panel.className='desktop-media-error';panel.setAttribute('role','alert');panel.append(node('p','无法打开预览：'+error.message));const close=node('button','关闭窗口');close.onclick=()=>window.close();panel.append(close);document.body.append(panel)}
 if(standalone){
  document.body.classList.add('desktop-media-window');document.title='仓鼠预览';
  loading=node('div','正在打开预览…');loading.className='desktop-media-loading';loading.setAttribute('role','status');document.body.append(loading);
  requestAnimationFrame(()=>requestAnimationFrame(()=>bridge.mediaReady?.().catch(()=>{})));
  startupTimer=setTimeout(()=>startupError(Error('加载超时，请关闭窗口后重试')),30000);
  window.addEventListener('hamster-explorer-error',event=>startupError(event.detail),{once:true});
 }
 function sync(value){preferences=value;for(const [key,input]of Object.entries(controls)){if(input.type==='checkbox')input.checked=Boolean(value[key]);else input.value=value[key]}}
 const ready=bridge.mediaSettings().then(value=>{sync(value);return value}).catch(()=>preferences);
 function historyKey(item){return{path:item.Path,size:item.Size||0,modified:String(item.Modified||'')}}
 function imageState(){try{return typeof imageViewer!=='undefined'?imageViewer?.snapshot?.():null}catch{return null}}
 function saveImage(){if(!preferences.remember||typeof rows==='undefined')return;const item=rows[previewIndex];if(item?.Kind!=='image')return;const record=imageState();if(record)bridge.setMediaHistory({...historyKey(item),record}).catch(()=>{})}
 function applyAudioFocus(active){
  for(const media of document.querySelectorAll('#viewer video,#floatingLayer video,.floating-window video')){
   if(!preferences.focusAudio||active){if(temporarilyMuted.has(media)){media.muted=temporarilyMuted.get(media);temporarilyMuted.delete(media)}}else if(!media.muted&&!temporarilyMuted.has(media)){temporarilyMuted.set(media,false);media.muted=true}
  }
 }
 function beforeSwitch(){saveImage();activeBookmark?.dispose();activeBookmark=null}
 window.toolboxDesktopMedia={standalone,async openWindow(item){await ready;if(!canUseNativePrograms()||!['image','video'].includes(item.Kind))return false;try{await bridge.openMedia({path:item.Path,kind:item.Kind,size:item.Size,modified:String(item.Modified||''),offset,sort:$('sort').value,order:$('order').value});return true}catch(error){say('仓鼠预览窗口未能打开：'+error.message,true);return false}},beforeSwitch,
  installBookmark(media,playback,item,itemRoot,id){
   if(!preferences.remember||!canUseNativePrograms())return false;let alive=true,pending=null,last=0;const key=historyKey(item);
   bridge.getMediaHistory(key).then(saved=>{if(!alive||id!==previewRequest||!saved)return;pending=saved;if(typeof saved.volume==='number')media.volume=saved.volume;if(saved.speed){media.playbackRate=media.defaultPlaybackRate=saved.speed;if([...$('videoSpeed').options].some(o=>Number(o.value)===saved.speed))$('videoSpeed').value=String(saved.speed)};media.dataset.resumeAt=String(saved.time||0);restore()}).catch(()=>{});
   function restore(){if(!alive||!pending||id!==previewRequest||!media.duration||!media.src.includes('/raw?'))return;const at=pending.time||0;if(at>0&&at<media.duration-3)media.currentTime=at;pending=null;media.dataset.resumeAt='0'}
   media.addEventListener('loadedmetadata',restore);
   const save=force=>{if(!alive||id!==previewRequest||!force&&Date.now()-last<5000)return;last=Date.now();const p=playback.position();bridge.setMediaHistory({...key,record:{time:p.duration&&p.time>=p.duration-3?0:p.time||0,volume:media.volume,speed:media.playbackRate}}).catch(()=>{})};
   const onTime=()=>save(false),onPause=()=>save(true);media.addEventListener('timeupdate',onTime);media.addEventListener('pause',onPause);media.addEventListener('volumechange',onTime);
   activeBookmark={dispose(){save(true);alive=false;media.removeEventListener('loadedmetadata',restore);media.removeEventListener('timeupdate',onTime);media.removeEventListener('pause',onPause);media.removeEventListener('volumechange',onTime)}};return true;
  }
 };
 window.addEventListener('hamster-media-view',async event=>{
  const item=event.detail,epoch=++revision;if(!item)return;
  if(standalone){const url=new URL(location.href);url.searchParams.set('media-path',item.Path);url.searchParams.set('media-kind',item.Kind);url.searchParams.set('media-folder',path);url.searchParams.set('media-offset',String(offset));window.history.replaceState(null,'',url.href);document.title=item.Name+' · 仓鼠预览'}
  if(item.Kind==='image'&&preferences.remember){const saved=await bridge.getMediaHistory(historyKey(item)).catch(()=>null);if(saved&&epoch===revision&&rows[previewIndex]?.Path===item.Path)imageViewer?.restore?.(saved)}
  if(preferences.focusAudio)bridge.focusMedia().catch(()=>{});
 });
 document.addEventListener('pointerup',()=>{if(Date.now()-lastImageSave>2000){lastImageSave=Date.now();saveImage()}});document.addEventListener('wheel',()=>{clearTimeout(window.__desktopImageHistoryTimer);window.__desktopImageHistoryTimer=setTimeout(saveImage,500)},{passive:true});
 window.addEventListener('blur',saveImage);window.addEventListener('pagehide',beforeSwitch);
 bridge.onCommand(command=>{if(command.action==='media-settings'){sync(command.value);if(!preferences.focusAudio)applyAudioFocus(true)}if(command.action==='media-audio-focus')applyAudioFocus(command.active)});
 document.addEventListener('play',event=>{if(event.target instanceof HTMLMediaElement&&preferences.focusAudio)bridge.focusMedia().catch(()=>{})},true);
 window.addEventListener('focus',()=>{if(preferences.focusAudio)bridge.focusMedia().catch(()=>{})});
 async function mountSettings(){
  await ready;const host=document.getElementById('generalPlayback');if(!host)return;
  const section=node('section');section.className='desktop-options';section.id='desktopMediaSettings';section.append(node('strong','桌面图片与视频'));
  const status=node('small');status.setAttribute('role','status');
  const save=async(key,input)=>{input.disabled=true;try{sync(await bridge.saveMediaSettings({[key]:input.type==='checkbox'?input.checked:input.value}));status.textContent='已自动保存'}catch(error){sync(preferences);status.textContent='保存失败：'+error.message}finally{input.disabled=false}};
  for(const [key,text]of [['focusAudio','仅当前桌面预览窗口发声'],['remember','记住观看位置、音量与图片视图']]){const label=node('label',text),input=node('input');input.type='checkbox';controls[key]=input;input.onchange=()=>save(key,input);label.append(input);section.append(label)}
  const clear=node('button','清除桌面观看记录');clear.type='button';clear.onclick=async()=>{clear.disabled=true;try{await bridge.clearMediaHistory();status.textContent='已清除桌面观看记录'}catch(error){status.textContent=error.message}finally{clear.disabled=false}};
  section.append(node('small','图片和视频默认使用仓鼠独立预览窗口，可在上方分别更改。记录默认关闭，仅保存在本机。'),clear,status);host.append(section);sync(preferences);
 }
 mountSettings();
 window.addEventListener('hamster-explorer-ready',async()=>{
  await ready;if(!standalone||startupFailed||!canUseNativePrograms())return;
  const title=$('viewerTitle');document.title='独立预览 · 仓鼠工具箱';const pin=node('button','置顶');pin.type='button';pin.className='desktop-media-pin';pin.title='将此预览窗口保持在其他窗口上方';pin.setAttribute('aria-pressed','false');pin.onclick=async()=>{try{const value=await bridge.pinMedia(pin.getAttribute('aria-pressed')!=='true');pin.setAttribute('aria-pressed',String(value));pin.textContent=value?'取消置顶':'置顶'}catch(error){$('viewerHint').textContent=error.message}};title.closest('.viewer-header').insertBefore(pin,$('viewerClose'));
  $('viewer').addEventListener('close',()=>{saveImage();window.close()});
  try{
   const folder=params.get('media-folder'),target=params.get('media-path');if(!folder||!target)throw Error('未指定预览文件');
   if(['name','size','modified'].includes(params.get('media-sort')))$('sort').value=params.get('media-sort');if(['asc','desc'].includes(params.get('media-order')))$('order').value=params.get('media-order');
   await navigate(folder,{remember:false,page:Math.max(0,Number(params.get('media-offset'))||0)});let index=rows.findIndex(item=>sameBrowserPath(item.Path,target));
   if(index<0){$('search').value=target.split(/[\\/]/).pop();await navigate(folder,{remember:false});index=rows.findIndex(item=>sameBrowserPath(item.Path,target))}
   if(index<0||!['image','video'].includes(rows[index].Kind))throw Error('文件已移动、删除或不支持预览');await preview(index);$('viewerBody').tabIndex=-1;$('viewerBody').focus({preventScroll:true});
   clearTimeout(startupTimer);loading?.remove();loading=null;
  }catch(error){startupError(error)}
 },{once:true});
})();
