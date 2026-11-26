'use strict';

let nativePlayerState=null,nativePlayerTimer=null,nativePlayerInitialized=false,nativePlayerRevision=0;
function nativeDecoderLabel(value){return !value?'等待首帧解码':value==='no'?'CPU 软件解码':'显卡硬解 · '+value}
async function nativePlayerAction(action,extra={}){ const revision=++nativePlayerRevision;$('nativePlayerError').textContent='';try{const state=await api('native-player',{Action:action,...extra});if(revision===nativePlayerRevision)renderNativePlayer(state);scheduleNativePlayer();return true}catch(e){if(revision===nativePlayerRevision)$('nativePlayerError').textContent=e.message;return false} }
function renderNativePlayer(state){
  nativePlayerState=state;const sessions=state.Sessions||[];
  $('nativePlayer').textContent='原生播放器'+(sessions.length?' ('+sessions.length+')':'');
  $('nativePlayerStatus').textContent=state.Available?'mpv 已就绪 · 本机文件直接读取 · 硬件解码优先':'未找到 mpv，请选择程序或按下面指引下载。';
  if(document.activeElement!==$('nativePlayerProgram'))$('nativePlayerProgram').value=state.Program||'';
  if(!$('nativePlayerWindowSize').disabled&&document.activeElement!==$('nativePlayerWindowSize'))$('nativePlayerWindowSize').value=state.WindowSize||'compact';
  if(document.activeElement!==$('nativePlayerRemember'))$('nativePlayerRemember').checked=state.RememberWindows!==false;
  for(const id of ['nativePlayerCascade','nativePlayerTile','nativePlayerPauseAll','nativePlayerMuteAll'])$(id).disabled=!sessions.length;
  if(state.Error)$('nativePlayerError').textContent=state.Error;const list=$('nativePlayerSessions');
  const live=new Set(sessions.map(session=>session.ID));for(const row of [...list.children])if(!live.has(row.dataset.id))row.remove();
  for(const session of sessions){
    let row=[...list.children].find(row=>row.dataset.id===session.ID);
    if(row){updateNativePlayerRow(row,session);continue}
    row=document.createElement('div');row.className='native-player-row';row.dataset.id=session.ID;
    const name=document.createElement('strong');name.textContent=session.Name;
    const image=session.Kind==='image';
    const info=document.createElement('span');info.className='native-player-decoder';info.textContent=(image?'原生图片查看':nativeDecoderLabel(session.Decoder))+(session.Codec?' · '+session.Codec:'');
    const position=document.createElement('small');position.textContent=image?'独立窗口':mediaTime(session.Time)+' / '+mediaTime(session.Duration);
    const actions=document.createElement('div');actions.className='native-player-actions';
    const controls=[...(image?[['放大','zoom-in'],['缩小','zoom-out'],['适应窗口','zoom-reset']]:[['暂停','pause'],['静音','mute']]),['置顶','pin'],['关闭','close']];
    for(const [label,action] of controls){const b=button(label,async()=>{b.disabled=true;await nativePlayerAction(b.dataset.action,{ID:session.ID});if(b.isConnected)b.disabled=!row._session?.Connected&&action!=='close'});b.dataset.action=action;b.title=label;actions.append(b)}
    row.append(name,info,position,actions);
    if(!image){const sliders=document.createElement('div');sliders.className='native-player-sliders';
      for(const [key,label,max] of [['seek','进度',session.Duration||0],['volume','音量',100]]){const field=document.createElement('label'),input=document.createElement('input'),value=document.createElement('output');field.append(document.createTextNode(label),input,value);input.type='range';input.min=0;input.max=max;input.step=key==='seek'?0.1:1;input.dataset.control=key;input.setAttribute('aria-label',session.Name+' · '+label);input.oninput=()=>{input.dataset.editing='true';value.textContent=key==='seek'?mediaTime(input.value):input.value+'%'};input.onchange=async()=>{const selected=Number(input.value);input.disabled=true;await nativePlayerAction(key,{ID:session.ID,Value:selected});delete input.dataset.editing;input.disabled=!row._session?.Connected};sliders.append(field)}row.append(sliders)}
    list.append(row);updateNativePlayerRow(row,session);
  }
  if(!sessions.length){const empty=document.createElement('p');empty.className='hint';empty.textContent='右键本机图片或视频，选择“原生悬浮查看/播放（mpv）”。窗口可移到浏览器外，独立调整大小和置顶。';list.append(empty)}
}
function updateNativePlayerRow(row,session){
  row._session=session;const image=session.Kind==='image';row.querySelector('small').textContent=image?'独立窗口':mediaTime(session.Time)+' / '+mediaTime(session.Duration);
  const name=row.querySelector('strong'),duplicate=(nativePlayerState?.Sessions||[]).filter(item=>item.Name===session.Name).length>1,parent=(session.Path||'').replace(/[/\\][^/\\]*$/,'');name.textContent=session.Name+(duplicate&&parent?' · '+parent:'');name.title=session.Path||session.Name;
  row.querySelector('.native-player-decoder').textContent=(image?'原生图片查看':nativeDecoderLabel(session.Decoder))+(session.Codec?' · '+session.Codec:'')+(session.Width&&session.Height?' · 窗口 '+session.Width+'×'+session.Height:'');
  for(const control of row.querySelectorAll('button')){const action=control.dataset.action;
    if(['pause','resume'].includes(action)){control.dataset.action=session.Paused?'resume':'pause';control.textContent=session.Paused?'继续':'暂停'}
    if(['mute','unmute'].includes(action)){control.dataset.action=session.Muted?'unmute':'mute';control.textContent=session.Muted?'开启声音':'静音'}
    if(['pin','unpin'].includes(action)){control.dataset.action=session.OnTop?'unpin':'pin';control.textContent=session.OnTop?'取消置顶':'置顶'}
    control.disabled=!session.Connected&&action!=='close';control.title=control.textContent;
  }
  for(const input of row.querySelectorAll('input')){const seek=input.dataset.control==='seek';input.disabled=!session.Connected||(seek&&!session.Duration);if(input.dataset.editing||document.activeElement===input)continue;input.max=seek?(session.Duration||0):100;input.value=seek?(session.Time||0):(session.Volume??100);input.nextElementSibling.textContent=seek?mediaTime(input.value):Math.round(input.value)+'%'}
}
function scheduleNativePlayer(){clearTimeout(nativePlayerTimer);if(canUseNativePrograms()&&((nativePlayerState?.Sessions||[]).length||$('nativePlayerDialog').open))nativePlayerTimer=setTimeout(refreshNativePlayer,2000)}
async function refreshNativePlayer(){if(!canUseNativePrograms())return;const revision=nativePlayerRevision;try{const state=await api('native-player',{Action:'status'});if(revision===nativePlayerRevision)renderNativePlayer(state)}catch(e){if(revision===nativePlayerRevision&&$('nativePlayerDialog').open)$('nativePlayerError').textContent=e.message}finally{scheduleNativePlayer()}}
async function openNativeMedia(item){
  if(!canUseNativePrograms()||!item||item.Dir||!['image','video'].includes(item.Kind))return false;
  stopVideoHover();nativePlayerRevision++;$('nativePlayerError').textContent='';say('正在启动 mpv 原生播放器…');
  try{renderNativePlayer(await api('native-player',{Action:'open',Path:item.Path,Version:item.Version}));scheduleNativePlayer();say(item.Kind==='image'?'已打开原生图片窗口 · 滚轮缩放，Ctrl+左键拖动图片，按 0 复位；左键拖动移动窗口。':'已启动原生悬浮播放 · 直接读文件，硬件解码优先；实际解码方式可在“原生播放器”中查看。');return true}
  catch(e){$('nativePlayerDialog').showModal();await refreshNativePlayer();$('nativePlayerError').textContent=e.message;return false}
}
function initializeNativePlayer(){
  if(nativePlayerInitialized)return;nativePlayerInitialized=true;$('nativePlayer').hidden=!canUseNativePrograms();if(!canUseNativePrograms())return;
  $('nativePlayer').onclick=()=>{$('nativePlayerDialog').showModal();refreshNativePlayer()};$('nativePlayerClose').onclick=()=>{$('nativePlayerDialog').close();scheduleNativePlayer()};
  const configure=async program=>{$('nativePlayerSave').disabled=$('nativePlayerDetect').disabled=true;$('nativePlayerError').textContent='';try{renderNativePlayer(await api('native-player',{Action:'configure',Program:program}));scheduleNativePlayer()}catch(e){$('nativePlayerError').textContent=e.message}finally{$('nativePlayerSave').disabled=$('nativePlayerDetect').disabled=false}};
  $('nativePlayerSave').onclick=()=>configure($('nativePlayerProgram').value);$('nativePlayerDetect').onclick=()=>configure('');
  for(const [id,action] of [['nativePlayerCascade','cascade'],['nativePlayerTile','tile'],['nativePlayerPauseAll','pause-all'],['nativePlayerMuteAll','mute-all']])$(id).onclick=async()=>{const control=$(id);control.disabled=true;await nativePlayerAction(action);control.disabled=!(nativePlayerState?.Sessions||[]).length};
  $('nativePlayerRemember').onchange=async()=>{const control=$('nativePlayerRemember'),enabled=control.checked;control.disabled=true;if(!await nativePlayerAction('remember-windows',{Enabled:enabled}))control.checked=!enabled;control.disabled=false};
  $('nativePlayerWindowSize').onchange=async()=>{const input=$('nativePlayerWindowSize'),previous=nativePlayerState?.WindowSize||'compact';input.disabled=true;$('nativePlayerSizeStatus').textContent='正在保存…';try{const state=await api('native-player',{Action:'window-size',WindowSize:input.value});input.disabled=false;renderNativePlayer(state);$('nativePlayerSizeStatus').textContent='已保存，下次打开窗口生效'}catch(e){input.value=previous;$('nativePlayerSizeStatus').textContent='保存失败：'+e.message}finally{input.disabled=false}};
  window.addEventListener('pagehide',()=>clearTimeout(nativePlayerTimer));refreshNativePlayer();
}
