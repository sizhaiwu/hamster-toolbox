const $ = id => document.getElementById(id);
const tools = [
  ['format', '目录统计命名', '按图片、视频、音频数量与总大小，统一目录名称。', '▤'],
  ['rename', '批量重命名', '批量替换名称、添加前后缀，支持正则表达式。', 'Aa'],
  ['number', '目录批量编号', '给新增资源目录连续编号，可指定起始值或接着已有编号继续。', '#'],
  ['describe', '媒体信息命名', '把音视频的分辨率、时长和体积添加到文件名。', '▷'],
  ['exif', '清理元数据', '清理图片和视频中的标题、注释、作者等元数据，保留原文件。', '◇'],
  ['pdf', 'PDF 图册索引', '每个直接子目录生成一页：中文目录名称 + 首张可读取的图片。', '▧'],
  ['image', '压缩图片', '缩小图片体积，保留原图与目录结构。', '▨'],
  ['video', '压缩视频', '按清晰度压缩视频，可选 H.264 / H.265，自动尝试显卡加速并保留原视频。', '▸'],
  ['thumbnail', '视频缩略图', '均匀截取视频画面，生成带文件信息和时间的预览拼图。', '▦'],
  ['archive', '打包资源', '使用 7-Zip 将目录与文件独立打包为 7z，支持分卷、加密、附加文件与自动校验。', '▣'],
  ['extract', '解压资源', '使用 7-Zip 批量解压 7z、ZIP、RAR 和分卷包，默认自动改名并保留原包。', '⇲'],
  ['dedupe', '文件去重', '使用 Video Duplicate Finder 查找重复或相似的视频和图片，支持打开本地软件与下载指引。', '≋']
];
const optionKeys = {
  dedupe: ['DedupeProgram'],
  extract: ['Recursive','Output','ExtractConflict','ExtractLayout','ExtractPassword','ExtractDeleteSource','ExtractDeleteMode'],
  archive: ['Output','Files','Dirs','ArchiveMode','ArchivePassword','ArchiveExtras','ArchiveMarker','ArchiveSplit','ArchiveFirstVolume','ArchiveVolume','ArchiveDirPrefix','ArchiveFilePrefix','ArchiveStart','ArchiveDigits','ArchiveLevel'],
  format: ['Recursive','FormatCleanOther','FormatDeleteMode'], rename: ['Recursive', 'Files', 'Dirs', 'Find', 'Replace', 'Regex', 'Prefix', 'Suffix', 'RenameMode', 'ExampleNumber', 'ExampleAtStart'],
  number: ['NumberName', 'StartNumber', 'NumberDigits', 'NumberStyle', 'NumberOrder', 'NumberFilter', 'NumberKeepOriginal'],
  describe: ['Recursive'], exif: ['Recursive', 'Output', 'MetadataTypes'], pdf: ['Output'],
  image: ['Recursive', 'Output', 'Quality', 'PngMode', 'PngQuality'], video: ['Recursive', 'Output', 'VideoMode', 'VideoAcceleration', 'CRF', 'MinMB', 'MinKbps'],
  thumbnail: ['Recursive', 'Output', 'ThumbnailColumns', 'ThumbnailRows', 'ThumbnailWidth', 'ThumbnailGap', 'ThumbnailQuality', 'ThumbnailFormat', 'ThumbnailBackground', 'ThumbnailInfo', 'ThumbnailTime']
};
const boolKeys = ['FormatCleanOther','ExtractDeleteSource','ArchiveMarker','Recursive', 'Files', 'Dirs', 'Regex', 'NumberKeepOriginal', 'ExampleNumber', 'ExampleAtStart', 'ThumbnailInfo', 'ThumbnailTime'], numericKeys = ['ArchiveStart','ArchiveDigits','ArchiveLevel','Quality', 'CRF', 'MinMB', 'MinKbps', 'StartNumber', 'NumberDigits', 'PngQuality', 'ThumbnailColumns', 'ThumbnailRows', 'ThumbnailWidth', 'ThumbnailGap', 'ThumbnailQuality'];
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
let dedupeAvailable=false, dedupeChecking=false, dedupeLaunching=false, dedupeRequest=0, dedupeTimer;
let savedPasswords = [], passwordTarget = 'ArchivePassword', passwordEditID = '', passwordLoading = false, passwordVaultBusy = false;
let previewPlan = '', previewOffset = 0, previewTotal = 0, previewRows = [], previewRequest = 0, previewLoading = false;
let lastActionError = '';
let followLogs = true;
async function api(path, data = {}) {
  const response = await fetch('/api/' + path, { method: 'POST', headers: { 'Content-Type': 'application/json', 'X-Toolbox-Token': token }, body: JSON.stringify(data) });
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
function updateActions() {
  const locked = busy || switching || submitting || detectingNumbering || passwordLoading || dedupeLaunching;
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
  for (const b of $('nav').children) b.disabled = locked;
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
    input.placeholder = d.Key === 'Font' ? '选择本机的字体文件' : '选择 ' + ({ FFmpeg: 'ffmpeg.exe', FFprobe: 'ffprobe.exe', Magick: 'magick.exe', Exiftool: 'exiftool.exe', SevenZip: '7z.exe' })[d.Key];
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
  if (tool !== 'video') return;
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
  if ((busy || switching) && !initial) return; switching = true; updateActions();
  try {
    if (!initial) await saveNow(); dependencyRequest++; clearTimeout(checkTimer); dedupeRequest++; clearTimeout(dedupeTimer); dedupeChecking=false;
    tool = id; checking = false; planID = ''; lastConfig = '';
    resetPreview();
    const t = tools.find(t => t[0] === id);
    $('title').textContent = $('crumb').textContent = t[1]; $('description').textContent = t[2]; $('tool-number').textContent = String(tools.indexOf(t) + 1).padStart(2, '0');
    for (const b of $('nav').children) b.classList.toggle('active', b.dataset.tool === id);
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
    const active = { ...defaults, ...(workspaces[id] || {}), Tool: id };
    if (!workspaces[id] && !initial) active.Root = $('Root').value;
    for (const [key, value] of Object.entries(active)) putValue(key, value);
    if (id === 'rename') {
      putValue('RenameMode', active.RenameMode || (active.Regex ? 'regex' : 'text'));
      if (!active.RenameMode) { putValue('ExampleNumber', true); putValue('ExampleAtStart', true); }
    }
    if (id === 'number' && !$('NumberTemplate').value) putValue('NumberTemplate', '{name} 第{number}期');
    if (id === 'image') { putValue('PngMode', active.PngMode || 'lossless'); putValue('PngQuality', active.PngQuality || 90); restoreImageFormats(active.ImageFormats); }
    if (id === 'extract') putValue('ExtractDeleteMode',active.ExtractDeleteMode || 'recycle');
    if (id === 'exif') putValue('MetadataTypes', active.MetadataTypes || (workspaces[id] ? 'images' : 'all'));
    if (id === 'video') {
      putValue('VideoMode', active.VideoMode || (workspaces[id] ? 'compatible' : 'efficient'));
      putValue('VideoAcceleration', active.VideoAcceleration || 'auto');
      updateVideoOptions();
    }
    // Blank outputs on a new tool; they must not leak from another workspace.
    if (!workspaces[id]) putValue('Output', '');
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
for (const el of $('form').querySelectorAll('input, textarea')) el.addEventListener('input', () => {
  $('qualityValue').textContent = $('Quality').value; if (el.id === 'CRF') updateVideoPreset(); changed();
});
for (const id of ['NumberStyle', 'NumberOrder']) $(id).addEventListener('change', () => changed());
for (const id of ['VideoMode', 'VideoAcceleration']) $(id).addEventListener('change', () => changed());
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
  }
  renderSavedPasswords();
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
    const data = await api('password-get',{ID:id}); putValue(key,data.Password); changed();
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
  $(key+'SavedPassword').onchange=async()=>{const id=$(key+'SavedPassword').value;if(!id||busy||switching)return;try{await useSavedPassword(id,key+'Password');}catch(e){showMessage(e.message,true);}finally{$(key+'SavedPassword').value='';}};
  $('manage'+key+'Passwords').onclick=async()=>{if(busy||switching)return;passwordTarget=key+'Password';resetPasswordEditor();$('passwordError').textContent='';$('passwordDialog').showModal();setPasswordVaultBusy(true);try{await refreshSavedPasswords();}catch(e){$('passwordError').textContent=e.message;}finally{setPasswordVaultBusy(false);}};
}
$('closePasswordDialog').onclick=()=>$('passwordDialog').close();
$('newPassword').onclick=()=>resetPasswordEditor();
$('savePassword').onclick=async()=>{
  if(passwordVaultBusy)return;setPasswordVaultBusy(true);$('passwordError').textContent='';
  try {const password=$('PasswordValue').value;await api('password-save',{ID:passwordEditID,Name:$('PasswordName').value,Password:password});await refreshSavedPasswords();putValue(passwordTarget,password);changed();$('passwordDialog').close();}
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
$('preview').onclick = async () => {
  if (tool === 'dedupe') return;
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
$('form').onsubmit = event => { event.preventDefault(); $('preview').click(); };
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
    if (tool === 'video' && !item.Conflict && previewPlan) comparison.append(videoCommandDetails(item, previewPlan));
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
      : /^(正在|视频进度)/.test(message) ? 'progress' : 'info';
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
  const summary = document.createElement('summary'); summary.textContent = '查看 / 复制压缩命令';
  const message = document.createElement('p'); message.className = 'hint'; message.setAttribute('role', 'status');
  const selector = document.createElement('select'); selector.setAttribute('aria-label', '选择压缩命令'); selector.hidden = true;
  const code = document.createElement('textarea'); code.readOnly = true; code.rows = 5; code.hidden = true; code.setAttribute('aria-label', 'PowerShell 压缩命令'); code.spellcheck = false;
  const copy = document.createElement('button'); copy.type = 'button'; copy.textContent = '复制 PowerShell 命令'; copy.hidden = true;
  const note = document.createElement('p'); note.className = 'hint'; note.hidden = true;
  note.textContent = '粘贴到 Windows PowerShell 执行。自动创建输出目录，不覆盖已有文件。独立命令不含工具箱的自动回退、结果校验和“未变小就丢弃”；完成后请自行检查输出。';
  details.append(summary, message, selector, code, copy, note);
  let loaded = false, loading = false, commands = [];
  details.addEventListener('toggle', async () => {
    if (!details.open || loaded || loading) return;
    loading = true; message.textContent = '正在检测编码器并生成命令，不会压缩文件…';
    try {
      commands = await api('video-commands', { PlanID: id, Source: item.Source });
      if (!commands.length) throw Error('没有可用命令');
      for (const command of commands) { const option = document.createElement('option'); option.textContent = command.Label; selector.append(option); }
      selector.selectedIndex = 0; selector.hidden = commands.length < 2;
      code.value = commands[0].PowerShell; code.hidden = copy.hidden = note.hidden = false;
      message.textContent = '按本次扫描的参数生成；修改设置后请重新扫描，备用命令需手动选择执行。'; loaded = true;
    } catch (e) { message.textContent = '生成失败：' + e.message + '。关闭后重新展开可重试。'; }
    finally { loading = false; }
  });
  selector.addEventListener('change', () => { code.value = commands[selector.selectedIndex].PowerShell; copy.textContent = '复制 PowerShell 命令'; });
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
    const videoLine = sameTool && tool === 'video' && state.Status === 'running'
      ? [...(state.Logs || [])].reverse().find(line => /视频进度：|正在压缩视频：|正在回退：|正在重试：|视频编码完成：/.test(line)) : '';
    $('videoProgress').hidden = !videoLine;
    $('videoProgress').textContent = videoLine || '';
    if (text !== lastLogs || displayTool !== tool) { lastLogs = text; displayTool = tool; renderLogs(text); }
    updateActions();
  } catch (e) { $('message').textContent = '连接失败：' + e.message + '。请确认工具箱程序仍在运行。'; }
  finally { pollPending = false; }
}
function choosePath(path) {
  if (browseTarget === 'ArchiveExtras') { const lines = $('ArchiveExtras').value.split(/\r?\n/).filter(Boolean); if (!lines.includes(path)) lines.push(path); putValue(browseTarget, lines.join('\n')); } else putValue(browseTarget, path);
  if (browseTarget === 'DedupeProgram') { changed(); inspectDedupeProgram(); $('folderDialog').close(); return; }
  if (browseTarget === 'FFmpeg' && $('FFprobe') && !$('FFprobe').value) putValue('FFprobe', path.replace(/ffmpeg\.exe$/i, 'ffprobe.exe'));
  if (browseTarget === 'FFprobe' && $('FFmpeg') && !$('FFmpeg').value) putValue('FFmpeg', path.replace(/ffprobe\.exe$/i, 'ffmpeg.exe'));
  changed(!['Root','ArchiveExtras'].includes(browseTarget)); $('folderDialog').close();
}
async function browse(path) {
  try {
    const data = await api('browse', { Path: path, Kind: browseKind }); currentPath = data.Path; parentPath = data.Parent;
    $('browsePath').value = data.Path; $('folderList').replaceChildren(); $('browseError').textContent = '';
    $('selectFolder').hidden = browseKind !== ''; $('selectFolder').disabled = !data.Path;
    for (const p of data.Dirs) { const b = document.createElement('button'); b.textContent = '▸ ' + p; b.onclick = () => browse(p); $('folderList').append(b); }
    for (const p of data.Files || []) { const b = document.createElement('button'); b.textContent = '选择文件 · ' + p; b.className = 'file-choice'; b.onclick = () => choosePath(p); $('folderList').append(b); }
    if (!data.Dirs.length && !(data.Files || []).length) $('browseError').textContent = browseKind ? '此目录没有可选择的文件。可返回上一级继续查找。' : '此目录没有子目录，可以直接选择当前目录。';
  } catch (e) { $('browseError').textContent = e.message; }
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
    await selectTool(tools.some(t => t[0] === preferences.LastTool) ? preferences.LastTool : 'format', true); await refreshSavedPasswords().catch(() => {}); setInterval(poll, 1200);
  } catch (e) { $('message').textContent = '初始化失败：' + e.message; switching = false; updateActions(); }
})();
