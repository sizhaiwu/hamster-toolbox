const $ = id => document.getElementById(id);
const tools = [
  ['format', '目录统计命名', '按图片、视频、音频数量与总大小，统一目录名称。'],
  ['rename', '批量重命名', '批量替换名称、添加前后缀，支持正则表达式。'],
  ['number', '目录批量编号', '给新增资源目录连续编号，可指定起始值或接着已有编号继续。'],
  ['describe', '媒体信息命名', '把音视频的分辨率、时长和体积添加到文件名。'],
  ['exif', '清理元数据', '清理图片和视频中的标题、注释、作者等元数据，保留原文件。'],
  ['pdf', 'PDF 图册索引', '每个直接子目录生成一页：中文目录名称 + 首张可读取的图片。'],
  ['image', '压缩图片', '缩小图片体积，保留原图与目录结构。'],
  ['video', '压缩视频', '按清晰度压缩视频，可选 H.264 / H.265，自动尝试显卡加速并保留原视频。'],
  ['thumbnail', '视频缩略图', '均匀截取视频画面，生成带文件信息和时间的预览拼图。'],
  ['archive', '打包资源', '使用 7-Zip 将目录与文件独立打包为 7z，支持分卷、加密、附加文件与自动校验。'],
  ['extract', '解压资源', '使用 7-Zip 批量解压 7z、ZIP、RAR、TAR 等归档和分卷包，默认自动改名并保留原包。'],
  ['dedupe', '文件去重', '使用 Video Duplicate Finder 查找重复或相似的视频和图片，支持打开本地软件与下载指引。']
];
const toolIconPaths={
  notes:['M5 3h10l4 4v14H5Z','M15 3v5h4M9 12h6M9 16h6'],
  files:['M3 6a2 2 0 0 1 2-2h5l2 2h7a2 2 0 0 1 2 2v11H3Z','M7 10h3v3H7Zm7 0h3v3h-3ZM7 16h10'],
  format:['M3 7a2 2 0 0 1 2-2h5l2 2h7a2 2 0 0 1 2 2v11H3Z','M7 17v-3m5 3v-6m5 6v-4'],
  rename:['M4 5h7M4 9h4M4 19h16','m10 13 8-8 3 3-8 8-4 1Zm6-6 3 3'],
  number:['M4 4h1v6M3 10h4M3 15a2 2 0 0 1 4 0c0 2-4 2-4 5h4','M11 7h10M11 17h10'],
  describe:['M5 3h9l5 5v13H5Z','M14 3v5h5M9 12h6M9 16h6M9 19h3'],
  exif:['M4 5h8l8 8-8 8-8-8Z','M8 9h.01m4 3 5 5m0-5-5 5'],
  pdf:['M5 3h10l4 4v14H5Z','M15 3v5h4'],
  image:['M3 3h18v18H3Z','m3 17 6-6 4 4 3-3 5 5M7 7h.01M15 4v4h4m-4 0 5-5'],
  video:['M3 5h18v14H3Z','m10 9 5 3-5 3ZM7 5v14M17 5v14M3 9h4m-4 6h4m10-6h4m-4 6h4'],
  thumbnail:['M3 4h18v16H3Z','M3 9h18M3 15h18M9 4v16M15 4v16'],
  archive:['M3 8h18v13H3ZM3 8l3-5h12l3 5','M12 10v7m-3-3 3 3 3-3'],
  extract:['M3 11v10h18V11M3 11h5m8 0h5','M12 16V3m-4 4 4-4 4 4'],
  dedupe:['M8 3h12v15M4 7h12v14H4Z','M7 12h6M7 16h6']
};
function toolNavIcon(kind){
  const svg=document.createElementNS('http://www.w3.org/2000/svg','svg');svg.setAttribute('viewBox','0 0 24 24');svg.setAttribute('width','21');svg.setAttribute('height','21');svg.setAttribute('fill','none');svg.setAttribute('stroke','currentColor');svg.setAttribute('stroke-width','1.7');svg.setAttribute('stroke-linecap','round');svg.setAttribute('stroke-linejoin','round');svg.setAttribute('aria-hidden','true');svg.setAttribute('focusable','false');svg.dataset.icon=kind;
  for(const d of toolIconPaths[kind]){const path=document.createElementNS(svg.namespaceURI,'path');path.setAttribute('d',d);svg.append(path)}
  if(kind==='pdf'){const text=document.createElementNS(svg.namespaceURI,'text');text.setAttribute('x','12');text.setAttribute('y','17');text.setAttribute('text-anchor','middle');text.setAttribute('fill','currentColor');text.setAttribute('stroke','none');text.setAttribute('font-size','5.4');text.setAttribute('font-family','Segoe UI, sans-serif');text.setAttribute('font-weight','700');text.textContent='PDF';svg.append(text)}return svg;
}
const optionKeys = {
  dedupe: ['DedupeProgram'],
  extract: ['ExtractSource','Recursive','Output','ExtractConflict','ExtractLayout','ExtractPassword','ExtractDeleteSource','ExtractDeleteMode'],
  archive: ['Output','Files','Dirs','ArchiveMode','ArchivePassword','ArchiveExtras','ArchiveMarker','ArchiveSplit','ArchiveFirstVolume','ArchiveVolume','ArchiveDirPrefix','ArchiveFilePrefix','ArchiveStart','ArchiveDigits','ArchiveLevel'],
  format: ['Recursive','FormatCleanOther','FormatDeleteMode'], rename: ['Recursive', 'Files', 'Dirs', 'Find', 'Replace', 'Regex', 'Prefix', 'Suffix', 'RenameMode', 'ExampleNumber', 'ExampleAtStart'],
  number: ['NumberName', 'StartNumber', 'NumberDigits', 'NumberStyle', 'NumberOrder', 'NumberFilter', 'NumberKeepOriginal'],
  describe: ['Recursive'], exif: ['Workers','Recursive', 'Output', 'MetadataTypes'], pdf: ['Output'],
  image: ['Workers','Recursive', 'Output', 'Quality', 'PngMode', 'PngQuality'], video: ['Workers','Recursive', 'Output', 'VideoContainer', 'VideoMode', 'VideoAcceleration', 'VideoHDR', 'CRF', 'MinMB', 'MinKbps'],
  thumbnail: ['Workers','Recursive', 'Output', 'ThumbnailColumns', 'ThumbnailRows', 'ThumbnailWidth', 'ThumbnailGap', 'ThumbnailQuality', 'ThumbnailFormat', 'ThumbnailBackground', 'ThumbnailInfo', 'ThumbnailTime']
};
const boolKeys = ['FormatCleanOther','ExtractDeleteSource','ArchiveMarker','Recursive', 'Files', 'Dirs', 'Regex', 'NumberKeepOriginal', 'ExampleNumber', 'ExampleAtStart', 'ThumbnailInfo', 'ThumbnailTime'], numericKeys = ['Workers','ArchiveStart','ArchiveDigits','ArchiveLevel','Quality', 'CRF', 'MinMB', 'MinKbps', 'StartNumber', 'NumberDigits', 'PngQuality', 'ThumbnailColumns', 'ThumbnailRows', 'ThumbnailWidth', 'ThumbnailGap', 'ThumbnailQuality'];
const imageFormatExtensions = '.jpg .jpeg .jpe .jfif .png .webp'.split(' ');
const jpegFormats = ['.jpg', '.jpeg', '.jpe', '.jfif'];
const token = new URLSearchParams(location.search).get('session');
$('openNotes').href='/notes/?session='+encodeURIComponent(token||'');
$('openFiles').href='/files/?session='+encodeURIComponent(token||'');
const statuses = { idle: '等待开始', scanning: '正在扫描', ready: '预览就绪', running: '正在执行', finished: '处理完成', cancelled: '已取消', error: '任务失败' };
let tool = 'format', defaults = {}, workspaces = {}, dependencyMetadata = [];
let busy = false, switching = true, dependenciesReady = true, checking = false, submitting = false;
let planID = '', lastConfig = '', lastLogs = '', lastItems = '', displayTool = '';
let currentPath = '', parentPath = '', browseTarget = 'Root', browseKind = '', browseRevision=0, browseConfirmed=false;
let saveTimer, checkTimer, saveRevision = 0, saveQueue = Promise.resolve(), pollPending = false, dependencyRequest = 0;
let detectingNumbering = false;
let dedupeAvailable=false, dedupeChecking=false, dedupeLaunching=false, dedupeRequest=0, dedupeTimer;
let savedPasswords = [], passwordTarget = 'ArchivePassword', passwordEditID = '', passwordLoading = false, passwordVaultBusy = false;
const passwordMemory={archive:{Supported:false,SavedID:''},extract:{Supported:false,SavedID:''}};
const passwordMemoryPending=new Map(),passwordMemoryTimers=new Map(),passwordMemoryRevisions=new Map();
let passwordMemoryQueue=Promise.resolve();
let previewPlan = '', previewOffset = 0, previewTotal = 0, previewRows = [], previewRequest = 0, previewLoading = false;
let lastActionError = '';
let taskHistory = null;
const toolPlanConfigs=new Map();
const toolPlanTasks=new Map();
let toolTaskStates=[];
let selectedTaskID='';
let followLogs = true;
let expandedTerminal = null;
const terminalFoldKey = 'hamster.terminal.fold.v1';
let terminalFoldState = {};
try { const saved=JSON.parse(localStorage.getItem(terminalFoldKey)||'{}'); if(saved && typeof saved==='object' && !Array.isArray(saved)) terminalFoldState=saved; } catch {}
function syncTerminalFold(panel) {
  const button=$(panel.id==='previewSection'?'previewCollapse':'logsCollapse');
  const name=panel.id==='previewSection'?'处理预览':'运行日志';
  const collapsed=panel.classList.contains('terminal-panel-collapsed')&&!panel.classList.contains('terminal-panel-expanded');
  button.textContent=collapsed?'展开':'收起';
  button.setAttribute('aria-expanded',String(!collapsed));
  button.setAttribute('aria-label',(collapsed?'展开':'收起')+name);
  button.title=(collapsed?'展开':'收起')+name;
}
function terminalScrollState(panel) {
  return [...panel.querySelectorAll('.table-wrap,#logs')].map(el => ({el,top:el.scrollTop,left:el.scrollLeft}));
}
function restoreTerminalScroll(positions, keepFollowing) {
  for (const {el,top,left} of positions) {
    el.scrollLeft = left;
    el.scrollTop = keepFollowing && el.id === 'logs' && followLogs ? el.scrollHeight : top;
  }
}
function exitTerminalFullscreen(restoreFocus = true) {
  if (!expandedTerminal) return;
  const state = expandedTerminal;
  const positions = state.touched ? terminalScrollState(state.panel) : state.positions;
  expandedTerminal = null;
  state.panel.classList.remove('terminal-panel-expanded');
  syncTerminalFold(state.panel);
  for (const [name,value] of state.attributes) {
    if (value === null) state.panel.removeAttribute(name); else state.panel.setAttribute(name,value);
  }
  state.placeholder.remove();
  for (const el of state.inert) el.removeAttribute('inert');
  document.body.classList.remove('terminal-view-open');
  setToolboxSidebar(toolboxSidebarCollapsed);
  state.button.textContent = '全屏';
  state.button.setAttribute('aria-pressed','false');
  state.button.setAttribute('aria-label','全屏查看'+state.name);
  state.button.title = '全屏查看'+state.name+'（Esc 退出）';
  restoreTerminalScroll(positions,true);
  window.scrollTo({left:state.x,top:state.y,behavior:'auto'});
  if (restoreFocus && !document.querySelector('dialog[open]')) state.button.focus({preventScroll:true});
}
function toggleTerminalFullscreen(panel,button,name) {
  if (expandedTerminal?.panel === panel) { exitTerminalFullscreen(); return; }
  exitTerminalFullscreen(false);
  const positions = terminalScrollState(panel);
  const x=window.scrollX,y=window.scrollY;
  const placeholder = document.createElement('div');
  placeholder.className = 'terminal-panel-placeholder';
  placeholder.style.height = panel.getBoundingClientRect().height+'px';
  placeholder.setAttribute('aria-hidden','true');
  panel.before(placeholder);
  const inert = [];
  // Preserve the original DOM and live updates. Only sibling branches become
  // inert; completion dialogs may still open above the enlarged panel.
  for (let branch=panel;branch.parentElement;branch=branch.parentElement) {
    for (const sibling of branch.parentElement.children) {
      if (sibling===branch || sibling===placeholder || /^(DIALOG|SCRIPT|STYLE|LINK)$/.test(sibling.tagName) || sibling.hasAttribute('inert')) continue;
      sibling.setAttribute('inert',''); inert.push(sibling);
    }
    if (branch.parentElement===document.body) break;
  }
  expandedTerminal = {panel,button,name,placeholder,inert,x,y,positions,touched:false,
    attributes:['role','aria-modal','aria-label'].map(key=>[key,panel.getAttribute(key)])};
  panel.classList.add('terminal-panel-expanded');
  syncTerminalFold(panel);
  panel.setAttribute('role','dialog');panel.setAttribute('aria-modal','true');panel.setAttribute('aria-label',name+' · 全屏查看');
  document.body.classList.add('terminal-view-open');
  button.textContent='退出全屏';button.setAttribute('aria-pressed','true');button.setAttribute('aria-label','退出'+name+'全屏');button.title='退出全屏（Esc）';
  restoreTerminalScroll(positions,true);
  button.focus({preventScroll:true});
}
for (const [panel,button,name] of [['previewSection','previewFullscreen','处理预览'],['logsSection','logsFullscreen','运行日志']]) {
  $(button).onclick=()=>toggleTerminalFullscreen($(panel),$(button),name);
  const section=$(panel),fold=$(panel==='previewSection'?'previewCollapse':'logsCollapse');
  section.classList.toggle('terminal-panel-collapsed',terminalFoldState[panel]===true);
  syncTerminalFold(section);
  fold.onclick=()=>{
    const collapsed=expandedTerminal?.panel===section||!section.classList.contains('terminal-panel-collapsed');
    if(expandedTerminal?.panel===section) exitTerminalFullscreen(false);
    section.classList.toggle('terminal-panel-collapsed',collapsed);
    terminalFoldState[panel]=collapsed;
    syncTerminalFold(section);
    try {localStorage.setItem(terminalFoldKey,JSON.stringify(terminalFoldState));} catch {}
    fold.focus({preventScroll:true});
  };
}
for (const type of ['wheel','touchmove','pointerdown','change','keydown']) document.addEventListener(type,event=>{
  if (!expandedTerminal || !expandedTerminal.panel.contains(event.target) || expandedTerminal.button.contains(event.target)) return;
  if (type==='keydown' && ['Escape','Tab'].includes(event.key)) return;
  expandedTerminal.touched=true;
},{capture:true,passive:true});
document.addEventListener('keydown',event=>{
  if (!expandedTerminal || document.querySelector('dialog[open]')) return;
  if (event.key==='Escape') {event.preventDefault();event.stopPropagation();exitTerminalFullscreen();return;}
  if (event.key!=='Tab') return;
  const {panel,button}=expandedTerminal;
  const controls=[...panel.querySelectorAll('button:not(:disabled),select:not(:disabled),input:not(:disabled),textarea:not(:disabled),a[href],summary,[tabindex="0"]')]
    .filter(el=>!el.closest('[hidden],[inert]') && (typeof el.checkVisibility!=='function'||el.checkVisibility()));
  const target=event.shiftKey?controls.at(-1):controls[0];
  if (!panel.contains(document.activeElement) || (event.shiftKey && document.activeElement===controls[0]) || (!event.shiftKey && document.activeElement===controls.at(-1))) {
    event.preventDefault();(target||button).focus({preventScroll:true});
  }
},true);

