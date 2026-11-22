const assert=require('node:assert/strict'),fs=require('node:fs');
const {setup,wait,submit}=require('./private-notes-check.cjs');
const fields=['master','masterConfirm','currentPassword','newPassword','newConfirm'];
(async()=>{
 const html=fs.readFileSync('web/notes.html','utf8'),css=fs.readFileSync('web/notes.css','utf8');
 assert(!/type="password"/.test(html),'initial markup must not register native password controls in Chrome');
 assert.match(css,/\.notes-secret:not\(\[data-secret-ready=true\]\)\{visibility:hidden\}/,'fields stay hidden until masking is initialized');
 assert.match(css,/\.notes-secret\[data-secret-hidden=true\]\{-webkit-text-security:disc\}/);
 for(const visualSecretMask of [true,false]){
  const t=setup(true,{visualSecretMask});await wait(()=>!t.$('unlock').disabled,'unlock initialized');
  const secret=' Test 密码 🔐 with spaces ';
  for(const id of fields){assert.equal(t.$(id).type,visualSecretMask?'text':'password');assert.equal(t.$(id).dataset.secretReady,'true');assert.equal(t.$(id).dataset.secretHidden,'true');assert.equal(t.$(id).autocomplete,'off')}
  t.data.failUnlock=true;t.$('master').value=secret;t.$('master').setSelectionRange(1,4);
  const eye=t.d.querySelector('[data-password-for=master]');eye.click();
  assert.equal(t.$('master').type,'text');assert.equal(t.$('master').dataset.secretHidden,'false');assert.equal(t.$('master').selectionStart,1);assert.equal(t.$('master').selectionEnd,4);assert.equal(eye.getAttribute('aria-pressed'),'true');
  eye.click();assert.equal(t.$('master').dataset.secretHidden,'true');assert.equal(t.$('master').type,visualSecretMask?'text':'password');
  submit(t,'unlockForm');await wait(()=>t.$('notice').textContent==='wrong password','failed attempt retained');assert.equal(t.$('master').value,secret);
  t.data.failUnlock=false;submit(t,'unlockForm');await wait(()=>!t.$('workspace').hidden,'unlocked');
  assert.equal(t.calls.find(c=>c.action==='unlock').body.Password,secret);
  for(const id of fields){assert.equal(t.$(id).value,'');assert.equal(t.$(id).dataset.secretHidden,'true');assert.equal(t.$(id).type,visualSecretMask?'text':'password')}
  t.$('security').click();t.$('changePassword').click();t.$('newPassword').value=secret;
  const newEye=t.d.querySelector('[data-password-for=newPassword]');newEye.click();assert.equal(t.$('newPassword').dataset.secretHidden,'false');assert.equal(t.$('newConfirm').dataset.secretHidden,'true');t.$('credentialsClose').click();assert.equal(t.$('newPassword').value,'');assert.equal(t.$('newPassword').dataset.secretHidden,'true');assert.equal(newEye.getAttribute('aria-pressed'),'false');
  assert.equal(t.errors.length,0,t.errors.join('\n'));t.dom.window.close();
 }
 console.log('PASS: note password visual masking/native fallback, eye state and caret, failure retention, exact submission and credential cleanup');
})().catch(error=>{console.error(error);process.exit(1)});
