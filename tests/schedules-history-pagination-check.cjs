const assert=require('node:assert/strict'),fs=require('node:fs'),{JSDOM}=require('jsdom');
const html=fs.readFileSync('web/schedules.html','utf8'),script=fs.readFileSync('web/schedules.js','utf8');
const flush=()=>new Promise(resolve=>setTimeout(resolve,15));
function setup(saved,blocked=false){
 const dom=new JSDOM(html,{url:'http://localhost/schedules.html?session=test',runScripts:'outside-only',pretendToBeVisual:true}),w=dom.window,d=w.document;
 w.HTMLDialogElement.prototype.showModal=function(){this.open=true};w.HTMLDialogElement.prototype.close=function(){this.open=false;this.dispatchEvent(new w.Event('close'))};
 let runs=Array.from({length:585},(_,i)=>({ID:'r'+i,JobID:'j'+i,Name:i===20?'<img src=x onerror=alert(1)>':'任务 '+i,Status:'success',Started:'2026-11-27T09:00:00Z'})),poll;
 const requests=[];w.setInterval=callback=>{poll=callback;return 1};
 if(saved)w.localStorage.setItem('hamster.schedules.history-page-size.v1',saved);
 if(blocked)Object.defineProperty(w,'localStorage',{get(){throw Error('storage blocked')}});
 w.fetch=async(url,options)=>{const q=JSON.parse(options.body);requests.push(q);return{ok:true,json:async()=>q.Action==='list'?{Jobs:[],Runs:runs,Platform:'windows'}:runs.find(r=>r.ID===q.ID)||{Output:'test log'}}};
 w.eval(script);
 return{w,d,requests,rows:()=>[...d.querySelectorAll('#runs .run')],setRuns:value=>{runs=value},poll:()=>poll(),close:()=>w.close()};
}
(async()=>{
 const t=setup();try{
  const {w,d}=t;await flush();assert(d.querySelector('#allRuns').open);assert.equal(t.rows().length,20);assert.equal(d.querySelector('#allRunCount').textContent,'（585）');assert.equal(d.querySelector('#runPageNumber').textContent,'1 / 30');assert.match(d.querySelector('#runPageInfo').textContent,/1–20/);assert(d.querySelector('#previousRuns').disabled);assert(d.querySelector('#firstRuns').disabled);
  d.querySelector('#nextRuns').click();assert.equal(t.rows()[0].firstChild.textContent,'<img src=x onerror=alert(1)>');assert(!d.querySelector('#runs img'),'names are plain text');assert.equal(d.querySelector('#runPageNumber').textContent,'2 / 30');
  t.poll();await flush();assert.equal(d.querySelector('#runPageNumber').textContent,'2 / 30','poll preserves current page');
  t.rows()[0].click();await flush();assert(d.querySelector('#output').open);assert(t.requests.some(q=>q.Action==='detail'&&q.ID==='r20'),'selected page opens matching log');d.querySelector('#output').close();
  d.querySelector('#lastRuns').click();assert.equal(t.rows().length,5);assert.equal(d.querySelector('#runPageNumber').textContent,'30 / 30');assert(d.querySelector('#nextRuns').disabled);assert(d.querySelector('#lastRuns').disabled);
  d.querySelector('#previousRuns').click();assert.equal(t.rows().length,20);assert.equal(d.querySelector('#runPageNumber').textContent,'29 / 30');
  d.querySelector('#runPageSize').value='50';d.querySelector('#runPageSize').dispatchEvent(new w.Event('change'));assert.equal(t.rows().length,50);assert.equal(d.querySelector('#runPageNumber').textContent,'1 / 12');assert.equal(w.localStorage.getItem('hamster.schedules.history-page-size.v1'),'50');assert.equal(d.querySelector('#pageSize').value,'20','task pagination independent');
  d.querySelector('#runPageSize').value='100';d.querySelector('#runPageSize').dispatchEvent(new w.Event('change'));assert.equal(t.rows().length,100);d.querySelector('#lastRuns').click();assert.equal(t.rows().length,85);
  t.setRuns([{ID:'remaining',JobID:'j',Name:'remaining',Started:'2026-11-27T09:00:00Z',Status:'success'}]);t.poll();await flush();assert.equal(t.rows().length,1);assert.equal(d.querySelector('#runPageNumber').textContent,'1 / 1','shrinking history clamps page');
  t.setRuns([]);t.poll();await flush();assert.equal(t.rows().length,0);assert.equal(d.querySelector('#runPageInfo').textContent,'共 0 条');assert.match(d.querySelector('#runs').textContent,/执行后/);assert(d.querySelector('#previousRuns').disabled&&d.querySelector('#nextRuns').disabled);
 }finally{t.close()}
 for(const [saved,expected,blocked] of [['50',50,false],['100',100,false],['invalid',20,false],['0',20,false],[null,20,true]]){
  const t=setup(saved,blocked);try{await flush();assert.equal(t.rows().length,expected);t.d.querySelector('#runPageSize').value='50';t.d.querySelector('#runPageSize').dispatchEvent(new t.w.Event('change'));assert.equal(t.rows().length,50)}finally{t.close()}
 }
 console.log('PASS schedule history pagination: 585 runs, page sizes and persistence, boundaries, refresh, shrinking/empty history, log targeting, safe names and unavailable storage');
})().catch(error=>{console.error(error);process.exitCode=1});
