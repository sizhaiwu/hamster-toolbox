'use strict';

let nativePlayerState=null,nativePlayerTimer=null,nativePlayerInitialized=false;
function nativeDecoderLabel(value){return !value?'等待首帧解码':value==='no'?'CPU 软件解码':'显卡硬解 · '+value}
function renderNativePlayer(state){
  nativePlayerState=state;const sessions=state.Sessions||[];
  $('nativePlayer').textContent='原生播放器'+(sessions.length?' ('+sessions.length+')':'');
  $('nativePlayerStatus').textContent=state.Available?'mpv 已就绪 · 本机文件直接读取 · 硬件解码优先':'未找到 mpv，请选择程序或按下面指引下载。';
  if(document.activeElement!==$('nativePlayerProgram'))$('nativePlayerProgram').value=state.Program||'';
  if(!$('nativePlayerWindowSize').disabled&&document.activeElement!==$('nativePlayerWindowSize'))$('nativePlayerWindowSize').value=state.WindowSize||'compact';
  if(state.Error)$('nativePlayerError').textContent=state.Error;const list=$('nativePlayerSessions');list.replaceChildren();
  for(const session of sessions){
    const row=document.createElement('div');row.className='native-player-row';
    const name=document.createElement('strong');name.textContent=session.Name;
    const image=session.Kind==='image';
    const info=document.createElement('span');info.className='native-player-decoder';info.textContent=(image?'原生图片查看':nativeDecoderLabel(session.Decoder))+(session.Codec?' · '+session.Codec:'');
    const position=document.createElement('small');position.textContent=image?'独立窗口':mediaTime(session.Time)+' / '+mediaTime(session.Duration);
    const actions=document.createElement('div');actions.className='native-player-actions';
    const controls=[...(image?[]:[[session.Paused?'继续':'暂停',session.Paused?'resume':'pause'],[session.Muted?'开启声音':'静音',session.Muted?'unmute':'mute']]),[session.OnTop?'取消置顶':'置顶',session.OnTop?'unpin':'pin'],['关闭','close']];
    for(const [label,action] of controls){const b=button(label,async()=>{b.disabled=true;$('nativePlayerError').textContent='';try{renderNativePlayer(await api('native-player',{Action:action,ID:session.ID}));scheduleNativePlayer()}catch(e){$('nativePlayerError').textContent=e.message;b.disabled=false}});b.disabled=!session.Connected&&action!=='close';actions.append(b)}
    row.append(name,info,position,actions);list.append(row);
  }
  if(!sessions.length){const empty=document.createElement('p');empty.className='hint';empty.textContent='右键本机图片或视频，选择“原生悬浮查看/播放（mpv）”。窗口可移到浏览器外，独立调整大小和置顶。';list.append(empty)}
}
function scheduleNativePlayer(){clearTimeout(nativePlayerTimer);if(canUseNativePrograms()&&((nativePlayerState?.Sessions||[]).length||$('nativePlayerDialog').open))nativePlayerTimer=setTimeout(refreshNativePlayer,2000)}
async function refreshNativePlayer(){if(!canUseNativePrograms())return;try{renderNativePlayer(await api('native-player',{Action:'status'}))}catch(e){if($('nativePlayerDialog').open)$('nativePlayerError').textContent=e.message}finally{scheduleNativePlayer()}}
async function openNativeMedia(item){
  if(!canUseNativePrograms()||!item||item.Dir||!['image','video'].includes(item.Kind))return false;
  stopVideoHover();$('nativePlayerError').textContent='';say('正在启动 mpv 原生播放器…');
  try{renderNativePlayer(await api('native-player',{Action:'open',Path:item.Path,Version:item.Version}));scheduleNativePlayer();say(item.Kind==='image'?'已打开原生图片窗口 · Ctrl+滚轮缩放，Ctrl+左键拖动图片，左键拖动移动窗口。':'已启动原生悬浮播放 · 直接读文件，硬件解码优先；实际解码方式可在“原生播放器”中查看。');return true}
  catch(e){$('nativePlayerDialog').showModal();await refreshNativePlayer();$('nativePlayerError').textContent=e.message;return false}
}
function initializeNativePlayer(){
  if(nativePlayerInitialized)return;nativePlayerInitialized=true;$('nativePlayer').hidden=!canUseNativePrograms();if(!canUseNativePrograms())return;
  $('nativePlayer').onclick=()=>{$('nativePlayerDialog').showModal();refreshNativePlayer()};$('nativePlayerClose').onclick=()=>{$('nativePlayerDialog').close();scheduleNativePlayer()};
  const configure=async program=>{$('nativePlayerSave').disabled=$('nativePlayerDetect').disabled=true;$('nativePlayerError').textContent='';try{renderNativePlayer(await api('native-player',{Action:'configure',Program:program}));scheduleNativePlayer()}catch(e){$('nativePlayerError').textContent=e.message}finally{$('nativePlayerSave').disabled=$('nativePlayerDetect').disabled=false}};
  $('nativePlayerSave').onclick=()=>configure($('nativePlayerProgram').value);$('nativePlayerDetect').onclick=()=>configure('');
  $('nativePlayerWindowSize').onchange=async()=>{const input=$('nativePlayerWindowSize'),previous=nativePlayerState?.WindowSize||'compact';input.disabled=true;$('nativePlayerSizeStatus').textContent='正在保存…';try{const state=await api('native-player',{Action:'window-size',WindowSize:input.value});input.disabled=false;renderNativePlayer(state);$('nativePlayerSizeStatus').textContent='已保存，下次打开窗口生效'}catch(e){input.value=previous;$('nativePlayerSizeStatus').textContent='保存失败：'+e.message}finally{input.disabled=false}};
  window.addEventListener('pagehide',()=>clearTimeout(nativePlayerTimer));refreshNativePlayer();
}
