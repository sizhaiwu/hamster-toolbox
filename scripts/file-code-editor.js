import {basicSetup} from 'codemirror';
import {EditorState,EditorSelection,Compartment,Transaction,countColumn} from '@codemirror/state';
import {EditorView,keymap,lineNumbers,drawSelection,highlightSpecialChars} from '@codemirror/view';
import {indentMore,indentLess,defaultKeymap} from '@codemirror/commands';
import {autocompletion,acceptCompletion,startCompletion,completionStatus} from '@codemirror/autocomplete';
import {StreamLanguage,HighlightStyle,syntaxHighlighting,indentUnit,getIndentUnit} from '@codemirror/language';
import {tags} from '@lezer/highlight';
import {javascript} from '@codemirror/lang-javascript';
import {json} from '@codemirror/lang-json';
import {html} from '@codemirror/lang-html';
import {css} from '@codemirror/lang-css';
import {markdown} from '@codemirror/lang-markdown';
import {python} from '@codemirror/lang-python';
import {sql} from '@codemirror/lang-sql';
import {xml} from '@codemirror/lang-xml';
import {yaml} from '@codemirror/lang-yaml';
import {go} from '@codemirror/lang-go';
import {shell} from '@codemirror/legacy-modes/mode/shell';
import {powerShell} from '@codemirror/legacy-modes/mode/powershell';
import {properties} from '@codemirror/legacy-modes/mode/properties';
import {toml} from '@codemirror/legacy-modes/mode/toml';
import {php} from '@codemirror/lang-php';
import {cpp,java} from '@codemirror/legacy-modes/mode/clike';

const modes={javascript:()=>javascript({jsx:true}),typescript:()=>javascript({typescript:true,jsx:true}),json,html,css,markdown,python,sql,xml,yaml,go,
 shell:()=>StreamLanguage.define(shell),powershell:()=>StreamLanguage.define(powerShell),ini:()=>StreamLanguage.define(properties),toml:()=>StreamLanguage.define(toml),php,cpp:()=>StreamLanguage.define(cpp),java:()=>StreamLanguage.define(java),text:()=>[]};
