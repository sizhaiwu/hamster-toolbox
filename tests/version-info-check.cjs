const assert=require('node:assert/strict');
const {createUI,until,setState}=require('./ui-check.cjs');
const {setup,wait}=require('./explorer-check.cjs');
const ok=value=>({ok:true,json:async()=>({...value})});
(async()=>{
 setState({Status:'idle'});let calls=[],failure=false,pending;
 let state={Latest:'v0.2.1',State:'development',Available:false,Message:'你正在使用开发版；公开正式版较旧，无需降级。',URL:'https://github.com/sizhaiwu/hamster-toolbox/releases/tag/v0.2.1'};
 const dom=createUI(17401,true,390,w=>{const base=w.fetch;w.fetch=(url,options)=>{
  if(url==='/api/info')return Promise.resolve(ok({Version:'0.2.2-dev',BuildTime:'synthetic-build'}));
  if(url==='/api/version-update'){calls.push(JSON.parse(options.body));if(failure)return Promise.reject(Error('offline'));if(pending)return new Promise(resolve=>{pending.finish=()=>resolve(ok(state))});return Promise.resolve(ok(state));}
  return base(url,options);
 }});
 const w=dom.window,$=id=>w.document.getElementById(id);
 await until(()=>!$('openVersionInfo').hidden&&$('versionLatest').textContent==='v0.2.1','background version check');
 assert.equal($('sidebarVersion').textContent,'v0.2.2-dev（开发版）');assert.equal($('versionBadge').hidden,true);assert.equal($('versionInfoDialog').open,false,'no unsolicited modal');
 assert($('openVersionInfo').closest('footer'),'toolbox About entry is in the page footer');assert(!$('openVersionInfo').closest('aside'),'footer version no longer consumes sidebar space');assert($('appVersion').closest('#openVersionInfo'),'version and About share one entry');
 assert(w.document.querySelector('footer .github-link'),'repository link is beside footer version');assert(!w.document.querySelector('aside .github-link'),'repository link no longer consumes sidebar space');
 const navIcons=[...$('nav').querySelectorAll('button[data-tool]')];assert.equal(navIcons.length,12);for(const button of navIcons){const icon=button.querySelector('svg');assert(icon,'each tool has a vector icon');assert.equal(icon.dataset.icon,button.dataset.tool);assert.equal(icon.getAttribute('aria-hidden'),'true');assert(button.title,'tool description remains available')}
 assert.equal(calls.length,1);assert.equal(calls[0].Force,false);$('openVersionInfo').click();assert($('versionInfoDialog').open);assert.match($('versionCurrent').textContent,/开发版/);assert.match($('versionCheckStatus').textContent,/无需降级/);assert.equal($('versionBuild').textContent,'synthetic-build');
 state={Latest:'v0.3.0',State:'available',Available:true,Message:'发现新版',URL:'https://github.com/sizhaiwu/hamster-toolbox/releases/tag/v0.3.0'};
 $('checkVersionUpdate').click();await until(()=>$('versionLatest').textContent==='v0.3.0','new release');assert.equal(calls.at(-1).Force,true);assert(!$('versionBadge').hidden);assert.equal($('versionReleaseLink').href,'https://github.com/sizhaiwu/hamster-toolbox/releases');
 failure=true;$('checkVersionUpdate').click();await until(()=>$('versionLatest').textContent==='检查失败','offline');assert.match($('versionCheckStatus').textContent,/暂时无法/);assert(!/已是最新/.test($('versionCheckStatus').textContent));assert(!$('checkVersionUpdate').disabled);failure=false;
 state={Latest:'<img src=x onerror=alert(1)>',State:'current',Available:false,Message:'<script>bad</script>',URL:'javascript:alert(1)'};$('checkVersionUpdate').click();await until(()=>$('versionLatest').textContent===state.Latest,'safe text');assert.equal($('versionInfoDialog').querySelectorAll('img,script').length,0);assert.equal($('versionReleaseLink').href,'https://github.com/sizhaiwu/hamster-toolbox/releases');
 pending={};$('checkVersionUpdate').click();await until(()=>!!pending.finish,'slow check');$('checkVersionUpdate').click();assert($('checkVersionUpdate').disabled);$('closeVersionInfo').click();state={State:'unpublished',Message:'仓库尚未发布正式版本。'};pending.finish();pending=null;await until(()=>$('versionLatest').textContent==='暂无正式版','late response');assert(!$('versionInfoDialog').open);assert.equal(w.document.activeElement,$('openVersionInfo'));dom.window.close();
 const local=setup(true,{globalFeedback:true});await wait(()=>!local.$('openVersionInfo').hidden&&local.$('versionLatest').textContent==='v0.2.1','local explorer version');assert.match(local.$('sidebarVersion').textContent,/开发版/);assert(local.$('openVersionInfo').closest('footer'),'Explorer version entry also moves to page footer');assert(!local.$('openVersionInfo').closest('aside'),'Explorer sidebar no longer contains About');assert(!local.d.querySelector('[data-version-links]').hidden,'owner footer links are visible');local.$('openVersionInfo').click();assert(local.$('versionInfoDialog').open);assert.equal(local.errors.length,0);local.dom.window.close();
 const guest=setup(false,{globalFeedback:true});await wait(()=>guest.$('entries').children.length>0,'guest ready');assert(guest.$('openVersionInfo').hidden);assert(guest.d.querySelector('[data-version-links]').hidden,'guest does not expose owner footer controls');assert(!guest.calls.some(c=>c.route==='version-update'||c.route==='info'));guest.dom.window.close();
 console.log('PASS: visible current/development version, background/manual checks, newer release, safe links/text, offline/no-release states, no task interruption or unsolicited modal, close/focus and guest isolation');
})().catch(error=>{console.error(error);process.exit(1)});
