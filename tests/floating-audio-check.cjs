const assert=require('node:assert/strict');
const {setup,wait,contextActions}=require('./explorer-check.cjs');
const clips=[{Name:'audio-one.mp4',Path:'E:/demo/audio-one.mp4',Kind:'video',Size:200},{Name:'audio-two.mp4',Path:'E:/demo/audio-two.mp4',Kind:'video',Size:200}];
async function fixture(options={}){
 const t=setup(true,{extraEntries:clips,...options});await wait(()=>t.$('entries').children.length,'ready');t.$('location').value='E:/demo';t.$('go').click();await wait(()=>!t.$('newFolder').disabled,'folder');
 // JSDOM does not enqueue the native volumechange event after a muted assignment.
 const property=Object.getOwnPropertyDescriptor(t.w.HTMLMediaElement.prototype,'muted');Object.defineProperty(t.w.HTMLMediaElement.prototype,'muted',{get(){return property.get.call(this)},set(value){property.set.call(this,value);this.dispatchEvent(new t.w.Event('volumechange'))},configurable:true});
 return t;
}
function open(t,index=4){t.$('entries').children[index].querySelector('.entry-more').click();contextActions(t).find(button=>button.textContent==='浏览器悬浮播放').click();const node=[...t.d.querySelectorAll('.floating-window')].at(-1);return{node,media:node.querySelector('video'),sound:node.querySelector('.floating-enable-sound')}}
(async()=>{
 const normal=await fixture();let a=open(normal);assert(!a.media.muted);assert(a.sound.hidden);assert(a.media.controls);a.node.querySelector('[aria-label="关闭悬浮窗口"]').click();assert.equal(normal.errors.length,0);normal.dom.window.close();
 const configured=await fixture();configured.w.toolboxSettings={BrowserMuted:true};a=open(configured);const b=open(configured,5);assert(a.media.muted&&b.media.muted);assert(!a.sound.hidden&&!b.sound.hidden);a.media.currentTime=23;a.sound.click();assert(!a.media.muted&&!a.media.defaultMuted&&!a.media.hasAttribute('muted'));assert(a.sound.hidden);assert(b.media.muted&&!b.sound.hidden,'one window cannot change another window sound');assert.equal(a.media.currentTime,23);a.media.volume=0;a.media.dispatchEvent(new configured.w.Event('volumechange'));assert(!a.sound.hidden);a.sound.click();assert.equal(a.media.volume,1);assert(a.sound.hidden);assert.equal(configured.errors.length,0);configured.dom.window.close();
 let gesture=false;
 const blocked=await fixture({playHook(media,w){if(!media.muted&&!gesture)return Promise.reject(new w.DOMException('audible autoplay denied','NotAllowedError'));Object.defineProperty(media,'paused',{value:false,writable:true,configurable:true});media.dispatchEvent(new w.Event('play'));return Promise.resolve()}});a=open(blocked);await wait(()=>a.media.muted&&!a.sound.hidden,'muted policy fallback with visible action');assert.equal(a.media.testPlayCalls,2);gesture=true;a.sound.click();assert(!a.media.muted&&a.sound.hidden);assert.equal(a.media.testPlayCalls,3);assert.equal(blocked.errors.length,0);blocked.dom.window.close();
 const denied=await fixture({playHook(media,w){return Promise.reject(new w.DOMException('all autoplay denied','NotAllowedError'))}});a=open(denied);await wait(()=>a.node.textContent.includes('请点击播放器的播放按钮'),'manual play guidance');assert.equal(a.media.testPlayCalls,2);assert(!a.sound.hidden);assert.equal(denied.errors.length,0);denied.dom.window.close();
 let reject;
 const closed=await fixture({playHook:()=>new Promise((resolve,no)=>{reject=no})});a=open(closed);a.node.querySelector('[aria-label="关闭悬浮窗口"]').click();reject(new closed.w.DOMException('late denial','NotAllowedError'));await new Promise(r=>setTimeout(r,20));assert.equal(a.media.testPlayCalls,1);assert(!closed.d.querySelector('.floating-window'));assert.equal(closed.errors.length,0);closed.dom.window.close();
 console.log('PASS: audible floating playback, remembered mute setting, explicit per-window sound/zero volume, policy fallback/retry and closed-window isolation');
})().catch(error=>{console.error(error);process.exit(1)});
