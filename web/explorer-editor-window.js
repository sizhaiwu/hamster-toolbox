'use strict';

const editorUI=id=>document.getElementById(id);
let editorMinimized=false,editorSuspendedCloses=0,editorWindowRect=null,editorWindowGesture=null;

function editorWindowViewport(){
 const viewport=window.visualViewport;
 return {x:viewport?.offsetLeft||0,y:viewport?.offsetTop||0,w:viewport?.width||innerWidth,h:viewport?.height||innerHeight};
}
function clampEditorWindow(rect){
 const v=editorWindowViewport(),maxW=Math.max(1,v.w-12),maxH=Math.max(1,v.h-12);
 const w=Math.min(maxW,Math.max(Math.min(520,maxW),rect.w)),h=Math.min(maxH,Math.max(Math.min(260,maxH),rect.h));
 return {w,h,x:Math.max(v.x+6,Math.min(rect.x,v.x+v.w-w-6)),y:Math.max(v.y+6,Math.min(rect.y,v.y+v.h-h-6))};
}
function defaultEditorWindowRect(){
 const v=editorWindowViewport(),w=Math.min(1040,v.w-32),h=Math.min(720,v.h-48);
 return clampEditorWindow({w,h,x:v.x+(v.w-w)/2,y:v.y+(v.h-h)/2});
}
function applyEditorWindowRect(rect,remember=true){
 const dialog=editorUI('fileEditor'),value=clampEditorWindow(rect);
 if(remember)editorWindowRect={...value};
 dialog.classList.add('editor-positioned');
 for(const [name,amount]of Object.entries({left:value.x,top:value.y,width:value.w,height:value.h}))dialog.style.setProperty('--editor-'+name,amount+'px');
 fileCode?.resize();return value;
}
function updateEditorDock(){
 const dock=editorUI('fileEditorDock');dock.hidden=!editorMinimized||!editorTabs.length;
 const dirty=editorTabs.filter(tabDirty).length;
 const label='编辑器 · '+editorTabs.length+' 个文件'+(dirty?' · '+dirty+' 未保存':'')+(savingEditor()?' · 正在保存':'');
 setEditorLabel(editorUI('fileEditorDockLabel'),label);const title='恢复文件编辑器'+(fileEdit?'：'+fileEdit.item.Name:'');
 if(dock.title!==title)dock.title=title;setEditorLabel(editorUI('fileEditorWindowTitle'),fileEdit?.item.Name||'文件编辑器');
}
function minimizeEditorWindow(){
 const dialog=editorUI('fileEditor');if(!dialog.open||editorUI('fileEditorDiscard').open)return;
 finishEditorWindowGesture();captureEditorTab();editorUI('fileEditorSettings').open=false;
 editorMinimized=true;editorSuspendedCloses++;dialog.close();updateEditorDock();editorUI('fileEditorDock').focus({preventScroll:true});
}
function restoreEditorWindow(){
 if(!editorMinimized||!editorTabs.length)return;
 editorMinimized=false;const dialog=editorUI('fileEditor');dialog.showModal();updateEditorDock();resizeEditorWindow();
 if(fileEdit?.session&&fileCode)fileCode.restore(fileEdit.session);
 fileCode?.view.focus();if(!fileCode)editorUI('fileEditorContent').focus({preventScroll:true});
}
function toggleEditorMaximize(){
 finishEditorWindowGesture();const dialog=editorUI('fileEditor');
 dialog.classList.toggle('editor-expanded');editorUI('fileEditorFullscreen').textContent=dialog.classList.contains('editor-expanded')?'还原':'最大化';
 resizeEditorWindow();
}
function resizeEditorWindow(){
 const dialog=editorUI('fileEditor');if(!dialog.open)return;
 const v=editorWindowViewport();dialog.classList.toggle('editor-short',v.h<600);dialog.style.setProperty('--editor-vh',v.h+'px');dialog.style.setProperty('--editor-vw',v.w+'px');dialog.style.setProperty('--editor-expanded-left',(v.x+6)+'px');
 if(dialog.classList.contains('editor-expanded'))dialog.style.setProperty('--editor-top',(v.y+6)+'px');
 else applyEditorWindowRect(editorWindowRect||defaultEditorWindowRect(),false);
 fileCode?.resize();
}
function disposeEditorWindow(){
 finishEditorWindowGesture();editorMinimized=false;editorWindowRect=null;editorUI('fileEditorDock').hidden=true;
 editorUI('fileEditorSettings').open=false;editorUI('fileEditor').classList.remove('editor-positioned');
}
function startEditorWindowGesture(event,resize=false){
 const dialog=editorUI('fileEditor');if(event.button!==0||!dialog.open||editorWindowGesture)return;
 if(!resize&&event.target.closest('button,a,input,select,textarea,summary'))return;
 if(resize&&dialog.classList.contains('editor-expanded'))return;
 const expanded=dialog.classList.contains('editor-expanded'),box=dialog.getBoundingClientRect(),v=editorWindowViewport();
 const rect=expanded?{x:v.x+6,y:v.y+6,w:v.w-12,h:v.h-12}:clampEditorWindow(editorWindowRect||defaultEditorWindowRect());
 editorWindowGesture={pointer:event.pointerId,target:event.currentTarget,startX:event.clientX,startY:event.clientY,rect,expanded,resize,started:false,previous:editorWindowRect?{...editorWindowRect}:null,ratio:Math.max(0,Math.min(1,(event.clientX-box.left)/(box.width||rect.w)))};
 event.preventDefault();try{event.currentTarget.setPointerCapture?.(event.pointerId)}catch{}
}
function moveEditorWindowGesture(event){
 const gesture=editorWindowGesture;if(!gesture||event.pointerId!==gesture.pointer)return;
 const dx=event.clientX-gesture.startX,dy=event.clientY-gesture.startY;if(!gesture.started&&Math.hypot(dx,dy)<4)return;
 event.preventDefault();const dialog=editorUI('fileEditor');
 if(!gesture.started){
  gesture.started=true;document.body.classList.add('editor-window-dragging');
  if(gesture.expanded){
   dialog.classList.remove('editor-expanded');editorUI('fileEditorFullscreen').textContent='最大化';
   const restored=clampEditorWindow(editorWindowRect||defaultEditorWindowRect());
   gesture.rect={...restored,x:gesture.startX-restored.w*gesture.ratio,y:gesture.startY-16};
  }
 }
 const value=gesture.resize?{...gesture.rect,w:gesture.rect.w+dx,h:gesture.rect.h+dy}:{...gesture.rect,x:gesture.rect.x+dx,y:gesture.rect.y+dy};
 applyEditorWindowRect(value);
}
function finishEditorWindowGesture(cancel=false){
 const gesture=editorWindowGesture;if(!gesture)return;editorWindowGesture=null;document.body.classList.remove('editor-window-dragging');
 try{gesture.target.releasePointerCapture?.(gesture.pointer)}catch{}
 if(cancel&&gesture.started){
  editorWindowRect=gesture.previous;editorUI('fileEditor').classList.toggle('editor-expanded',gesture.expanded);
  editorUI('fileEditorFullscreen').textContent=gesture.expanded?'还原':'最大化';resizeEditorWindow();
 }
}
const editorTitlebar=editorUI('fileEditorTitlebar'),editorResizeHandle=editorUI('fileEditorResize');
editorTitlebar.addEventListener('pointerdown',event=>startEditorWindowGesture(event));
editorResizeHandle.addEventListener('pointerdown',event=>startEditorWindowGesture(event,true));
document.addEventListener('pointermove',moveEditorWindowGesture,{passive:false});
document.addEventListener('pointerup',event=>{if(event.pointerId===editorWindowGesture?.pointer)finishEditorWindowGesture()});
document.addEventListener('pointercancel',event=>{if(event.pointerId===editorWindowGesture?.pointer)finishEditorWindowGesture(true)});
window.addEventListener('blur',()=>finishEditorWindowGesture(true));
document.addEventListener('keydown',event=>{if(editorWindowGesture&&event.key==='Escape'){event.preventDefault();event.stopPropagation();finishEditorWindowGesture(true)}},true);
for(const handle of [editorTitlebar,editorResizeHandle])handle.addEventListener('lostpointercapture',event=>{if(event.pointerId===editorWindowGesture?.pointer)finishEditorWindowGesture(true)});
editorTitlebar.addEventListener('dblclick',event=>{if(!event.target.closest('button'))toggleEditorMaximize()});
for(const handle of [editorTitlebar,editorResizeHandle])handle.addEventListener('keydown',event=>{
 if(event.target!==handle||!['ArrowLeft','ArrowRight','ArrowUp','ArrowDown'].includes(event.key))return;
 event.preventDefault();event.stopPropagation();const dialog=editorUI('fileEditor');
 if(dialog.classList.contains('editor-expanded')){if(handle===editorResizeHandle)return;toggleEditorMaximize()}
 const rect=clampEditorWindow(editorWindowRect||defaultEditorWindowRect()),amount=event.shiftKey?64:16;
 if(handle===editorResizeHandle){if(event.key==='ArrowLeft')rect.w-=amount;if(event.key==='ArrowRight')rect.w+=amount;if(event.key==='ArrowUp')rect.h-=amount;if(event.key==='ArrowDown')rect.h+=amount}
 else{if(event.key==='ArrowLeft')rect.x-=amount;if(event.key==='ArrowRight')rect.x+=amount;if(event.key==='ArrowUp')rect.y-=amount;if(event.key==='ArrowDown')rect.y+=amount}
 applyEditorWindowRect(rect);
});
editorUI('fileEditorMinimize').onclick=minimizeEditorWindow;editorUI('fileEditorDock').onclick=restoreEditorWindow;
