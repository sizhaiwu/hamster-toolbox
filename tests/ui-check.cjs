// Isolated DOM tests: no browser, network, or real user folders are accessed.
// Setup: npm ci --ignore-scripts
const assert = require('node:assert/strict');
const fs = require('node:fs');
const { JSDOM } = require('jsdom');
const html = fs.readFileSync('web/index.html', 'utf8'), script = fs.readFileSync('web/app.js', 'utf8');
const defaults = { Tool: '', Root: '', Output: '', Recursive: true, Files: true, Dirs: true, Find: '', Replace: '', Regex: false, Prefix: '', Suffix: '', Quality: 60, CRF: 23, MinMB: 100, MinKbps: 1000, FFmpeg: 'C:/ffmpeg/ffmpeg.exe', FFprobe: 'C:/ffmpeg/ffprobe.exe', Magick: 'C:/magick/magick.exe', Exiftool: '', Font: 'C:/Windows/Fonts/msyh.ttc', NumberName:'',StartNumber:1,NumberDigits:3,NumberStyle:'dash',NumberOrder:'name',NumberFilter:'',NumberKeepOriginal:true };
Object.assign(defaults,{ThumbnailColumns:5,ThumbnailRows:5,ThumbnailWidth:1024,ThumbnailGap:5,ThumbnailQuality:90,ThumbnailFormat:'jpg',ThumbnailBackground:'#ffffff',ThumbnailInfo:true,ThumbnailTime:true});
const requirements = { format: [], rename: [], number:[], describe: ['FFprobe'], image: ['Magick'], exif: ['Exiftool'], pdf: ['Magick', 'Font'], video: ['FFmpeg', 'FFprobe'], thumbnail:['FFmpeg','FFprobe','Magick','Font'] };
const storage = { Version: 1, LastTool: 'format', Workspaces: {
  // Old/stale values from an unrelated workspace must not block native clicks.
  format: { ...defaults, Tool:'format', CRF:99, MinMB:-1 },
  rename: { ...defaults, Tool:'rename', Root:'E:/测试输入', CRF:99, MinKbps:-1 }
} };
let planMode = 'conflicts';
let state = { Status: 'idle', Tool: '', Items: [], Logs: [], Result: {}, Total: 0 };
const saved = [], requests = [], failures = [];
const delay = ms => new Promise(resolve => setTimeout(resolve, ms));
async function until(test, label) { for (let i=0;i<120;i++) { if (test()) return; await delay(25); } throw Error('Timed out: '+label); }
function createUI(port) {
  const dom = new JSDOM(html, { url:'http://localhost:'+port+'/?session=test', runScripts:'outside-only' });
  const w=dom.window;
  w.addEventListener('error', e => failures.push(e.error || e.message));
  w.HTMLDialogElement.prototype.showModal = function(){this.open=true};
  w.HTMLDialogElement.prototype.close = function(){this.open=false};
  w.fetch = async (url, options) => {
    const body=JSON.parse(options.body), path=url.replace('/api/','');requests.push({path,body});
    let value;
    if(path==='defaults') value=defaults;
    else if(path==='info') value={Name:'仓鼠工具箱',Version:'0.1.0'};
    else if(path==='settings') value=storage;
    else if(path==='number-info') value={Name:'Sample作者',Style:body.NumberStyle==='custom'?'custom':'dash',MaxNumber:98,NextNumber:99,Digits:3,Existing:98,Pending:100};
    else if(path==='save-settings') { const c={...body};saved.push(c);storage.Workspaces[c.Tool]=c;storage.LastTool=c.Tool;value=storage; }
    else if(path==='dependencies') value=requirements[body.Config.Tool].map(key=>({
      Key:key,Name:key,Purpose:'测试用途',Path:body.Config[key]||'',Available:Boolean(body.Config[key])&&!body.Config[key].includes('missing'),
      Message:Boolean(body.Config[key])&&!body.Config[key].includes('missing')?'已找到':'尚未配置',
      Instructions:'解压后选择程序文件。',DownloadURL:'https://example.com/download',GuideURL:'https://example.com/help'
    }));
    else if(path==='state') value=state;
    else if(path==='preview') {
      let rows=state.Items||[];if(body.Filter==='conflicts')rows=rows.filter(x=>x.Conflict);if(body.Filter==='ready')rows=rows.filter(x=>!x.Conflict);
      value={PlanID:state.PlanID,Items:rows.slice(body.Offset,body.Offset+body.Limit),Offset:body.Offset,Limit:body.Limit,Total:rows.length};
    }
    else if(path==='execute') return {ok:false,text:async()=> '测试：预览已失效，请重新扫描'};
    else if(path==='plan') {
      if(planMode==='error')return {ok:false,text:async()=> '输入目录不存在'};
      const empty=planMode==='zero';
      state={Status:'ready',Tool:body.Tool,Items:[],Logs:[],Result:{},Total:empty?0:2,Actionable:0,Conflicts:empty?0:2,PlanID:'test-plan-'+requests.filter(r=>r.path==='plan').length};value={ok:true};
      if(body.Tool==='number')state={...state,Total:100,Actionable:100,Conflicts:0,Items:[{Source:'资源1 [1P-1B]',Target:body.NumberName+'_NO.099 资源1 [1P-1B]',Conflict:''}]};
    }
    else throw Error('Unexpected API '+path);
    return {ok:true,json:async()=>JSON.parse(JSON.stringify(value))};
  };
  w.eval(script);return dom;
}
(async()=>{
  let dom=createUI(10001),w=dom.window,$=id=>w.document.getElementById(id);
  const switchTool=async id=>{
    w.document.querySelector('[data-tool="'+id+'"]').click();
    await until(()=>$('title').textContent===({video:'压缩视频',image:'压缩图片',pdf:'PDF 图册索引',exif:'清除图片信息',format:'目录统计命名',describe:'媒体信息命名',rename:'批量重命名',number:'目录批量编号',thumbnail:'视频缩略图'})[id]&&!$('nav').firstChild.disabled,'switch '+id);
  };
  const input=(id,value)=>{$(id).value=value;$(id).dispatchEvent(new w.Event('input',{bubbles:true}));};
  await until(()=>$('builtInReady').hidden===false&&!$('preview').disabled,'built-in ready');
  assert.equal($('dependencies').hidden,true);assert.equal($('FFmpeg'),null);
  assert.equal($('CRF').disabled,true);assert.equal($('MinMB').disabled,true);
  input('Root','E:/测试输入');
  planMode='zero';let planCount=requests.filter(r=>r.path==='plan').length;
  $('preview').click();
  await until(()=>requests.filter(r=>r.path==='plan').length===planCount+1&&$('message').textContent.includes('没有需要修改'),'native format click and empty feedback');
  await switchTool('rename');assert.equal($('CRF').disabled,true);assert.equal($('MinKbps').disabled,true);
  planCount=requests.filter(r=>r.path==='plan').length;$('preview').click();
  await until(()=>requests.filter(r=>r.path==='plan').length===planCount+1&&$('message').textContent.includes('没有需要修改'),'native rename click and empty feedback');
  planMode='error';$('preview').click();await until(()=>$('message').textContent.includes('输入目录不存在'),'request failure shown beside button');
  assert.equal($('preview').disabled,false);
  planMode='conflicts';await switchTool('format');
  input('Root','E:/测试输入');await switchTool('video');
  assert.ok($('FFmpeg'));assert.ok($('FFprobe'));assert.equal($('Magick'),null);assert.equal($('Font'),null);
  assert.equal($('Root').value,'E:/测试输入');assert.equal($('Output').value,'');
  assert.equal(w.document.querySelectorAll('.dependency-links a').length,4);
  for(const a of w.document.querySelectorAll('.dependency-links a'))assert.equal(a.rel,'noopener noreferrer');
  input('Output','E:/视频输出');$('videoQuality').value='28';$('videoQuality').dispatchEvent(new w.Event('change'));assert.equal($('CRF').value,'28');
  input('FFprobe','C:/missing/ffprobe.exe');await until(()=>$('dependency-status-FFprobe').textContent==='尚未配置','missing dependency');
  assert.equal($('preview').disabled,true);assert.equal($('dependency-help-FFprobe').open,true);
  input('FFprobe','C:/special/ffprobe.exe');await until(()=>!$('preview').disabled,'dependency restored');
  await switchTool('image');assert.equal($('Output').value,'');assert.ok($('Magick'));assert.equal($('FFmpeg'),null);
  assert.equal($('ImageFormatMode').value,'jpeg');assert.match($('imageFormatSummary').textContent,/JPG/);
  $('ImageFormatMode').value='all';$('ImageFormatMode').dispatchEvent(new w.Event('change'));
  assert.deepEqual([...$('imageFormatCheckboxes').querySelectorAll('input')].map(input=>input.value),['.jpg','.jpeg','.jpe','.jfif','.png','.webp']);
  planCount=requests.filter(r=>r.path==='plan').length;$('preview').click();await until(()=>requests.filter(r=>r.path==='plan').length===planCount+1&&!$('preview').disabled,'supported formats only');
  assert.deepEqual(requests.filter(r=>r.path==='plan').at(-1).body.ImageFormats,['.jpg','.jpeg','.jpe','.jfif','.png','.webp']);
  $('ImageFormatMode').value='png';$('ImageFormatMode').dispatchEvent(new w.Event('change'));assert.match($('imageFormatSummary').textContent,/本次仅处理：PNG/);
  assert.equal($('pngOptions').hidden,false);assert.equal($('PngMode').value,'lossless');assert.equal($('pngQualityOptions').hidden,true);assert.equal($('generalImageQuality').hidden,true);
  $('PngMode').value='webp-png';$('PngMode').dispatchEvent(new w.Event('change'));input('PngQuality','92');
  assert.equal($('pngQualityOptions').hidden,false);assert.equal($('pngQualityValue').textContent,'92');assert.match($('pngModeHint').textContent,/内部实际是 WebP/);assert.match($('pngModeHint').textContent,/可能打不开/);
  planCount=requests.filter(r=>r.path==='plan').length;$('preview').click();await until(()=>requests.filter(r=>r.path==='plan').length===planCount+1&&!$('preview').disabled,'PNG-only preview');
  assert.deepEqual(requests.filter(r=>r.path==='plan').at(-1).body.ImageFormats,['.png']);
  assert.equal(requests.filter(r=>r.path==='plan').at(-1).body.PngMode,'webp-png');assert.equal(requests.filter(r=>r.path==='plan').at(-1).body.PngQuality,92);
  $('ImageFormatMode').value='custom';$('ImageFormatMode').dispatchEvent(new w.Event('change'));
  const formatInput=ext=>[...$('imageFormatCheckboxes').querySelectorAll('input')].find(input=>input.value===ext);
  formatInput('.jpg').click();assert.match($('imageFormatSummary').textContent,/JPG/);assert.match($('imageFormatSummary').textContent,/PNG/);
  planCount=requests.filter(r=>r.path==='plan').length;$('preview').click();await until(()=>requests.filter(r=>r.path==='plan').length===planCount+1&&!$('preview').disabled,'custom formats preview');
  assert.deepEqual(requests.filter(r=>r.path==='plan').at(-1).body.ImageFormats,['.jpg','.png']);
  formatInput('.jpg').click();formatInput('.png').click();assert.equal($('preview').disabled,true);assert.match($('imageFormatSummary').textContent,/至少勾选一种/);
  $('ImageFormatMode').value='png';$('ImageFormatMode').dispatchEvent(new w.Event('change'));assert.equal($('preview').disabled,false);
  input('Output','E:/图片输出');input('Quality','75');await switchTool('thumbnail');
  assert.equal($('thumbnailOptions').hidden,false);assert.equal($('imageOptions').hidden,true);assert.equal($('videoOptions').hidden,true);
  assert.equal($('Output').value,'');assert.match($('outputHint').textContent,/视频预览图/);assert.match($('thumbnailSummary').textContent,/5 列 × 5 行，共 25/);
  for(const key of ['FFmpeg','FFprobe','Magick','Font'])assert.ok($(key));
  assert.equal($('CRF').disabled,true);assert.equal($('Quality').disabled,true);assert.equal($('ThumbnailWidth').disabled,false);
  input('ThumbnailColumns','4');input('ThumbnailRows','3');input('ThumbnailWidth','1280');input('ThumbnailGap','7');input('ThumbnailBackground','#224466');
  $('ThumbnailInfo').click();$('ThumbnailFormat').value='png';$('ThumbnailFormat').dispatchEvent(new w.Event('change'));
  assert.match($('thumbnailSummary').textContent,/4 列 × 3 行，共 12/);assert.equal($('thumbnailQualityOptions').hidden,true);assert.equal($('ThumbnailQuality').disabled,true);
  input('Output','E:/缩略图输出');planCount=requests.filter(r=>r.path==='plan').length;$('preview').click();
  await until(()=>requests.filter(r=>r.path==='plan').length===planCount+1&&!$('preview').disabled,'thumbnail preview');
  const thumbnailRequest=requests.filter(r=>r.path==='plan').at(-1).body;
  assert.equal(thumbnailRequest.Tool,'thumbnail');assert.equal(thumbnailRequest.ThumbnailFormat,'png');assert.equal(thumbnailRequest.ThumbnailInfo,false);assert.equal(thumbnailRequest.ThumbnailTime,true);assert.equal(thumbnailRequest.ThumbnailColumns,4);assert.equal(thumbnailRequest.ThumbnailRows,3);assert.equal(thumbnailRequest.ThumbnailBackground,'#224466');assert.equal('CRF' in thumbnailRequest,false);assert.equal('ImageFormats' in thumbnailRequest,false);
  await switchTool('pdf');assert.equal($('thumbnailOptions').hidden,true);assert.equal($('ThumbnailWidth').disabled,true);
  assert.equal($('Output').value,'');assert.equal($('recursiveOption').hidden,true);assert.ok($('Font'));assert.equal($('outputLabel').textContent,'保存为 PDF 文件');
  await switchTool('video');assert.equal($('Output').value,'E:/视频输出');assert.equal($('CRF').value,'28');assert.equal($('FFprobe').value,'C:/special/ffprobe.exe');
  await switchTool('describe');assert.equal($('FFprobe').value,defaults.FFprobe);assert.equal($('FFmpeg'),null);
  await switchTool('exif');assert.equal($('preview').disabled,true);assert.ok($('Exiftool'));assert.equal($('Magick'),null);
  await switchTool('rename');assert.equal($('dependencies').hidden,true);assert.equal($('preview').disabled,false);
  input('Find','^test\\s*第\\d+期\\s*');input('Replace','');
  assert.equal($('Regex').checked,false);$('RenameMode').value='regex';$('RenameMode').dispatchEvent(new w.Event('change'));
  assert.equal($('Regex').checked,true);assert.match($('renameModeHint').textContent,/正则模式已启用/);assert.equal($('renameRegexHelp').hidden,false);
  $('preview').click();
  await until(()=>$('statusBadge').textContent==='预览就绪'&&$('statusText').textContent.includes('冲突 2'),'conflict preview');assert.equal($('execute').disabled,true);
  const regexRequest=requests.filter(r=>r.path==='plan').at(-1).body;assert.equal(regexRequest.Regex,true);assert.equal(regexRequest.Find,'^test\\s*第\\d+期\\s*');assert.equal(regexRequest.Replace,'');
  $('RenameMode').value='text';$('RenameMode').dispatchEvent(new w.Event('change'));assert.match($('renameModeHint').textContent,/普通文字模式/);assert.equal($('renameRegexHelp').hidden,true);
  $('RenameMode').value='example';$('RenameMode').dispatchEvent(new w.Event('change'));input('Find','test 第001期');input('Replace','新名称_{编号}');
  assert.equal($('renameExampleOptions').hidden,false);assert.equal($('renameAdvancedOptions').hidden,true);assert.equal($('renameRegexHelp').hidden,true);assert.equal($('Regex').checked,false);
  assert.equal($('ExampleNumber').checked,true);assert.equal($('ExampleAtStart').checked,true);
  planCount=requests.filter(r=>r.path==='plan').length;$('preview').click();
  await until(()=>requests.filter(r=>r.path==='plan').length===planCount+1&&!$('preview').disabled,'example rename submission');
  const exampleRequest=requests.filter(r=>r.path==='plan').at(-1).body;assert.equal(exampleRequest.RenameMode,'example');assert.equal(exampleRequest.Find,'test 第001期');assert.equal(exampleRequest.Replace,'新名称_{编号}');assert.equal(exampleRequest.ExampleNumber,true);assert.equal(exampleRequest.ExampleAtStart,true);assert.equal(exampleRequest.Regex,false);
  $('RenameMode').value='regex';$('RenameMode').dispatchEvent(new w.Event('change'));
  $('renamePreset').value='periodPrefix';$('renamePreset').dispatchEvent(new w.Event('change'));assert.equal($('Regex').checked,true);assert.equal($('Find').value,'^.*?第\\d+期\\s*');assert.equal($('Replace').value,'');
  assert.ok(saved.every(c=>!(c.Tool==='rename'&&'FFmpeg' in c)));
  await switchTool('number');assert.equal($('dependencies').hidden,true);assert.equal($('recursiveOption').hidden,true);
  assert.equal($('NumberName').value,'Sample作者');assert.equal($('StartNumber').value,'99');assert.equal($('NumberDigits').value,'3');assert.equal($('NumberKeepOriginal').checked,true);
  $('NumberStyle').value='space';$('NumberStyle').dispatchEvent(new w.Event('change'));assert.match($('numberSample').textContent,/Sample作者 NO\.099/);
  $('NumberStyle').value='custom';$('NumberStyle').dispatchEvent(new w.Event('change'));input('NumberTemplate','{name} 第{number}期');
  assert.equal($('numberTemplateOption').hidden,false);assert.equal($('NumberTemplate').disabled,false);assert.match($('numberSample').textContent,/Sample作者 第099期/);
  await delay(550);assert.equal(saved.filter(c=>c.Tool==='number').at(-1).NumberTemplate,'{name} 第{number}期');
  input('NumberTemplate','{name} {number}');assert.match($('numberSample').textContent,/Sample作者 099/);assert.equal($('numberSample').textContent.includes('NO'),false);
  $('detectNumbering').click();await until(()=>!$('detectNumbering').disabled,'custom numbering inspection');
  assert.equal($('NumberStyle').value,'custom');assert.equal($('NumberTemplate').value,'{name} {number}');
  assert.equal(requests.filter(r=>r.path==='number-info').at(-1).body.NumberTemplate,'{name} {number}');
  assert.equal(saved.filter(c=>c.Tool==='number').at(-1).NumberTemplate,'{name} {number}');
  $('NumberStyle').value='underscore';$('NumberStyle').dispatchEvent(new w.Event('change'));input('StartNumber','99');
  assert.equal($('numberTemplateOption').hidden,true);assert.equal($('NumberTemplate').disabled,true);
  assert.match($('numberSample').textContent,/Sample作者_NO\.099/);
  planCount=requests.filter(r=>r.path==='plan').length;$('preview').click();
  await until(()=>requests.filter(r=>r.path==='plan').length===planCount+1&&$('message').textContent.includes('可执行 100'),'number preview');
  assert.equal($('execute').disabled,false);assert.match($('items').textContent,/NO\.099/);
  const numberRequest=requests.filter(r=>r.path==='plan').at(-1).body;assert.equal(numberRequest.StartNumber,99);assert.equal(numberRequest.NumberDigits,3);assert.equal(numberRequest.NumberKeepOriginal,true);assert.equal('FFmpeg' in numberRequest,false);
  state={...state,PlanID:'large-preview',Total:1205,Actionable:1204,Conflicts:1,Items:Array.from({length:1205},(_,i)=>({Source:'source-'+i,Target:'target-'+i,Conflict:i===1204?'最后一项冲突':''}))};
  await until(()=>$('previewPageInfo').textContent.includes('1–100 / 1205'),'paged large preview');
  assert.equal($('items').children.length,100);$('previewNext').click();await until(()=>$('previewPageInfo').textContent.includes('101–200'),'next preview page');assert.match($('items').firstChild.textContent,/source-100/);
  $('previewFilter').value='conflicts';$('previewFilter').dispatchEvent(new w.Event('change'));await until(()=>$('previewPageInfo').textContent.includes('1–1 / 1'),'conflict filter');assert.match($('items').textContent,/最后一项冲突/);
  $('previewFilter').value='all';$('previewFilter').dispatchEvent(new w.Event('change'));await until(()=>$('previewPageInfo').textContent.includes('1–100 / 1205'),'restore all preview');
  const executeCount=requests.filter(r=>r.path==='execute').length;$('execute').click();$('execute').click();
  await until(()=>$('message').textContent.includes('预览已失效'),'execute error surfaced');assert.equal(requests.filter(r=>r.path==='execute').length,executeCount+1);
  await delay(1300);assert.match($('message').textContent,/预览已失效/);
  await switchTool('image');await delay(550);dom.window.close();
  dom=createUI(20002);w=dom.window;$=id=>w.document.getElementById(id);
  await until(()=>$('title').textContent==='压缩图片'&&!$('nav').firstChild.disabled,'restart on another port');
  assert.equal($('Output').value,'E:/图片输出');assert.equal($('Quality').value,'75');
  assert.equal($('ImageFormatMode').value,'png');assert.deepEqual(storage.Workspaces.image.ImageFormats,['.png']);
  assert.equal($('PngMode').value,'webp-png');assert.equal($('PngQuality').value,'92');
  assert.equal(storage.Workspaces.number.NumberTemplate,'{name} {number}');
  await switchTool('thumbnail');assert.equal($('Output').value,'E:/缩略图输出');assert.equal($('ThumbnailColumns').value,'4');assert.equal($('ThumbnailRows').value,'3');assert.equal($('ThumbnailFormat').value,'png');assert.equal($('ThumbnailBackground').value,'#224466');assert.equal($('ThumbnailInfo').checked,false);
  assert.equal(failures.length,0,failures.map(String).join('\n'));dom.window.close();
  console.log('PASS: tool-scoped dependencies, download links, missing-program guidance, presets, output isolation, independent settings, conflict blocking, directory numbering from 099, restart persistence');
})().catch(e=>{console.error(e);process.exitCode=1;setTimeout(()=>process.exit(1),100)});
