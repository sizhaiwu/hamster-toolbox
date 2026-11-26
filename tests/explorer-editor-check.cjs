const assert=require('node:assert/strict');
const {setup,wait}=require('./explorer-check.cjs');
const response=value=>({ok:true,status:200,json:async()=>value});
(async()=>{
 let conflict=false,saves=0;
 const original='<script>window.editorInjected=true</script>\nhello world';
 const item={Name:'config.json',Path:'E:\\demo\\config.json',Kind:'text'};
 const t=setup(true,{requestHook:({route,body})=>{
  if(route==='text-open')return response({Text:original,Revision:'revision-one',Encoding:'UTF-8',Newline:'CRLF',BOM:false});
  if(route==='text-save'){saves++;if(conflict)return {ok:false,status:409,text:async()=> '文件内容已变化'};assert.equal(body.Revision,'revision-one');return response({Text:body.Text,Revision:'revision-two',Encoding:'UTF-8',Newline:'CRLF',Backup:'E:\\demo\\.toolbox-replaced-demo\\config.json'})}
 }});
 await wait(()=>t.$('entries').children.length,'initial directory');
 await t.w.openTextEditor(item);
 assert(t.$('fileEditor').open);assert.equal(t.$('fileEditorContent').value,original);assert.equal(t.w.editorInjected,undefined);assert(t.$('fileEditorSave').disabled);
 t.$('fileEditorContent').value=original+'\nnew line';t.$('fileEditorContent').dispatchEvent(new t.w.Event('input'));
 assert(!t.$('fileEditorSave').disabled);
 t.$('fileEditorContent').dispatchEvent(new t.w.KeyboardEvent('keydown',{key:'s',ctrlKey:true,bubbles:true,cancelable:true}));
 await wait(()=>saves===1&&!t.$('fileEditorClose').disabled,'saved');assert.match(t.$('fileEditorStatus').textContent,/已保存/);assert(t.$('fileEditorSave').disabled);
 assert(t.$('fileEditorFormat').title.includes('.toolbox-replaced-'));
 t.$('fileEditorFind').value='hello';t.$('fileEditorReplacement').value='goodbye';t.$('fileEditorFindNext').click();assert.equal(t.$('fileEditorContent').value.slice(t.$('fileEditorContent').selectionStart,t.$('fileEditorContent').selectionEnd),'hello');t.$('fileEditorReplace').click();assert(t.$('fileEditorContent').value.includes('goodbye world'));
 assert(t.$('fileEditor').classList.contains('editor-expanded'));t.$('fileEditorFullscreen').click();assert(!t.$('fileEditor').classList.contains('editor-expanded'));t.$('fileEditorFullscreen').click();
 t.$('fileEditorFont').value='18';t.$('fileEditorFont').dispatchEvent(new t.w.Event('change'));assert.equal(t.w.localStorage.getItem('hamster.files.editor.font'),'18');assert(!JSON.stringify(t.w.localStorage).includes('goodbye'));
 conflict=true;t.$('fileEditorSave').click();await wait(()=>saves===2&&!t.$('fileEditorClose').disabled,'conflict');assert(t.$('fileEditorError').textContent.includes('文件内容已变化'));assert(t.$('fileEditorContent').value.includes('goodbye'));
 t.$('fileEditorClose').click();assert(t.$('fileEditorDiscard').open);t.$('fileEditorKeep').click();assert(t.$('fileEditor').open);t.$('fileEditorClose').click();t.$('fileEditorDiscardConfirm').click();assert(!t.$('fileEditor').open);assert.equal(t.$('fileEditorContent').value,'');assert.equal(t.$('fileEditorFormat').title,'');
 let resolveRead;t.w.fetch=async()=>new Promise(resolve=>resolveRead=resolve);const pending=t.w.openTextEditor(item);assert(t.$('fileEditor').open);assert(!t.$('fileEditorClose').disabled);t.$('fileEditorClose').click();resolveRead(response({Text:'late',Revision:'late',Encoding:'UTF-8',Newline:'LF'}));await pending;assert(!t.$('fileEditor').open);assert.equal(t.$('fileEditorContent').value,'');
 assert.equal(t.errors.length,0,t.errors.map(String).join('\n'));t.dom.window.close();
 const guest=setup(false);await wait(()=>guest.$('entries').children.length,'guest listing');await guest.w.openTextEditor(item);assert(!guest.$('fileEditor').open);assert(!guest.calls.some(c=>c.route==='text-open'));guest.dom.window.close();
 const writer=setup(false,{roots:[{ID:'share-id',Name:'Demo',Writable:true}],requestHook:({route})=>route==='text-open'?response({Text:'shared',Revision:'shared-one',Encoding:'UTF-8',Newline:'LF'}):null});await wait(()=>writer.$('entries').children.length,'writable guest listing');await writer.w.openTextEditor({Name:'note.txt',Path:'note.txt',Kind:'text'});assert(writer.$('fileEditor').open);assert.equal(writer.calls.find(c=>c.route==='text-open').url.searchParams.get('root'),'share-id');writer.$('fileEditorClose').click();writer.dom.window.close();
 console.log('PASS: file editor save/shortcut, safe text, conflict retention, backup information, find/replace, fullscreen, font memory, unsaved-close confirmation, stale read cancellation and guest permissions');
})().catch(error=>{console.error(error);process.exit(1)});
