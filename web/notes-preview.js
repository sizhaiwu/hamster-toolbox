// This fixed bridge runs in an opaque-origin iframe with only this script hash allowed.
(()=>{
 for(const code of document.querySelectorAll('pre code'))if(code.textContent.length<=200000)window.hamsterNoteSyntax.paintCode(code);
 let anchors=[];
 function measure(){anchors=Array.from(document.querySelectorAll('[data-note-line]')).map(el=>({line:Number(el.dataset.noteLine),top:el.getBoundingClientRect().top+scrollY})).filter(a=>Number.isFinite(a.line)).sort((a,b)=>a.line-b.line);parent.postMessage({type:'hamster-note-preview-ready'},'*')}
 let measured=false;function queue(){if(measured)return;measured=true;requestAnimationFrame(()=>{measured=false;measure()})}
 addEventListener('message',event=>{const data=event.data;if(event.source!==parent||data?.type!=='hamster-note-scroll'||!Number.isFinite(data.line)||!Number.isFinite(data.ratio))return;
 const max=Math.max(0,document.documentElement.scrollHeight-innerHeight);let top=max*Math.min(1,Math.max(0,data.ratio));
 if(anchors.length){let low=0,high=anchors.length;while(low<high){const mid=(low+high)>>1;if(anchors[mid].line<=data.line)low=mid+1;else high=mid}const before=anchors[Math.max(0,low-1)],after=anchors[low];top=before.top;if(after&&after.line>before.line)top+=(after.top-before.top)*Math.min(1,Math.max(0,(data.line-before.line)/(after.line-before.line)));else top+=Math.max(0,max-before.top)*Math.min(1,Math.max(0,data.fraction||0));}
 if(data.ratio>=.999)top=max;else if(data.ratio<=0)top=0;scrollTo({top:Math.max(0,Math.min(max,top)),behavior:'instant'});
 });
 addEventListener('keydown',event=>{if(event.key==='Escape')parent.postMessage({type:'hamster-note-preview-escape'},'*')});
 addEventListener('load',queue);addEventListener('resize',queue);document.addEventListener('load',queue,true);new ResizeObserver(queue).observe(document.body);queue();
})();
