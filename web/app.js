const $ = id => document.getElementById(id);
const tools = [
  ['format', '目录统计命名', '按图片、视频、音频数量与总大小，统一目录名称。', '▤'],
  ['rename', '批量重命名', '批量替换名称、添加前后缀，支持正则表达式。', 'Aa'],
  ['number', '目录批量编号', '给新增资源目录连续编号，可指定起始值或接着已有编号继续。', '#'],
  ['describe', '媒体信息命名', '把音视频的分辨率、时长和体积添加到文件名。', '▷'],
  ['exif', '清除图片信息', '清除图片副本中的拍摄信息等元数据，保留原图。', '◇'],
  ['pdf', 'PDF 图册索引', '每个直接子目录生成一页：中文目录名称 + 首张可读取的图片。', '▧'],
  ['image', '压缩图片', '缩小图片体积，保留原图与目录结构。', '▨'],
  ['video', '压缩视频', '压缩为兼容常见播放器的 MP4，保留原视频。', '▸'],
  ['thumbnail', '视频缩略图', '均匀截取视频画面，生成带文件信息和时间的预览拼图。', '▦']
];
const optionKeys = {
  format: ['Recursive'], rename: ['Recursive', 'Files', 'Dirs', 'Find', 'Replace', 'Regex', 'Prefix', 'Suffix', 'RenameMode', 'ExampleNumber', 'ExampleAtStart'],
  number: ['NumberName', 'StartNumber', 'NumberDigits', 'NumberStyle', 'NumberOrder', 'NumberFilter', 'NumberKeepOriginal'],
  describe: ['Recursive'], exif: ['Recursive', 'Output'], pdf: ['Output'],
  image: ['Recursive', 'Output', 'Quality', 'PngMode', 'PngQuality'], video: ['Recursive', 'Output', 'CRF', 'MinMB', 'MinKbps'],
  thumbnail: ['Recursive', 'Output', 'ThumbnailColumns', 'ThumbnailRows', 'ThumbnailWidth', 'ThumbnailGap', 'ThumbnailQuality', 'ThumbnailFormat', 'ThumbnailBackground', 'ThumbnailInfo', 'ThumbnailTime']
};
const boolKeys = ['Recursive', 'Files', 'Dirs', 'Regex', 'NumberKeepOriginal', 'ExampleNumber', 'ExampleAtStart', 'ThumbnailInfo', 'ThumbnailTime'], numericKeys = ['Quality', 'CRF', 'MinMB', 'MinKbps', 'StartNumber', 'NumberDigits', 'PngQuality', 'ThumbnailColumns', 'ThumbnailRows', 'ThumbnailWidth', 'ThumbnailGap', 'ThumbnailQuality'];
const imageFormatExtensions = '.jpg .jpeg .jpe .jfif .png .webp'.split(' ');
const jpegFormats = ['.jpg', '.jpeg', '.jpe', '.jfif'];
const token = new URLSearchParams(location.search).get('session');
const statuses = { idle: '等待开始', scanning: '正在扫描', ready: '预览就绪', running: '正在执行', finished: '处理完成', cancelled: '已取消', error: '任务失败' };
let tool = 'format', defaults = {}, workspaces = {}, dependencyMetadata = [];
let busy = false, switching = true, dependenciesReady = true, checking = false, submitting = false;
let planID = '', lastConfig = '', lastLogs = '', lastItems = '', displayTool = '';
let currentPath = '', parentPath = '', browseTarget = 'Root', browseKind = '';
let saveTimer, checkTimer, saveRevision = 0, saveQueue = Promise.resolve(), pollPending = false, dependencyRequest = 0;
let detectingNumbering = false;
let previewPlan = '', previewOffset = 0, previewTotal = 0, previewRows = [], previewRequest = 0, previewLoading = false;
let lastActionError = '';
async function api(path, data = {}) {
  const response = await fetch('/api/' + path, { method: 'POST', headers: { 'Content-Type': 'application/json', 'X-Toolbox-Token': token }, body: JSON.stringify(data) });
  if (!response.ok) throw Error((await response.text()).trim());
  return path === 'log' ? response.text() : response.json();
}
function readInput(id) { const el = $(id); return boolKeys.includes(id) ? el.checked : numericKeys.includes(id) ? Number(el.value) : el.value; }
function config() {
  const result = { Tool: tool, Root: $('Root').value };
  for (const id of [...optionKeys[tool], ...dependencyMetadata.map(d => d.Key)]) result[id] = readInput(id);
  if (tool === 'number') result.NumberTemplate = $('NumberTemplate').value;
  if (tool === 'image') result.ImageFormats = selectedImageFormats();
  return result;
}
function putValue(id, value) { const el = $(id); if (!el) return; if (el.type === 'checkbox') el.checked = Boolean(value); else el.value = value ?? ''; }
function updateActions() {
  const locked = busy || switching || submitting || detectingNumbering;
  const activeInputs = new Set(['Root', ...optionKeys[tool], ...dependencyMetadata.map(d => d.Key)]);
  if (tool === 'video') activeInputs.add('videoQuality');
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
  if (tool === 'number' && $('NumberStyle').value === 'custom') activeInputs.add('NumberTemplate');
  $('preview').disabled = locked || checking || !dependenciesReady || (tool === 'image' && !selectedImageFormats().length);
  $('preview').textContent = submitting ? '正在提交…' : busy ? '任务处理中…' : '扫描并预览 →';
  $('execute').disabled = locked || !planID; $('cancel').hidden = !busy;
  for (const b of $('nav').children) b.disabled = locked;
  // CSS-hidden inputs still participate in native HTML form validation unless disabled.
  for (const el of $('form').querySelectorAll('input, select')) el.disabled = locked || !activeInputs.has(el.id);
  for (const el of $('form').querySelectorAll('button[data-file-key]')) el.disabled = locked;
  $('browse').disabled = locked;
  $('autoDetect').disabled = $('checkDependencies').disabled = locked || checking;
  $('checkDependencies').textContent = checking ? '正在检查…' : '检查是否可用';
  $('detectNumbering').disabled = locked;
  $('detectNumbering').textContent = detectingNumbering ? '正在读取…' : '识别名称并续号';
}
function showMessage(text, error = false) {
  $('message').textContent = text;
  $('message').classList.toggle('error', error);
}
function invalidate() { planID = ''; lastActionError = ''; showMessage(''); updateActions(); }
function queueSave(snapshot) {
  const revision = ++saveRevision; workspaces[snapshot.Tool] = { ...snapshot };
  $('settingsStatus').textContent = '正在保存本工具的设置…';
  const task = saveQueue.then(() => api('save-settings', snapshot)); saveQueue = task.catch(() => {});
  return task.then(() => { if (revision === saveRevision) { $('settingsStatus').textContent = '本工具设置已保存 · 重启后仍会保留'; $('settingsStatus').classList.remove('bad'); } })
    .catch(error => { $('settingsStatus').textContent = '设置未能保存：' + error.message; $('settingsStatus').classList.add('bad'); });
}
function saveNow() { clearTimeout(saveTimer); return queueSave(config()); }
function changed(dependency = false) {
  updateRenameMode();
  invalidate(); clearTimeout(saveTimer); const snapshot = config(); workspaces[tool] = snapshot;
  updateNumberSample();
  updateImageFormatSummary();
  updateThumbnailOptions();
  saveTimer = setTimeout(() => queueSave(snapshot), 500);
  if (dependency) { dependencyRequest++; checking = false; dependenciesReady = false; clearTimeout(checkTimer); checkTimer = setTimeout(() => inspectDependencies(false), 400); updateActions(); }
}
function applyDependencyStatus(deps) {
  dependenciesReady = deps.every(d => d.Available);
  for (const d of deps) {
    const status = $('dependency-status-' + d.Key);
    if (status) { status.textContent = d.Message; status.className = 'dependency-status ' + (d.Available ? 'ok' : 'bad'); }
    const detail = $('dependency-help-' + d.Key); if (detail && !d.Available) detail.open = true;
  }
  const missing = deps.filter(d => !d.Available);
  $('dependencySummary').textContent = missing.length ? '还需设置：' + missing.map(d => d.Name).join('、') + '。按下方步骤操作即可。' : '已找到所需程序，可以开始预览。';
  $('dependencySummary').className = missing.length ? 'bad' : 'ok'; updateActions();
}
function renderDependencies(deps) {
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
    input.placeholder = d.Key === 'Font' ? '选择本机的字体文件' : '选择 ' + ({ FFmpeg: 'ffmpeg.exe', FFprobe: 'ffprobe.exe', Magick: 'magick.exe', Exiftool: 'exiftool.exe' })[d.Key];
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
  } catch (e) { if (requestID === dependencyRequest) { dependenciesReady = false; $('dependencySummary').textContent = '检查失败：' + e.message; } }
  finally { if (requestID === dependencyRequest) { checking = false; updateActions(); } }
}
function updateVideoPreset() { const value = String($('CRF').value); $('videoQuality').value = ['18', '23', '28'].includes(value) ? value : 'custom'; }
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
  const name = $('NumberName').value.trim() || '资源名称';
  const width = Math.min(9, Math.max(1, Number($('NumberDigits').value) || 3));
  const number = String(Math.max(0, Number($('StartNumber').value) || 0)).padStart(width, '0');
  let formatted;
  if ($('NumberStyle').value === 'custom') formatted = ($('NumberTemplate').value || '{name} 第{number}期').replace(/\{name\}|\{number\}/g, token => token === '{name}' ? name : number);
  else formatted = name + ({underscore:'_',space:' ',dash:' - '})[$('NumberStyle').value] + 'NO.' + number;
  $('numberSample').textContent = '示例：' + formatted + ($('NumberKeepOriginal').checked ? ' 原资源名称 [统计信息]' : '');
}
async function detectNumbering(save = true) {
  if (!$('Root').value.trim()) { showMessage('请先选择资源根目录。', true); return; }
  detectingNumbering = true; updateActions();
  try {
    const info = await api('number-info', { Root: $('Root').value, NumberFilter: $('NumberFilter').value, NumberStyle: $('NumberStyle').value, NumberTemplate: $('NumberTemplate').value });
    putValue('NumberName', info.Name); putValue('StartNumber', info.NextNumber); putValue('NumberDigits', info.Digits); putValue('NumberStyle', info.Style);
    $('numberInfo').textContent = '识别到 ' + info.Existing + ' 个同名称的已编号目录，最大编号 ' + String(info.MaxNumber).padStart(info.Digits, '0') + '；待编号 ' + info.Pending + ' 个目录。起始编号已设为 ' + String(info.NextNumber).padStart(info.Digits, '0') + '，可以手动修改。';
    invalidate(); updateNumberSample(); if (save) await saveNow();
  } catch (e) { showMessage('读取编号失败：' + e.message, true); }
  finally { detectingNumbering = false; updateActions(); }
}
async function selectTool(id, initial = false) {
  if ((busy || switching) && !initial) return; switching = true; updateActions();
  try {
    if (!initial) await saveNow(); dependencyRequest++; clearTimeout(checkTimer);
    tool = id; checking = false; planID = ''; lastConfig = '';
    resetPreview();
    const t = tools.find(t => t[0] === id);
    $('title').textContent = $('crumb').textContent = t[1]; $('description').textContent = t[2]; $('tool-number').textContent = String(tools.indexOf(t) + 1).padStart(2, '0');
    for (const b of $('nav').children) b.classList.toggle('active', b.dataset.tool === id);
    $('renameOptions').hidden = id !== 'rename'; $('outputOptions').hidden = !['image', 'video', 'exif', 'pdf', 'thumbnail'].includes(id);
    $('thumbnailOptions').hidden = id !== 'thumbnail';
    $('imageOptions').hidden = id !== 'image'; $('videoOptions').hidden = id !== 'video'; $('numberOptions').hidden = id !== 'number'; $('recursiveOption').hidden = ['pdf', 'number'].includes(id);
    $('outputLabel').textContent = id === 'pdf' ? '保存为 PDF 文件' : '输出目录';
	$('outputHint').textContent = id === 'pdf' ? '填写完整 PDF 文件路径，或留空自动生成。已有同名文件时会提示冲突。' : '留空即可：自动生成输出文件夹，保留原文件与目录结构。';
    if (id === 'thumbnail') $('outputHint').textContent = '留空保存到输入目录下的“视频预览图”，也可自定义目录。命名为“视频名.mp4.jpg”，保留视频扩展名，避免不同格式重名。已有结果会跳过。';
    $('Output').placeholder = id === 'pdf' ? '留空自动生成 toolbox-output/目录索引.pdf' : '留空自动输出到 toolbox-output/' + id;
    if (id === 'thumbnail') $('Output').placeholder = '留空保存到输入目录下的 视频预览图';
    $('recursiveHint').textContent = id === 'format' ? '关闭时只改直接子目录，统计仍包含目录内全部内容' : '';
    $('videoAdvanced').open = false;
    const active = { ...defaults, ...(workspaces[id] || {}), Tool: id };
    if (!workspaces[id] && !initial) active.Root = $('Root').value;
    for (const [key, value] of Object.entries(active)) putValue(key, value);
    if (id === 'rename') {
      putValue('RenameMode', active.RenameMode || (active.Regex ? 'regex' : 'text'));
      if (!active.RenameMode) { putValue('ExampleNumber', true); putValue('ExampleAtStart', true); }
    }
    if (id === 'number' && !$('NumberTemplate').value) putValue('NumberTemplate', '{name} 第{number}期');
    if (id === 'image') { putValue('PngMode', active.PngMode || 'lossless'); putValue('PngQuality', active.PngQuality || 90); restoreImageFormats(active.ImageFormats); }
    // Blank outputs on a new tool; they must not leak from another workspace.
    if (!workspaces[id]) putValue('Output', '');
    $('qualityValue').textContent = $('Quality').value; updateVideoPreset(); $('message').textContent = '';
    updateRenameMode();
    updateThumbnailOptions();
    const deps = await api('dependencies', { Config: active, Verify: false }); renderDependencies(deps);
    if (id === 'number') {
      $('numberInfo').textContent = '仅处理直接子目录，符合所选编号格式的目录会自动跳过。自定义格式无需包含 NO；先填模板，再识别续号。';
      if (!workspaces[id] && $('Root').value.trim()) await detectNumbering(false);
      updateNumberSample();
    }
    await queueSave(config());
  } catch (e) { dependenciesReady = false; $('message').textContent = e.message; }
  finally { switching = false; updateActions(); await poll(); }
}
for (const t of tools) {
  const b = document.createElement('button'); b.type = 'button'; b.dataset.tool = t[0];
  const icon = document.createElement('span'); icon.textContent = t[3]; b.append(icon, document.createTextNode(t[1])); b.onclick = () => selectTool(t[0]); $('nav').append(b);
}
for (const [index, ext] of imageFormatExtensions.entries()) {
  const label = document.createElement('label'); label.className = 'check';
  const input = document.createElement('input'); input.type = 'checkbox'; input.id = 'image-format-' + index; input.value = ext;
  label.append(input, document.createTextNode(ext.slice(1).toUpperCase())); $('imageFormatCheckboxes').append(label);
}
setImageFormats(null);
for (const el of $('form').querySelectorAll('input')) el.addEventListener('input', () => {
  $('qualityValue').textContent = $('Quality').value; if (el.id === 'CRF') updateVideoPreset(); changed();
});
for (const id of ['NumberStyle', 'NumberOrder']) $(id).addEventListener('change', () => changed());
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
$('form').onsubmit = async event => {
  event.preventDefault();
  if (busy || switching || submitting || checking || detectingNumbering) { showMessage('任务或设置正在处理中，请稍候。'); return; }
  if (!dependenciesReady) { showMessage('请先完成本工具需要的程序设置。', true); return; }
  if (tool === 'image' && !selectedImageFormats().length) { showMessage('请至少选择一种要压缩的图片格式。', true); return; }
  if (!$('Root').value.trim()) { showMessage('请先选择输入目录。', true); $('Root').focus(); return; }
  if (!$('form').checkValidity()) { showMessage('请检查当前工具的参数是否在允许范围内。', true); if (tool === 'video') $('videoAdvanced').open = true; $('form').reportValidity(); return; }
  submitting = true; planID = ''; lastActionError = ''; showMessage('正在扫描目录，请稍候…'); updateActions();
  try {
    const snapshot = config(); await saveNow(); lastConfig = JSON.stringify(snapshot);
    await api('plan', snapshot); busy = true;
  } catch (e) { busy = false; lastConfig = ''; showMessage('无法开始扫描：' + e.message, true); }
  finally { submitting = false; updateActions(); }
  await poll();
};
$('execute').onclick = async () => {
  if (busy || submitting || !planID) return;
  submitting = true; lastActionError = ''; updateActions();
  try { if (JSON.stringify(config()) !== lastConfig) throw Error('参数已改变，请重新预览'); await api('execute', { PlanID: planID }); planID = ''; busy = true; showMessage('正在执行，请稍候…'); }
  catch (e) { lastActionError = '无法开始执行：' + e.message; showMessage(lastActionError, true); }
  finally { submitting = false; updateActions(); }
  await poll();
};
$('cancel').onclick = async () => { try { await api('cancel'); } catch (e) { $('message').textContent = e.message; } };
$('download').onclick = async () => {
  try { const text = await api('log'), a = document.createElement('a'); a.href = URL.createObjectURL(new Blob([text], { type: 'text/plain;charset=utf-8' })); a.download = 'toolbox-log.txt'; a.click(); setTimeout(() => URL.revokeObjectURL(a.href), 5000); }
  catch (e) { $('message').textContent = e.message; }
};
function renderItems(items, total, status) {
  const signature = JSON.stringify([items, total, status === 'ready', tool]); if (signature === lastItems) return;
  lastItems = signature; $('items').replaceChildren();
  if (!items.length) {
    const tr = document.createElement('tr'), td = document.createElement('td'); td.colSpan = 3; td.className = 'empty';
    td.textContent = status === 'ready' ? '没有需要处理的项目：请检查输入目录、子目录选项或过滤条件。' : '点击“扫描并预览”，查看处理前后的路径'; tr.append(td); $('items').append(tr);
  }
  for (const item of items) {
    const tr = document.createElement('tr'); for (const value of [item.Source, item.Target, item.Conflict || (item.Note ? '可执行 · ' + item.Note : '可执行')]) { const td = document.createElement('td'); td.textContent = value; tr.append(td); }
    tr.lastChild.className = item.Conflict ? 'bad' : 'ok'; $('items').append(tr);
  }
  $('previewCount').textContent = total ? '共 ' + total + ' 项' : '';
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
async function poll() {
  if (pollPending) return; pollPending = true;
  try {
    const state = await api('state', { Summary: true }); if (submitting) return;
    busy = ['scanning', 'running'].includes(state.Status); const sameTool = state.Tool === tool;
    $('statusBadge').textContent = sameTool ? statuses[state.Status] : '等待开始'; $('count').firstChild.textContent = (sameTool ? state.Total || 0 : 0) + ' ';
    $('progress').max = sameTool ? state.Total || 1 : 1; $('progress').value = sameTool ? state.Done || 0 : 0;
    if (sameTool && state.Status === 'ready' && JSON.stringify(config()) === lastConfig && state.Actionable > 0) planID = state.PlanID;
    else if (!sameTool || state.Status !== 'ready') planID = '';
    $('statusText').textContent = !sameTool ? '选择目录后，点击“扫描并预览”。' :
      state.Status === 'running' ? '已处理 ' + state.Done + ' / ' + state.Total :
      state.Status === 'ready' ? '可执行 ' + state.Actionable + ' 项，冲突 ' + state.Conflicts + ' 项。' :
      state.Status === 'finished' ? '任务结束，可在下方查看或导出日志。' :
      state.Status === 'scanning' ? '正在读取目录和媒体信息…' :
      state.Status === 'cancelled' ? '任务已取消，已完成的项目会保留。' : '选择目录与参数，先预览处理结果。';
    const matchingConfig = sameTool && lastConfig && JSON.stringify(config()) === lastConfig;
    if (matchingConfig && !lastActionError) {
      if (state.Status === 'scanning') showMessage('正在扫描目录，请稍候…');
      else if (state.Status === 'ready') showMessage(state.Total === 0 ? (tool === 'number' ? '扫描完成，没有待编号目录。请检查直接子目录是否已符合所选编号格式，或调整筛选条件。' : '扫描完成，没有需要修改的项目。名称可能已符合规则，或查找内容没有匹配。') : '扫描完成：可执行 ' + state.Actionable + ' 项，冲突 ' + state.Conflicts + ' 项。请查看下方预览。');
      else if (state.Status === 'error') showMessage('扫描失败：' + state.Error, true);
      else if (state.Status === 'cancelled') showMessage('任务已取消。');
      else if (state.Status === 'finished') showMessage('处理完成：成功 ' + (state.Result?.Success || 0) + ' 项，跳过 ' + (state.Result?.Skipped || 0) + ' 项，失败 ' + (state.Result?.Failed || 0) + ' 项。');
    }
    for (const [id, key] of [['success', 'Success'], ['failed', 'Failed'], ['skipped', 'Skipped']]) $(id).textContent = sameTool ? state.Result?.[key] || 0 : 0;
    if (sameTool && state.PlanID) { if (previewPlan !== state.PlanID && !previewLoading) await loadPreview(state.PlanID); }
    else { if (previewPlan) resetPreview(); renderItems([], 0, sameTool ? state.Status : 'idle'); }
    const text = sameTool ? (state.Logs || []).join('\n') : '';
    if (text !== lastLogs || displayTool !== tool) { lastLogs = text; displayTool = tool; $('logs').textContent = text || '准备就绪。'; $('logs').scrollTop = $('logs').scrollHeight; }
    updateActions();
  } catch (e) { $('message').textContent = '连接失败：' + e.message + '。请确认工具箱程序仍在运行。'; }
  finally { pollPending = false; }
}
function choosePath(path) {
  putValue(browseTarget, path);
  if (browseTarget === 'FFmpeg' && $('FFprobe') && !$('FFprobe').value) putValue('FFprobe', path.replace(/ffmpeg\.exe$/i, 'ffprobe.exe'));
  if (browseTarget === 'FFprobe' && $('FFmpeg') && !$('FFmpeg').value) putValue('FFmpeg', path.replace(/ffprobe\.exe$/i, 'ffmpeg.exe'));
  changed(browseTarget !== 'Root'); $('folderDialog').close();
}
async function browse(path) {
  try {
    const data = await api('browse', { Path: path, Kind: browseKind }); currentPath = data.Path; parentPath = data.Parent;
    $('browsePath').value = data.Path; $('folderList').replaceChildren(); $('browseError').textContent = '';
    $('selectFolder').hidden = browseKind !== ''; $('selectFolder').disabled = !data.Path;
    for (const p of data.Dirs) { const b = document.createElement('button'); b.textContent = '▸ ' + p; b.onclick = () => browse(p); $('folderList').append(b); }
    for (const p of data.Files || []) { const b = document.createElement('button'); b.textContent = '选择文件 · ' + p; b.className = 'file-choice'; b.onclick = () => choosePath(p); $('folderList').append(b); }
    if (!data.Dirs.length && !(data.Files || []).length) $('browseError').textContent = browseKind ? '此目录没有可选择的程序或字体文件。可返回上一级继续查找。' : '此目录没有子目录，可以直接选择当前目录。';
  } catch (e) { $('browseError').textContent = e.message; }
}
function openPicker(key) {
  browseTarget = key; browseKind = key === 'Root' ? '' : key === 'Font' ? 'font' : 'program';
  $('pickerTitle').textContent = key === 'Root' ? '选择输入目录' : key === 'Font' ? '选择字体文件' : '选择程序文件';
  const value = $(key).value; let path = key === 'Root' ? value : value.replace(/[\\/][^\\/]+$/, '');
  if (path === value && key !== 'Root') path = ''; $('folderDialog').showModal(); browse(path);
}
$('browse').onclick = () => openPicker('Root'); $('closeDialog').onclick = () => $('folderDialog').close();
$('goto').onclick = () => browse($('browsePath').value);
$('browsePath').onkeydown = event => { if (event.key === 'Enter') { event.preventDefault(); browse($('browsePath').value); } };
$('drives').onclick = () => browse(''); $('up').onclick = () => browse(parentPath); $('selectFolder').onclick = () => choosePath(currentPath);
(async () => {
  try {
    const [detected, preferences, info] = await Promise.all([api('defaults'), api('settings'), api('info')]); defaults = detected; workspaces = preferences.Workspaces || {};
    $('appVersion').textContent = info.Version === 'dev' ? '开发版' : 'v' + info.Version;
    $('bundleNotice').hidden = !info.BundleWarning; $('bundleNotice').textContent = info.BundleWarning || '';
    if (!Object.keys(workspaces).length) {
      try { const old = JSON.parse(localStorage.getItem('media-toolbox-settings') || '{}'); if (tools.some(t => t[0] === old.Tool)) { workspaces[old.Tool] = old; preferences.LastTool = old.Tool; } } catch {}
    }
    await selectTool(tools.some(t => t[0] === preferences.LastTool) ? preferences.LastTool : 'format', true); setInterval(poll, 1200);
  } catch (e) { $('message').textContent = '初始化失败：' + e.message; switching = false; updateActions(); }
})();
