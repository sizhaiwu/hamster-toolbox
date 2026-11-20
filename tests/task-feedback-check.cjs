const assert=require('node:assert/strict');
const {createUI,until,storage,requests,setState}=require('./ui-check.cjs');
const key='hamster.toolbox.task-notices.v1';
let tones=0,contexts=0;
const audio=w=>{
  w.AudioContext=class{
    constructor(){contexts++;this.state='suspended';this.currentTime=0;this.destination={}}
    async resume(){this.state='running'}
    createOscillator(){return {frequency:{value:0},connect(){},disconnect(){},start(){tones++},stop(){}}}
    createGain(){return {gain:{setValueAtTime(){},linearRampToValueAtTime(){},exponentialRampToValueAtTime(){}},connect(){},disconnect(){}}}
  };
};
const terminal=(id,status='finished',result={Success:2,Failed:0,Skipped:1})=>({PlanID:id,Tool:'image',Status:status,Total:3,Done:3,Result:result,Items:[],Logs:[]});
(async()=>{
  setState({Status:'idle'});delete storage.TaskSound;delete storage.TaskPopup;
  let dom=createUI(15200,true,1280,audio),w=dom.window,$=id=>w.document.getElementById(id);
  await until(()=>$('title').textContent&&!$('preview').disabled,'initialized');
  assert($('TaskSound').checked);assert($('TaskPopup').checked);
  $('openGeneralSettings').dispatchEvent(new w.Event('pointerdown',{bubbles:true}));$('openGeneralSettings').click();
  $('testTaskSound').click();await until(()=>tones===2,'test sound unlocked');
  assert.equal(contexts,1);$('closeGeneralSettings').click();
  for(const Status of ['scanning','ready','running']){setState({...terminal('not-finished'),Status});await w.poll();assert(!$('taskResultDialog').open)}
  setState(terminal('done'));await w.poll();assert($('taskResultDialog').open);assert.equal($('taskResultTitle').textContent,'任务完成');assert.equal($('taskResultTool').textContent,'压缩图片','result uses task tool even when another form is open');assert.match($('taskResultMessage').textContent,/成功 2 项/);assert.equal(tones,4);
  await w.poll();assert.equal(tones,4,'polling cannot repeat sound');
  // Reload before acknowledgement preserves popup, without replaying its sound.
  let notice=w.sessionStorage.getItem(key);dom.window.close();
  dom=createUI(15200,true,1280,w=>{audio(w);w.sessionStorage.setItem(key,notice)});w=dom.window;$=id=>w.document.getElementById(id);
  await until(()=>$('taskResultDialog').open,'unfinished notice restored');assert.equal(tones,4);
  $('taskResultClose').click();notice=w.sessionStorage.getItem(key);await w.poll();assert(!$('taskResultDialog').open);
  dom.window.close();dom=createUI(15201,true,390,w=>{audio(w);w.sessionStorage.setItem(key,notice)});w=dom.window;$=id=>w.document.getElementById(id);
  await until(()=>$('title').textContent&&!$('preview').disabled,'restored');assert(!$('taskResultDialog').open,'acknowledged notice stays dismissed after refresh');
  const cases=[['partial','finished',{Success:1,Failed:1},'任务完成，部分失败','warning'],['failed','finished',{Failed:2},'任务失败','error'],['skip','finished',{Skipped:3},'任务结束，全部跳过','warning'],['cancel','cancelled',{Success:1},'任务已取消','warning'],['error','error',{},'任务失败','error']];
  for(const [id,status,result,title,kind] of cases){setState({...terminal(id,status,result),Error:status==='error'?'<img src=x onerror=alert(1)>':''});await w.poll();assert.equal($('taskResultTitle').textContent,title);assert.equal($('taskResultDialog').dataset.kind,kind);assert(!$('taskResultMessage').querySelector('img'));if(status==='cancelled')assert.match($('taskResultMessage').textContent,/保留/);$('taskResultClose').click()}
  setState({Status:'error',Tool:'image',Error:'scan failed'});await w.poll();assert(!$('taskResultDialog').open,'scan failures do not masquerade as executed tasks');
  $('openGeneralSettings').click();$('TaskSound').click();await until(()=>storage.TaskSound===false&&!$('TaskSound').disabled,'sound off persisted');
  const before=tones;setState(terminal('silent'));await w.poll();assert($('taskResultDialog').open);assert($('taskSoundStatus').hidden);assert.equal(tones,before);$('taskResultClose').click();
  $('TaskPopup').click();await until(()=>storage.TaskPopup===false&&!$('TaskPopup').disabled,'popup off persisted');setState(terminal('both-disabled'));await w.poll();assert(!$('taskResultDialog').open);assert.equal(tones,before);
  $('TaskSound').click();await until(()=>storage.TaskSound===true&&!$('TaskSound').disabled,'sound only');$('testTaskSound').click();await until(()=>tones===before+2,'sound-only context initialized');setState(terminal('sound-only'));await w.poll();assert(!$('taskResultDialog').open);assert.equal(tones,before+4);
  // Failed settings saves roll the control back and preserve the effective value.
  const originalFetch=w.fetch;w.fetch=async(url,options)=>url==='/api/ui-settings'?{ok:false,text:async()=> 'disk full'}:originalFetch(url,options);
  $('TaskSound').click();await until(()=>$('generalSettingsStatus').textContent.includes('disk full'),'save failure');assert($('TaskSound').checked);assert.equal(storage.TaskSound,true);w.fetch=originalFetch;
  // New page restores shared settings even when the app runs on another port.
  dom.window.close();setState({Status:'idle'});dom=createUI(15202);w=dom.window;$=id=>w.document.getElementById(id);await until(()=>$('title').textContent&&!$('preview').disabled,'settings restore');assert(!$('TaskPopup').checked);assert($('TaskSound').checked);
  $('openGeneralSettings').click();$('TaskPopup').click();await until(()=>storage.TaskPopup===true&&!$('TaskPopup').disabled,'enable popup');
  setState(terminal('no-audio'));await w.poll();assert($('taskResultDialog').open);assert(!$('taskSoundStatus').hidden,'blocked browser sound has visible explanation');$('taskResultClose').click();
  // Storage failures do not stop tasks or cause repeated notifications in the same page.
  Object.defineProperty(w,'sessionStorage',{get(){throw Error('storage denied')}});setState(terminal('storage-failure'));await w.poll();$('taskResultClose').click();await w.poll();assert(!$('taskResultDialog').open);
  assert(requests.some(r=>r.path==='ui-settings'&&r.body.TaskSound===false));dom.window.close();
  console.log('PASS: task result states, fast finish/reload, acknowledgement dedupe, sound unlocking/preview/mute, persistent independent settings, failed saves and blocked browser storage/audio');
})().catch(error=>{console.error(error);process.exit(1)});
