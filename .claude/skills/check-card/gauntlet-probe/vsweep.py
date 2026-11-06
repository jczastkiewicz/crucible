import os,re,sys,collections
go=[]
for d,_,fs in os.walk('crucible/internal'):
    for f in fs:
        if f.endswith('.go') and not f.endswith('_test.go'):
            go.append(open(os.path.join(d,f),encoding='utf-8').read().lower())
allgo='\n'.join(go)
VK=re.compile(r'(?:^|\|)\s*(Valid\w*|ChangeType|Affected|Choices|ConditionPresent|IsPresent|ChangeValid|TargetType|RestrictValid|CheckSVar|PresentCompare|DefinedCards|AffectedDefined|ValidCards|Defined|Origin|Destination)\$\s*([^|]*)')
CV=re.compile(r'Count\$(?:Valid|ValidGraveyard|ValidHand|ValidLibrary|ValidExile)\s+([^/|\s]+)')
comp=re.compile(r'(EQ|NE|LT|LE|GT|GE)[\w.+-]*$')
hits=collections.defaultdict(set)
def check(tok,name):
    t=tok.strip().lstrip('!')
    if not t or t[0].isdigit(): return
    base=comp.sub('',t)
    for cand in (t,base):
        c=cand.lower()
        if f'"{c}"' in allgo or f'"{c}' in allgo: return
    hits[t].add(name)
for line in open(sys.argv[1]):
    name,p=line.rstrip('\n').split('\t')
    for l in open(p,encoding='utf-8'):
        if l.startswith(('Oracle:','Name:')): continue
        vals=[v for k,v in VK.findall(l) if k not in ('Defined','Origin','Destination','DefinedCards','AffectedDefined','CheckSVar')]+CV.findall(l)
        for v in vals:
            for alt in re.split(r'[,&]| or ',v):
                parts=alt.strip().split('.',1)
                if len(parts)==2:
                    for prop in re.split(r'\+',parts[1]): check(prop,name)
for t,ns in sorted(hits.items(), key=lambda x:(-len(x[1]),x[0])):
    print(f"{t}\t{len(ns)}\t{'; '.join(sorted(ns))}")
