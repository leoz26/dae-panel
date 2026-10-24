import ssl,time,json,http.client
ctx=ssl._create_unverified_context()
hosts=[("www.google.com","/"),("api.ipify.org","/?format=json"),("www.cloudflare.com","/cdn-cgi/trace")]
res={}
for h,p in hosts:
    ok=fail=0; errs={}; tls=[]
    for i in range(8):
        t0=time.time()
        try:
            c=http.client.HTTPSConnection(h,443,timeout=12,context=ctx); c.request("GET",p,headers={"User-Agent":"proxbench/1.0"}); r=c.getresponse(); b=r.read(); c.close()
            ok+=1; tls.append(round((time.time()-t0)*1000))
        except Exception as e:
            fail+=1; k=str(e)[:70]; errs[k]=errs.get(k,0)+1
        time.sleep(0.4)
    res[h]={"ok":ok,"fail":fail,"ms":tls,"errs":errs}
print(json.dumps(res,ensure_ascii=False,indent=1))
