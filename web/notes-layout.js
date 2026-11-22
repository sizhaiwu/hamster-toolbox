// Layout preferences contain no note content or credentials.
(()=>{
 const $=id=>document.getElementById(id),key='hamster.notes.sidebar-collapsed.v1';
 let collapsed=false,focused=false,wantFullscreen=false,nativeActive=false;
 try{collapsed=localStorage.getItem(key)==='true'}catch{}
 const mobile=()=>innerWidth<=760;
 let viewportBaseline=innerHeight,baselineWidth=innerWidth,revealTimer;
 const editingElement=()=>{const element=document.activeElement;return element?.matches('input,textarea,[contenteditable=true]')?element:null};
 function revealEditor(){
  if(!mobile()||!document.body.classList.contains('notes-keyboard-open')||document.querySelector('dialog[open]'))return;
  const element=editingElement();if(!element?.closest('.editor'))return;
  const viewport=window.visualViewport,top=(viewport?.offsetTop||0)+16,bottom=(viewport?.offsetTop||0)+(viewport?.height||innerHeight)-16;
  let rect=element.getBoundingClientRect();if(!rect.height)return;
  if(element.matches('[contenteditable=true]')){
   const selection=window.getSelection();
   if(selection?.rangeCount&&element.contains(selection.anchorNode)){
    const range=selection.getRangeAt(0),caret=range.getBoundingClientRect?.();
    if(caret?.height){
     const host=$('richEditor'),box=host.getBoundingClientRect();
     if(caret.bottom>box.bottom-8)host.scrollTop+=caret.bottom-box.bottom+8;
     else if(caret.top<box.top+8)host.scrollTop+=caret.top-box.top-8;
     rect=range.getBoundingClientRect();
    }
   }
  }
  const delta=rect.bottom>bottom?rect.bottom-bottom:rect.top<top?rect.top-top:0;
  if(delta)window.scrollBy({top:delta,behavior:'instant'});
 }
 function scheduleReveal(){if(!mobile())return;clearTimeout(revealTimer);revealTimer=setTimeout(revealEditor,80)}
 // iOS keyboards resize the visual viewport rather than the layout viewport.
 function refreshViewport(){
  const viewport=window.visualViewport;
  if(!mobile()){document.documentElement.style.removeProperty('--notes-viewport-height');document.body.classList.remove('notes-keyboard-open');return}
  const height=viewport&&viewport.scale<=1.05?viewport.height:innerHeight;
  document.documentElement.style.setProperty('--notes-viewport-height',Math.round(height)+'px');
  if(Math.abs(innerWidth-baselineWidth)>80){viewportBaseline=innerHeight;baselineWidth=innerWidth;}
  viewportBaseline=Math.max(viewportBaseline,innerHeight,height);
  document.body.classList.toggle('notes-keyboard-open',Boolean(editingElement()&&(!viewport||viewport.scale<=1.05)&&viewportBaseline-height>140));
  if(document.body.classList.contains('notes-keyboard-open'))scheduleReveal();
 }
 window.visualViewport?.addEventListener('resize',refreshViewport);addEventListener('resize',refreshViewport);
 document.addEventListener('focusin',()=>setTimeout(refreshViewport,0));document.addEventListener('focusout',()=>setTimeout(refreshViewport,0));refreshViewport();
 document.addEventListener('input',scheduleReveal);document.addEventListener('keyup',scheduleReveal);
 function refresh(){
  const workspace=$('workspace'),open=mobile()?workspace.classList.contains('list-open'):!collapsed;
  workspace.classList.toggle('notes-list-collapsed',!mobile()&&collapsed);
  $('showList').setAttribute('aria-expanded',String(open));$('showList').setAttribute('aria-label',open?'收起笔记列表':'展开笔记列表');$('showList').title=open?'收起笔记列表':'展开笔记列表';
  $('hideNotesList').title='收起笔记列表';
  document.body.classList.toggle('notes-fullscreen',focused);
  $('notesFullscreen').textContent=focused?'退出全屏':'全屏';$('notesFullscreen').setAttribute('aria-pressed',String(focused));$('notesFullscreen').title=focused?'退出全屏（Esc）':'全屏显示笔记工作区';
 }
 function setList(open){
  if(mobile())$('workspace').classList.toggle('list-open',open);
  else{collapsed=!open;try{localStorage.setItem(key,String(collapsed))}catch{}}
  refresh();if(open&&!mobile())$('searchNotes').focus({preventScroll:true});else if(!open)$('showList').focus({preventScroll:true});
 }
 function toggleList(){setList(mobile()?!$('workspace').classList.contains('list-open'):collapsed)}
 async function fullscreen(value){
  wantFullscreen=Boolean(value);focused=wantFullscreen;refresh();
  if(wantFullscreen){
   if(document.fullscreenElement===document.documentElement)return;
   try{await document.documentElement.requestFullscreen?.()}catch{/* Keep the in-page focus layout when native fullscreen is unavailable. */}
   if(!wantFullscreen&&document.fullscreenElement===document.documentElement)try{await document.exitFullscreen?.()}catch{}
  }else if(document.fullscreenElement===document.documentElement)try{await document.exitFullscreen?.()}catch{}
 }
 function escape(){if(!focused||document.querySelector('dialog[open]')||!$('findBar').hidden)return false;void fullscreen(false);$('notesFullscreen').focus({preventScroll:true});return true}
 $('showList').onclick=toggleList;$('hideNotesList').onclick=()=>setList(false);$('notesFullscreen').onclick=()=>void fullscreen(!wantFullscreen);
 document.addEventListener('fullscreenchange',()=>{const native=document.fullscreenElement===document.documentElement;if(native){nativeActive=true;if(!wantFullscreen){void fullscreen(false);return}focused=true}else if(nativeActive){nativeActive=false;focused=false;wantFullscreen=false}refresh()});
 document.addEventListener('keydown',event=>{if(event.key==='Escape'&&!event.defaultPrevented&&escape()){event.preventDefault();event.stopPropagation()}},true);
 window.addEventListener('message',event=>{if(event.source===$('preview').contentWindow&&event.data?.type==='hamster-note-preview-escape')escape()});
 window.addEventListener('resize',refresh);
 window.hamsterNotesLayout={refresh,toggleList,fullscreen,reset(){void fullscreen(false);$('workspace').classList.remove('list-open');refresh()}};
 refresh();
})();
