import os,sys,unicodedata
root='forge-gui/res/cardsfolder'
def norm(s): return unicodedata.normalize('NFKD',s).encode('ascii','ignore').decode().lower().strip()
idx={}
for d,_,fs in os.walk(root):
    for f in fs:
        if not f.endswith('.txt'): continue
        p=os.path.join(d,f)
        for line in open(p,encoding='utf-8'):
            if line.startswith('Name:'):
                idx.setdefault(norm(line[5:]),p)
names=[l.strip() for l in open(sys.argv[1]) if l.strip()]
miss=[]
for n in names:
    k=norm(n.split(' // ')[0].split('/')[0])
    p=idx.get(k)
    if p: print(f"{n}\t{p}")
    else: miss.append(n)
for m in miss: print("MISSING\t"+m, file=sys.stderr)
