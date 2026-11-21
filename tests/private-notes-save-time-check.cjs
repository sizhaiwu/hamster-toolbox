const assert=require('node:assert/strict');const {setup,unlock,input,wait}=require('./private-notes-check.cjs');
(async()=>{const t=setup(true);try{
 await unlock(t);const time=t.$('lastSavedAt');assert(!time.hidden);assert.equal(time.dateTime,'2026-01-01T00:00:00.000Z','load persisted server timestamp');assert(time.title.includes('2026'));assert.equal(time.getAttribute('aria-label'),time.title);
 t.data.savedAt='2026-02-03T12:34:56Z';input(t,'markdown','saved body');t.$('save').click();await wait(()=>time.dateTime==='2026-02-03T12:34:56.000Z','successful save timestamp');assert.equal(t.$('saveStatus').textContent,'已加密保存');const saved=time.textContent;
 t.data.failSave=true;input(t,'markdown','unsaved body');assert.equal(time.textContent,saved,'typing retains previous successful save');t.$('save').click();await wait(()=>t.$('saveStatus').textContent.includes('保存失败'),'failed save');assert.equal(time.dateTime,'2026-02-03T12:34:56.000Z','failure must not fabricate a timestamp');
 t.data.failSave=false;t.data.savedAt='2026-02-03T13:00:00Z';t.$('save').click();await wait(()=>time.dateTime==='2026-02-03T13:00:00.000Z','retry success');t.$('lock').click();await wait(()=>t.$('workspace').hidden,'lock');assert(time.hidden);assert.equal(time.textContent,'');assert.equal(time.getAttribute('datetime'),null);
 console.log('PASS: persisted save time, successful save/retry update, unsaved/failed retention and clearing on vault lock');
 }finally{t.dom.window.close()}})().catch(e=>{console.error(e);process.exitCode=1});
