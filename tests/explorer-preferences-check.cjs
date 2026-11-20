const assert=require('node:assert/strict');
const {setup,wait}=require('./explorer-check.cjs');
const ready=async t=>wait(()=>t.$('entries').children.length>0,'browser initialized');
const enter=async(t,path)=>{t.$('location').value=path;t.$('go').click();await wait(()=>t.$('location').value===path&&!t.$('newFolder').disabled,'directory entered')};
const legacy={
 'hamster.files.favorites':JSON.stringify(['E:\\legacy']),
 'hamster.files.view':JSON.stringify({view:'list',tileSize:290})
};
(async()=>{
 const store={value:null};let active=0,maxActive=0;
 const first=setup(true,{url:'http://localhost:18001/files/?session=first',preferencesStore:store,preferenceHook:async()=>{active++;maxActive=Math.max(active,maxActive);await new Promise(r=>setTimeout(r,15));active--;}});
 await ready(first);await enter(first,'E:\\demo');first.$('favorite').click();
 await wait(()=>store.value?.favorites.includes('E:\\demo')&&first.$('notice').textContent.includes('已添加到快速访问'),'favorite acknowledged by server');
 assert(first.$('notice').textContent.includes('重启后仍会保留'));
 first.$('tileSize').value='320';first.$('tileSize').dispatchEvent(new first.w.Event('input'));first.$('tileSize').dispatchEvent(new first.w.Event('change'));
 first.$('view').click();first.$('view').click();first.$('view').click();
 await wait(()=>store.value?.tileSize===320&&store.value.view==='list'&&first.calls.filter(c=>c.route==='preferences').length===5&&active===0,'rapid changes saved in order');
 assert.equal(maxActive,1,'preference writes must be serialized');
 assert.deepEqual(store.value.favorites,['E:\\demo']);first.dom.window.close();

 // A fresh browser origin has no local storage, but receives owner settings on boot.
 const restarted=setup(true,{url:'http://localhost:18002/files/?session=second',preferencesStore:store,storage:legacy});await ready(restarted);
 assert.equal(restarted.$('favorites').querySelector('.location-name').textContent,'demo');assert.equal(restarted.$('tileSize').value,'320');assert(restarted.$('entries').classList.contains('list'));
 assert.equal(restarted.calls.filter(c=>c.route==='preferences').length,0,'stale browser cache must not replace server settings');
 await enter(restarted,'E:\\demo');assert(restarted.$('favorite').disabled,'adding an existing quick access location does not remove it');restarted.$('favorites').querySelector('.quick-access-more').click();[...restarted.$('contextMenu').children].find(b=>b.textContent==='从快速访问取消固定').click();await wait(()=>store.value.favorites.length===0&&!restarted.$('favorite').disabled,'favorite removed');restarted.dom.window.close();
 const empty=setup(true,{url:'http://localhost:18003/files/?session=third',preferencesStore:store,storage:legacy});await ready(empty);assert.equal(empty.$('favorites').children.length,0,'removed favorites must not reappear from stale cache');assert.equal(empty.calls.filter(c=>c.route==='preferences').length,0);empty.dom.window.close();
 console.log('PASS: favorites and display settings survive changed ports; ordered partial saves and intentional removals');

 const migratedStore={value:null};const migrated=setup(true,{preferencesStore:migratedStore,storage:legacy});await ready(migrated);
 assert.deepEqual(migratedStore.value.favorites,['E:\\legacy']);assert.equal(migratedStore.value.tileSize,290);assert.equal(migratedStore.value.view,'list');assert.equal(migrated.calls.filter(c=>c.route==='preferences').length,1);migrated.dom.window.close();
 const failed=setup(true,{preferencesStore:{value:null},preferenceHook:async()=>({ok:false,status:400,text:async()=> '数据目录无法写入'})});await ready(failed);await enter(failed,'E:\\demo');failed.$('favorite').click();await wait(()=>!failed.$('favorite').disabled&&failed.$('notice').classList.contains('error'),'failed save is visible');assert.equal(failed.$('favorites').children.length,0,'failed favorite must roll back');assert(failed.$('notice').textContent.includes('保存失败'));assert(failed.$('notice').textContent.includes('数据目录无法写入'));assert.equal(failed.errors.length,0);failed.dom.window.close();
 const damaged=setup(true,{preferencesError:'资源管理器设置文件损坏，已保留原文件',storage:legacy});await ready(damaged);assert(damaged.$('notice').textContent.includes('设置文件损坏'));assert.equal(damaged.calls.filter(c=>c.route==='preferences').length,0,'do not automatically overwrite a damaged settings file');damaged.dom.window.close();
 console.log('PASS: legacy migration, acknowledged favorites, visible save failures and damaged-settings protection');

 const guest=setup(false,{preferencesStore:store,storage:legacy});await ready(guest);assert.equal(guest.$('favorites').children.length,0);assert.equal(guest.$('favorite').hidden,true);guest.$('view').click();guest.$('tileSize').dispatchEvent(new guest.w.Event('change'));assert.equal(guest.calls.filter(c=>c.route==='preferences').length,0,'guest settings must remain in their own browser');assert.equal(guest.errors.length,0);guest.dom.window.close();
 console.log('PASS: guest display preferences stay local and private owner favorites remain hidden');

 const gapStore={value:{version:1,view:'grid',tileSize:220,favorites:[]}},gap=setup(true,{preferencesStore:gapStore});await ready(gap);
 assert.equal(gap.$('tileGap').value,'16','old settings use default spacing');
 const listingCalls=gap.calls.filter(c=>c.route==='list').length;
 gap.$('tileGap').value='0';gap.$('tileGap').dispatchEvent(new gap.w.Event('input'));gap.$('tileGap').dispatchEvent(new gap.w.Event('change'));
 await wait(()=>gapStore.value.tileGap===0,'zero spacing saved');assert.equal(gap.$('entries').style.getPropertyValue('--tile-gap'),'0px');assert.equal(gap.$('entries').style.getPropertyValue('--tile-width'),'220px');assert.equal(gap.calls.filter(c=>c.route==='list').length,listingCalls,'spacing changes do not rescan files');gap.dom.window.close();
 const gapRestart=setup(true,{url:'http://localhost:19001/files/?session=new',preferencesStore:gapStore});await ready(gapRestart);assert.equal(gapRestart.$('tileGap').value,'0');gapRestart.$('view').click();assert(gapRestart.$('viewOptions').hidden);assert(gapRestart.$('tileGapControl').hidden);await wait(()=>gapStore.value.view==='list','view saved');gapRestart.dom.window.close();
 const guestGap=setup(false);await ready(guestGap);guestGap.$('tileGap').value='6';guestGap.$('tileGap').dispatchEvent(new guestGap.w.Event('input'));guestGap.$('tileGap').dispatchEvent(new guestGap.w.Event('change'));assert.equal(JSON.parse(guestGap.w.localStorage.getItem('hamster.files.view')).tileGap,6);assert(!guestGap.calls.some(c=>c.route==='preferences'));const guestStorage=guestGap.w.localStorage.getItem('hamster.files.view');guestGap.dom.window.close();
 const guestReload=setup(false,{storage:{'hamster.files.view':guestStorage}});await ready(guestReload);assert.equal(guestReload.$('tileGap').value,'6');assert.equal(guestReload.errors.length,0);guestReload.dom.window.close();
 console.log('PASS: independent gap, zero-spacing restart, legacy defaults, no rescans, list isolation and guest-local persistence');
})().catch(e=>{console.error(e);process.exitCode=1});
