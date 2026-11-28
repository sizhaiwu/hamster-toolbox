const assert=require('node:assert/strict'),p=require('../desktop-client/policy.cjs');
const token='a'.repeat(48),address='http://127.0.0.1:18787/?session='+token;
assert.equal(p.launchArguments(['--toolbox-url='+address,'--toolbox-profile=E:\\test']).origin,'http://127.0.0.1:18787');
for(const url of ['https://127.0.0.1:18787/?session='+token,'http://localhost:18787/?session='+token,'http://127.0.0.1.evil:18787/?session='+token,'file:///test','http://u:p@127.0.0.1:18787/?session='+token])assert.throws(()=>p.launchArguments(['--toolbox-url='+url,'--toolbox-profile=E:\\test']));
assert.throws(()=>p.launchArguments(['--toolbox-url='+address]));
assert(p.navigationAllowed('http://127.0.0.1:18787/notes/',new URL(address).origin));
for(const url of ['file:///etc/passwd','javascript:alert(1)','http://127.0.0.1:9999/','https://evil.example'])assert(!p.navigationAllowed(url,new URL(address).origin));
assert(p.externalAllowed('https://github.com/sizhaiwu/hamster-toolbox'));
for(const url of ['file:///test','javascript:alert(1)','ms-settings:','http://127.0.0.1:1234/','http://u:p@evil.example'])assert(!p.externalAllowed(url));
const settings=p.webPreferences();assert.equal(settings.nodeIntegration,false);assert.equal(settings.contextIsolation,true);assert.equal(settings.sandbox,true);assert.equal(settings.webSecurity,true);assert.equal(settings.webviewTag,false);assert(!settings.preload);
for(const permission of ['clipboard-sanitized-write','fullscreen']){assert(p.permissionAllowed(permission,new URL(address).origin,new URL(address).origin));assert(!p.permissionAllowed(permission,'https://evil.example',new URL(address).origin))}
for(const permission of ['media','notifications','geolocation','clipboard-read','unknown'])assert(!p.permissionAllowed(permission,address,new URL(address).origin));
console.log('PASS desktop client policy: exact loopback launch, same-origin navigation/clipboard-write/fullscreen, safe external URLs, sandbox/context isolation and no Node/preload access');

for(const [destination,route]of Object.entries({toolbox:'/',files:'/files/',notes:'/notes/',schedules:'/schedules.html',settings:'/'})){
 const u=new URL(p.trayAddress(address+'&tool=image#old',destination));assert.equal(u.origin,new URL(address).origin);assert.equal(u.pathname,route);assert.equal(u.searchParams.get('session'),token);assert.equal(u.searchParams.size,1);assert.equal(u.hash,destination==='settings'?'#settings':'');
}
assert.throws(()=>p.trayAddress(address,'https://example.com'));assert.throws(()=>p.trayAddress('https://example.com/?session='+token,'toolbox'));assert.throws(()=>p.trayAddress('http://127.0.0.1:1234/','toolbox'));
