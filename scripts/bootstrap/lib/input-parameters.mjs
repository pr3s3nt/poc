// Explicit nonsecret environment parameters; product placeholders are untouched.
export function validateParameters(value) {
 if(!value||Array.isArray(value)||typeof value!=='object')throw Error('parameters must be an object');
 for(const [key,v]of Object.entries(value))if(!/^[A-Za-z][A-Za-z0-9_]*$/.test(key)||!['string','number','boolean'].includes(typeof v)||(typeof v==='number'&&!Number.isFinite(v))||String(v).includes('${bootstrap.'))throw Error('Invalid nonsecret bootstrap parameter: '+key);
 return value;
}
export function expandParameters(value,parameters={}) {
 if(Array.isArray(value))return value.map(v=>expandParameters(v,parameters));
 if(value&&typeof value==='object')return Object.fromEntries(Object.entries(value).map(([k,v])=>[k,expandParameters(v,parameters)]));
 if(typeof value!=='string')return value;
 const pattern=/\$\{bootstrap\.([A-Za-z][A-Za-z0-9_]*)\}/g;
 const get=key=>{if(!Object.hasOwn(parameters,key))throw Error('Missing bootstrap parameter: '+key);return parameters[key];};
 const exact=/^\$\{bootstrap\.([A-Za-z][A-Za-z0-9_]*)\}$/.exec(value);
 const result=exact?get(exact[1]):value.replace(pattern,(_,key)=>String(get(key)));
 if(typeof result==='string'&&result.includes('${bootstrap.'))throw Error('Invalid bootstrap parameter expression');
 return result;
}
