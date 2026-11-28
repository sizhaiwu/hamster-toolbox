(()=>{
 'use strict';
 const bridge=window.hamsterDesktop;if(!bridge)return;
 let settings={},saving=false,lastWorkspace='',workspaceTimer,findOpen=false,foreground=true;
 const node=(tag,text,cls)=>{const n=document.createElement(tag);if(text)n.textContent=text;if(cls)n.className=cls;return n};
 const button=(text,action)=>{const b=node('button',text);b.type='button';b.onclick=action;return b};

 const panel=node('section',undefined,'desktop-options');panel.id='desktopExperienceSettings';panel.append(node('strong','桌面窗口'));
 const controls={};
 function select(key,label,options){const row=node('label',label),input=node('select');input.id='desktop-'+key;for(const [v,t]of options){const option=node('option',t);option.value=v;input.append(option)}row.append(input);panel.append(row);controls[key]=input;input.onchange=()=>save({[key]:key==='zoom'?Number(input.value):input.value})}
 function toggle(key,label){const row=node('label',label),input=node('input');input.id='desktop-'+key;input.type='checkbox';row.append(input);panel.append(row);controls[key]=input;input.onchange=()=>save({[key]:input.checked})}
 select('closeAction','点击窗口关闭按钮',[['ask','每次询问'],['tray','退出到系统托盘'],['exit','退出工具箱']]);
 select('zoom','桌面界面缩放',[75,80,90,100,110,125,150,175,200].map(n=>[String(n),n+'%']));
 toggle('restoreWorkspace','启动时恢复上次工作区');toggle('notifications','后台任务系统通知');toggle('backgroundSaving','窗口隐藏后减少刷新与预加载');
 panel.append(node('small','工作区只记住页面、目录和编辑文件位置。笔记仍需解锁；未保存内容请先保存。'));
 const settingsMessage=node('small');settingsMessage.setAttribute('role','status');panel.append(settingsMessage,button('桌面快捷键',showHelp));
 function sync(value){settings=value;for(const [key,input]of Object.entries(controls)){if(input.type==='checkbox')input.checked=Boolean(value[key]);else input.value=String(value[key]);input.disabled=saving}window.toolboxDesktopBackgroundSaving=settings.backgroundSaving&&(!foreground||document.hidden)}
 async function save(change){if(saving)return;const before=settings;saving=true;sync(before);settingsMessage.textContent='正在保存…';try{sync(await bridge.saveSettings(change));settingsMessage.textContent='已自动保存'}catch(error){sync(before);settingsMessage.textContent='保存失败：'+error.message}finally{saving=false;sync(settings)}}
 let helper;
 function showHelp(){if(!helper){helper=node('dialog',undefined,'desktop-help');helper.append(node('h2','桌面快捷键'));const table=node('table');for(const [label,key]of [['放大界面','Ctrl + +'],['缩小界面','Ctrl + -'],['恢复默认缩放','Ctrl + 0'],['查找当前页面','Ctrl + F'],['刷新页面','F5 / Ctrl + R'],['全屏 / 退出全屏','F11'],['快捷键说明','F1'],['视频：暂停 / 快进退','空格 / ← →'],['视频：音量 / 静音 / 全屏','↑ ↓ / M / F'],['图片：旋转 / 适应窗口','R / Shift+R / 0'],['关闭查找栏 / 弹窗','Esc']]){const row=node('tr');row.append(node('td',label),node('td',key));table.append(row)}helper.append(table,button('关闭',()=>helper.close()));document.body.append(helper)}if(!helper.open)helper.showModal()}
 const find=node('div',undefined,'desktop-find');find.hidden=true;find.setAttribute('role','search');const term=node('input');term.type='search';term.placeholder='查找当前页面';term.setAttribute('aria-label','查找当前页面');const count=node('small','0 / 0');
 const search=forward=>{if(term.value.trim())bridge.find(term.value,forward).catch(error=>{count.textContent=error.message});else bridge.stopFind().catch(()=>{})};
 const closeFind=()=>{find.hidden=true;findOpen=false;bridge.stopFind().catch(()=>{})};
 find.append(term,count,button('上一处',()=>search(false)),button('下一处',()=>search(true)),button('关闭',closeFind));document.body.append(find);
 term.oninput=()=>search(true);term.onkeydown=event=>{if(event.key==='Enter'){event.preventDefault();search(!event.shiftKey)}if(event.key==='Escape'){event.preventDefault();event.stopPropagation();closeFind()}};
 function showFind(){const editor=document.getElementById('fileEditor');if(editor?.open){if(document.getElementById('fileEditorSearchPanel')?.hidden)document.getElementById('fileEditorSearchToggle')?.click();document.getElementById('fileEditorFind')?.focus();return}const modal=[...document.querySelectorAll('dialog[open]')].at(-1);(modal||document.body).append(find);find.hidden=false;findOpen=true;term.focus();term.select()}
 bridge.onChange(sync);bridge.onCommand(command=>{
  if(command.action==='settings'){document.getElementById('openGeneralSettings')?.click();document.getElementById('generalTabPlayback')?.click()}
  if(command.action==='find')showFind();if(command.action==='shortcuts')showHelp();
  if(command.action==='find-result')count.textContent=command.activeMatchOrdinal+' / '+command.matches;
  if(command.action==='background'){foreground=false;window.toolboxDesktopBackgroundSaving=Boolean(command.saving);window.dispatchEvent(new Event('hamster-desktop-background'));captureWorkspace()}
  if(command.action==='foreground'){foreground=true;window.toolboxDesktopBackgroundSaving=false;window.dispatchEvent(new Event('hamster-desktop-foreground'))}
 });
 document.addEventListener('keydown',event=>{if(event.key==='Escape'&&findOpen){event.preventDefault();closeFind()}},true);
 function captureWorkspace(){if(!settings.restoreWorkspace||!window.toolboxDesktopWorkspace)return;try{const value=window.toolboxDesktopWorkspace.capture();if(!value)return;const text=JSON.stringify(value);if(text!==lastWorkspace){lastWorkspace=text;bridge.workspace(value).catch(()=>{lastWorkspace=''})}}catch{}}
 function queueWorkspace(){clearTimeout(workspaceTimer);workspaceTimer=setTimeout(captureWorkspace,350)}
 window.addEventListener('hamster-desktop-workspace',queueWorkspace);window.addEventListener('blur',captureWorkspace);window.addEventListener('pagehide',captureWorkspace);
 document.addEventListener('visibilitychange',()=>{window.toolboxDesktopBackgroundSaving=settings.backgroundSaving&&document.hidden;if(!document.hidden)window.dispatchEvent(new Event('hamster-desktop-foreground'))});
 window.addEventListener('hamster-explorer-ready',()=>{window.toolboxDesktopWorkspace?.restore(bridge.initialWorkspace).catch(()=>{});queueWorkspace()},{once:true});
 // Capture is based on navigation/editor events, never on the note body.
 let dragTarget;
 const targetFor=event=>event.target.closest?.('#entries,#Root,#ArchiveExtras,#form');
 document.addEventListener('dragover',event=>{if(!event.dataTransfer?.types?.includes('Files'))return;const target=targetFor(event);if(!target)return;if(target.closest('#entries')&&!window.toolboxDesktopWorkspace)return;event.preventDefault();event.dataTransfer.dropEffect=event.shiftKey?'move':'copy';dragTarget?.classList.remove('desktop-drop-target');dragTarget=target;target.classList.add('desktop-drop-target')},true);
 document.addEventListener('dragleave',event=>{if(!event.relatedTarget){dragTarget?.classList.remove('desktop-drop-target');dragTarget=null}},true);
 document.addEventListener('drop',async event=>{dragTarget?.classList.remove('desktop-drop-target');dragTarget=null;if(!event.dataTransfer?.files?.length)return;const target=targetFor(event);if(!target)return;event.preventDefault();event.stopImmediatePropagation();const files=Array.from(event.dataTransfer.files),shiftKey=event.shiftKey;try{const items=await bridge.droppedPaths(files);if(target.closest('#entries'))window.dispatchEvent(new CustomEvent('hamster-desktop-drop',{detail:{items,shiftKey}}));else{const input=target.closest('#ArchiveExtras')||document.getElementById('Root');if(!input||input.disabled)throw Error('当前工具暂不能修改路径');if(input.id==='ArchiveExtras')input.value=[input.value,...items.filter(i=>!i.Dir).map(i=>i.Path)].filter(Boolean).join('\n');else{if(items.length!==1)throw Error('输入目录一次只接收一个文件夹或文件');const item=items[0];input.value=item.Dir?item.Path:item.Path.slice(0,Math.max(item.Path.lastIndexOf('\\'),item.Path.lastIndexOf('/'))+1)}input.dispatchEvent(new Event('input',{bubbles:true}));input.focus();settingsMessage.textContent='已填写拖入路径，请扫描预览后执行。';window.dispatchEvent(new CustomEvent('hamster-desktop-path',{detail:input.id==='ArchiveExtras'?'已加入附加文件。':'已填写输入目录；拖入文件时使用其所在目录，请扫描预览核对范围。'}))}}catch(error){showError(error.message)}},true);
 function showError(message){const d=node('dialog',undefined,'desktop-help');d.append(node('h3','拖入文件'),node('p',message),button('关闭',()=>d.close()));d.addEventListener('close',()=>d.remove(),{once:true});document.body.append(d);d.showModal()}
 bridge.settings().then(value=>{sync(value);const host=document.getElementById('generalStartup');if(host)host.append(panel);queueWorkspace()}).catch(()=>{});
})();
