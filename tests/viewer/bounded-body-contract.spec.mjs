import { test, expect } from "@playwright/test";

// Browser source-only memory-budget oracles: no real backend, user or artifact.
const id = c => "job-" + c.repeat(64);

test("oversize announced length denies without showing PDF or download", async({page})=>{
  await page.goto("/viewer?job_id="+id("e"));
  await expect(page.locator("#preview")).toHaveAttribute("data-state","denied");
  await expect(page.locator("#page-canvas")).toBeHidden();
  await expect(page.locator("#download-original")).toBeHidden();
});

test("missing length with over-limit chunked body is capped and denied",async({page})=>{
  const requests=[];
  page.on("request",req=>requests.push(req.url()));
  await page.goto("/viewer?job_id="+id("8"));
  await expect(page.locator("#preview")).toHaveAttribute("data-state","denied");
  await expect(page.locator("#page-canvas")).toBeHidden();
  await expect(page.locator("#download-original")).toBeHidden();
  expect(requests.every(u=>new URL(u).origin==="http://127.0.0.1:18763")).toBe(true);
});

test("normal bounded synthetic PDF remains readable",async({page})=>{
  await page.goto("/viewer?job_id="+id("a"));
  await expect(page.locator("#preview")).toHaveAttribute("data-state","ready");
  await expect(page.locator("#download-original")).toBeVisible();
});

// Direct reader oracles prove the reason for denial; a truncated HTTP fixture
// otherwise yields a false green even if the early header gate is removed.
test("declared oversize cancels before any source read",async({page})=>{
  await page.goto("/");
  const result=await page.evaluate(async()=>{
    const {readBoundedSyntheticPDF}=await import("/bounded_reader.mjs");
    let pulls=0,cancels=0,denied=false;
    const body=new ReadableStream({
      pull(controller){pulls++;controller.enqueue(new Uint8Array(4096));},
      cancel(){cancels++;}
    },{highWaterMark:0});
    const response=new Response(body,{headers:{"Content-Length":"10485761"}});
    try{await readBoundedSyntheticPDF(response);}catch{denied=true;}
    return {pulls,cancels,denied};
  });
  expect(result).toEqual({pulls:0,cancels:1,denied:true});
});

test("invalid numeric header is rejected before source read",async({page})=>{
  await page.goto("/");
  const result=await page.evaluate(async()=>{
    const {readBoundedSyntheticPDF}=await import("/bounded_reader.mjs");
    const statuses=[];
    for(const value of ["01","-1","1e3","9007199254740992","unknown"]){
      let pulls=0,cancels=0,denied=false;
      const body=new ReadableStream({
        pull(controller){pulls++;controller.enqueue(new Uint8Array(1024));},
        cancel(){cancels++;}
      },{highWaterMark:0});
      try{await readBoundedSyntheticPDF(new Response(body,{headers:{"Content-Length":value}}));}
      catch{denied=true;}
      statuses.push({value,pulls,cancels,denied});
    }
    return statuses;
  });
  expect(result).toHaveLength(5);
  for(const item of result){expect(item).toMatchObject({pulls:0,cancels:1,denied:true});}
});

test("unannounced progressive body is stopped at cap and reader canceled",async({page})=>{
  await page.goto("/");
  const result=await page.evaluate(async()=>{
    const {readBoundedSyntheticPDF}=await import("/bounded_reader.mjs");
    let pulls=0,cancels=0,denied=false;
    const body=new ReadableStream({
      pull(controller){pulls++;controller.enqueue(new Uint8Array(1024*1024));},
      cancel(){cancels++;}
    },{highWaterMark:0});
    try{await readBoundedSyntheticPDF(new Response(body));}catch{denied=true;}
    return {pulls,cancels,denied};
  });
  expect(result).toEqual({pulls:11,cancels:1,denied:true});
});

