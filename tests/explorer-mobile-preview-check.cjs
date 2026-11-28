const assert=require('node:assert/strict');
const {setup,wait,activateEntry}=require('./explorer-check.cjs');
(async()=>{
 const t=setup(true,{prepare(w){w.innerWidth=390},extraEntries:[{Name:'长文件名用于手机预览测试.mp4',Path:'E:\\demo\\sample.mp4',Kind:'video',Size:20000}]}),{$,d,w}=t;
 try{
  await wait(()=>$('entries').children.length,'boot');$('location').value='E:\\demo';$('go').click();await wait(()=>!$('newFolder').disabled,'directory');
  activateEntry(d.querySelectorAll('.entry-open')[1]);
  const more=$('viewerMore'),panel=$('viewerMorePanel'),heading=d.querySelector('.viewer-heading'),download=$('viewerDownload'),zoom=$('zoomIn');
  assert.equal(heading.children.length,1,'only a single-line title remains in the phone heading');
  assert(panel.contains($('viewerInfo')));assert(panel.contains(download));assert(panel.contains(zoom));assert.equal($('viewerMoreName').textContent,'演示.jpg');
  more.open=true;d.dispatchEvent(new w.MouseEvent('pointerdown',{bubbles:true}));assert(!more.open);assert($('viewer').open);
  more.open=true;more.dispatchEvent(new w.KeyboardEvent('keydown',{key:'Escape',bubbles:true,cancelable:true}));assert(!more.open);assert($('viewer').open);
  more.open=true;$('viewerQueueToggle').click();assert(!more.open);assert(!$('viewerQueue').hidden);
  more.open=true;w.innerWidth=1100;w.dispatchEvent(new w.Event('resize'));assert(!more.open);
  assert(heading.contains($('viewerInfo')));assert.equal(download.parentElement,d.querySelector('.viewer-header'));assert.equal(zoom,$('zoomIn'),'responsive layout preserves original handlers');assert(!panel.contains(zoom));
  w.innerWidth=320;w.dispatchEvent(new w.Event('resize'));assert(panel.contains(zoom));
  $('viewerNext').click();assert.equal($('viewerMoreName').textContent,'第二张.png');assert(!more.open);
  $('viewerClose').click();activateEntry(d.querySelectorAll('.entry-open')[4]);await wait(()=>$('viewerBody').querySelector('video'),'video');
  assert(panel.contains($('videoTools')));more.open=true;$('viewerVideoOptions').click();assert(!more.open);assert(!$('videoPreviewOptions').hidden);
  $('viewerClose').click();activateEntry(d.querySelectorAll('.entry-open')[3]);await wait(()=>$('viewerBody').querySelector('pre')?.textContent.includes('script'),'text');assert(panel.contains($('viewerTextActions')));assert(!$('viewerTextActions').hidden);
  w.innerWidth=1024;w.dispatchEvent(new w.Event('resize'));assert.equal($('viewerTextActions').parentElement,d.querySelector('.viewer-header'));
  assert.equal(t.errors.length,0,String(t.errors));
  console.log('PASS compact mobile preview: shared actions retain identity, file information, outside/Escape/action dismissal, responsive restoration, image/video/text switching');
 }finally{if($('viewer').open)$('viewerClose').click();await new Promise(resolve=>setTimeout(resolve,0));w.close()}
 // iPhone Chrome reserves space in browser chrome; Safari and fullscreen do not
 // use this workaround. Changing display mode must update an already open viewer.
 for(const [ua,standalone,expected] of [['iPhone CriOS/145.0 Mobile Safari/604.1',false,true],['iPhone Version/26.0 Mobile Safari/604.1',false,false],['Android Chrome/145.0 Mobile Safari/537.36',false,false],['iPhone CriOS/145.0 Mobile Safari/604.1',true,false]]){
  const c=setup(true,{prepare(win){win.innerWidth=390;Object.defineProperty(win.navigator,'userAgent',{value:ua});Object.defineProperty(win.navigator,'standalone',{value:standalone})}});
  try{
   await wait(()=>c.$('entries').children.length,'safe-area boot');const viewer=c.$('viewer');assert.equal(viewer.classList.contains('ios-chrome-browser'),expected,ua+' standalone='+standalone);
   Object.defineProperty(c.d,'fullscreenElement',{value:viewer,configurable:true});c.d.dispatchEvent(new c.w.Event('fullscreenchange'));assert(!viewer.classList.contains('ios-chrome-browser'),'fullscreen must retain the notch inset');
   Object.defineProperty(c.d,'fullscreenElement',{value:null,configurable:true});c.d.dispatchEvent(new c.w.Event('fullscreenchange'));assert.equal(viewer.classList.contains('ios-chrome-browser'),expected,'return from fullscreen restores browser policy');assert.equal(c.errors.length,0,String(c.errors));
  }finally{c.w.close()}
 }
 console.log('PASS iOS preview safe area: Chrome browser avoids duplicate top inset; Safari, Android, standalone and fullscreen retain their own layout');
})().catch(error=>{console.error(error);process.exitCode=1});
