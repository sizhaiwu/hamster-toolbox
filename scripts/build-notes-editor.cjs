// Bundle the offline editor and keep its distribution notices in sync.
const fs=require('node:fs'),path=require('node:path'),esbuild=require('esbuild');
(async()=>{
 const repo=path.resolve(__dirname,'..');
 const result=await esbuild.build({absWorkingDir:repo,entryPoints:['scripts/notes-rich-editor.js'],outfile:'web/notes-rich.js',bundle:true,minify:true,format:'iife',target:'es2020',metafile:true});
 const packages=new Map();
 for(const input of Object.keys(result.metafile.inputs)){
  let directory=path.dirname(path.resolve(repo,input));
  while(path.dirname(directory)!==directory){
   const manifest=path.join(directory,'package.json');
   if(fs.existsSync(manifest)){if(directory.split(path.sep).includes('node_modules')){const info=JSON.parse(fs.readFileSync(manifest,'utf8'));packages.set(info.name,{directory,info})}break}
   directory=path.dirname(directory);
  }
 }
 const marker='Bundled private notes editor (local browser runtime)';
 const sections=[marker+'\nThese packages are included in web/notes-rich.js; no CDN is used.\n'];
 for(const [name,{directory,info}] of [...packages].sort(([a],[b])=>a.localeCompare(b,'en'))){
  const license=fs.readdirSync(directory).sort().find(n=>/^(license|licence|copying)/i.test(n)&&fs.statSync(path.join(directory,n)).isFile());
  if(!license)throw Error('Missing license for '+name);
  sections.push('\n'+'='.repeat(72)+'\n'+name+' '+info.version+'\n'+info.license+'\n'+'='.repeat(72)+'\n'+fs.readFileSync(path.join(directory,license),'utf8'));
 }
 const notice=path.join(repo,'release','THIRD-PARTY.txt');
 const original=fs.readFileSync(notice,'utf8'),codeMarker='Bundled file code editor (local browser runtime)';
 const codeSection=original.includes(codeMarker)?original.slice(original.indexOf(codeMarker)):'';
 const existing=original.split(marker)[0].split(codeMarker)[0].trimEnd();
 fs.writeFileSync(notice,(existing+'\n\n'+sections.join('\n')).trimEnd()+'\n'+(codeSection?'\n'+codeSection:''),'utf8');
 console.log('Bundled private notes editor; included '+packages.size+' dependency licenses.');
})().catch(e=>{console.error(e);process.exitCode=1});
