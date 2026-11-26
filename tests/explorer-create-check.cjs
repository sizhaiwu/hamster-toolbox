const assert=require('node:assert/strict');
const {setup,wait}=require('./explorer-check.cjs');
const response=value=>({ok:true,status:200,json:async()=>value});
(async()=>{
 let reject=false,hold;
 const t=setup(true,{mutateHook:({body})=>{
  if(body.Action!=='create')return null;
  if(reject)return {ok:false,status:400,text:async()=>'目标已存在，不会覆盖'};
  return new Promise(resolve=>hold=()=>resolve(response({OK:true,Path:body.Path+'\\'+body.Name,Kind:'text'})));
 },requestHook:({route})=>route==='text-open'?response({Text:'',Revision:'empty',Encoding:'UTF-8',Newline:'LF'}):null});
 await wait(()=>t.$('entries').children.length,'initial');assert(t.$('newFile').disabled);
 t.$('location').value='E:\\demo';t.$('go').click();await wait(()=>!t.$('newFile').disabled,'directory ready');
 t.$('moreActions').querySelector('summary').click();t.$('newFile').click();assert.equal(t.$('nameTitle').textContent,'新建文件');assert.equal(t.$('newName').value,'新建文件.txt');
 t.$('newName').value='new.json';t.$('newName').dispatchEvent(new t.w.KeyboardEvent('keydown',{key:'Enter',bubbles:true,cancelable:true}));await wait(()=>hold,'create accepted');assert(t.$('nameSave').disabled);t.$('nameSave').click();assert.equal(t.calls.filter(c=>c.route==='mutate'&&c.body.Action==='create').length,1);hold();await wait(()=>t.$('fileEditor').open&&!t.$('fileEditorContent').disabled,'new text editor');
 assert.equal(t.$('fileEditorName').textContent,'new.json');assert.equal(t.$('fileEditorContent').value,'');const request=t.calls.find(c=>c.route==='mutate'&&c.body.Action==='create').body;assert.equal(request.Path,'E:\\demo');assert.equal(request.Version,'');t.$('fileEditorClose').click();
 reject=true;t.$('newFile').click();t.$('newName').value='existing.txt';t.$('nameSave').click();await wait(()=>!t.$('nameSave').disabled&&t.$('nameError').textContent.includes('不会覆盖'),'duplicate error');assert(t.$('nameDialog').open);assert.equal(t.$('newName').value,'existing.txt');t.$('nameCancel').click();
 t.d.getElementById('entries').dispatchEvent(new t.w.MouseEvent('contextmenu',{bubbles:true,clientX:250,clientY:250}));const menu=[...t.$('contextMenu').querySelectorAll('button')].find(b=>b.textContent==='新建文件');assert(menu);assert(menu.querySelector('svg'));menu.click();assert(t.$('nameDialog').open);t.$('nameCancel').click();assert.equal(t.errors.length,0,t.errors.map(String).join('\n'));t.dom.window.close();
 for(const writable of [false,true]){const g=setup(false,{roots:[{ID:'share-id',Name:'Demo',Writable:writable}]});await wait(()=>g.$('entries').children.length,'guest');assert.equal(g.$('newFile').disabled,!writable);g.dom.window.close()}
 console.log('PASS: new file menu/icon, computer-root and guest permissions, Enter submission, duplicate-submit lock, selected directory, editor integration and duplicate-error retention');
})().catch(error=>{console.error(error);process.exit(1)});
