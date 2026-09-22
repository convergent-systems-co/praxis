import json,hashlib,base64
B="docs/research/dogfood/praxis-human-interface/bootstrap-v4/specification-bundle/"
m=json.load(open(B+"manifest.json"))
recs=[]
def rd(p): return open(B+p,'rb').read()
for c in m["candidates"]:
    b=rd(c["path"]); d="sha256:"+hashlib.sha256(b).hexdigest(); assert d==c["sha256"] or ("sha256:"+c["sha256"])==d or c["sha256"]==d.split(":")[1], (c["id"],c["sha256"],d)
    recs.append(("candidate",c["id"],d,b))
for r in m["requirements"]:
    b=rd(r["path"]); d="sha256:"+hashlib.sha256(b).hexdigest()
    recs.append(("requirement",r["id"],d,b))
for r in m["relationships"]:
    b=rd(r["path"]); d="sha256:"+hashlib.sha256(b).hexdigest()
    recs.append(("relationship",r["dependent"]+"\x00"+r["prerequisite"],d,b))
recs.sort(key=lambda x:(x[0],x[1]))
payload=json.dumps([{"kind":k,"key":key,"digest":d,"bytes":base64.b64encode(b).decode()} for k,key,d,b in recs],separators=(",",":"),ensure_ascii=True)
# go escapes <,>,& in strings; base64 has none; keys/ids checked
assert not any(ch in payload for ch in "<>&  ")
print("counts",{k:sum(1 for r in recs if r[0]==k) for k in("candidate","requirement","relationship")})
print("sha256:"+hashlib.sha256(payload.encode()).hexdigest())
print("manifest claims",m["canonical_contract_digest"])