function detect(name){const ext=name.split('.').pop().toLowerCase();const extensions={js:'javascript',jsx:'javascript',mjs:'javascript',cjs:'javascript',ts:'typescript',tsx:'typescript',mts:'typescript',cts:'typescript',json:'json',html:'html',htm:'html',vue:'html',css:'css',md:'markdown',markdown:'markdown',py:'python',sql:'sql',xml:'xml',svg:'xml',yaml:'yaml',yml:'yaml',go:'go',sh:'shell',bash:'shell',zsh:'shell',ps1:'powershell',ini:'ini',conf:'ini',cfg:'ini',env:'ini',properties:'ini',toml:'toml',php:'php',c:'cpp',h:'cpp',cpp:'cpp',hpp:'cpp',cc:'cpp',java:'java'};return Object.hasOwn(extensions,ext)?extensions[ext]:'text'}
function documentWords(context){
 const token=context.matchBefore(/[\w$]+/);if(!token||!context.explicit&&token.text.length<2)return null;
 const from=Math.max(0,context.pos-16000),text=context.state.sliceDoc(from,Math.min(context.state.doc.length,context.pos+16000)),seen=new Set(),options=[];
 for(const match of text.matchAll(/[A-Za-z_$][\w$]{1,63}/g)){if(from+match.index===token.from||seen.has(match[0]))continue;seen.add(match[0]);options.push({label:match[0],type:'variable'});if(options.length>=300)break}
 return {from:token.from,options,validFor:/^[\w$]*$/};
}
function insertSpaces(view){
 const {state}=view;if(state.readOnly)return false;
 if(completionStatus(state)==='active'&&acceptCompletion(view))return true;
 if(state.selection.ranges.some(range=>!range.empty))return indentMore(view);
 const unit=getIndentUnit(state),changes=state.changeByRange(range=>{const line=state.doc.lineAt(range.head),column=countColumn(line.text.slice(0,range.head-line.from),state.tabSize),spaces=' '.repeat(unit-column%unit);return {changes:{from:range.head,insert:spaces},range:EditorSelection.cursor(range.head+spaces.length)}});
 view.dispatch(state.update(changes,{scrollIntoView:true,userEvent:'input.indent'}));return true;
}
const colors=HighlightStyle.define([
 {tag:tags.keyword,color:'#7c3aed'},{tag:[tags.string,tags.regexp],color:'#157347'},{tag:[tags.number,tags.bool,tags.null],color:'#b45309'},
 {tag:[tags.comment,tags.meta],color:'#6b7b8e',fontStyle:'italic'},{tag:[tags.propertyName,tags.attributeName],color:'#b45309'},
 {tag:[tags.function(tags.variableName),tags.function(tags.propertyName)],color:'#0369a1'},{tag:[tags.typeName,tags.className,tags.tagName],color:'#0e7490'},
 {tag:tags.heading,color:'#1d4ed8',fontWeight:'bold'},{tag:[tags.operator,tags.punctuation],color:'#475569'}
]);
const theme=EditorView.theme({'&':{height:'100%',fontSize:'var(--editor-font,16px)',backgroundColor:'#fbfcfe',color:'#243247'},'.cm-scroller':{overflow:'auto',fontFamily:'Consolas,"SFMono-Regular",monospace',lineHeight:'1.65'},'.cm-content':{padding:'12px 0',caretColor:'#1e6c59'},'.cm-gutters':{backgroundColor:'#f2f5f9',color:'#8894a6',borderRight:'1px solid #e3e9f0'},'.cm-activeLine':{backgroundColor:'#edf5ff88'},'.cm-activeLineGutter':{backgroundColor:'#e6eef8'},'&.cm-focused':{outline:'none'},'.cm-foldPlaceholder':{backgroundColor:'#e5ecf5',color:'#365779',border:'none'}});
const darkTheme=EditorView.theme({'&':{height:'100%',fontSize:'var(--editor-font,16px)',backgroundColor:'#24282e',color:'#dbe2eb'},'.cm-scroller':{overflow:'auto',fontFamily:'Consolas,"SFMono-Regular",monospace',lineHeight:'1.65'},'.cm-content':{padding:'8px 0',caretColor:'#e5edf5'},'.cm-gutters':{backgroundColor:'#21252b',color:'#718093',borderRight:'1px solid #363d47'},'.cm-activeLine':{backgroundColor:'#30374188'},'.cm-activeLineGutter':{backgroundColor:'#303741'},'&.cm-focused':{outline:'none'},'.cm-foldPlaceholder':{backgroundColor:'#3b4552',color:'#bed7ec',border:'none'}},{dark:true});
const darkSelection=EditorView.theme({
 '&.cm-editor .cm-selectionBackground':{backgroundColor:'#3b4f6b'},
 '&.cm-editor.cm-focused > .cm-scroller > .cm-selectionLayer .cm-selectionBackground':{backgroundColor:'#315f96'},
 '&.cm-editor .cm-content ::selection':{backgroundColor:'#315f96',color:'#fff'},
 '&.cm-editor .cm-selectionMatch':{backgroundColor:'#655729',outline:'1px solid #9f873d'},
 '&.cm-editor .cm-searchMatch':{backgroundColor:'#655729',outline:'1px solid #9f873d'},
 '&.cm-editor .cm-searchMatch-selected':{backgroundColor:'#805426',outline:'1px solid #e5a351'},
 '&.cm-editor .cm-tooltip-autocomplete':{backgroundColor:'#29313d',color:'#e6edf7',border:'1px solid #52647c'},
 '&.cm-editor .cm-tooltip-autocomplete > ul > li[aria-selected=true]':{backgroundColor:'#315f96',color:'#fff'}
});
const darkColors=HighlightStyle.define([{tag:tags.keyword,color:'#c792ea'},{tag:[tags.string,tags.regexp],color:'#b5d58c'},{tag:[tags.number,tags.bool,tags.null],color:'#f4b67a'},{tag:[tags.comment,tags.meta],color:'#8595a7',fontStyle:'italic'},{tag:[tags.propertyName,tags.attributeName],color:'#e7c17d'},{tag:[tags.function(tags.variableName),tags.function(tags.propertyName)],color:'#83c8f3'},{tag:[tags.typeName,tags.className,tags.tagName],color:'#77d9d1'},{tag:tags.heading,color:'#91b8ff',fontWeight:'bold'},{tag:[tags.operator,tags.punctuation],color:'#bdcad8'}]);
window.hamsterCodeEditor={detect,preview({parent,text,name,nonce,wrap=false}){
 const wrapping=new Compartment();
 const view=new EditorView({state:EditorState.create({doc:text,extensions:[
  EditorView.cspNonce.of(nonce||''),darkTheme,darkSelection,syntaxHighlighting(darkColors),lineNumbers(),highlightSpecialChars(),drawSelection(),keymap.of(defaultKeymap),
  modes[detect(name)](),EditorState.readOnly.of(true),EditorView.editable.of(false),wrapping.of(wrap?EditorView.lineWrapping:[]),
  EditorView.contentAttributes.of({role:'textbox','aria-label':'预览内容','aria-readonly':'true','aria-multiline':'true',tabindex:'0',spellcheck:'false'})
 ]}),parent});
 return {view,setText(value){view.dispatch({changes:{from:0,to:view.state.doc.length,insert:value},selection:{anchor:0}})},setWrap(value){view.dispatch({effects:wrapping.reconfigure(value?EditorView.lineWrapping:[])})},resize(){view.requestMeasure()},destroy(){view.destroy()}};
},mount({parent,source,name,nonce,themeMode='light',indentSize=2,syncSource=true,onChange,onSelection,onSave,onSaveAll,onSearch}){
 const language=new Compartment(),editable=new Compartment(),appearance=new Compartment(),indentation=new Compartment();let quiet=false,disabled=false,sessionEpoch=0;
 const palettes={dark:[darkTheme,darkSelection,syntaxHighlighting(darkColors)],light:[theme,syntaxHighlighting(colors)]};
 const indents={2:[indentUnit.of('  '),EditorState.tabSize.of(2)],4:[indentUnit.of('    '),EditorState.tabSize.of(4)]};
 let textDoc=null,textValue='',cleanValue=null,cleanDoc=null,comparedDoc=null,comparedBaseline=null,comparedClean=false;
 const readText=()=>{if(textDoc!==view.state.doc){textDoc=view.state.doc;textValue=textDoc.toString()}return textValue};
 const mirrorText=()=>{source.value=readText()};
 const palette=()=>palettes[themeMode==='dark'?'dark':'light'];
 const languageMode=mode=>modes[mode]?.()||[];
 const makeState=(doc,fileName,mode='auto')=>EditorState.create({doc,extensions:[
  EditorView.cspNonce.of(nonce||''),appearance.of(palette()),
  keymap.of([{key:'Mod-s',run:()=>{onSave();return true}},{key:'Mod-Shift-s',run:()=>{(onSaveAll||onSave)();return true}},{key:'Mod-f',run:()=>{onSearch();return true}},{key:'Tab',run:insertSpaces,shift:indentLess}]),
  basicSetup,autocompletion({activateOnTyping:true,activateOnTypingDelay:140}),EditorState.languageData.of(()=>[{autocomplete:documentWords}]),language.of(languageMode(mode==='auto'?detect(fileName):mode)),editable.of([]),indentation.of(indents[indentSize===4?4:2]),
  EditorView.contentAttributes.of({'aria-label':'文件内容',autocapitalize:'off',spellcheck:'false'}),
  EditorView.updateListener.of(update=>{if(update.docChanged){if(syncSource)mirrorText();if(!quiet)onChange()}if(update.selectionSet){if(syncSource){const selection=update.state.selection.main;source.setSelectionRange(selection.from,selection.to)}if(!quiet)onSelection?.()}})
 ]});
 const view=new EditorView({state:makeState(source.value,name),parent});source.hidden=true;textDoc=view.state.doc;textValue=source.value;
 return {view,getText:readText,selection:()=>({start:view.state.selection.main.from,end:view.state.selection.main.to}),
  cursor(){const at=view.state.selection.main.head,line=view.state.doc.lineAt(at);return {line:line.number,column:at-line.from+1}},
  isClean(value){if(cleanValue!==value||!cleanDoc){cleanValue=value;cleanDoc=textDoc===view.state.doc&&textValue===value?view.state.doc:EditorState.create({doc:value}).doc}if(comparedDoc!==view.state.doc||comparedBaseline!==cleanDoc){comparedDoc=view.state.doc;comparedBaseline=cleanDoc;comparedClean=comparedDoc.eq(cleanDoc)}return comparedClean},
  select(start,end){view.dispatch({selection:{anchor:start,head:end},scrollIntoView:true});view.focus()},
  replace(text){view.dispatch(view.state.replaceSelection(text));view.focus()},
  setText(text,recordHistory=false){quiet=true;try{view.dispatch({changes:{from:0,to:view.state.doc.length,insert:text},selection:{anchor:0},annotations:Transaction.addToHistory.of(recordHistory)});source.value=text;textDoc=view.state.doc;textValue=source.value}finally{quiet=false}},
  setLanguage(mode){view.dispatch({effects:language.reconfigure(languageMode(mode==='auto'?detect(name):mode))})},
  setIndent(size){indentSize=Number(size)===4?4:2;if(indentation.get(view.state)!==indents[indentSize])view.dispatch({effects:indentation.reconfigure(indents[indentSize])})},
  complete(){if(view.state.readOnly)return false;view.focus();return startCompletion(view)},
  setTheme(mode){themeMode=mode==='dark'?'dark':'light';if(appearance.get(view.state)!==palette())view.dispatch({effects:appearance.reconfigure(palette())})},
  snapshot(){return {state:view.state,name,top:view.scrollDOM.scrollTop,left:view.scrollDOM.scrollLeft}},
  restore(session){const epoch=++sessionEpoch;quiet=true;try{name=session.name;view.setState(session.state);disabled=view.state.readOnly;textDoc=view.state.doc;textValue=textDoc.toString();source.value=textValue;const selection=view.state.selection.main;source.setSelectionRange(selection.from,selection.to);view.requestMeasure({read:()=>null,write:()=>{if(epoch===sessionEpoch){view.scrollDOM.scrollTop=session.top;view.scrollDOM.scrollLeft=session.left}}})}finally{quiet=false}},
  reset(text,fileName,mode='auto'){sessionEpoch++;name=fileName;quiet=true;try{view.setState(makeState(text,fileName,mode));disabled=false;source.value=text;textDoc=view.state.doc;textValue=source.value;source.setSelectionRange(0,0);view.scrollDOM.scrollTop=0;view.scrollDOM.scrollLeft=0}finally{quiet=false}},
  setDisabled(next){if(disabled===next)return;disabled=next;view.dispatch({effects:editable.reconfigure([EditorState.readOnly.of(disabled),EditorView.editable.of(!disabled)])})},
  resize(){view.requestMeasure()},destroy(){sessionEpoch++;view.destroy();source.hidden=false;textDoc=cleanDoc=comparedDoc=comparedBaseline=null;textValue='';cleanValue=null}
 };
}};