async function api(path, data = {}, taskTool=tool) {
  const response = await fetch('/api/' + path, { method: 'POST', headers: { 'Content-Type': 'application/json', 'X-Toolbox-Token': token, 'X-Toolbox-Tool': taskTool }, body: JSON.stringify(data) });
  if (!response.ok) throw Error((await response.text()).trim());
  return path === 'log' ? response.text() : response.json();
}
function readInput(id) { const el = $(id); return boolKeys.includes(id) ? el.checked : numericKeys.includes(id) ? Number(el.value) : el.value; }
function config() {
  if (tool === 'dedupe') return {Tool:tool,DedupeProgram:$('DedupeProgram').value};
  const result = { Tool: tool, Root: $('Root').value };
  for (const id of [...optionKeys[tool], ...dependencyMetadata.map(d => d.Key)]) result[id] = readInput(id);
  if (tool === 'number') result.NumberTemplate = $('NumberTemplate').value;
  if (tool === 'image') result.ImageFormats = selectedImageFormats();
  return result;
}
function putValue(id, value) { const el = $(id); if (!el) return; if (el.type === 'checkbox') el.checked = Boolean(value); else el.value = value ?? ''; }
function matchesTaskConfig(snapshot) {
  return !snapshot || Object.entries(config()).every(([key,value])=>['ArchivePassword','ExtractPassword'].includes(key)||JSON.stringify(value)===JSON.stringify(snapshot[key]));
}
function showRunningTaskConfig(snapshot) {
  for(const [key,value] of Object.entries(snapshot))if(!['ArchivePassword','ExtractPassword'].includes(key))putValue(key,value);
  if(tool==='image')restoreImageFormats(snapshot.ImageFormats);
  if(tool==='number')updateNumberSample();
  updateRenameMode();updateVideoPreset();updateVideoOptions();updateThumbnailOptions();updateArchiveOptions();updateExtractOptions();
  $('qualityValue').textContent=$('Quality').value;
  // A task started in another page may require a different dependency set.
  inspectDependencies(false);
}
function updateActions() {
  const locked = busy || switching || submitting || detectingNumbering || passwordLoading || dedupeLaunching;
  taskHistory?.syncUI(locked,tool,$('Root').value);
  const activeInputs = new Set(['Root', ...optionKeys[tool], ...dependencyMetadata.map(d => d.Key)]);
  if (tool === 'archive') {
    if ($('ArchiveMode').value !== 'extras') { activeInputs.delete('ArchiveExtras'); activeInputs.delete('ArchiveMarker'); }
    if ($('ArchiveSplit').value === 'none') { activeInputs.delete('ArchiveVolume'); activeInputs.delete('ArchiveFirstVolume'); }
    else if ($('ArchiveSplit').value === 'uniform') activeInputs.delete('ArchiveFirstVolume');
  }
  if (tool === 'extract' && !$('ExtractDeleteSource').checked) activeInputs.delete('ExtractDeleteMode');
  if (tool === 'format' && !$('FormatCleanOther').checked) activeInputs.delete('FormatDeleteMode');
  $('formatDeleteOptions').hidden = !$('FormatCleanOther').checked;
  if (tool === 'archive') activeInputs.add('ArchiveSavedPassword');
  if (tool === 'extract') activeInputs.add('ExtractSavedPassword');
  if(passwordMemory[tool]?.Supported)activeInputs.add((tool==='archive'?'Archive':'Extract')+'RememberPassword');
  $('clearExtractSource').disabled=locked;
  $('manageArchivePasswords').disabled = $('manageExtractPasswords').disabled = locked;
  if (tool === 'video') activeInputs.add('videoQuality');
  if (tool === 'video' && $('VideoMode').value === 'archive') activeInputs.delete('VideoAcceleration');
  if (tool === 'thumbnail' && $('ThumbnailFormat').value === 'png') activeInputs.delete('ThumbnailQuality');
  if (tool === 'image') {
    activeInputs.add('ImageFormatMode');
    const selected = selectedImageFormats();
    if (!selected.includes('.png')) { activeInputs.delete('PngMode'); activeInputs.delete('PngQuality'); }
    else if ($('PngMode').value === 'lossless') activeInputs.delete('PngQuality');
    if (!selected.some(ext => ext !== '.png')) activeInputs.delete('Quality');
    if ($('ImageFormatMode').value === 'custom') for (const input of imageFormatInputs()) activeInputs.add(input.id);
  }
  if (tool === 'rename') {
    activeInputs.delete('Regex');
    if ($('RenameMode').value === 'regex') activeInputs.add('renamePreset');
    if ($('RenameMode').value !== 'example') { activeInputs.delete('ExampleNumber'); activeInputs.delete('ExampleAtStart'); }
  }
  if (tool === 'number') {
    if ($('NumberStyle').value === 'custom') activeInputs.add('NumberTemplate');
    if ($('NumberStyle').value === 'number' || ($('NumberStyle').value === 'custom' && !$('NumberTemplate').value.includes('{name}'))) activeInputs.delete('NumberName');
  }
  if (tool === 'dedupe') activeInputs.delete('Root');
  $('DedupeProgram').disabled = $('browseDedupe').disabled = locked;
  $('detectDedupe').disabled = locked || dedupeChecking;
  $('launchDedupe').disabled = locked || dedupeChecking || !dedupeAvailable;
  $('launchDedupe').textContent = dedupeLaunching ? '正在打开…' : '打开去重软件';
  $('preview').disabled = locked || checking || !dependenciesReady || (tool === 'image' && !selectedImageFormats().length);
  $('preview').textContent = submitting ? '正在提交…' : busy ? '任务处理中…' : '扫描并预览 →';
  $('execute').disabled = locked || !planID; $('cancel').hidden = !busy;
  for (const b of $('nav').querySelectorAll('button[data-tool]')) b.disabled = switching || submitting;
  // CSS-hidden inputs still participate in native HTML form validation unless disabled.
  for (const el of $('form').querySelectorAll('input, select, textarea')) el.disabled = locked || !activeInputs.has(el.id);
  for (const el of $('form').querySelectorAll('button[data-file-key]')) el.disabled = locked;
  $('browse').disabled = locked;
  $('autoDetect').disabled = $('checkDependencies').disabled = locked || checking;
  $('checkDependencies').textContent = checking ? '正在检查…' : '检查是否可用';
  $('detectNumbering').disabled = locked;
  $('detectNumbering').textContent = detectingNumbering ? '正在读取…' : '识别并续号';
}
function showMessage(text, error = false) {
  $('message').textContent = text;
  $('message').classList.toggle('error', error);
}
let cliCommandSet = null;
$('showCLI').onclick = async () => {
  $('cliCode').value='';$('cliMessage').textContent='';$('cliNotice').textContent='正在生成命令…';$('cliAction').value='Preview';$('copyCLI').disabled=true;cliCommandSet=null;$('cliDialog').showModal();
  try {cliCommandSet=await api('cli-commands',config());$('cliNotice').textContent=cliCommandSet.Notice;$('cliCode').value=cliCommandSet.Preview;$('copyCLI').disabled=false;}
  catch(e){$('cliMessage').textContent=e.message;}
};
$('cliAction').onchange=()=>{if(cliCommandSet)$('cliCode').value=cliCommandSet[$('cliAction').value];$('cliMessage').textContent='';};
$('closeCLI').onclick=()=>{$('cliDialog').close();$('cliCode').value='';cliCommandSet=null;};
$('copyCLI').onclick=async()=>{try{await navigator.clipboard.writeText($('cliCode').value);$('cliMessage').textContent='已复制，请粘贴到本机终端执行。';}catch(e){$('cliCode').select();$('cliMessage').textContent='请按 Ctrl+C 复制选中的命令。';}};
function invalidate() { planID = ''; lastActionError = ''; showMessage(''); updateActions(); }
function setSettingsStatus(text,bad=false) { for (const id of ['settingsStatus','dedupeSettingsStatus']) {$(id).textContent=text;$(id).classList.toggle('bad',bad);} }
function queueSave(snapshot) {
  const revision = ++saveRevision; workspaces[snapshot.Tool] = { ...snapshot };
  setSettingsStatus('正在保存本工具的设置…');
  const persisted = { ...snapshot }; delete persisted.ArchivePassword; delete persisted.ExtractPassword;
  const task = saveQueue.then(() => api('save-settings', persisted)); saveQueue = task.catch(() => {});
  return task.then(() => { if (revision === saveRevision) { setSettingsStatus('本工具设置已保存 · 重启后仍会保留'); } })
    .catch(error => { setSettingsStatus('设置未能保存：' + error.message,true); });
}
function saveNow() { clearTimeout(saveTimer); return queueSave(config()); }
function changed(dependency = false) {
  updateRenameMode();
  invalidate(); clearTimeout(saveTimer); const snapshot = config(); workspaces[tool] = snapshot;
  updateNumberSample();
  updateImageFormatSummary();
  updateThumbnailOptions();
  updateArchiveOptions();
  updateExtractOptions();
  updateVideoOptions();
  saveTimer = setTimeout(() => queueSave(snapshot), 500);
  if (dependency) { dependencyRequest++; checking = false; dependenciesReady = false; clearTimeout(checkTimer); checkTimer = setTimeout(() => inspectDependencies(false), 400); updateActions(); }
}
function applyDependencyStatus(deps) {
  dependenciesReady = deps.every(d => d.Available);
  for (const d of deps) {
    const status = $('dependency-status-' + d.Key);
    if (status) { status.textContent = d.Message; status.className = 'dependency-status ' + (d.Available ? 'ok' : 'bad'); }
    const detail = $('dependency-help-' + d.Key);
    if (detail) {
      if (!d.Available) detail.open = true;
      detail.querySelector('summary').textContent = d.Available ? '更换程序 / 查看下载步骤' : '设置程序 / 查看下载步骤';
    }
  }
  const missing = deps.filter(d => !d.Available);
  $('dependencySummary').textContent = missing.length ? '还需设置：' + missing.map(d => d.Name).join('、') + '。展开查看设置与下载步骤。' : '已找到所需程序，可以开始预览。';
  $('dependencySummary').className = missing.length ? 'bad' : 'ok'; updateActions();
}
function renderDependencies(deps) {
  $('dependencies').open = false;
  dependencyMetadata = deps; $('dependencyList').replaceChildren(); $('dependencies').hidden = deps.length === 0; $('builtInReady').hidden = deps.length !== 0;
  for (const d of deps) {
    const card = document.createElement('div'); card.className = 'dependency-card';
    const heading = document.createElement('div'); heading.className = 'dependency-card-heading';
    const title = document.createElement('b'); title.textContent = d.Name;
    const status = document.createElement('span'); status.id = 'dependency-status-' + d.Key; heading.append(title, status);
    const purpose = document.createElement('p'); purpose.textContent = d.Purpose;
    const detail = document.createElement('details'); detail.id = 'dependency-help-' + d.Key; detail.open = !d.Available;
    const summary = document.createElement('summary'); summary.textContent = d.Available ? '更换程序 / 查看下载步骤' : '设置程序 / 查看下载步骤';
    const instructions = document.createElement('p'); instructions.className = 'dependency-instructions'; instructions.textContent = d.Instructions;
    const links = document.createElement('div'); links.className = 'dependency-links';
    for (const [label, url] of [['打开下载页 ↗', d.DownloadURL], ['安装说明 ↗', d.GuideURL]]) {
      const a = document.createElement('a'); a.textContent = label; a.href = url; a.target = '_blank'; a.rel = 'noopener noreferrer'; links.append(a);
    }
    const label = document.createElement('label'); label.textContent = d.Key === 'Font' ? '字体文件' : '程序文件';
    const row = document.createElement('div'); row.className = 'input-row';
    const input = document.createElement('input'); input.id = d.Key; input.value = d.Path;
    input.placeholder = d.Key === 'Font' ? '选择本机的字体文件' : '选择 ' + ({ FFmpeg: 'ffmpeg', FFprobe: 'ffprobe', Magick: 'magick', Exiftool: 'exiftool', SevenZip: '7z / 7zz' })[d.Key];
    input.addEventListener('input', () => changed(true));
    const button = document.createElement('button'); button.type = 'button'; button.textContent = '选择文件'; button.dataset.fileKey = d.Key; button.onclick = () => openPicker(d.Key);
    row.append(input, button); label.append(row); detail.append(summary, instructions, links, label); card.append(heading, purpose, detail); $('dependencyList').append(card);
  }
  applyDependencyStatus(deps);
}
async function inspectDependencies(verify) {
  clearTimeout(checkTimer); const requestID = ++dependencyRequest, snapshot = config(); checking = true; updateActions();
  try {
    const deps = await api('dependencies', { Config: snapshot, Verify: verify });
    if (requestID !== dependencyRequest || tool !== snapshot.Tool) return; applyDependencyStatus(deps);
  } catch (e) { if (requestID === dependencyRequest) { dependenciesReady = false; $('dependencySummary').textContent = '检查失败：' + e.message; $('dependencySummary').className = 'bad'; } }
  finally { if (requestID === dependencyRequest) { checking = false; updateActions(); } }
}
function updateVideoPreset() { const value = String($('CRF').value); $('videoQuality').value = ['18', '23', '28'].includes(value) ? value : 'custom'; }
const videoModeAdvice = {
  compatible: 'H.264：适合旧设备和分享，文件可能较大。',
  efficient: 'H.265：日常省空间，需播放器支持。',
  archive: 'H.265：CPU 慢速编码，适合愿意等待的存档任务。'
};
const videoAccelerationAdvice = {
  auto: '自动选择可用设备，显卡不可用时转 CPU。',
  nvidia: 'NVIDIA：显卡编码，优先处理速度；失败时转 CPU。',
  intel: 'Intel：利用核显或独显编码；失败时转 CPU。',
  amd: 'AMD：使用显卡编码；失败时转 CPU。',
  cpu: 'CPU：无需显卡编码支持，通常耗时更长、占用更高。'
};
function updateVideoOptions() {
  $('videoHDRHint').textContent = ({base:'用于带杜比信息的 HLG 视频：保留 HLG HDR，移除杜比动态效果。其他动态 HDR 仍跳过。',preserve:'HLG / HDR10 保持 HDR，使用 H.265 10-bit；含母版亮度信息时使用 CPU。',sdr:'转换亮度和色彩以适配普通屏幕；滤镜占用 CPU，建议先试压一段。',skip:'HDR 原片保留，仅压缩普通 SDR 视频。'})[$('VideoHDR').value] || '';

  if (tool !== 'video') return;
  const filters=[];
  if(Number($('MinMB').value)>0)filters.push('体积至少 '+$('MinMB').value+' MB');
  if(Number($('MinKbps').value)>0)filters.push('原码率至少 '+$('MinKbps').value+' kbps');
  $('videoFilterSummary').textContent=filters.length?'只扫描：'+filters.join('，')+'（高级设置可改）。':'扫描时不限制原文件体积和码率；格式、HDR 与安全检查仍然有效。';
  const mode = $('VideoMode').value, cpuOverride = mode === 'archive' || Number($('CRF').value) === 0;
  const acceleration = cpuOverride ? 'cpu' : $('VideoAcceleration').value;
  $('videoModeHint').textContent = videoModeAdvice[mode] || videoModeAdvice.efficient;
  $('videoAccelerationHint').textContent = cpuOverride
    ? (mode === 'archive' ? '精细模式固定使用 CPU 慢速编码。' : '画质数值为 0，固定使用 CPU 最高画质编码。')
    : videoAccelerationAdvice[acceleration] || videoAccelerationAdvice.auto;
  const matchesBenchmark = mode === 'efficient' && Number($('CRF').value) === 23;
  const reference = { nvidia: '省 28.6% · 4分01秒', intel: '省 54.7% · 7分30秒', cpu: '省 61.3% · 11分51秒' };
  $('videoBenchmarkQuick').hidden = !matchesBenchmark || !reference[acceleration];
  $('videoBenchmarkQuick').textContent = matchesBenchmark && reference[acceleration]
    ? '本组实测：' + reference[acceleration] + '（仅供参考，条件见下方）' : '';
  for (const row of document.querySelectorAll('[data-video-benchmark]')) {
    const selected = matchesBenchmark && row.dataset.videoBenchmark === acceleration;
    row.classList.toggle('is-current', selected);
    if (selected) row.setAttribute('aria-current', 'true'); else row.removeAttribute('aria-current');
  }
  $('videoBenchmarkScope').textContent = !matchesBenchmark
    ? '当前模式或清晰程度与上表不同：上表只测试了高效 H.265 的均衡档，不能将这些数字作为当前设置的预计结果。'
    : acceleration === 'auto'
    ? '自动选择没有固定的压缩率和耗时，取决于最终使用的设备。上表可帮助你了解本组样本的取舍。'
    : acceleration === 'amd'
    ? 'AMD 暂无同一批样本的实测数据，压缩率和耗时需在支持的电脑上试压确认。'
    : '已高亮当前所选方式在参考测试中的结果；实际体积与耗时仍取决于源视频、硬件、驱动和电脑负载。';
}
function updateExtractOptions() {
  if (tool !== 'extract') return;
  $('extractSourceScope').hidden=!$('ExtractSource').value; $('extractSourceName').textContent=$('ExtractSource').value;
  $('extractDeleteModeOptions').hidden = !$('ExtractDeleteSource').checked;
  $('extractDeleteHint').textContent = !$('ExtractDeleteSource').checked ? '默认保留原包；开启后可选择移入回收站或永久删除。' : $('ExtractDeleteMode').value === 'permanent' ? '完整解压、全部写出且无跳过后永久删除原包及实际分卷，不进入回收站。' : '完整解压、全部写出且无跳过后移入回收站；无法回收时保留，不会自动永久删除。';
  $('extractRuleHint').textContent = ({rename:'重名时把新文件改为“文件_1.ext”等，已有文件保留。',skip:'已有同名文件时跳过，保留原有内容。',overwrite:'已有同名文件会被替换，请确认输出位置。'})[$('ExtractConflict').value];
}
function updateArchiveOptions() {
  if (tool !== 'archive') return;
  const extras = $('ArchiveMode').value === 'extras', split = $('ArchiveSplit').value;
  $('archiveExtrasOptions').hidden = !extras;
  $('archiveVolumeOptions').hidden = split === 'none';
  $('archiveFirstVolumeOption').hidden = split !== 'script';
  const digits = Math.max(1, Math.min(9, Number($('ArchiveDigits').value) || 3));
  const n = String($('ArchiveStart').value || 1).padStart(digits, '0');
  $('archiveSummary').textContent = '文件夹 → ' + $('ArchiveDirPrefix').value + n + '.7z' + (split === 'none' ? '' : '.001') + '；文件 → ' + $('ArchiveFilePrefix').value + n + '.7z' + (split === 'none' ? '' : '.001') + '。' + (split === 'script' ? '首卷 ' + $('ArchiveFirstVolume').value + '，后续每卷 ' + $('ArchiveVolume').value + '。' : split === 'uniform' ? '每卷 ' + $('ArchiveVolume').value + '。' : '不分卷。');
}
function updateThumbnailOptions() {
  if (tool !== 'thumbnail') return;
  $('thumbnailQualityOptions').hidden = $('ThumbnailFormat').value !== 'jpg';
  $('thumbnailQualityValue').textContent = $('ThumbnailQuality').value;
  const columns = Number($('ThumbnailColumns').value), rows = Number($('ThumbnailRows').value);
  $('thumbnailSummary').textContent = '每个视频生成 1 张拼图：' + columns + ' 列 × ' + rows + ' 行，共 ' + (columns * rows) + ' 张画面，总宽 ' + $('ThumbnailWidth').value + ' 像素。按时间均匀截取，避开首尾；画面不裁剪。';
}
function imageFormatInputs() { return [...$('imageFormatCheckboxes').querySelectorAll('input')]; }
function selectedImageFormats() { return imageFormatInputs().filter(input => input.checked).map(input => input.value); }
function setImageFormats(formats) {
  const selected = (formats == null ? jpegFormats : formats).map(value => '.' + value.toLowerCase().replace(/^\./, ''));
  for (const input of imageFormatInputs()) input.checked = selected.includes(input.value);
}
function restoreImageFormats(formats) {
  setImageFormats(formats);
  const selected = selectedImageFormats();
  const same = other => selected.length === other.length && selected.every(ext => other.includes(ext));
  $('ImageFormatMode').value = same(jpegFormats) ? 'jpeg' : same(['.png']) ? 'png' : same(imageFormatExtensions) ? 'all' : 'custom';
  updateImageFormatSummary();
}
function updateImageFormatSummary() {
  if (tool !== 'image') return;
  const selected = selectedImageFormats(); $('imageFormatChoices').hidden = $('ImageFormatMode').value !== 'custom';
  $('imageFormatSummary').textContent = selected.length ? '本次仅处理：' + selected.map(ext => ext.slice(1).toUpperCase()).join('、') + '。未选格式不会进入预览或执行。' : '请至少勾选一种要压缩的图片格式。';
  $('imageFormatSummary').classList.toggle('error', !selected.length);
  updatePNGOptions();
}
function updatePNGOptions() {
  if (tool !== 'image') return;
  const selected = selectedImageFormats(), png = selected.includes('.png'), mode = $('PngMode').value || 'lossless';
  $('pngOptions').hidden = !png; $('pngQualityOptions').hidden = !png || mode === 'lossless';
  $('generalImageQuality').hidden = !selected.some(ext => ext !== '.png');
  $('generalQualityLabel').textContent = png ? '其他格式图片质量' : '图片质量';
  $('pngQualityValue').textContent = $('PngQuality').value;
  $('pngModeHint').textContent = mode === 'webp-png'
    ? '输出文件名仍是 xxx.png，内部实际是 WebP，便于按原名替换。部分按后缀解码的软件可能打不开，请先测试常用查看器。原图保留，已有同名输出会提示冲突。'
    : mode === 'webp' ? '输出为真正的 .webp 文件，分辨率保持不变。原图保留。'
    : '保持真正的 PNG 格式和原尺寸，优化压缩参数。已经充分压缩过的图片，体积可能不会明显减少。';
}
function updateRenameMode() {
  if (tool !== 'rename') return;
  const mode = $('RenameMode').value, regex = mode === 'regex', example = mode === 'example';
  $('Regex').checked = regex;
  $('renameFindLabel').textContent = example ? '要替换的示例' : regex ? '正则表达式' : '查找内容';
  $('Find').placeholder = example ? '复制一段名称，例如 test 第001期' : regex ? '例如 ^test\\s*第\\d+期\\s*' : '要替换的文字';
  $('renameModeHint').textContent = example
    ? '按示例替换：无需编写表达式。填 test 第001期，也能匹配 test 第002期、第003期等同格式名称。替换留空就是删除，填新文字就是替换。'
    : regex ? '正则模式已启用：请填写表达式，再点击“扫描并预览”。替换留空表示删除。'
    : '普通文字模式：按原样匹配查找内容。编号会变化时，选择“按示例（编号可变）”即可，不需要学习正则。';
  $('renameExampleOptions').hidden = !example;
  $('renameRegexHelp').hidden = $('renameAdvancedOptions').hidden = !regex;
}
function updateNumberSample() {
  if (tool !== 'number') return;
  $('numberTemplateOption').hidden = $('NumberStyle').value !== 'custom';
  const name = $('NumberName').value.trim();
  const width = Math.min(9, Math.max(1, Number($('NumberDigits').value) || 3));
  const number = String(Math.max(0, Number($('StartNumber').value) || 0)).padStart(width, '0');
  let formatted;
  if ($('NumberStyle').value === 'custom') formatted = ($('NumberTemplate').value || '{name} 第{number}期').replace(/\{name\}|\{number\}/g, token => token === '{name}' ? name : number);
  else if ($('NumberStyle').value === 'number') formatted = number + ($('NumberKeepOriginal').checked ? ' -' : '');
  else formatted = name ? name + ({underscore:'_',space:' ',dash:' - '})[$('NumberStyle').value] + 'NO.' + number : 'NO.' + number;
  formatted = formatted.trim();
  $('numberSample').textContent = '示例：' + formatted + ($('NumberKeepOriginal').checked ? ' 原资源名称 [统计信息]' : '');
}
async function detectNumbering(save = true) {
  if (!$('Root').value.trim()) { showMessage('请先选择资源根目录。', true); return; }
  detectingNumbering = true; updateActions();
  try {
    const info = await api('number-info', { Root: $('Root').value, NumberName: $('NumberName').value, NumberFilter: $('NumberFilter').value, NumberStyle: $('NumberStyle').value, NumberTemplate: $('NumberTemplate').value });
    putValue('NumberName', info.Name); putValue('StartNumber', info.NextNumber); putValue('NumberDigits', info.Digits); putValue('NumberStyle', info.Style);
    $('numberInfo').textContent = '识别到 ' + info.Existing + ' 个匹配规则的已编号目录，最大编号 ' + String(info.MaxNumber).padStart(info.Digits, '0') + '；待编号 ' + info.Pending + ' 个目录。起始编号已设为 ' + String(info.NextNumber).padStart(info.Digits, '0') + '，可以手动修改。';
    invalidate(); updateNumberSample(); if (save) await saveNow();
  } catch (e) { showMessage('读取编号失败：' + e.message, true); }
  finally { detectingNumbering = false; updateActions(); }
}
async function selectTool(id, initial = false) {
  exitTerminalFullscreen(false);
  if (switching && !initial) return; toolPlanConfigs.set(tool,lastConfig); switching = true; updateActions();
  let viewedTask;
  try {
    if (!initial) {await flushPasswordMemory(tool);await saveNow();} dependencyRequest++; clearTimeout(checkTimer); dedupeRequest++; clearTimeout(dedupeTimer); dedupeChecking=false;
    tool = id; busy=false; selectedTaskID=''; toolGroups.find(g=>g.tools.includes(id)).section.open=true; checking = false; planID = ''; lastConfig = toolPlanConfigs.get(id)||'';
    resetPreview();
    const t = tools.find(t => t[0] === id);
    $('title').textContent = $('crumb').textContent = t[1]; $('description').textContent = t[2]; $('tool-number').textContent = String(tools.indexOf(t) + 1).padStart(2, '0');
    for (const b of $('nav').querySelectorAll('button[data-tool]')) {const current=b.dataset.tool===id;b.classList.toggle('active',current);if(current)b.setAttribute('aria-current','page');else b.removeAttribute('aria-current');}
    closeToolboxNav();
    if(!initial)window.scrollTo({top:0,behavior:'auto'});
    $('dedupePanel').hidden = id !== 'dedupe';
    for (const key of ['batchWorkspace','previewSection','logsSection']) $(key).hidden = id === 'dedupe';
    if (id === 'dedupe') {
      dependencyMetadata=[]; dependenciesReady=true; checking=false;
      putValue('DedupeProgram',(workspaces[id] || defaults).DedupeProgram || '');
      $('dedupeMessage').textContent=''; await inspectDedupeProgram(); await queueSave(config()); return;
    }
    $('renameOptions').hidden = id !== 'rename'; $('outputOptions').hidden = !['image', 'video', 'exif', 'pdf', 'thumbnail', 'archive', 'extract'].includes(id);
    $('archiveOptions').hidden = id !== 'archive';
    $('extractOptions').hidden = id !== 'extract';
    $('formatOptions').hidden = id !== 'format';
    $('entryOptions').hidden = !['rename','archive'].includes(id);
    $('metadataOptions').hidden = id !== 'exif';
    $('thumbnailOptions').hidden = id !== 'thumbnail';
    $('imageOptions').hidden = id !== 'image'; $('videoOptions').hidden = id !== 'video'; $('numberOptions').hidden = id !== 'number'; $('recursiveOption').hidden = ['pdf', 'number', 'archive'].includes(id);
    $('outputLabel').textContent = id === 'pdf' ? '保存为 PDF 文件' : '输出目录';
	$('outputHint').textContent = id === 'pdf' ? '填写完整 PDF 文件路径，或留空自动生成。已有同名文件时会提示冲突。' : '留空即可：自动生成输出文件夹，保留原文件与目录结构。';
    if (id === 'thumbnail') $('outputHint').textContent = '留空保存到输入目录下的“视频预览图”，也可自定义目录。命名为“视频名.mp4.jpg”，保留视频扩展名，避免不同格式重名。已有结果会跳过。';
    $('Output').placeholder = id === 'pdf' ? '留空自动生成 toolbox-output/目录索引.pdf' : '留空自动输出到 toolbox-output/' + id;
    if (id === 'thumbnail') $('Output').placeholder = '留空保存到输入目录下的 视频预览图';
    if (id === 'archive') $('Output').placeholder = '留空保存到输入目录下的 0utput_7z';
    $('recursiveHint').textContent = id === 'format' ? '关闭时只改直接子目录，统计仍包含目录内全部内容' : '';
    if (id === 'archive') $('outputHint').textContent = '留空保存到输入目录下的 0utput_7z（首字符是数字 0），与原脚本一致。名称对照追加到目录.txt，同名包或分卷会提示冲突。';
    if (id === 'extract') { $('Output').placeholder = '留空保存到输入目录下的 解压资源'; $('outputHint').textContent = '默认输出到输入目录下的“解压资源”。可按包名建文件夹，也可全部解压到同一目录。'; }
    $('videoAdvanced').open = false;
    $('workerOptions').hidden=!['image','video','exif','thumbnail'].includes(id);$('Workers').max=id==='video'?'3':'8';
    putValue('Workers',workspaces[id]?.Workers|| (id==='video'?1:defaults.Workers||2));
    const active = { ...defaults, ...(workspaces[id] || {}), Tool: id };
    if (!workspaces[id] && !initial) active.Root = $('Root').value;
    const liveTask=await api('state',{Summary:true});
    viewedTask=liveTask;
    if(liveTask.Tool===id&&['scanning','running'].includes(liveTask.Status)&&liveTask.Config)Object.assign(active,liveTask.Config);
    for (const [key, value] of Object.entries(active)) putValue(key, value);
    if (id === 'rename') {
      putValue('RenameMode', active.RenameMode || (active.Regex ? 'regex' : 'text'));
      if (!active.RenameMode) { putValue('ExampleNumber', true); putValue('ExampleAtStart', true); }
    }
    if (id === 'number' && !$('NumberTemplate').value) putValue('NumberTemplate', '{name} 第{number}期');
    if (id === 'image') { putValue('PngMode', active.PngMode || 'lossless'); putValue('PngQuality', active.PngQuality || 90); restoreImageFormats(active.ImageFormats); }
    if (id === 'extract') {putValue('ExtractDeleteMode',active.ExtractDeleteMode || 'recycle');putValue('ExtractSource',active.ExtractSource||'');}
    if (id === 'exif') putValue('MetadataTypes', active.MetadataTypes || (workspaces[id] ? 'images' : 'all'));
    if (id === 'video') {
      putValue('VideoMode', active.VideoMode || (workspaces[id] ? 'compatible' : 'efficient'));
      putValue('VideoAcceleration', active.VideoAcceleration || 'auto');
      putValue('VideoHDR', active.VideoHDR || 'preserve');
      putValue('VideoContainer', active.VideoContainer || 'original');
      updateVideoOptions();
    }
    // Blank outputs on a new tool; they must not leak from another workspace.
    if (!workspaces[id] && !(liveTask.Tool===id&&['scanning','running'].includes(liveTask.Status)&&liveTask.Config)) putValue('Output', '');
    $('qualityValue').textContent = $('Quality').value; updateVideoPreset(); $('message').textContent = '';
    updateRenameMode();
    updateThumbnailOptions();
    updateArchiveOptions();
    updateExtractOptions();
    const deps = await api('dependencies', { Config: active, Verify: false }); renderDependencies(deps);
    if (id === 'number') {
      $('numberInfo').textContent = '仅处理直接子目录。选择“仅编号”即可添加 029 - 这样的前缀，无需名称；已有编号会自动跳过。';
      if (!workspaces[id] && $('Root').value.trim()) await detectNumbering(false);
      updateNumberSample();
    }
    await queueSave(config());
    if(id==='archive'||id==='extract')await loadPasswordMemory(id);
  } catch (e) { dependenciesReady = false; $('message').textContent = e.message; }
  finally { switching = false; updateActions(); await poll();if(tool===id&&viewedTask?.Tool===id&&dismissibleTaskBadge(viewedTask))window.toolboxFeedback?.markTaskRead(viewedTask); }
}
const toolGroups=[
 {id:'workspace',label:'工作空间',tools:[]},
 {id:'organize',label:'文件整理',tools:['format','rename','number','dedupe']},
 {id:'media',label:'媒体处理',tools:['describe','exif','pdf','image','video','thumbnail']},
 {id:'archive',label:'打包与解压',tools:['archive','extract']}
];
let navGroupsState={};try{navGroupsState=JSON.parse(localStorage.getItem('hamster.nav.groups.v1')||'{}')||{}}catch{}
for(const group of toolGroups){
 const section=document.createElement('details'),summary=document.createElement('summary'),body=document.createElement('div');
 section.className='tool-nav-group';section.dataset.group=group.id;summary.textContent=group.label;body.className='tool-nav-items';section.append(summary,body);section.open=typeof navGroupsState[group.id]==='boolean'?navGroupsState[group.id]:['workspace','organize'].includes(group.id);group.section=section;group.body=body;
 function persistGroup(){navGroupsState[group.id]=section.open;try{localStorage.setItem('hamster.nav.groups.v1',JSON.stringify(navGroupsState))}catch{}}
 summary.addEventListener('click',event=>{event.preventDefault();section.open=!section.open;persistGroup()});section.addEventListener('toggle',persistGroup);$('nav').append(section);
}
toolGroups[0].body.append($('openFiles'),$('openNotes'));
for (const t of tools) {
  const b = document.createElement('button'); b.type = 'button'; b.dataset.tool = t[0];
  const icon = document.createElement('span');icon.className='tool-nav-icon';icon.append(toolNavIcon(t[0]));icon.setAttribute('aria-hidden','true');b.title=t[2];const name=document.createElement('span');name.className='tool-nav-label';name.textContent=t[1];const status=document.createElement('span');status.className='tool-task-badge';status.hidden=true;status.setAttribute('aria-live','off');b.append(icon,name,status); b.onclick = () => selectTool(t[0]); toolGroups.find(g=>g.tools.includes(t[0])).body.append(b);
}
function dismissibleTaskBadge(task){return ['finished','cancelled','error'].includes(task.Status)||task.Status==='ready'&&(task.Total===0||task.Actionable===0);}
function renderToolTasks(states) {
 toolTaskStates=states;
 for(const group of toolGroups){const count=states.filter(s=>group.tools.includes(s.Tool)&&['running','scanning'].includes(s.Status)).length;let badge=group.section.querySelector('.tool-group-status');if(!badge){badge=document.createElement('span');badge.className='tool-group-status';group.section.querySelector('summary').append(badge)}badge.hidden=!count;badge.textContent=count?count+' 运行':'';}
 for(const button of $('nav').querySelectorAll('button[data-tool]')) {
  const task=states.find(s=>s.Tool===button.dataset.tool&&s.Status!=='idle');const badge=button.querySelector('.tool-task-badge');
  badge.hidden=!task;button.dataset.taskStatus=task?.Status==='finished'&&task.Result?.Failed?'error':task?.Status||'idle';
  if(task&&dismissibleTaskBadge(task)&&window.toolboxFeedback?.isTaskRead(task)){badge.hidden=true;badge.textContent='';button.dataset.taskStatus='idle';button.title=tools.find(t=>t[0]===button.dataset.tool)?.[2]||'';continue;}
  if(!task) {button.title=tools.find(t=>t[0]===button.dataset.tool)?.[2]||'';continue}
  let text=({scanning:'扫描中',ready:'待执行',finished:'已完成',cancelled:'已取消',error:'失败'})[task.Status]||'运行中';
  if(task.Status==='ready'&&task.Total===0){text='无项目';button.dataset.taskStatus='empty';}
  else if(task.Status==='ready'&&task.Actionable===0){text='全跳过';button.dataset.taskStatus='skipped';}
  if(task.Status==='finished'&&task.Result?.Failed)text=task.Result.Success?'部分失败':'失败';
  if(task.Status==='running') text=task.Tool==='video'&&task.Total===1&&task.Operation?.PercentKnown?'运行 '+Math.round(task.Operation.Percent)+'%':`${task.Done||0}/${task.Total||0}`;
  const detail=task.Status==='ready'?text+' · 共 '+(task.Total||0)+' 项'+(Number.isFinite(task.Actionable)?'，可执行 '+task.Actionable+' 项，跳过 '+(task.Conflicts||0)+' 项':''):(task.Status==='finished'?text:statuses[task.Status]||task.Status)+' · '+(task.Done||0)+'/'+(task.Total||0);
  badge.textContent=text;button.title=(tools.find(t=>t[0]===task.Tool)?.[1]||task.Tool)+' · '+detail+(task.Error?' · '+task.Error:'');
 }
}
window.addEventListener('toolbox-task-read',()=>renderToolTasks(toolTaskStates));
$('openFiles').firstElementChild.replaceChildren(toolNavIcon('files'));
for (const [index, ext] of imageFormatExtensions.entries()) {
  const label = document.createElement('label'); label.className = 'check';
  const input = document.createElement('input'); input.type = 'checkbox'; input.id = 'image-format-' + index; input.value = ext;
  label.append(input, document.createTextNode(ext.slice(1).toUpperCase())); $('imageFormatCheckboxes').append(label);
}
setImageFormats(null);
for (const el of $('form').querySelectorAll('input, textarea')) el.addEventListener('input', () => {
  if(el.id==='Root')putValue('ExtractSource','');
  $('qualityValue').textContent = $('Quality').value; if (el.id === 'CRF') updateVideoPreset(); changed();
});
for (const id of ['NumberStyle', 'NumberOrder']) $(id).addEventListener('change', () => changed());
for (const id of ['VideoMode', 'VideoAcceleration', 'VideoHDR', 'VideoContainer']) $(id).addEventListener('change', () => changed());
for (const id of ['ArchiveMode','ArchiveSplit','ArchiveLevel','ExtractConflict','ExtractLayout','ExtractDeleteMode','FormatDeleteMode']) $(id).addEventListener('change', () => changed());
$('addArchiveExtra').onclick = () => openPicker('ArchiveExtras');
// Archive secrets are not website credentials. Native password controls make
// Chrome infer a login when a scan disables fields or an AJAX request succeeds.
// Prefer visual masking; unsupported browsers keep native masking as a fallback.
const supportsSecretMask = !!window.CSS?.supports?.('-webkit-text-security', 'disc');
for (const [key,label] of [['ArchivePassword','压缩包密码'],['ExtractPassword','解压密码'],['PasswordValue','常用密码']]) {
  if (!supportsSecretMask) $(key).type = 'password';
  $('toggle'+key).onclick = () => {
    const visible = $(key).dataset.secretHidden === 'true';
    $(key).dataset.secretHidden = String(!visible);
    if (!supportsSecretMask) $(key).type = visible ? 'text' : 'password';
    $('toggle'+key).textContent = visible ? '隐藏密码' : '显示密码';
    $('toggle'+key).setAttribute('aria-pressed', String(visible));
    $('toggle'+key).setAttribute('aria-label', (visible ? '隐藏' : '显示')+label);
  };
}
async function refreshSavedPasswords() {
  savedPasswords = await api('passwords');
  for (const key of ['Archive','Extract']) {
    const select = $(key+'SavedPassword'); select.replaceChildren();
    const placeholder = document.createElement('option'); placeholder.value = ''; placeholder.textContent = savedPasswords.length ? '选择常用密码…' : '暂无常用密码，请点击管理添加'; select.append(placeholder);
    for (const choice of savedPasswords) { const option = document.createElement('option'); option.value = choice.ID; option.textContent = choice.Name; select.append(option); }
    const remembered=passwordMemory[key==='Archive'?'archive':'extract'];
    if(remembered.SavedID&&!savedPasswords.some(choice=>choice.ID===remembered.SavedID)){remembered.SavedID='';putValue(key+'Password','');if(tool===(key==='Archive'?'archive':'extract'))changed();}
    select.value=remembered.SavedID||'';
  }
  renderSavedPasswords();
}
function passwordPrefix(id){return id==='archive'?'Archive':'Extract';}
function passwordMemoryStatus(id,text,bad=false){const status=$(passwordPrefix(id)+'PasswordMemoryStatus');status.textContent=text;status.classList.toggle('bad',bad);}
async function loadPasswordMemory(id){
  const prefix=passwordPrefix(id);passwordMemoryStatus(id,'正在读取密码记忆…');
  try{
    const memory=await api('password-memory',{Tool:id},id);passwordMemory[id]={Supported:memory.Supported,Remember:memory.Remember,SavedID:memory.SavedID||''};
    $(prefix+'RememberPassword').checked=Boolean(memory.Remember);
    if(memory.Remember){putValue(prefix+'Password',memory.Password);$(prefix+'SavedPassword').value=memory.SavedID||'';}
    $(prefix+'Password').dataset.secretHidden='true';if(!supportsSecretMask)$(prefix+'Password').type='password';
    $('toggle'+prefix+'Password').textContent='显示密码';$('toggle'+prefix+'Password').setAttribute('aria-pressed','false');$('toggle'+prefix+'Password').setAttribute('aria-label','显示'+(id==='archive'?'压缩包密码':'解压密码'));
    passwordMemoryStatus(id,!memory.Supported?'当前平台暂不支持本机加密记忆，仅在本次页面保留密码。':memory.Remember?(memory.Password?'已恢复上次密码；后续修改会加密记住。':'已开启记忆，输入或选择密码后自动保存。'):'未开启记忆；临时密码不保存到磁盘。');
  }catch(error){passwordMemory[id]={Supported:false,SavedID:''};$(prefix+'RememberPassword').checked=false;passwordMemoryStatus(id,'密码记忆读取失败：'+error.message,true);}
}
function schedulePasswordMemory(id,immediate=false){
  if(!passwordMemory[id]?.Supported)return Promise.resolve();
  const prefix=passwordPrefix(id),remember=$(prefix+'RememberPassword').checked;
  if(!remember&&passwordMemory[id].Remember===false)return Promise.resolve();
  passwordMemory[id].Remember=remember;
  passwordMemoryPending.set(id,{Tool:id,Remember:remember,Password:remember?$(prefix+'Password').value:'',SavedID:remember?passwordMemory[id].SavedID||'':''});
  clearTimeout(passwordMemoryTimers.get(id));
  passwordMemoryStatus(id,'正在保存密码记忆…');
  if(immediate)return flushPasswordMemory(id);
  passwordMemoryTimers.set(id,setTimeout(()=>flushPasswordMemory(id),400));
  return Promise.resolve();
}
function flushPasswordMemory(id){
  clearTimeout(passwordMemoryTimers.get(id));passwordMemoryTimers.delete(id);
  const snapshot=passwordMemoryPending.get(id);
  if(snapshot){
    passwordMemoryPending.delete(id);const revision=(passwordMemoryRevisions.get(id)||0)+1;passwordMemoryRevisions.set(id,revision);
    passwordMemoryQueue=passwordMemoryQueue.catch(()=>{}).then(()=>api('password-memory-save',snapshot,id)).then(()=>{
      if(passwordMemoryRevisions.get(id)===revision&&!passwordMemoryPending.has(id))passwordMemoryStatus(id,!snapshot.Remember?'已关闭记忆并清除对应密码；常用密码列表保留。':snapshot.Password?'已加密记住，下次自动填入。':'已清空记忆密码。');
    }).catch(error=>{if(passwordMemoryRevisions.get(id)===revision)passwordMemoryStatus(id,'密码记忆未保存：'+error.message,true);});
  }
  return passwordMemoryQueue;
}
function setPasswordVaultBusy(value) {
  passwordVaultBusy = value;
  for (const el of $('passwordDialog').querySelectorAll('button,input')) el.disabled = value;
}
function resetPasswordEditor() {
  passwordEditID = ''; $('passwordEditorTitle').textContent = '添加常用密码'; $('PasswordName').value = ''; $('PasswordValue').value = $(passwordTarget).value;
}
async function useSavedPassword(id,key) {
  passwordLoading = true; updateActions();
  try {
    const data = await api('password-get',{ID:id}); putValue(key,data.Password);
    const target=key==='ArchivePassword'?'archive':'extract';passwordMemory[target].SavedID=id;$(passwordPrefix(target)+'SavedPassword').value=id;changed();await schedulePasswordMemory(target,true);
    if ($('passwordDialog').open) $('passwordDialog').close();
  } finally { passwordLoading = false; updateActions(); }
}
function renderSavedPasswords() {
  $('passwordList').replaceChildren();
  if (!savedPasswords.length) { const p = document.createElement('p'); p.textContent = '还没有常用密码，可在下方添加。'; $('passwordList').append(p); }
  for (const choice of savedPasswords) {
    const row = document.createElement('div'); row.className = 'saved-password-row'; const name = document.createElement('b'); name.textContent = choice.Name; row.append(name);
    for (const [text,action] of [['使用',async()=>useSavedPassword(choice.ID,passwordTarget)],['编辑',async()=>{const data=await api('password-get',{ID:choice.ID});passwordEditID=choice.ID;$('passwordEditorTitle').textContent='编辑常用密码';$('PasswordName').value=choice.Name;$('PasswordValue').value=data.Password;}],['删除',async()=>{await api('password-delete',{ID:choice.ID});await refreshSavedPasswords();if(passwordEditID===choice.ID)resetPasswordEditor();}]]) {
      const b = document.createElement('button'); b.type='button'; b.textContent=text; b.onclick=async()=>{if(passwordVaultBusy)return;setPasswordVaultBusy(true);$('passwordError').textContent='';try{await action();}catch(e){$('passwordError').textContent=e.message;}finally{setPasswordVaultBusy(false);}};row.append(b);
    }
    $('passwordList').append(row);
  }
}
for (const key of ['Archive','Extract']) {
  const target=key==='Archive'?'archive':'extract';
  $(key+'SavedPassword').onchange=async()=>{const id=$(key+'SavedPassword').value;if(!id||busy||switching)return;try{await useSavedPassword(id,key+'Password');}catch(e){showMessage(e.message,true);}};
  $(key+'Password').addEventListener('input',()=>{passwordMemory[target].SavedID='';$(key+'SavedPassword').value='';schedulePasswordMemory(target);});
  $(key+'Password').addEventListener('change',()=>flushPasswordMemory(target));
  $(key+'RememberPassword').addEventListener('change',()=>schedulePasswordMemory(target,true));
  $('manage'+key+'Passwords').onclick=async()=>{if(busy||switching)return;passwordTarget=key+'Password';resetPasswordEditor();$('passwordError').textContent='';$('passwordDialog').showModal();setPasswordVaultBusy(true);try{await refreshSavedPasswords();}catch(e){$('passwordError').textContent=e.message;}finally{setPasswordVaultBusy(false);}};
}
$('closePasswordDialog').onclick=()=>$('passwordDialog').close();
$('newPassword').onclick=()=>resetPasswordEditor();
$('savePassword').onclick=async()=>{
  if(passwordVaultBusy)return;setPasswordVaultBusy(true);$('passwordError').textContent='';
  try {const password=$('PasswordValue').value,name=$('PasswordName').value.trim();await api('password-save',{ID:passwordEditID,Name:name,Password:password});await refreshSavedPasswords();const id=savedPasswords.find(choice=>choice.Name===name)?.ID;if(id)await useSavedPassword(id,passwordTarget);else{putValue(passwordTarget,password);changed();await schedulePasswordMemory(passwordTarget==='ArchivePassword'?'archive':'extract',true);}$('passwordDialog').close();}
  catch(e){$('passwordError').textContent=e.message;}finally{setPasswordVaultBusy(false);}
};
$('passwordForm').onsubmit=event=>{event.preventDefault();$('savePassword').click();};
$('MetadataTypes').addEventListener('change', () => selectTool('exif'));
$('PngMode').addEventListener('change', () => changed());
$('ThumbnailFormat').addEventListener('change', () => changed());
$('RenameMode').addEventListener('change', () => changed());
$('ImageFormatMode').onchange = () => {
  const mode = $('ImageFormatMode').value;
  if (mode === 'jpeg') setImageFormats(jpegFormats);
  else if (mode === 'png') setImageFormats(['.png']);
  else if (mode === 'all') setImageFormats(imageFormatExtensions);
  changed();
};
$('renamePreset').onchange = () => {
  const presets = {
    periodPrefix: '^.*?第\\d+期\\s*',
    noPrefix: '(?i)^.*?NO[. _-]*\\d+\\s*',
    bracketSuffix: '\\s*\\[[^\\[\\]]*\\]$'
  };
  const pattern = presets[$('renamePreset').value]; if (!pattern) return;
  putValue('Find', pattern); putValue('Replace', ''); putValue('RenameMode', 'regex'); changed();
};
$('detectNumbering').onclick = () => detectNumbering();
$('videoQuality').onchange = () => {
  if ($('videoQuality').value === 'custom') { $('videoAdvanced').open = true; $('CRF').focus(); return; }
  $('CRF').value = $('videoQuality').value; changed();
};
$('autoDetect').onclick = async () => {
  if (busy || switching) return; checking = true; updateActions();
  try { const found = await api('defaults'); for (const d of dependencyMetadata) if (found[d.Key]) putValue(d.Key, found[d.Key]); invalidate(); await saveNow(); await inspectDependencies(false); }
  catch (e) { $('message').textContent = e.message; } finally { checking = false; updateActions(); }
};
$('checkDependencies').onclick = () => inspectDependencies(true);
async function startPreview(retryTaskID='') {
  if (tool === 'dedupe') return;
  if (busy || switching || submitting || checking || detectingNumbering) { showMessage('任务或设置正在处理中，请稍候。'); return; }
  if (!dependenciesReady) { showMessage('请先完成本工具需要的程序设置。', true); return; }
  if (tool === 'image' && !selectedImageFormats().length) { showMessage('请至少选择一种要压缩的图片格式。', true); return; }
  if (!$('Root').value.trim()) { showMessage('请先选择输入目录。', true); $('Root').focus(); return; }
  if (!$('form').checkValidity()) { showMessage('请检查当前工具的参数是否在允许范围内。', true); if (tool === 'video') $('videoAdvanced').open = true; $('form').reportValidity(); return; }
  submitting = true; planID = ''; lastActionError = ''; showMessage('正在扫描目录，请稍候…'); updateActions();
  try {
    const snapshot = config(); await flushPasswordMemory(tool);await saveNow(); lastConfig = JSON.stringify(snapshot);
    const accepted=await api('plan', retryTaskID?{...snapshot,RetryTaskID:retryTaskID}:snapshot);
    if(accepted.TaskID)toolPlanTasks.set(tool,accepted.TaskID);else toolPlanTasks.delete(tool);
    busy = true;
  } catch (e) { busy = false; lastConfig = ''; showMessage('无法开始扫描：' + e.message, true); }
  finally { submitting = false; updateActions(); }
  await poll();
};
$('preview').onclick=()=>startPreview();
$('form').onsubmit = event => { event.preventDefault(); $('preview').click(); };
$('execute').onclick = async () => {
  if (busy || submitting || !planID) return;
  submitting = true; lastActionError = ''; updateActions();
  try { if (JSON.stringify(config()) !== lastConfig) throw Error('参数已改变，请重新预览'); await api('execute', { PlanID: planID }); planID = ''; busy = true; showMessage('正在执行，请稍候…'); }
  catch (e) { lastActionError = '无法开始执行：' + e.message; showMessage(lastActionError, true); }
  finally { submitting = false; updateActions(); }
  await poll();
};
$('cancel').onclick = async () => { try { await api('cancel',{TaskID:selectedTaskID}); } catch (e) { $('message').textContent = e.message; } };
$('download').onclick = async () => {
  try { const text = await api('log'), a = document.createElement('a'); a.href = URL.createObjectURL(new Blob([text], { type: 'text/plain;charset=utf-8' })); a.download = 'toolbox-log.txt'; a.click(); setTimeout(() => URL.revokeObjectURL(a.href), 5000); }
  catch (e) { $('message').textContent = e.message; }
};
function renderItems(items, total, status) {
  const signature = JSON.stringify([items, total, status === 'ready', tool]); if (signature === lastItems) return;
  lastItems = signature; $('items').replaceChildren();
  if (!items.length) {
    const tr = document.createElement('tr'), td = document.createElement('td'); td.colSpan = 2; td.className = 'empty';
    td.textContent = status === 'ready' ? '没有需要处理的项目：请检查输入目录、子目录选项或过滤条件。' : '点击“扫描并预览”，查看处理前后的路径'; tr.append(td); $('items').append(tr);
  }
  for (const item of items) {
    const tr = document.createElement('tr'), comparison = previewComparison(item), result = document.createElement('td');
    const badge = document.createElement('span'), detail = document.createElement('span');
    badge.className = 'terminal-status'; badge.textContent = item.Conflict ? '跳过' : '就绪';
    detail.className = 'preview-result-detail'; detail.textContent = item.Conflict || item.Note || '可以执行'; result.append(badge, detail);
    tr.append(comparison, result);
    tr.lastChild.className = item.Conflict ? 'bad' : 'ok';
    if (['video','image','describe','exif','archive','extract'].includes(tool) && !item.Conflict && previewPlan) comparison.append(videoCommandDetails(item, previewPlan));
    $('items').append(tr);
  }
  $('previewCount').textContent = total ? '共 ' + total + ' 项' : '';
}
function previewPathParts(value) {
  const path = String(value || '').replace(/[\\/]+$/, ''), index = Math.max(path.lastIndexOf('\\'), path.lastIndexOf('/'));
  return { name: path.slice(index + 1), directory: index < 0 ? '' : path.slice(0, index) };
}
function renderLogs(text) {
  const container = $('logs'), previousTop = container.scrollTop, fragment = document.createDocumentFragment();
  for (const raw of (text || '准备就绪。').split('\n')) {
    const match = raw.match(/^(?:(\d{2}:\d{2}:\d{2})\s+)?([\s\S]*)$/), message = match[2];
    const line = document.createElement('div'), stamp = document.createElement('span'), body = document.createElement('span');
    const tone = /^(失败|当前编码失败|日志保存失败|扫描失败|所有编码方式)/.test(message) ? 'error'
      : /^(跳过|加速不可用|正在回退|正在重试|兼容模式|检测到旧版)/.test(message) ? 'warning'
      : /^(完成|视频编码完成|扫描完成|PDF 已生成)/.test(message) ? 'success'
      : /^(正在|视频进度|解压进度)/.test(message) ? 'progress' : 'info';
    line.className = 'terminal-log-line log-' + tone;
    stamp.className = 'log-time'; stamp.textContent = match[1] ? match[1] + ' ' : '';
    body.className = 'log-message'; body.textContent = message;
    line.append(stamp, body); fragment.append(line);
  }
  container.replaceChildren(fragment);
  container.scrollTop = followLogs ? container.scrollHeight : previousTop;
}
function setLogFollow(value) {
  followLogs = value; $('followLogs').textContent = value ? '跟随最新' : '继续跟随';
  $('followLogs').setAttribute('aria-pressed', String(value));
}
$('followLogs').onclick = () => { setLogFollow(!followLogs); if (followLogs) $('logs').scrollTop = $('logs').scrollHeight; };
$('logs').addEventListener('scroll', () => {
  const el = $('logs'); if (followLogs && el.scrollHeight - el.scrollTop - el.clientHeight > 24) setLogFollow(false);
});
function previewName(container, name, other, before) {
  if (!name) { container.textContent = '—'; return; }
  if (name === other) { container.textContent = name; return; }
  const chars = Array.from(name), previous = Array.from(other);
  let start = 0, end = 0;
  while (start < chars.length && start < previous.length && chars[start] === previous[start]) start++;
  while (end < chars.length - start && end < previous.length - start && chars[chars.length - end - 1] === previous[previous.length - end - 1]) end++;
  container.append(document.createTextNode(chars.slice(0, start).join('')));
  if (start < chars.length - end) {
    const mark = document.createElement('mark'); mark.className = before ? 'preview-removed' : 'preview-added';
    mark.textContent = chars.slice(start, chars.length - end).join(''); container.append(mark);
  }
  container.append(document.createTextNode(end ? chars.slice(-end).join('') : ''));
}
function previewComparison(item) {
  const td = document.createElement('td'); td.className = 'preview-comparison';
  const cleanup = item.Action === 'recycle' ? '移入回收站' : item.Action === 'delete' ? '永久删除（不进回收站）' : '';
  const source = previewPathParts(item.Source), target = cleanup ? {name:cleanup,directory:source.directory} : previewPathParts(item.Target);
  for (const [label, parts, other, before] of [['原名称', source, target, true], ['处理后', target, source, false]]) {
    const line = document.createElement('div'); line.className = 'preview-name-line';
    const tag = document.createElement('span'); tag.className = 'preview-name-label'; tag.textContent = label;
    const body = document.createElement('div'), name = document.createElement('span'); name.className = 'preview-name';
    previewName(name, parts.name, other.name, before); body.append(name);
    if (parts.directory && source.directory !== target.directory) {
      const location = document.createElement('span'); location.className = 'preview-location'; location.textContent = parts.directory; location.title = parts.directory; body.append(location);
    }
    line.append(tag, body); td.append(line);
  }
  const paths = document.createElement('details'); paths.className = 'preview-full-path';
  const summary = document.createElement('summary'); summary.textContent = '完整路径'; paths.append(summary);
  for (const [label, path] of [['原路径：', item.Source], [cleanup ? '处理方式：' : '目标路径：', cleanup || item.Target]]) {
    const line = document.createElement('div'), caption = document.createElement('span'), value = document.createElement('code');
    caption.textContent = label; value.textContent = path || '—'; line.append(caption, value); paths.append(line);
  }
  td.append(paths); return td;
}
function videoCommandDetails(item, id) {
  const details = document.createElement('details'); details.className = 'video-command';
  const summary = document.createElement('summary'); summary.textContent = tool==='video' ? '查看 / 复制压缩命令' : '查看 / 复制原生命令（高级）';
  const message = document.createElement('p'); message.className = 'hint'; message.setAttribute('role', 'status');
  const selector = document.createElement('select'); selector.setAttribute('aria-label', '选择压缩命令'); selector.hidden = true;
  const code = document.createElement('textarea'); code.readOnly = true; code.rows = 5; code.hidden = true; code.setAttribute('aria-label', '压缩命令'); code.spellcheck = false;
  const copy = document.createElement('button'); copy.type = 'button'; copy.textContent = '复制压缩命令'; copy.hidden = true;
  const note = document.createElement('p'); note.className = 'hint'; note.hidden = true;
  note.textContent = '粘贴到本机终端执行（Windows 使用 PowerShell）。自动创建输出目录，不覆盖已有文件。独立命令不含工具箱的自动回退、结果校验和“未变小就丢弃”；完成后请自行检查输出。';
  if(tool!=='video')note.textContent='原生命令直接调用外部程序，不包含完整安全校验、回退、清理原包或“未变小就跳过”；解压按所选重名规则处理，覆盖模式会覆盖文件。多步流程请使用页面顶部的工具箱 CLI。加密包需先设置指定密码环境变量。';
  details.append(summary, message, selector, code, copy, note);
  let loaded = false, loading = false, commands = [];
  details.addEventListener('toggle', async () => {
    if (!details.open || loaded || loading) return;
    loading = true; message.textContent = '正在检测编码器并生成命令，不会压缩文件…';
    try {
      commands = await api(tool==='video' ? 'video-commands' : 'native-commands', { PlanID: id, Source: item.Source });
      if (!commands.length) throw Error('没有可用命令');
      for (const command of commands) { const option = document.createElement('option'); option.textContent = command.Label; selector.append(option); }
      selector.selectedIndex = 0; selector.hidden = commands.length < 2;
      code.value = commands[0].PowerShell; code.hidden = copy.hidden = note.hidden = false;
      message.textContent = '按本次扫描的参数生成；修改设置后请重新扫描，备用命令需手动选择执行。'; loaded = true;
    } catch (e) { message.textContent = '生成失败：' + e.message + '。关闭后重新展开可重试。'; }
    finally { loading = false; }
  });
  selector.addEventListener('change', () => { code.value = commands[selector.selectedIndex].PowerShell; copy.textContent = '复制压缩命令'; });
  copy.addEventListener('click', async () => {
    try { await navigator.clipboard.writeText(code.value); copy.textContent = '已复制'; }
    catch { code.focus(); code.select(); message.textContent = '无法自动复制，已选中命令，请按 Ctrl+C。'; }
  });
  return details;
}
function updatePreviewControls() {
  $('previewFilter').disabled = !previewPlan || previewLoading;
  $('previewPrevious').disabled = !previewPlan || previewLoading || previewOffset === 0;
  $('previewNext').disabled = !previewPlan || previewLoading || previewOffset + 100 >= previewTotal;
  $('previewPageInfo').textContent = previewLoading ? '正在加载…' : previewPlan ? (previewTotal ? (previewOffset + 1) + '–' + Math.min(previewOffset + 100, previewTotal) + ' / ' + previewTotal + ' 项' : '此筛选下没有项目') : '每页 100 项';
}
function resetPreview() {
  previewRequest++; previewPlan = ''; previewOffset = previewTotal = 0; previewRows = []; previewLoading = false;
  $('previewFilter').value = 'all'; updatePreviewControls(); renderItems([], 0, 'idle');
}
async function loadPreview(id, offset = 0) {
  const request = ++previewRequest, activeTool = tool, filter = $('previewFilter').value;
  previewLoading = true; updatePreviewControls();
  try {
    const page = await api('preview', { PlanID: id, Offset: offset, Limit: 100, Filter: filter });
    if (request !== previewRequest || tool !== activeTool) return;
    previewPlan = page.PlanID; previewOffset = page.Offset; previewTotal = page.Total; previewRows = page.Items || [];
    renderItems(previewRows, previewTotal, 'ready');
  } catch (e) { if (request === previewRequest) showMessage('预览加载失败：' + e.message, true); }
  finally { if (request === previewRequest) { previewLoading = false; updatePreviewControls(); } }
}
$('previewFilter').onchange = () => { if (previewPlan) loadPreview(previewPlan, 0); };
$('previewPrevious').onclick = () => loadPreview(previewPlan, Math.max(0, previewOffset - 100));
$('previewNext').onclick = () => loadPreview(previewPlan, previewOffset + 100);
function renderExtractProgress(state,sameTool){
  const p=state.Operation,active=sameTool&&tool==='extract'&&state.Status==='running'&&p?.Tool==='extract';
  $('extractProgressPanel').hidden=!active;if(!active)return;
  const labels={extract:'正在解压',validate:'检查解压结果',publish:'写入输出文件',cleanup:'清理原压缩包',complete:'本包处理完成'};
  const percent=Number(p.Percent),known=p.PercentKnown&&Number.isFinite(percent),phase=labels[p.Phase]||'正在处理';
  $('extractProgressLabel').textContent='当前压缩包'+(p.ItemNumber>0&&p.ItemTotal>0?' · '+p.ItemNumber+' / '+p.ItemTotal:'');
  const source=String(p.Source||'');$('extractProgressSource').textContent=source.split(/[\\/]/).pop();$('extractProgressSource').title=source;
  $('extractProgressPhase').textContent=phase;$('extractProgressPercent').textContent=known?Math.max(0,Math.min(100,percent)).toFixed(1)+'%':'进行中';
  if(known)$('extractProgressBar').value=Math.max(0,Math.min(100,percent));else $('extractProgressBar').removeAttribute('value');
  $('extractProgressBar').setAttribute('aria-valuetext',phase+(known?' '+percent.toFixed(1)+'%':'，进度暂不可计算'));
  const seconds=value=>{const n=Math.max(0,Math.round(value));return n>=3600?Math.floor(n/3600)+'小时'+Math.floor(n%3600/60)+'分':n>=60?Math.floor(n/60)+'分'+n%60+'秒':n+'秒'};
  const age=value=>{const t=Date.parse(value);return Number.isFinite(t)?Math.max(0,(Date.now()-t)/1000):0};
  const details=['已用 '+seconds(age(p.Started))];
  if(p.Phase==='validate'&&p.FilesDone>0)details.push('已检查 '+p.FilesDone+' 个文件');
  if(p.Phase==='publish')details.push('已处理 '+(p.FilesDone||0)+' / '+(p.FilesTotal||0)+' 个文件');
  const elapsed=age(p.PhaseStarted);if(known&&percent>=1&&percent<99.9&&elapsed>=3){const eta=elapsed*(100-percent)/percent;if(eta>0&&eta<7*24*3600)details.push('预计本阶段剩余约 '+seconds(eta))}
  $('extractProgressDetail').textContent=details.join(' · ');
}
async function poll() {
  if (pollPending) return; pollPending = true;
  const requestedTool=tool;
  try {
    const state = await api('state', { Summary: true });
    window.toolboxFeedback?.observe(state);
    renderToolTasks(state.Tasks||[state]);
    if(submitting||switching||requestedTool!==tool) return;
    taskHistory?.observe(state);
    const sameTool = state.Tool === tool;busy = sameTool && ['scanning', 'running'].includes(state.Status);
    if(busy&&state.Config&&(selectedTaskID!==state.TaskID||!matchesTaskConfig(state.Config))) {
      showRunningTaskConfig(state.Config);lastConfig=JSON.stringify(config());lastActionError='';
    }
    selectedTaskID=sameTool?(state.TaskID||''):'';
    const matchesSnapshot=sameTool&&matchesTaskConfig(state.Config);
    const secretPlan=['archive','extract'].includes(tool);
    if(matchesSnapshot&&state.Status==='ready'&&!toolPlanTasks.has(tool)&&!secretPlan&&state.TaskID)toolPlanTasks.set(tool,state.TaskID);
    const matchesTask=!state.TaskID||toolPlanTasks.get(tool)===state.TaskID;
    if(matchesSnapshot&&matchesTask&&!lastConfig&&state.Config&&!secretPlan)lastConfig=JSON.stringify(config());
    $('statusBadge').textContent = sameTool ? statuses[state.Status] : '等待开始'; $('count').firstChild.textContent = (sameTool ? state.Total || 0 : 0) + ' ';
    $('statusBadge').dataset.status=sameTool?state.Status:'idle';
    $('progress').max = sameTool ? state.Total || 1 : 1; $('progress').value = sameTool ? state.Done || 0 : 0;
    $('progress').setAttribute('aria-label',tool==='extract'?'整个任务已完成的压缩包数量':'任务已完成项目数量');
    renderExtractProgress(state,sameTool);
    planID=matchesSnapshot&&matchesTask&&state.Status==='ready'&&JSON.stringify(config())===lastConfig&&state.Actionable>0?state.PlanID:'';
    $('statusText').textContent = !sameTool ? '选择目录后，点击“扫描并预览”。' :
      state.Status === 'running' ? '已处理 ' + state.Done + ' / ' + state.Total :
      state.Status === 'ready' ? '可执行 ' + state.Actionable + ' 项，冲突 ' + state.Conflicts + ' 项。' :
      state.Status === 'finished' ? '任务结束，可在下方查看或导出日志。' :
      state.Status === 'scanning' ? '正在读取目录和媒体信息…' :
      state.Status === 'cancelled' ? '任务已取消，已完成的项目会保留。' : '选择目录与参数，先预览处理结果。';
    const matchingConfig = matchesSnapshot && matchesTask && lastConfig && JSON.stringify(config()) === lastConfig;
    if(sameTool&&state.Status==='ready'&&!matchesSnapshot&&!lastActionError)showMessage('当前参数与服务端预览不同，请重新扫描后再执行。',true);
    else if(sameTool&&state.Status==='ready'&&!matchesTask&&!lastActionError)showMessage(secretPlan?'此预览来自其他页面或刷新前。请核对密码并重新扫描后再执行。':'此工具的预览已在其他页面更新，请重新扫描后再执行。',true);
    if (matchingConfig && !lastActionError) {
      if (state.Status === 'scanning') showMessage('正在扫描目录，请稍候…');
      else if (state.Status === 'ready') showMessage(state.Total === 0 ? ({number:'扫描完成，没有待编号目录。请检查直接子目录是否已符合所选编号格式，或调整筛选条件。',video:'扫描完成，没有符合条件的视频。请检查目录、包含子目录开关，以及高级设置的体积和原码率筛选；两个门槛设为 0 可取消这两项筛选。',image:'扫描完成，没有选定格式的待压缩图片。请检查目录、包含子目录开关和处理哪些格式。'})[tool]||'扫描完成，没有需要修改的项目。请核对目录、筛选条件与工具设置。' : '扫描完成：可执行 ' + state.Actionable + ' 项，冲突 ' + state.Conflicts + ' 项。请查看下方预览。');
      else if (state.Status === 'error') showMessage('扫描失败：' + state.Error, true);
      else if (state.Status === 'cancelled') showMessage('任务已取消。');
      else if (state.Status === 'finished') showMessage('处理完成：成功 ' + (state.Result?.Success || 0) + ' 项，跳过 ' + (state.Result?.Skipped || 0) + ' 项，失败 ' + (state.Result?.Failed || 0) + ' 项。');
    }
    for (const [id, key] of [['success', 'Success'], ['failed', 'Failed'], ['skipped', 'Skipped']]) $(id).textContent = sameTool ? state.Result?.[key] || 0 : 0;
    if (sameTool && state.PlanID) { if (previewPlan !== state.PlanID && !previewLoading) await loadPreview(state.PlanID); }
    else { if (previewPlan) resetPreview(); renderItems([], 0, sameTool ? state.Status : 'idle'); }
    if(requestedTool!==tool||switching)return;
    const text = sameTool ? (state.Logs || []).join('\n') : '';
    const videoLine = sameTool && tool === 'video' && state.Status === 'running'
      ? [...(state.Logs || [])].reverse().find(line => /视频进度：|正在压缩视频：|正在回退：|正在重试：|视频编码完成：/.test(line)) : '';
    $('videoProgress').hidden = !videoLine;
    $('videoProgress').textContent = videoLine || '';
    if (text !== lastLogs || displayTool !== tool) { lastLogs = text; displayTool = tool; renderLogs(text); }
    updateActions();
  } catch (e) { if(requestedTool===tool&&!switching)$('message').textContent = '连接失败：' + e.message + '。请确认工具箱程序仍在运行。'; }
  finally { pollPending = false;if(requestedTool!==tool&&!switching)void poll(); }
}
function choosePath(path) {
  if(!browseConfirmed){$('browseError').textContent='请先点击前往，确认当前目录。';return}
  if(browseTarget==='Root')putValue('ExtractSource','');
  if (browseTarget === 'ArchiveExtras') { const lines = $('ArchiveExtras').value.split(/\r?\n/).filter(Boolean); if (!lines.includes(path)) lines.push(path); putValue(browseTarget, lines.join('\n')); } else putValue(browseTarget, path);
  if (browseTarget === 'DedupeProgram') { changed(); inspectDedupeProgram(); $('folderDialog').close(); return; }
  if (browseTarget === 'FFmpeg' && $('FFprobe') && !$('FFprobe').value) putValue('FFprobe', path.replace(/ffmpeg(\.exe)?$/i, defaults.Platform&&defaults.Platform!=='windows'?'ffprobe':'ffprobe.exe'));
  if (browseTarget === 'FFprobe' && $('FFmpeg') && !$('FFmpeg').value) putValue('FFmpeg', path.replace(/ffprobe(\.exe)?$/i, defaults.Platform&&defaults.Platform!=='windows'?'ffmpeg':'ffmpeg.exe'));
  changed(!['Root','ArchiveExtras'].includes(browseTarget)); $('folderDialog').close();
}
async function browse(path) {
  const revision=++browseRevision,target=browseTarget;browseConfirmed=false;$('selectFolder').disabled=true;
  try {
    const data = await api('browse', { Path: path, Kind: browseKind });if(revision!==browseRevision||target!==browseTarget||!$('folderDialog').open)return;browseConfirmed=true;currentPath = data.Path; parentPath = data.Parent;
    $('browsePath').value = data.Path; $('folderList').replaceChildren(); $('browseError').textContent = '';
    $('selectFolder').hidden = browseKind !== ''; $('selectFolder').disabled = !data.Path;
    for (const p of data.Dirs) { const b = document.createElement('button'); b.textContent = '▸ ' + p; b.onclick = () => browse(p); $('folderList').append(b); }
    for (const p of data.Files || []) { const b = document.createElement('button'); b.textContent = '选择文件 · ' + p; b.className = 'file-choice'; b.onclick = () => choosePath(p); $('folderList').append(b); }
    if (!data.Dirs.length && !(data.Files || []).length) $('browseError').textContent = browseKind ? '此目录没有可选择的文件。可返回上一级继续查找。' : '此目录没有子目录，可以直接选择当前目录。';
  } catch (e) { if(revision===browseRevision)$('browseError').textContent = e.message; }
}
function openPicker(key) {
  browseTarget = key; browseKind = key === 'Root' ? '' : key === 'ArchiveExtras' ? 'file' : key === 'Font' ? 'font' : 'program';
  $('pickerTitle').textContent = key === 'Root' ? '选择输入目录' : key === 'ArchiveExtras' ? '选择要加入每个压缩包的文件' : key === 'Font' ? '选择字体文件' : '选择程序文件';
  const value = key === 'ArchiveExtras' ? $(key).value.split(/\r?\n/).filter(Boolean).at(-1) || '' : $(key).value; let path = key === 'Root' ? value : value.replace(/[\\/][^\\/]+$/, '');
  if (path === value && key !== 'Root') path = ''; $('folderDialog').showModal(); browse(path);
}
async function inspectDedupeProgram(auto=false) {
  clearTimeout(dedupeTimer);const request=++dedupeRequest;dedupeChecking=true;dedupeAvailable=false;updateActions();$('dedupeStatus').textContent='正在检查本地软件…';
  try {
    const selectedPath=$('DedupeProgram').value;
    let info=await api('dedupe-info',{Path:auto?'':selectedPath});
    if(auto&&!info.Available&&selectedPath){const selected=await api('dedupe-info',{Path:selectedPath});if(selected.Available)info=selected;}
    if(request!==dedupeRequest||tool!=='dedupe')return;
    if(info.Path) putValue('DedupeProgram',info.Path);
    dedupeAvailable=info.Available;$('dedupeStatus').textContent=info.Message;$('dedupeStatus').className='hint '+(info.Available?'ok':'bad');$('dedupeDownloadHelp').hidden=info.Available;
    if(auto) await queueSave(config());
  } catch(e){if(request===dedupeRequest){$('dedupeStatus').textContent='检查失败：'+e.message;$('dedupeDownloadHelp').hidden=false;}}
  finally{if(request===dedupeRequest){dedupeChecking=false;updateActions();}}
}
$('DedupeProgram').oninput=()=>{dedupeRequest++;dedupeAvailable=false;dedupeChecking=true;$('dedupeMessage').textContent='';changed();clearTimeout(dedupeTimer);dedupeTimer=setTimeout(()=>inspectDedupeProgram(),400);};
$('browseDedupe').onclick=()=>openPicker('DedupeProgram');
$('detectDedupe').onclick=()=>inspectDedupeProgram(true);
$('launchDedupe').onclick=async()=>{
  if(busy||switching||dedupeLaunching||dedupeChecking||!dedupeAvailable)return;
  dedupeLaunching=true;updateActions();$('dedupeMessage').textContent='';
  try{await saveNow();const result=await api('dedupe-open',{Path:$('DedupeProgram').value});$('dedupeMessage').textContent=result.Message;$('dedupeMessage').className='ok';}
  catch(e){$('dedupeMessage').textContent=e.message;$('dedupeMessage').className='bad';await inspectDedupeProgram();}
  finally{dedupeLaunching=false;updateActions();}
};
$('browse').onclick = () => openPicker('Root'); $('closeDialog').onclick = () => $('folderDialog').close();
$('goto').onclick = () => browse($('browsePath').value);
$('browsePath').oninput=()=>{browseRevision++;browseConfirmed=false;$('selectFolder').disabled=true;$('browseError').textContent='请点击前往或按 Enter，确认目录。'};
$('folderDialog').addEventListener('close',()=>{browseRevision++;browseConfirmed=false});
$('browsePath').onkeydown = event => { if (event.key === 'Enter') { event.preventDefault(); browse($('browsePath').value); } };
$('drives').onclick = () => browse(''); $('up').onclick = () => browse(parentPath); $('selectFolder').onclick = () => choosePath(currentPath);
(async () => {
  try {
    const [detected, preferences, info] = await Promise.all([api('defaults'), api('settings'), api('info')]); defaults = detected; workspaces = preferences.Workspaces || {};
    setToolboxSidebar(preferences.SidebarCollapsed===true);
    restoreArchiveDisclosure(preferences.ArchiveAdvancedOpen===true);
    window.toolboxFeedback?.init(preferences, changes => api('ui-settings', changes), id => tools.find(t => t[0] === id)?.[1] || '工具箱任务', api);
    window.toolboxVersion?.init(info,api);
    $('appVersion').textContent = window.toolboxVersion?.label(info.Version)||(info.Version === 'dev' ? '开发版' : 'v' + info.Version);
    $('bundleNotice').hidden = !info.BundleWarning; $('bundleNotice').textContent = info.BundleWarning || '';
    if(defaults.Platform&&defaults.Platform!=='windows'){for(const row of document.querySelectorAll('.password-tools'))row.hidden=true;$('DedupeProgram').placeholder='选择当前系统的 VDF.GUI 图形程序';$('dedupeDownloadHelp').querySelector('ol').textContent='下载适合当前系统和架构的图形界面版本，完整解压后选择 VDF.GUI 可执行文件。需要桌面环境，具体平台支持以软件发布页为准。'}
    const incoming=new URLSearchParams(location.search),requestedTool=incoming.get('tool');
    await selectTool(tools.some(t=>t[0]===requestedTool)?requestedTool:tools.some(t => t[0] === preferences.LastTool) ? preferences.LastTool : 'format', true);
    if(incoming.has('root')&&tools.some(t=>t[0]===requestedTool)){putValue('Root',incoming.get('root'));putValue('ExtractSource','');if(requestedTool==='extract'&&incoming.get('extract-source')){putValue('ExtractSource',incoming.get('extract-source'));putValue('Output','');}changed();}
 if(requestedTool==='extract'&&incoming.get('extract-source')){const clean=new URL(location.href);for(const key of ['tool','root','extract-source'])clean.searchParams.delete(key);history.replaceState(history.state,'',clean.href);}
 await refreshSavedPasswords().catch(() => {}); if(requestedTool==='extract'&&incoming.get('extract-source')&&dependenciesReady&&!busy)$('preview').click(); setInterval(poll, 1200);
  } catch (e) { $('message').textContent = '初始化失败：' + e.message; switching = false; updateActions(); }
})();

