const assert=require('node:assert/strict');
const {setup,wait}=require('./explorer-check.cjs');
const go=async(t,path)=>{t.$('location').value=path;t.$('go').click();await wait(()=>t.$('location').value===path&&!t.$('newFolder').disabled,'directory ready')};

(async()=>{
 let release;
 const t=setup(true,{listHook:({body})=>{
  if(body.Path==='E:\\slow')return new Promise(resolve=>{release=()=>resolve({ok:true,json:async()=>({Path:'E:\\slow',Parent:'E:\\',Entries:[],Total:0,Offset:0})})});
  if(body.Path==='E:\\fast')return {ok:true,json:async()=>({Path:'E:\\fast',Parent:'E:\\',Entries:[],Total:201,Offset:body.Offset||0})};
 }});await wait(()=>t.$('entries').children.length===1,'initial locations');await go(t,'E:\\source');t.$('selectMode').click();t.d.querySelectorAll('.entry-check input')[1].click();t.$('move').click();await wait(()=>!t.$('moveSave').disabled,'first move target');
 t.$('movePath').value='E:\\slow';t.$('movePath').dispatchEvent(new t.w.Event('input'));assert.equal(t.$('moveSave').disabled,true,'typed target requires confirmation');t.$('moveGo').click();await wait(()=>Boolean(release),'slow move listing');assert.equal(t.$('moveSave').disabled,true,'must not move while a new target is loading');
 t.$('movePath').value='E:\\fast';t.$('moveGo').click();await wait(()=>t.$('movePath').value==='E:\\fast'&&!t.$('moveSave').disabled,'new target ready');release();await new Promise(resolve=>setTimeout(resolve,20));assert.equal(t.$('movePath').value,'E:\\fast','old response cannot replace the chosen target');assert.equal(t.$('moveNext').disabled,false);t.$('moveNext').click();await wait(()=>t.$('movePage').textContent==='2 / 2','move pagination');
 t.$('moveSave').click();await wait(()=>!t.$('moveDialog').open&&t.$('notice').textContent.includes('传输完成'),'move finished');const call=t.calls.find(call=>call.route==='transfer'&&call.body.Action==='move');assert(call.body.Target.startsWith('E:\\fast\\'));assert(t.calls.filter(call=>call.route==='list'&&call.body.DirectoriesOnly).length>=3);assert.equal(t.errors.length,0);t.dom.window.close();

 for(const action of ['move','delete']){
  const failure=setup(true,{mutateHook:()=>({ok:false,status:400,text:async()=>'fixture operation failed'})});await wait(()=>failure.$('entries').children.length===1,'failure fixture');await go(failure,'E:\\source');failure.$('selectMode').click();failure.d.querySelectorAll('.entry-check input')[1].click();failure.$(action==='move'?'move':'remove').click();if(action==='move')await wait(()=>!failure.$('moveSave').disabled,'failure move target');failure.$(action==='move'?'moveSave':'deleteSave').click();
  await wait(()=>!failure.$(action==='move'?'moveDialog':'deleteDialog').open&&failure.$('notice').textContent.includes('fixture operation failed'),'failed operation returns to refreshed directory');assert(failure.$('notice').textContent.includes('未全部完成'));assert.equal(failure.d.querySelectorAll('.entry-check input:checked').length,0);assert.equal(failure.errors.length,0);failure.dom.window.close();
 }
 console.log('PASS: move target stale-response isolation, loading lock, folder pagination and safe recovery from failed move/delete');
})().catch(error=>{console.error(error);process.exitCode=1});
