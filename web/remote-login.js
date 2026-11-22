'use strict';
(()=>{
 const field=document.getElementById('adminPassword'),submit=document.getElementById('adminLogin'),message=document.getElementById('adminError'),show=document.getElementById('adminShow');
 if(new URLSearchParams(location.search).get('reason')==='expired')message.textContent='管理员会话已过期，请重新登录。';
 show.onclick=()=>{const hidden=field.dataset.hidden==='true';field.dataset.hidden=String(!hidden);show.textContent=hidden?'隐藏':'显示'};
 submit.onclick=async()=>{
  if(submit.disabled)return;submit.disabled=true;message.textContent='';
  try{const response=await fetch('/admin/login',{method:'POST',credentials:'same-origin',cache:'no-store',headers:{'Content-Type':'application/json','X-Hamster-Browser':'1'},body:JSON.stringify({Password:field.value})});if(!response.ok)throw Error((await response.text()).trim());const result=await response.json();field.value='';location.replace(result.URL);}
  catch(error){message.textContent=error.message||'连接失败，请重试'}finally{submit.disabled=false}
 };
 field.addEventListener('keydown',event=>{if(event.key==='Enter'){event.preventDefault();submit.click()}});
})();