let sidebarWrites=Promise.resolve(),toolboxSidebarCollapsed=false;
let archiveDisclosureWrites=Promise.resolve(),archiveDisclosureOpen=false,archiveDisclosureSaved=false,archiveDisclosureRevision=0;
function restoreArchiveDisclosure(open){
  archiveDisclosureOpen=archiveDisclosureSaved=open;
  $('archiveAdvanced').open=open;
}
$('archiveAdvanced').addEventListener('toggle',()=>{
  const open=$('archiveAdvanced').open;
  if(open===archiveDisclosureOpen)return;
  archiveDisclosureOpen=open;
  const revision=++archiveDisclosureRevision;
  archiveDisclosureWrites=archiveDisclosureWrites.catch(()=>{}).then(async()=>{
    try{await api('ui-settings',{ArchiveAdvancedOpen:open});archiveDisclosureSaved=open;}
    catch(e){
      if(revision===archiveDisclosureRevision){archiveDisclosureOpen=archiveDisclosureSaved;$('archiveAdvanced').open=archiveDisclosureSaved;}
      setSettingsStatus('展开状态保存失败：'+e.message,true);
    }
  });
});
function setToolboxSidebar(collapsed){
  toolboxSidebarCollapsed=collapsed;
  const mobile=window.innerWidth<=760,expanded=mobile?document.body.classList.contains('nav-open'):!collapsed;
  document.body.classList.toggle('sidebar-collapsed',collapsed);
  $('toolboxSidebar').inert=!expanded;$('toolboxNavBackdrop').hidden=!mobile||!expanded;
  const label=expanded?'收起左侧导航':'展开左侧导航';
  $('toggleSidebar').title=label;$('toggleSidebar').setAttribute('aria-label',label);$('toggleSidebar').setAttribute('aria-expanded',String(expanded));
}
function closeToolboxNav(restoreFocus=false){
  document.body.classList.remove('nav-open');setToolboxSidebar(toolboxSidebarCollapsed);
  if(restoreFocus)$('toggleSidebar').focus();
}
$('toggleSidebar').onclick=()=>{
  if(window.innerWidth<=760){document.body.classList.toggle('nav-open');setToolboxSidebar(toolboxSidebarCollapsed);return}
  const collapsed=!toolboxSidebarCollapsed;setToolboxSidebar(collapsed);
  sidebarWrites=sidebarWrites.catch(()=>{}).then(()=>api('ui-settings',{SidebarCollapsed:collapsed})).catch(e=>showMessage('导航设置保存失败：'+e.message));
};
$('closeToolboxNav').onclick=$('toolboxNavBackdrop').onclick=()=>closeToolboxNav(true);
document.addEventListener('keydown',event=>{if(event.key==='Escape'&&document.body.classList.contains('nav-open')&&!document.querySelector('dialog[open]')){event.preventDefault();closeToolboxNav(true)}});
window.addEventListener('resize',()=>{if(window.innerWidth>760)document.body.classList.remove('nav-open');setToolboxSidebar(toolboxSidebarCollapsed)});

