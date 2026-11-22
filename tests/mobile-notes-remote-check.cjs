const assert=require('node:assert/strict'),fs=require('node:fs'),{JSDOM}=require('jsdom');
const {setup,unlock,wait}=require('./private-notes-check.cjs');
(async()=>{
 const t=setup(true,{width:390,viewport:{height:844,scale:1}});
 Object.defineProperty(t.w,'innerHeight',{value:844,configurable:true});await unlock(t);
 t.$('showList').click();assert(t.$('workspace').classList.contains('list-open'));assert.notEqual(t.d.activeElement,t.$('searchNotes'),'opening mobile list must not focus search or open keyboard');t.$('hideNotesList').click();
 t.$('markdown').focus();t.w.visualViewport.height=420;t.w.visualViewport.dispatchEvent(new t.w.Event('resize'));
 assert.equal(t.d.documentElement.style.getPropertyValue('--notes-viewport-height'),'420px');assert(t.d.body.classList.contains('notes-keyboard-open'));
 t.w.visualViewport.height=844;t.w.visualViewport.dispatchEvent(new t.w.Event('resize'));assert(!t.d.body.classList.contains('notes-keyboard-open'));
 t.w.visualViewport.scale=1.5;t.w.visualViewport.height=400;t.w.visualViewport.dispatchEvent(new t.w.Event('resize'));assert(!t.d.body.classList.contains('notes-keyboard-open'),'pinch zoom must not be treated as keyboard');assert.equal(t.d.documentElement.style.getPropertyValue('--notes-viewport-height'),'844px');
 assert.equal(t.errors.length,0,t.errors.join('\n'));t.dom.window.close();
 for(const page of ['index.html','explorer.html','notes.html']){
  const dom=new JSDOM(fs.readFileSync('web/'+page,'utf8'),{url:'http://localhost/?session=remote-test-only',runScripts:'outside-only'}),w=dom.window,d=w.document;let requests=0;
  w.HTMLDialogElement.prototype.showModal=function(){this.open=true};w.confirm=()=>false;w.fetch=async()=>{requests++;return{ok:false,status:500,text:async()=> 'synthetic logout failure'}};w.eval(fs.readFileSync('web/remote-workspace.js','utf8'));
  const menu=d.getElementById('remoteSessionMenu');assert(menu);assert(!menu.open);assert(!d.querySelector('.remote-workspace-bar'));assert(menu.closest(page==='notes.html'?'.head-actions':'aside'),'session entry belongs in navigation, not over content');
  menu.open=true;menu.querySelector('button').click();assert.equal(requests,0,'cancel must preserve session');w.confirm=()=>true;menu.querySelector('button').click();await wait(()=>menu.querySelector('p').textContent==='synthetic logout failure','logout retry feedback');assert(!menu.querySelector('button').disabled);
  menu.dispatchEvent(new w.KeyboardEvent('keydown',{key:'Escape',bubbles:true,cancelable:true}));assert(!menu.open);menu.open=true;d.body.dispatchEvent(new w.Event('pointerdown',{bubbles:true}));assert(!menu.open);dom.window.close();
 }
 console.log('PASS: mobile list does not focus search, keyboard viewport/pinch isolation, remote session menus in navigation, cancel/logout failure/Escape/outside close');
})().catch(error=>{console.error(error);process.exit(1)});
