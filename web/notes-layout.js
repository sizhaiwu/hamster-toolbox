// Layout preferences contain no note content or credentials.
(()=>{
 const $=id=>document.getElementById(id),key='hamster.notes.sidebar-collapsed.v1';
 let collapsed=false,focused=false,wantFullscreen=false,nativeActive=false;
 try{collapsed=localStorage.getItem(key)==='true'}catch{}
 const mobile=()=>innerWidth<=760;
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
  refresh();if(open)$('searchNotes').focus({preventScroll:true});else $('showList').focus({preventScroll:true});
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