// History restores parameters only; every execution still requires a fresh preview.
function canConfigureTaskTool(id=tool){return !switching&&!submitting&&!(id===tool&&busy)&&!toolTaskStates.some(state=>state.Tool===id&&['running','scanning'].includes(state.Status));}
taskHistory=window.setupTaskHistory?.({api,$,toolName:id=>tools.find(t=>t[0]===id)?.[1]||id,
  canChange:canConfigureTaskTool,
  beforeOpen:()=>exitTerminalFullscreen(false),
  restore:async c=>{
    if(!canConfigureTaskTool(c.Tool))throw Error('目标工具正在处理任务，请等待完成或先取消');
    const live=await api('state',{Summary:true},c.Tool);
    if(live.Tool===c.Tool&&['running','scanning'].includes(live.Status))throw Error('目标工具已在其他页面开始任务，请等待完成或先取消');
    await saveNow();workspaces[c.Tool]={...c,ArchivePassword:'',ExtractPassword:''};await selectTool(c.Tool,true);
    if(busy)throw Error('目标工具已开始任务，已显示实际运行参数，未替换正在执行的任务');
    lastConfig='';planID='';updateActions();
  },
  scan:id=>startPreview(id),
  message:(text,error=false)=>showMessage(text,error)
});

$('clearExtractSource').onclick=()=>{if(busy||switching||submitting)return;putValue('ExtractSource','');changed();};

$('openNotes').firstElementChild.replaceChildren(toolNavIcon('notes'));
