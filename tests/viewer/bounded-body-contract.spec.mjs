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
