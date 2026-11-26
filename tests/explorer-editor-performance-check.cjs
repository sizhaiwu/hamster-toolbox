const assert=require('node:assert/strict'),fs=require('node:fs');
const {setup,wait}=require('./explorer-check.cjs');
const text='const sampleValue = 42;\n'.repeat(70000);
const files=[0,1,2].map(i=>({Name:'large-'+i+'.ts',Path:'E:\synthetic\large-'+i+'.ts',Kind:'text'}));
(async()=>{
 let finishSave,sentText,editor;
 const t=setup(true,{requestHook:({route,body})=>route==='text-open'?{ok:true,json:async()=>({Text:text,Revision:'one',Encoding:'UTF-8',Newline:'LF'})}:route==='text-save'?new Promise(resolve=>{sentText=body.Text;finishSave=()=>resolve({ok:true,json:async()=>({Revision:'two',Encoding:'UTF-8',Newline:'LF'})})}):null});
 t.w.Range.prototype.getClientRects=()=>[];t.w.Range.prototype.getBoundingClientRect=()=>({left:0,right:0,top:0,bottom:0,width:0,height:0});t.w.cancelAnimationFrame=id=>t.w.clearTimeout(id);t.w.scrollBy=()=>{};
 t.w.eval(fs.readFileSync('web/file-code-editor.js','utf8'));const realMount=t.w.hamsterCodeEditor.mount;t.w.hamsterCodeEditor.mount=options=>(editor=realMount(options));
 await wait(()=>t.$('entries').children.length,'boot');for(const file of files)await t.w.openTextEditor(file);await new Promise(resolve=>setTimeout(resolve,15));
 for(let i=0;i<200;i++){const node=t.d.createElement('button');node.dataset.filePath='E:\synthetic\tree-'+i+'.ts';node.textContent='tree-'+i;t.$('fileEditorTree').append(node)}
 const view=editor.view,source=t.$('fileEditorContent'),descriptor=Object.getOwnPropertyDescriptor(t.w.HTMLTextAreaElement.prototype,'value');let fullWrites=0;
 Object.defineProperty(source,'value',{get(){return descriptor.get.call(this)},set(value){fullWrites++;descriptor.set.call(this,value)}});
 const tab=t.$('fileEditorTabs').querySelector('[aria-selected=true]'),treeObserver=new t.w.MutationObserver(()=>{});treeObserver.observe(t.$('fileEditorTree'),{attributes:true,subtree:true});
 for(let i=0;i<30;i++){const end=view.state.doc.length;view.dispatch({changes:{from:end,insert:'x'},selection:{anchor:end+1},userEvent:'input.performance'})}
 assert.equal(fullWrites,0,'typing must not copy the full 1.68 MB document to a hidden textarea');assert.equal(treeObserver.takeRecords().length,0,'typing must not rescan/rewrite the directory tree');assert.equal(t.$('fileEditorTabs').querySelector('[aria-selected=true]'),tab,'tab controls must keep their identity while typing');assert(!t.$('fileEditorSave').disabled);assert.equal(t.$('fileEditorCursor').textContent,'行 70001，列 31');
 view.dispatch({changes:{from:text.length,to:view.state.doc.length},selection:{anchor:0},userEvent:'delete'});assert(t.$('fileEditorSave').disabled,'restoring identical content clears dirty state without serialization');assert.equal(fullWrites,0);assert.equal(t.$('fileEditorCursor').textContent,'行 1，列 1');
 const state=view.state;editor.setTheme('dark');editor.setIndent(2);assert.equal(view.state,state,'unchanged theme/indent must not reconfigure the editor');
 view.dispatch({changes:{from:0,insert:'new '},userEvent:'input.performance'});t.$('fileEditorSave').click();await wait(()=>finishSave,'save snapshot');assert.equal(sentText,'new '+text);assert.equal(source.value,sentText,'save synchronizes its exact snapshot on demand');
 view.dispatch({changes:{from:0,insert:'continued '},userEvent:'input.performance'});t.$('fileEditorTabs').querySelector('[role=tab]').click();finishSave();await wait(()=>!t.$('fileEditorClose').disabled,'saved in background');
 [...t.$('fileEditorTabs').querySelectorAll('[role=tab]')].find(node=>node.title===files[2].Path).click();assert.equal(editor.getText(),'continued new '+text);assert(!t.$('fileEditorSave').disabled,'typing while saving retains its newer dirty draft');
 t.$('fileEditorFind').value='continued';t.$('fileEditorReplacement').value='kept';t.$('fileEditorFindNext').click();t.$('fileEditorReplace').click();assert(editor.getText().startsWith('kept new '),'find/replace must read live content, not a stale mirror');
 t.$('fileEditorMinimize').click();assert(t.$('fileEditorDockLabel').textContent.includes('1 未保存'));t.$('fileEditorDock').click();assert(editor.getText().startsWith('kept new '));
 t.$('fileEditorClose').click();t.$('fileEditorDiscardConfirm').click();assert.equal(t.d.querySelectorAll('.cm-editor').length,0);assert.equal(t.$('fileEditorTabs').children.length,0);assert(t.$('fileEditorDock').hidden);assert.equal(t.errors.length,0,t.errors.map(String).join('\n'));treeObserver.disconnect();t.dom.window.close();
 console.log('PASS: 1.68 MB documents, stable tabs, zero per-key full-text writes/tree scans, indexed caret, clean-content recognition, lazy save/find snapshots, background-save isolation and minimize/close cleanup');
})().catch(error=>{console.error(error);process.exitCode=1});
