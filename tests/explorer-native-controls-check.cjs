const assert=require('node:assert/strict');
const {setup,wait}=require('./explorer-check.cjs');
(async()=>{
 const video={ID:'video',Name:'example.mp4',Kind:'video',Connected:true,Time:12,Duration:120,Volume:70,OnTop:true},image={ID:'image',Name:'picture.png',Kind:'image',Connected:true,Width:720,Height:500};
 let fail=false,state={Available:true,RememberWindows:true,WindowSize:'compact',Sessions:[video,image]};
 const t=setup(true,{nativePlayerHook:async({body})=>{if(body.Action==='remember-windows'){if(fail)return {ok:false,status:400,text:async()=> 'test failure'};state.RememberWindows=body.Enabled}if(body.Action==='volume')video.Volume=body.Value;if(body.Action==='seek')video.Time=body.Value;return {ok:true,json:async()=>state}}});
 await wait(()=>t.$('nativePlayerSessions').children.length===2,'native sessions');t.$('nativePlayer').click();
 const row=t.d.querySelector('[data-id=video]'),seek=row.querySelector('[data-control=seek]'),volume=row.querySelector('[data-control=volume]');assert.equal(seek.value,'12');assert.equal(volume.value,'70');
 seek.value='35';seek.dispatchEvent(new t.w.Event('input'));t.$('nativePlayerClose').click();t.$('nativePlayer').click();await new Promise(r=>setTimeout(r,0));assert.equal(row,t.d.querySelector('[data-id=video]'),'polling must preserve controls');assert.equal(seek.value,'35','polling must not reset a dragged slider');
 seek.dispatchEvent(new t.w.Event('change'));await wait(()=>t.calls.some(c=>c.body.Action==='seek'&&c.body.Value===35),'seek');volume.value='25';volume.dispatchEvent(new t.w.Event('input'));volume.dispatchEvent(new t.w.Event('change'));await wait(()=>t.calls.some(c=>c.body.Action==='volume'&&c.body.Value===25),'volume');
 for(const id of ['nativePlayerCascade','nativePlayerTile','nativePlayerPauseAll','nativePlayerMuteAll']){t.$(id).click();await new Promise(r=>setTimeout(r,0))}
 for(const action of ['cascade','tile','pause-all','mute-all'])assert(t.calls.some(c=>c.body.Action===action));
 const photo=t.d.querySelector('[data-id=image]');for(const action of ['zoom-in','zoom-out','zoom-reset']){photo.querySelector('[data-action='+action+']').click();await new Promise(r=>setTimeout(r,0));assert(t.calls.some(c=>c.body.Action===action&&c.body.ID==='image'))}assert(!photo.querySelector('input'),'pictures have no video sliders');
 t.$('nativePlayerRemember').click();await wait(()=>!state.RememberWindows&&!t.$('nativePlayerRemember').disabled,'memory disabled');fail=true;t.$('nativePlayerRemember').click();await wait(()=>t.$('nativePlayerError').textContent.includes('test failure'),'failed setting feedback');assert.equal(t.$('nativePlayerRemember').checked,false,'failed preference must roll back');
 assert.equal(t.errors.length,0);t.dom.window.close();console.log('PASS: native grouping, per-video seek/volume, image zoom, stable slider nodes and memory preference rollback');
})().catch(e=>{console.error(e);process.exitCode=1;setTimeout(()=>process.exit(1),100)});
