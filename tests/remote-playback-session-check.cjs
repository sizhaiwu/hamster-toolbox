const assert=require('node:assert/strict'),fs=require('node:fs'),{JSDOM}=require('jsdom');
const {setup,wait,activateEntry,contextActions}=require('./explorer-check.cjs');
(async()=>{
 const t=setup(true,{remoteAdmin:true,globalFeedback:true,generalSettings:{DefaultPlayer:'mpv'},extraEntries:[{Name:'sample.mp4',Path:'E:/demo/sample.mp4',Kind:'video',Size:200}]});
 await wait(()=>t.$('entries').children.length,'remote admin ready');assert(t.$('nativePlayer').hidden);assert(t.$('openLocal').hidden);assert.equal(t.$('modeLabel').textContent,'远程文件');
 t.$('location').value='E:/demo';t.$('go').click();await wait(()=>!t.$('newFolder').disabled,'remote file permissions retained');activateEntry(t.d.querySelectorAll('.entry-open')[4]);assert(t.$('viewer').open,'host default mpv must not start on phone');assert(t.$('viewerBody').querySelector('video'));assert(t.$('viewerNative').hidden);t.$('viewerClose').click();
 t.d.querySelectorAll('.entry-more')[4].click();assert(!contextActions(t).some(button=>/mpv|其他播放器|仓鼠预览窗口/.test(button.textContent)));assert(!t.calls.some(c=>['native-player','open-local','open-with'].includes(c.route)));assert.equal(await t.w.openNativeMedia({Kind:'video',Path:'E:/demo/sample.mp4'}),false);assert.equal(t.errors.length,0,t.errors.join('\n'));t.dom.window.close();
 const dom=new JSDOM(fs.readFileSync('web/index.html','utf8'),{url:'http://localhost/?session=remote-test',runScripts:'outside-only'}),w=dom.window,d=w.document;
 w.HTMLDialogElement.prototype.showModal=function(){this.open=true};
 w.fetch=async url=>({ok:false,status:401,headers:{get:()=>String(url).includes('/api/notes/')?'':'expired-admin'}});
 w.eval(fs.readFileSync('web/remote-workspace.js','utf8'));await w.fetch('/api/notes/list');assert(!d.getElementById('remoteSessionExpired'),'note lock must not be mistaken for admin expiry');
 await w.fetch('/files/api/list');assert(d.getElementById('remoteSessionExpired').open);assert.equal(d.querySelector('#remoteSessionExpired a').getAttribute('href'),'/admin/?reason=expired');assert.match(d.getElementById('remoteSessionExpired').textContent,/管理员会话已过期/);await w.fetch('/api/state');assert.equal(d.querySelectorAll('#remoteSessionExpired').length,1);dom.window.close();
 console.log('PASS: remote admin uses browser playback despite host mpv preference, no host launch controls/requests, exact expired-admin signal and one re-login dialog');
})().catch(error=>{console.error(error);process.exit(1)});
