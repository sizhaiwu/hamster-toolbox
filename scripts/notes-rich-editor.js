import {Schema} from 'prosemirror-model';
import {schema as baseSchema,defaultMarkdownParser,defaultMarkdownSerializer,MarkdownParser,MarkdownSerializer} from 'prosemirror-markdown';
import MarkdownIt from 'markdown-it';
import {EditorState,TextSelection,Plugin} from 'prosemirror-state';
import {EditorView,Decoration,DecorationSet} from 'prosemirror-view';
import {baseKeymap,toggleMark,setBlockType,wrapIn,chainCommands,exitCode} from 'prosemirror-commands';
import {keymap} from 'prosemirror-keymap';
import {history,undo,redo} from 'prosemirror-history';
import {wrapInList,splitListItem,sinkListItem,liftListItem} from 'prosemirror-schema-list';

const nodes=baseSchema.spec.nodes.update('image',{
 ...baseSchema.spec.nodes.get('image'),
 toDOM(node){return ['span',{'class':'notes-image-placeholder','data-image-source':node.attrs.src},'[图片已隐藏：'+(node.attrs.alt||'图片')+']']},
 parseDOM:[{tag:'span[data-image-source]',getAttrs:n=>({src:n.getAttribute('data-image-source'),alt:n.textContent})}]
}).append({
 table:{group:'block',content:'table_row+',isolating:true,toDOM:()=>['table',['tbody',0]],parseDOM:[{tag:'table'}]},
 table_row:{content:'(table_header|table_cell)+',toDOM:()=>['tr',0],parseDOM:[{tag:'tr'}]},
 table_header:{content:'inline*',attrs:{align:{default:''}},toDOM:()=>['th',0],parseDOM:[{tag:'th'}]},
 table_cell:{content:'inline*',attrs:{align:{default:''}},toDOM:()=>['td',0],parseDOM:[{tag:'td'}]}
});
const marks=baseSchema.spec.marks.update('link',{...baseSchema.spec.marks.get('link'),toDOM:mark=>['span',{'class':'notes-inert-link','data-note-link':mark.attrs.href},0],parseDOM:[{tag:'a[href]',getAttrs:n=>({href:n.getAttribute('href')})},{tag:'span[data-note-link]',getAttrs:n=>({href:n.getAttribute('data-note-link')})}]}).append({strike:{parseDOM:[{tag:'s'},{tag:'del'}],toDOM:()=>['s',0]}});
const schema=new Schema({nodes,marks});
const align=t=>(t.attrGet('style')||'').replace('text-align:','').trim();
const tokenizer=new MarkdownIt({html:false,linkify:false,typographer:false});
const parser=new MarkdownParser(schema,tokenizer,{...defaultMarkdownParser.tokens,
 table:{block:'table'},tr:{block:'table_row'},thead:{ignore:true},tbody:{ignore:true},th:{block:'table_header',getAttrs:t=>({align:align(t)})},td:{block:'table_cell',getAttrs:t=>({align:align(t)})},s:{mark:'strike'}
});
const serializer=new MarkdownSerializer({...defaultMarkdownSerializer.nodes,table(state,node){
 node.forEach((row,_,index)=>{const values=[];row.forEach(cell=>{const paragraph=schema.nodes.paragraph.create(null,cell.content);values.push(serializer.serialize(schema.nodes.doc.create(null,paragraph)).replace(/\n/g,' ').replace(/(?<!\\)\|/g,'\\|'))});state.write('| '+values.join(' | ')+' |\n');if(index===0){const separators=[];row.forEach(cell=>{separators.push(cell.attrs.align==='center'?':---:':cell.attrs.align==='right'?'---:':cell.attrs.align==='left'?':---':'---')});state.write('| '+separators.join(' | ')+' |\n')}});state.closeBlock(node)
}},{...defaultMarkdownSerializer.marks,strike:{open:'~~',close:'~~',mixable:true,expelEnclosingWhitespace:true}});
const codeColors=new Plugin({state:{init:(_,state)=>colorCode(state.doc),apply:(tr,old)=>tr.docChanged?colorCode(tr.doc):old},props:{decorations:state=>codeColors.getState(state)}});
function colorCode(doc){const decorations=[];doc.descendants((node,pos)=>{if(node.type.name==='code_block'&&node.textContent.length<=200000)for(const token of window.hamsterNoteSyntax?.codeTokens(node.textContent)||[])decorations.push(Decoration.inline(pos+1+token.from,pos+1+token.to,{class:'syntax-'+token.kind}))});return DecorationSet.create(doc,decorations)}
let view=null,onChange=null,scrollSource=null,scrollTokens=[];const imageViews=new Set();
function commands(){return {
 bold:toggleMark(schema.marks.strong),italic:toggleMark(schema.marks.em),strike:toggleMark(schema.marks.strike),code:toggleMark(schema.marks.code),
 paragraph:setBlockType(schema.nodes.paragraph),h1:setBlockType(schema.nodes.heading,{level:1}),h2:setBlockType(schema.nodes.heading,{level:2}),
 bullet:wrapInList(schema.nodes.bullet_list),ordered:wrapInList(schema.nodes.ordered_list),quote:wrapIn(schema.nodes.blockquote),blockcode:setBlockType(schema.nodes.code_block),undo,redo,
 table:(state,dispatch)=>{const cell=()=>schema.nodes.table_cell.create(null,schema.text('内容'));const head=()=>schema.nodes.table_header.create(null,schema.text('标题'));const table=schema.nodes.table.create(null,[schema.nodes.table_row.create(null,[head(),head()]),schema.nodes.table_row.create(null,[cell(),cell()])]);if(dispatch)dispatch(state.tr.replaceSelectionWith(table));return true}
}}
window.hamsterRichEditor={
 mount(host,source,callback,readOnly=false,attachmentURL=()=>''){this.destroy();onChange=callback;view=new EditorView(host,{state:EditorState.create({doc:parser.parse(source),plugins:[history(),codeColors,keymap({'Mod-z':undo,'Mod-y':redo,'Mod-Shift-z':redo,'Mod-b':toggleMark(schema.marks.strong),'Mod-i':toggleMark(schema.marks.em),Enter:splitListItem(schema.nodes.list_item),Tab:sinkListItem(schema.nodes.list_item),'Shift-Tab':liftListItem(schema.nodes.list_item),'Mod-Enter':chainCommands(exitCode,baseKeymap.Enter)}),keymap(baseKeymap)]}),editable:()=>!readOnly,nodeViews:{image(node){const dom=document.createElement('span'),image=document.createElement('img'),label=document.createElement('span');dom.className='notes-inline-image';dom.contentEditable='false';image.alt=node.attrs.alt||'笔记图片';image.loading='lazy';label.textContent='[图片已隐藏：'+(node.attrs.alt||'图片')+']';dom.append(image,label);const refresh=()=>{const match=/^attachment:([a-f0-9]{32})$/.exec(node.attrs.src||''),url=match&&attachmentURL(match[1]);if(url&&url.startsWith('blob:')){image.src=url;image.hidden=false;label.hidden=true}else{image.removeAttribute('src');image.hidden=true;label.hidden=false}};imageViews.add(refresh);refresh();return{dom,ignoreMutation:()=>true,destroy(){imageViews.delete(refresh)}}}},attributes:{class:'notes-prose','aria-label':'富文本正文',role:'textbox','aria-multiline':'true'},dispatchTransaction(tr){const state=view.state.apply(tr);view.updateState(state);if(tr.docChanged)onChange?.(serializer.serialize(state.doc))},handleDOMEvents:{drop:(_v,e)=>{if(e.dataTransfer?.files?.length){e.preventDefault();return true}return false}}});},
 command(name){if(!view)return false;const fn=commands()[name];if(!fn)return false;const ok=fn(view.state,view.dispatch,view);view.focus();return ok},
 readOnly(value){view?.setProps({editable:()=>!value})},
 find(text){if(!view||!text)return false;const start=view.state.selection.to;let found=-1;const all=[];view.state.doc.descendants((node,pos)=>{if(node.isText){let at=-1;while((at=node.text.toLocaleLowerCase().indexOf(text.toLocaleLowerCase(),at+1))>=0)all.push(pos+at)}});found=all.find(pos=>pos>=start)??all[0];if(found==null)return false;view.dispatch(view.state.tr.setSelection(TextSelection.create(view.state.doc,found,found+text.length)).scrollIntoView());view.focus();return true},
 insertAttachment(id,name,isImage){if(!view)return;let tr=view.state.tr;const content=isImage?schema.nodes.image.create({src:'attachment:'+id,alt:name}):schema.text(name,[schema.marks.link.create({href:'attachment:'+id})]);tr=tr.replaceSelectionWith(schema.nodes.paragraph.create(null,content));view.dispatch(tr);},
 refreshAttachments(){for(const refresh of imageViews)refresh()},
 scrollAnchor(source){if(!view)return null;if(source!==scrollSource){scrollSource=source;scrollTokens=tokenizer.parse(source,{}).filter(t=>t.level===0&&t.map&&(t.nesting===1||['fence','code_block','hr'].includes(t.type)))}const tokens=scrollTokens,children=Array.from(view.dom.children),host=view.dom.parentElement,top=host.getBoundingClientRect().top;let index=0;while(index+1<children.length&&children[index+1].getBoundingClientRect().top<=top+12)index++;const rect=children[index]?.getBoundingClientRect(),fraction=rect?Math.max(0,Math.min(1,(top-rect.top)/Math.max(1,rect.height))):0,token=tokens[index];return token?{line:token.map[0]+(token.map[1]-token.map[0])*fraction,fraction}:null},
 normalized(source){return serializer.serialize(parser.parse(source))},
 destroy(){view?.destroy();view=null;onChange=null;scrollSource=null;scrollTokens=[];imageViews.clear()}
};
