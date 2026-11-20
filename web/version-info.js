// Shared by the toolbox and the owner's Explorer; guests have no version API.
(() => {
  const $=id=>document.getElementById(id),entry=$('openVersionInfo');
  if(!entry)return;
  const releases='https://github.com/sizhaiwu/hamster-toolbox/releases';
  const dialog=document.createElement('dialog');dialog.id='versionInfoDialog';dialog.setAttribute('aria-labelledby','versionInfoTitle');
  dialog.innerHTML='<div class="version-heading"><h2 id="versionInfoTitle">关于与更新</h2><button id="closeVersionInfo" type="button">关闭</button></div><dl class="version-facts"><div><dt>当前版本</dt><dd id="versionCurrent"></dd></div><div id="versionBuildRow" hidden><dt>构建时间</dt><dd id="versionBuild"></dd></div><div><dt>最新正式版</dt><dd id="versionLatest">尚未检查</dd></div></dl><p id="versionCheckStatus" role="status"></p><p class="version-note">仅检查版本，不会自动下载或替换文件。更新前请完成任务并退出工具箱。</p><div class="version-actions"><button id="checkVersionUpdate" class="primary" type="button">检查更新</button><a id="versionReleaseLink" href="'+releases+'" target="_blank" rel="noopener noreferrer">打开发布页 ↗</a></div>';
  document.body.append(dialog);
  let api,busy=false,initialized=false;
  const label=value=>{if(!value||value==='dev')return '开发版';const version=value.replace(/^[vV]/,'');return 'v'+version+(version.split('+')[0].includes('-')?'（开发版）':'')};
  function render(state){
    $('versionLatest').textContent=state.Latest||({unpublished:'暂无正式版',unavailable:'检查失败'})[state.State]||'暂未获取';
    $('versionCheckStatus').textContent=state.Message||'暂时无法获取版本信息，请打开发布页查看。';
    $('versionInfoDialog').dataset.state=state.State||'unavailable';
    $('versionBadge').hidden=!state.Available;$('versionBadge').textContent=state.Available?'有新版':'';
    entry.title=state.Available?'发现 '+state.Latest+'，点击查看更新':'查看当前版本、最新正式版与更新入口';
    $('versionReleaseLink').href=releases;
  }
  async function check(force=false){
    if(busy||!api)return;busy=true;$('checkVersionUpdate').disabled=true;$('versionCheckStatus').textContent='正在检查 GitHub 正式版本…';
    try{render(await api('version-update',{Force:force}))}
    catch{render({State:'unavailable',Message:'暂时无法检查更新，请检查网络或打开发布页查看。'})}
    finally{busy=false;$('checkVersionUpdate').disabled=false}
  }
  entry.onclick=()=>dialog.showModal();$('closeVersionInfo').onclick=()=>dialog.close();
  dialog.addEventListener('close',()=>entry.focus());$('checkVersionUpdate').onclick=()=>check(true);
  window.toolboxVersion={label,init(info,request){
    if(initialized)return;initialized=true;api=request;
    $('versionCurrent').textContent=label(info.Version);($('appVersion')||$('sidebarVersion')).textContent=label(info.Version);
    const built=info.BuildTime&&info.BuildTime!=='unknown'?info.BuildTime:'';
    $('versionBuildRow').hidden=!built;$('versionBuild').textContent=built;
    entry.hidden=false;const links=entry.closest('[data-version-links]');if(links)links.hidden=false;check();
  }};
})();
