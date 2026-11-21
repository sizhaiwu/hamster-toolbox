const assert=require('node:assert/strict');
const {setup,wait,activateEntry}=require('./explorer-check.cjs');
const key=(t,k,extra={})=>{const e=new t.w.KeyboardEvent('keydown',{key:k,bubbles:true,cancelable:true,...extra});(t.d.activeElement||t.d).dispatchEvent(e);return e};
async function ready(t){await wait(()=>t.$('entries').children.length,'initial');t.$('location').value='E:\\demo';t.$('go').click();await wait(()=>!t.$('newFolder').disabled,'directory')}
(async()=>{
 const t=setup(true,{listHook:({body})=>body.Path?.endsWith('相册')?{ok:true,status:200,json:async()=>({Path:body.Path,Parent:body.Path.slice(0,body.Path.lastIndexOf(String.fromCharCode(92))),Entries:[],Total:0})}:null});await ready(t);const opens=()=>[...t.d.querySelectorAll('.entry-open')];
 opens()[1].click();opens()[1].focus();assert(key(t,'F2').defaultPrevented);assert(t.$('nameDialog').open);assert.equal(t.$('newName').value,'演示.jpg');assert.equal(t.$('newName').selectionEnd,2);t.$('nameCancel').click();opens()[1].focus();
 key(t,'Delete');assert(t.$('deleteDialog').open);assert.equal(t.$('permanent').checked,false);assert(!t.calls.some(c=>c.route==='mutate'));t.$('deleteCancel').click();opens()[1].focus();key(t,'Delete',{repeat:true});assert(!t.$('deleteDialog').open);
 for(const input of [t.$('location'),t.$('search')]){input.focus();for(const k of ['F2','Delete','Backspace','ArrowUp'])assert(!key(t,k).defaultPrevented);assert(!t.$('deleteDialog').open)}
 opens()[1].focus();key(t,'Home');assert(t.$('entries').children[0].classList.contains('selected'));key(t,'ArrowDown');assert(t.$('entries').children[1].classList.contains('selected'));key(t,'ArrowDown',{shiftKey:true});assert.equal(t.d.querySelectorAll('.entry.selected').length,2);assert([...t.d.querySelectorAll('.entry-check')].every(x=>x.hidden));
 key(t,'End');assert(t.$('entries').children[3].classList.contains('selected'));
 [...t.$('entries').children].forEach((c,i)=>c.getBoundingClientRect=()=>({left:i%2*160,right:i%2*160+150,top:Math.floor(i/2)*100,bottom:Math.floor(i/2)*100+90,width:150,height:90}));
 opens()[1].click();opens()[1].focus();key(t,'ArrowDown');assert(t.$('entries').children[3].classList.contains('selected'));key(t,'ArrowUp');assert(t.$('entries').children[1].classList.contains('selected'));
 t.$('view').click();opens()[1].click();opens()[1].focus();key(t,'ArrowDown');assert(t.$('entries').children[2].classList.contains('selected'));
 t.$('deleteDialog').showModal();opens()[2].focus();key(t,'F2');assert(!t.$('nameDialog').open);t.$('deleteDialog').close();
 opens()[0].click();opens()[0].focus();key(t,'Enter');await wait(()=>t.$('location').value.endsWith('相册')&&!t.$('newFolder').disabled,'child');t.d.body.focus();key(t,'Backspace');await wait(()=>t.$('location').value==='E:\\demo'&&!t.$('newFolder').disabled,'backspace history');
 await wait(()=>t.d.activeElement===opens()[0],'restored keyboard focus');assert(t.$('entries').children[0].classList.contains('selected'));assert.equal(t.d.querySelectorAll('.entry.selected').length,1);key(t,'ArrowDown');assert(t.$('entries').children[1].classList.contains('selected'),'continue from restored item');
 const listed=t.calls.filter(c=>c.route==='list').length;t.d.body.focus();assert(key(t,'F5').defaultPrevented);await wait(()=>t.calls.filter(c=>c.route==='list').length>listed&&!t.$('newFolder').disabled,'F5 refresh');assert(t.calls.filter(c=>c.route==='list').at(-1).body.Force);
 opens()[0].click();opens()[0].focus();key(t,'Enter');await wait(()=>t.$('location').value.endsWith('相册')&&!t.$('newFolder').disabled,'child again');t.d.body.focus();assert(key(t,'ArrowUp',{altKey:true}).defaultPrevented);await wait(()=>t.$('location').value==='E:\\demo'&&!t.$('newFolder').disabled,'Alt Up parent');
 await wait(()=>t.d.activeElement===opens()[0],'parent focus restoration');assert(t.$('entries').children[0].classList.contains('selected'));
 const firstTab=t.d.querySelector('#fileTabs [role=tab]').id;t.$('newTab').click();await wait(()=>t.d.querySelectorAll('#fileTabs [role=tab]').length===2&&!t.$('newFolder').disabled,'new tab');t.d.getElementById(firstTab).click();await wait(()=>t.d.activeElement===opens()[0],'selection survives tab switch');assert(t.$('entries').children[0].classList.contains('selected'));
 assert(!/directory-selection|fixture-version|\"selection\"/.test(t.w.sessionStorage.getItem('hamster.files.tabs.v1.local')),'selection stays in memory');
 t.entries.shift();key(t,'F5');await wait(()=>t.$('entries').children.length===3&&!t.$('newFolder').disabled,'removed entry refresh');assert.equal(t.d.querySelectorAll('.entry.selected').length,0,'removed paths cannot select another item');
 t.dom.window.close();
 const guest=setup(false);await wait(()=>guest.$('entries').children.length,'guest');const file=guest.d.querySelectorAll('.entry-open')[1];file.click();file.focus();key(guest,'F2');key(guest,'Delete');assert(!guest.$('nameDialog').open&&!guest.$('deleteDialog').open);assert(!guest.calls.some(c=>c.route==='mutate'));guest.dom.window.close();
 console.log('PASS: F2 stem selection, delete confirmation, input/modal isolation, list/grid arrows, range selection without checkboxes, history back and read-only permissions');
})().catch(e=>{console.error(e);process.exit(1)});
