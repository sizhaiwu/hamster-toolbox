const assert=require('node:assert/strict');
const {setup,wait}=require('./explorer-check.cjs');
const workspace=(paths,guest=false)=>JSON.stringify({version:1,active:'tab-0',tabs:paths.map((entry,i)=>({id:'tab-'+i,root:guest?entry.root:'',path:guest?entry.path:entry,back:[],forward:[],view:'grid'}))});
const labels=t=>[...t.d.querySelectorAll('.tab-label')].map(button=>({name:button.querySelector('.location-name').textContent,context:button.querySelector('.location-context')?.textContent||'',title:button.title}));
const favoriteLabels=t=>[...t.$('favorites').querySelectorAll('.location-button')].map(button=>({name:button.querySelector('.location-name').textContent,context:button.querySelector('.location-context')?.textContent||'',title:button.title}));
async function localCase(paths,options={}){
 const t=setup(true,{...options,preferencesStore:{value:{version:1,view:'grid',tileSize:180,favorites:paths}},workspace:workspace(paths)});
 await wait(()=>t.$('entries').children.length>0&&t.$('notice').textContent!=='正在读取目录…','saved locations ready');return t;
}
(async()=>{
 const drives=await localCase(['E:\\download','F:\\download']);
 assert.deepEqual(labels(drives).map(item=>item.context),['E:','F:']);assert.deepEqual(favoriteLabels(drives).map(item=>item.context),['E:','F:']);assert.equal(labels(drives)[0].title,'E:\\download');assert.equal(drives.$('favorites').querySelector('button.active').title,'E:\\download');
 const saved=drives.w.sessionStorage.getItem('hamster.files.tabs.v1.local');
 drives.$('favorites').querySelectorAll('.location-button')[1].click();await wait(()=>drives.$('location').value==='F:\\download'&&drives.$('notice').textContent!=='正在读取目录…','favorite navigation');assert.equal(drives.$('favorites').querySelector('button.active').title,'F:\\download');
 assert.deepEqual(labels(drives).map(item=>item.context),['',''],'two tabs at the identical location remain compact');
 // Labels derive from saved paths, so refresh keeps their meaning without migrating preferences.
 drives.dom.window.close();
 const restored=setup(true,{workspace:saved});await wait(()=>restored.$('entries').children.length>0,'restored duplicate tabs');assert.deepEqual(labels(restored).map(item=>item.context),['E:','F:']);restored.dom.window.close();

 const deeper=await localCase(['E:\\alpha\\common\\download','E:\\beta\\common\\download','F:\\download']);
 const contexts=labels(deeper).map(item=>item.context);assert.equal(new Set(contexts).size,3);assert(contexts[0].includes('alpha'));assert(contexts[1].includes('beta'));assert.equal(contexts[2],'F:');assert.deepEqual(favoriteLabels(deeper).map(item=>item.context),contexts);deeper.dom.window.close();
 const unc=await localCase(['\\\\server-a\\share\\download','\\\\server-b\\share\\download']);assert.deepEqual(labels(unc).map(item=>item.context),['\\\\server-a\\share','\\\\server-b\\share']);unc.dom.window.close();
 const unix=await localCase(['/mnt/alpha/download','/mnt/beta/download'],{platform:'linux'});assert.deepEqual(labels(unix).map(item=>item.name),['download','download']);assert(labels(unix)[0].context.includes('alpha'));assert(labels(unix)[1].context.includes('beta'));unix.dom.window.close();
 const unique=await localCase(['E:\\photos','F:\\videos']);assert.deepEqual(labels(unique).map(item=>item.context),['','']);assert.deepEqual(favoriteLabels(unique).map(item=>item.context),['','']);unique.dom.window.close();
 const alias=await localCase(['E:\\download','e:/DOWNLOAD']);assert.deepEqual(labels(alias).map(item=>item.context),['',''],'same Windows path does not need an artificial distinction');alias.dom.window.close();
 const shrinking=await localCase(['E:\\download','F:\\download']);shrinking.d.querySelectorAll('.tab-close')[1].click();assert.equal(labels(shrinking)[0].context,'','closing the other location restores a compact title');assert.equal(favoriteLabels(shrinking)[0].context,'E:','favorites are disambiguated independently');shrinking.dom.window.close();
 const roots=[{ID:'one',Name:'共享资源',Writable:false},{ID:'two',Name:'共享资源',Writable:false}];const guest=setup(false,{roots,workspace:workspace([{root:'one',path:'download'},{root:'two',path:'download'}],true)});await wait(()=>guest.$('entries').children.length>0,'guest tabs');assert.deepEqual(labels(guest).map(item=>item.context),['共享资源 1','共享资源 2']);assert(labels(guest).every(item=>!item.title.includes('E:')));assert.equal(guest.$('favorites').children.length,0);assert.equal(guest.errors.length,0);guest.dom.window.close();
 console.log('PASS: visible drive/parent labels for favorites and tabs, UNC/Unix/shared paths, alias handling and dynamic compact titles');
})().catch(error=>{console.error(error);process.exitCode=1});
