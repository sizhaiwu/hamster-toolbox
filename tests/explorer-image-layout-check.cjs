const assert=require('node:assert/strict');
const {setup,wait,activateEntry}=require('./explorer-check.cjs');
const tick=()=>new Promise(resolve=>setTimeout(resolve,0));
function geometry(img,stage,size={w:600,h:400,nw:1200,nh:800}){
 for(const [key,field] of [['naturalWidth','nw'],['naturalHeight','nh']])Object.defineProperty(img,key,{get:()=>size[field],configurable:true});
 for(const [key,field] of [['clientWidth','w'],['clientHeight','h']])Object.defineProperty(stage,key,{get:()=>size[field],configurable:true});
 return size;
}
(async()=>{
 const t=setup(true),{$,d,w}=t;await wait(()=>$('entries').children.length,'boot');$('location').value='E:\\demo';$('go').click();await wait(()=>!$('newFolder').disabled,'directory');
 activateEntry(d.querySelectorAll('.entry-open')[1]);let img=$('viewerBody').querySelector('img'),stage=img.parentElement;
 assert.equal(img.style.visibility,'hidden');assert.equal(img.style.width,'','never paint a fabricated 640x480 image');assert.equal($('zoomValue').textContent,'…');assert.equal($('imageTools').hidden,false,'controls participate in layout before first measurement');
 const footer=t.d.querySelector('.viewer-footer'),header=t.d.querySelector('.viewer-header'),originalZoom=$('zoomIn');
 assert(header.contains($('viewerTools')),'image controls share the top bar');assert(footer.hidden,'no image footer consumes viewing space');assert($('viewerImageHint').contains($('viewerHint')),'hints move into the top-bar help popover');
 $('viewerImageHelp').querySelector('summary').click();assert($('viewerImageHelp').open);$('viewerImageHelp').dispatchEvent(new w.KeyboardEvent('keydown',{key:'Escape',bubbles:true,cancelable:true}));assert(!$('viewerImageHelp').open);assert($('viewer').open,'closing help does not close preview');
 $('viewerImageHelp').querySelector('summary').click();d.dispatchEvent(new w.MouseEvent('pointerdown',{bubbles:true}));assert(!$('viewerImageHelp').open);
 w.dispatchEvent(new w.Event('resize'));assert.equal(img.style.width,'','zero intrinsic dimensions must not trigger layout');
 const size=geometry(img,stage);let resolveDecode;img.decode=()=>new Promise(resolve=>resolveDecode=resolve);img.dispatchEvent(new w.Event('load'));
 assert.equal(img.style.visibility,'hidden','load event alone must not reveal before decoding');size.h=350;w.dispatchEvent(new w.Event('resize'));assert.equal(img.style.visibility,'hidden');
 resolveDecode();await wait(()=>img.style.visibility==='visible','decoded image');assert.equal(img.style.width,'1200px');assert.equal(img.style.height,'800px');assert(img.style.transform.endsWith('scale(0.4375)'));assert.equal(stage.getAttribute('aria-busy'),'false');assert.equal($('zoomValue').textContent,'44%');
 $('zoomActual').click();assert.equal($('zoomValue').textContent,'100%');size.w=500;w.dispatchEvent(new w.Event('resize'));assert.equal($('zoomValue').textContent,'100%','resizing preserves deliberate zoom');
 $('viewerNext').click();img=$('viewerBody').querySelector('img');stage=img.parentElement;const compatible=geometry(img,stage);let rawDecode,compatibleDecode;img.decode=()=>new Promise(resolve=>{if(img.dataset.compatible)compatibleDecode=resolve;else rawDecode=resolve});img.dispatchEvent(new w.Event('load'));img.dispatchEvent(new w.Event('error'));assert.equal(img.dataset.compatible,'1');rawDecode();await tick();assert.equal(img.style.visibility,'hidden','old source decode cannot reveal a replacement');
 assert.equal($('zoomIn'),originalZoom,'switching reuses controls and handlers');assert(header.contains(originalZoom));
 compatible.nw=300;compatible.nh=600;img.dispatchEvent(new w.Event('load'));compatibleDecode();await wait(()=>img.style.visibility==='visible','compatible decode');assert.equal(img.style.width,'300px');assert.equal(img.style.height,'600px');
 $('viewerPrevious').click();const previous=$('viewerBody').querySelector('img');geometry(previous,previous.parentElement);let late;previous.decode=()=>new Promise(resolve=>late=resolve);previous.dispatchEvent(new w.Event('load'));$('viewerNext').click();assert.equal(previous.getAttribute('src'),null,'leaving releases image requests');assert.equal(previous.onload,null);late();await tick();assert.equal($('zoomValue').textContent,'…','late decode cannot update the next image');
 img=$('viewerBody').querySelector('img');geometry(img,img.parentElement);img.decode=()=>Promise.reject(Error('synthetic decode rejection'));img.dispatchEvent(new w.Event('load'));await wait(()=>img.style.visibility==='visible','valid loaded image survives decode API rejection');
 $('viewerClose').click();assert.equal(img.getAttribute('src'),null);assert(footer.contains($('viewerTools')));assert(footer.contains($('viewerHint')));assert($('viewerImageActions').hidden);assert(!$('viewerImageHelp').open);
 activateEntry(d.querySelectorAll('.entry-open')[3]);await wait(()=>$('viewerBody').querySelector('pre'),'text preview');assert(footer.contains($('viewerTools')));assert(!footer.hidden,'text status stays available');$('viewerClose').click();
 assert.equal(t.errors.length,0);t.dom.window.close();
 console.log('PASS: intrinsic-size-only first paint, decode/layout readiness, compatible-source and stale-decode isolation, zoom preservation and image request release');
})().catch(e=>{console.error(e);process.exitCode=1;setTimeout(()=>process.exit(1),100)});
