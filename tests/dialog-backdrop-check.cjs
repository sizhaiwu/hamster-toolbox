const assert=require('node:assert/strict');const {createUI,until,requests}=require('./ui-check.cjs');const {setup,wait}=require('./explorer-check.cjs');
function rect(d){d.getBoundingClientRect=()=>({left:100,right:400,top:100,bottom:300,width:300,height:200})}
function send(w,d,type,x=10,y=10){d.dispatchEvent(new w.MouseEvent(type,{bubbles:true,cancelable:true,button:0,clientX:x,clientY:y}))}
function click(w,d){send(w,d,'pointerdown');send(w,d,'pointerup');send(w,d,'click')}
(async()=>{
 const dom=createUI(30010),w=dom.window,$=id=>w.document.getElementById(id);await until(()=>$('appVersion').textContent==='v0.1.0'&&!$('nav').querySelector('button').disabled,'settings ready');const dialog=$('generalSettingsDialog');rect(dialog);dialog.showModal();click(w,dialog);assert(dialog.open,'default off');
 $('CloseDialogOnBackdrop').checked=true;$('CloseDialogOnBackdrop').dispatchEvent(new w.Event('change'));await until(()=>w.toolboxSettings.CloseDialogOnBackdrop===true,'saved toggle');assert(requests.some(r=>r.path==='ui-settings'&&r.body.CloseDialogOnBackdrop===true));
 send(w,dialog,'pointerdown',120,120);send(w,dialog,'pointerup',120,120);send(w,dialog,'click',120,120);assert(dialog.open,'dialog padding is inside');
 send(w,dialog,'pointerdown',120,120);send(w,dialog,'pointerup');send(w,dialog,'click');assert(dialog.open,'drag from inside must not close');
 send(w,dialog,'pointerdown');send(w,dialog,'pointercancel');send(w,dialog,'pointerup');send(w,dialog,'click');assert(dialog.open,'cancelled pointer must not close');
 send(w,dialog,'pointerdown');send(w,dialog,'pointermove',50,50);send(w,dialog,'pointerup',50,50);send(w,dialog,'click',50,50);assert(dialog.open,'dragging the backdrop must not close');
 dialog.addEventListener('cancel',e=>e.preventDefault(),{once:true});click(w,dialog);assert(dialog.open,'respect existing cancel veto');click(w,dialog);assert(!dialog.open);$('showCLI').click();await until(()=>$('cliCode').value.length>0,'CLI ready');rect($('cliDialog'));click(w,$('cliDialog'));assert(!$('cliDialog').open);assert.equal($('cliCode').value,'','backdrop follows close-button cleanup');dom.window.close();
 const t=setup(true,{globalFeedback:true,generalSettings:{CloseDialogOnBackdrop:true,TaskPopup:false,TaskSound:false}});await wait(()=>t.$('entries').children.length&&t.w.toolboxSettings.CloseDialogOnBackdrop,'owner ready');t.$('location').value='E:\\demo';t.$('go').click();await wait(()=>!t.$('newFolder').disabled,'directory');t.d.querySelectorAll('.entry-open')[1].click();t.$('remove').click();rect(t.$('deleteDialog'));click(t.w,t.$('deleteDialog'));assert(!t.$('deleteDialog').open);assert(!t.calls.some(c=>c.route==='mutate'),'backdrop cancels deletion');t.dom.window.close();
 console.log('PASS: backdrop preference default/save, inside clicks, drag/cancel isolation, cancel veto, normal close and delete cancellation');
})().catch(e=>{console.error(e);process.exit(1)});
