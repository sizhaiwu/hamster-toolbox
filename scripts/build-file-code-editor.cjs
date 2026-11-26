const fs=require('node:fs'),path=require('node:path'),esbuild=require('esbuild');
(async()=>{
 const repo=path.resolve(__dirname,'..'),marker='Bundled file code editor (local browser runtime)';
 const result=await esbuild.build({absWorkingDir:repo,entryPoints:['scripts/file-code-editor.js'],outfile:'web/file-code-editor.js',bundle:true,minify:true,format:'iife',target:'es2020',metafile:true});
 const packages=new Map();
 for(const input of Object.keys(result.metafile.inputs)){let dir=path.dirname(path.resolve(repo,input));while(path.dirname(dir)!==dir){const p=path.join(dir,'package.json');if(fs.existsSync(p)){if(dir.split(path.sep).includes('node_modules')){const info=JSON.parse(fs.readFileSync(p,'utf8'));packages.set(info.name,{dir,info})}break}dir=path.dirname(dir)}}
 const sections=[marker+'\nThese packages are included in web/file-code-editor.js; no CDN is used.\n'];
 for(const [name,{dir,info}] of [...packages].sort(([a],[b])=>a.localeCompare(b,'en'))){const license=fs.readdirSync(dir).sort().find(n=>/^(license|licence|copying)/i.test(n)&&fs.statSync(path.join(dir,n)).isFile());if(!license)throw Error('Missing license: '+name);sections.push('\n'+'='.repeat(72)+'\n'+name+' '+info.version+'\n'+info.license+'\n'+'='.repeat(72)+'\n'+fs.readFileSync(path.join(dir,license),'utf8'))}
 const notice=path.join(repo,'release/THIRD-PARTY.txt'),before=fs.readFileSync(notice,'utf8').split(marker)[0].trimEnd();fs.writeFileSync(notice,before+'\n\n'+sections.join('\n').trimEnd()+'\n');
 console.log('Bundled offline code editor and '+packages.size+' dependency licenses.');
})().catch(error=>{console.error(error);process.exitCode=1});
