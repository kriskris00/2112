const assert = require('node:assert/strict');
const fs = require('node:fs');
const vm = require('node:vm');

async function main() {
  const html = fs.readFileSync('web.go', 'utf8');
  const script = html.match(/<script>([\s\S]*?)<\/script>/)[1].trim();
  assert.equal(script, fs.readFileSync('web_check.js', 'utf8').trim(), 'embedded UI and checked script drifted');
  new vm.Script(script);
  const context = vm.createContext({
    document: {}, window: {location: {pathname: '/secret/'}},
    AbortController, setTimeout, clearTimeout,
    fetch: async (path) => ({ok:true,json:async()=>({path})}),
  });
  vm.runInContext(script.slice(0,script.indexOf('let toastTimer')), context);
  assert.equal(vm.runInContext("apiPath('/api/exits')",context), '/secret/api/exits');
  context.window.location.pathname='/secret/index.html';
  assert.equal(vm.runInContext("apiPath('/api/exits')",context), '/secret/api/exits');
  assert.equal(vm.runInContext("esc('<img src=x>')",context), '&lt;img src=x&gt;');
  assert.equal((await vm.runInContext("api('/api/exits')",context)).path, '/secret/api/exits');
  context.fetch=async()=>({ok:false,status:403,json:async()=>({error:'denied'})});
  await assert.rejects(vm.runInContext("api('/api/exits')",context), /denied/);

  const worker = (await import('data:text/javascript;base64,' + fs.readFileSync('worker/worker.js').toString('base64'))).default;
  const originalFetch=globalThis.fetch;
  let calls=0;
  globalThis.fetch=async(url,opts)=>{calls++; assert.equal(url,'https://www.vpngate.net/api/iphone/'); assert.equal(opts.headers.get('authorization'),null); return new Response('data',{headers:{'cache-control':'public,max-age=999','set-cookie':'secret'}});};
  try {
    let response=await worker.fetch(new Request('https://proxy.example/vpngate'),{ACCESS_KEY:'key'});
    assert.equal(response.status,404); assert.equal(calls,0);
    response=await worker.fetch(new Request('https://proxy.example/vpngate',{headers:{'x-fanout-key':'key','authorization':'private'}}),{ACCESS_KEY:'key'});
    assert.equal(response.status,200); assert.equal(response.headers.get('cache-control'),'no-store'); assert.equal(response.headers.get('set-cookie'),null);
    assert.equal(await response.text(),'data'); assert.equal(calls,1);
  } finally {globalThis.fetch=originalFetch;}
  console.log('PASS: embedded UI helpers, API error handling, script synchronization, worker authorization/cache/header isolation');
}
main().catch(e=>{console.error(e);process.exitCode=1;});