test("exact declared size succeeds; short advertised size fails",async({page})=>{
  await page.goto("/");
  const result=await page.evaluate(async()=>{
    const {readBoundedSyntheticPDF}=await import("/bounded_reader.mjs");
    const bytes=new Uint8Array(32).fill(0x50);
    const good=await readBoundedSyntheticPDF(new Response(bytes,{headers:{"Content-Length":"32"}}));
    let mismatchDenied=false;
    try{await readBoundedSyntheticPDF(new Response(bytes,{headers:{"Content-Length":"33"}}));}
    catch{mismatchDenied=true;}
    return {length:good.byteLength,correct:good.every(v=>v===0x50),mismatchDenied};
  });
  expect(result).toEqual({length:32,correct:true,mismatchDenied:true});
});

// Hostile slow-origin oracle: a response can be within its size budget yet
// never send EOF. This is a synthetic 75ms read contract, not a product SLA.
test("stalled stream after first chunk is cancelled at read deadline",async({page})=>{
  await page.goto("/");
  const result=await page.evaluate(async()=>{
    const {readBoundedSyntheticPDF}=await import("/bounded_reader.mjs");
    let cancels=0,denied=false;
    const body=new ReadableStream({
      start(controller){controller.enqueue(new Uint8Array(64).fill(0x50));},
      cancel(){cancels++;}
    },{highWaterMark:0});
    try{await readBoundedSyntheticPDF(new Response(body),{readDeadlineMs:75});}
    catch{denied=true;}
    return {cancels,denied};
  });
  expect(result).toEqual({cancels:1,denied:true});
});

test("invalid deadline refuses body before first stream pull",async({page})=>{
  await page.goto("/");
  const result=await page.evaluate(async()=>{
    const {readBoundedSyntheticPDF}=await import("/bounded_reader.mjs");
    const outcomes=[];
    for(const deadline of [0,-1,30001,1.5]){
      let pulls=0,cancels=0,denied=false;
      const body=new ReadableStream({
        pull(controller){pulls++;controller.enqueue(new Uint8Array(64));},
        cancel(){cancels++;}
      },{highWaterMark:0});
      try{await readBoundedSyntheticPDF(new Response(body),{readDeadlineMs:deadline});}
      catch{denied=true;}
      outcomes.push({pulls,cancels,denied});
    }
    return outcomes;
  });
  expect(result).toHaveLength(4);
  for(const resultItem of result)expect(resultItem).toEqual({pulls:0,cancels:1,denied:true});
});

test("hostile cancellation promise cannot suspend denial across browser profiles",async({page})=>{
  await page.goto("/");
  const result=await page.evaluate(async()=>{
    const {readBoundedSyntheticPDF}=await import("/bounded_reader.mjs");
    let cancellations=0;
    const body=new ReadableStream({
      cancel(){cancellations++;return new Promise(()=>{});}
    },{highWaterMark:0});
    const outcome=await Promise.race([
      readBoundedSyntheticPDF(new Response(body,{headers:{"Content-Length":"10485761"}}))
        .then(()=>"ADMITTED",e=>e.message),
      new Promise(resolve=>setTimeout(()=>resolve("CLEANUP_HUNG"),500))
    ]);
    return {outcome,cancellations};
  });
  expect(result).toEqual({outcome:"invalid or oversized declared fixture",cancellations:1});
});

test("empty stream chunks are rejected before clock starvation",async({page})=>{
  await page.goto("/");
  const result=await page.evaluate(async()=>{
    const {readBoundedSyntheticPDF}=await import("/bounded_reader.mjs");
    let cancels=0;
    const stream=new ReadableStream({
      pull(c){c.enqueue(new Uint8Array(0));},
      cancel(){cancels++;}
    },{highWaterMark:0});
    const reason=await readBoundedSyntheticPDF(new Response(stream))
      .then(()=>"ADMITTED",e=>e.message);
    return {reason,cancels};
  });
  expect(result).toEqual({reason:"empty or excessive streamed fixture chunks",cancels:1});
});
