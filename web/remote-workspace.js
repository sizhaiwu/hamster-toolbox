'use strict';
(()=>{
 const token=new URLSearchParams(location.search).get('session')||'';
 const panel=document.createElement('details');panel.className='remote-workspace-menu';panel.id='remoteSessionMenu';
 const summary=document.createElement('summary');summary.textContent='远程会话';summary.title='查看远程会话与退出登录';
 const content=document.createElement('div');content.className='remote-session-content';
 const label=document.createElement('p');label.textContent='远程管理 · 文件与任务位于服务端电脑';label.setAttribute('role','status');
 const logout=document.createElement('button');logout.type='button';logout.textContent='退出远程';content.append(label,logout);panel.append(summary,content);
 const settings=document.getElementById('openGeneralSettings');
 if(settings?.closest('aside'))settings.after(panel);
 else if(document.querySelector('.head-actions'))document.querySelector('.head-actions').append(panel);
 else document.body.append(panel);
 document.body.classList.add('remote-workspace');
 let expired=false,checkTimer;
 function sessionExpired(){
  if(expired)return;expired=true;clearInterval(checkTimer);
  const dialog=document.createElement('dialog');dialog.className='remote-expired-dialog';dialog.id='remoteSessionExpired';dialog.setAttribute('role','alertdialog');dialog.setAttribute('aria-labelledby','remoteExpiredTitle');
  const title=document.createElement('h2');title.id='remoteExpiredTitle';title.textContent='管理员会话已过期';
  const message=document.createElement('p');message.textContent='请重新登录后继续。已开始的工具任务会继续运行。';
  const link=document.createElement('a');link.href='/admin/?reason=expired';link.textContent='重新登录';link.className='remote-login-again';
  dialog.append(title,message,link);dialog.addEventListener('cancel',event=>event.preventDefault());document.body.append(dialog);dialog.showModal();
 }
 window.toolboxRemoteSessionExpired=sessionExpired;
 const originalFetch=window.fetch.bind(window);
 window.fetch=async(...args)=>{const response=await originalFetch(...args);if(response.status===401&&response.headers?.get('X-Hamster-Session')==='expired-admin')sessionExpired();return response;};
 const check=async()=>{try{const response=await originalFetch('/admin/status',{method:'POST',credentials:'same-origin',headers:{'X-Toolbox-Token':token,'Content-Type':'application/json'},body:'{}'});if(response.status===401||response.status===404)sessionExpired()}catch{}};
 checkTimer=setInterval(check,15000);window.addEventListener('pagehide',()=>clearInterval(checkTimer));window.addEventListener('focus',()=>{if(!expired)void check()});
 document.addEventListener('pointerdown',event=>{if(panel.open&&!panel.contains(event.target))panel.open=false});
 panel.addEventListener('keydown',event=>{if(event.key==='Escape'){event.preventDefault();event.stopPropagation();panel.open=false;summary.focus()}});
 logout.onclick=async()=>{
  if(!confirm('退出远程工作台？请先确认笔记显示“已加密保存”。已开始的工具任务会继续运行。'))return;
  logout.disabled=true;
  try{const response=await fetch('/admin/logout',{method:'POST',credentials:'same-origin',headers:{'X-Toolbox-Token':token,'Content-Type':'application/json'},body:'{}'});if(!response.ok&&response.status!==401)throw Error((await response.text()).trim());location.replace('/admin/');}
  catch(error){label.textContent=error.message||'退出失败，请重试';logout.disabled=false}
 };
})();
