'use strict';
const {contextBridge,ipcRenderer,webUtils}=require('electron');
// Each operation has its own fixed channel. Never expose ipcRenderer or Node APIs.
if(process.isMainFrame){
 let bootstrap={};
 try{bootstrap=ipcRenderer.sendSync('hamster:desktop-bootstrap')||{};if(bootstrap.settings?.restoreWorkspace&&bootstrap.workspace?.tabs)sessionStorage.setItem('hamster.files.tabs.v1.local',JSON.stringify(bootstrap.workspace.tabs))}catch{}
 const call=(action,value)=>ipcRenderer.invoke('hamster:desktop',action,value);
 contextBridge.exposeInMainWorld('hamsterDesktop',{
  initialWorkspace:bootstrap.workspace||{},
  mediaSettings:()=>ipcRenderer.invoke('hamster:media','settings'),saveMediaSettings:value=>ipcRenderer.invoke('hamster:media','settings-save',value),
  openMedia:value=>ipcRenderer.invoke('hamster:media','open',value),pinMedia:value=>ipcRenderer.invoke('hamster:media','pin',value),
  mediaReady:()=>ipcRenderer.invoke('hamster:media','ready'),
  getMediaHistory:value=>ipcRenderer.invoke('hamster:media','history-get',value),setMediaHistory:value=>ipcRenderer.invoke('hamster:media','history-set',value),clearMediaHistory:()=>ipcRenderer.invoke('hamster:media','history-clear'),focusMedia:()=>ipcRenderer.invoke('hamster:media','focus'),
  settings:()=>call('settings'),saveSettings:value=>call('settings-save',value),
  zoom:value=>call('zoom',value),find:(text,forward=true)=>call('find',{text,forward}),stopFind:()=>call('find-stop'),
  droppedPaths:files=>call('drop-paths',Array.from(files).map(file=>webUtils.getPathForFile(file)).filter(Boolean)),
  workspace:state=>call('workspace',state),restoreWorkspace:()=>call('workspace-get'),
  onChange:callback=>{const listener=(_event,state)=>callback(state);ipcRenderer.on('hamster:desktop-state',listener);return()=>ipcRenderer.removeListener('hamster:desktop-state',listener)},
  onCommand:callback=>{const listener=(_event,command)=>callback(command);ipcRenderer.on('hamster:desktop-command',listener);return()=>ipcRenderer.removeListener('hamster:desktop-command',listener)}
 });
}
